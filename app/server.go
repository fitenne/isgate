package app

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"isgate/webapp"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"time"

	"github.com/a-h/templ"
	"github.com/alexedwards/scs/v2"
	"github.com/go-playground/validator/v10"
	echootel "github.com/labstack/echo-opentelemetry"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/zitadel/oidc/v3/pkg/client/rp"
	httphelper "github.com/zitadel/oidc/v3/pkg/http"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"go.opentelemetry.io/otel/trace"
)

const (
	sessionKeySessionToken = "sessionToken"
)

const (
	signatureQueryKey = "_sig"
	expiresQueryKey   = "_expires"
)

type OIDCState struct {
	OriginUrl string
	Timestamp time.Time
}

type App struct {
	Echo *echo.Echo

	logger     *slog.Logger
	appBaseUrl string
	session    *scs.SessionManager
	oidc       rp.RelyingParty
	cipher     cipher.AEAD
	signer     func() hash.Hash

	allowedOrigins map[string]struct{}
	proxyToken     string
}

type gvalidator struct {
	validator *validator.Validate
}

type R struct {
	Result any            `json:"result,omitzero"`
	Error  map[string]any `json:"error,omitzero"`
}

func (v *gvalidator) Validate(i any) error {
	if err := v.validator.Struct(i); err != nil {
		return echo.ErrBadRequest.Wrap(err)
	}
	return nil
}

func NewApp(c *Config) (*App, error) {
	sessionManager, err := NewSessionManager(c)
	if err != nil {
		return nil, fmt.Errorf("error new session manager: %w", err)
	}

	block, err := aes.NewCipher(c.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("error new cipher: %w", err)
	}
	cipher, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("error new cipher: %w", err)
	}
	signer := func() hash.Hash { return hmac.New(sha256.New, c.SecretKey) }

	oidc, err := NewOIDCRP(c)
	if err != nil {
		return nil, fmt.Errorf("error new oidc: %w", err)
	}

	app := &App{
		logger:     slog.Default(),
		session:    sessionManager,
		appBaseUrl: c.BaseURL,
		oidc:       oidc,
		cipher:     cipher,
		signer:     signer,
		allowedOrigins: maps.Collect(func(yield func(string, struct{}) bool) {
			for _, v := range c.AllowedOrigins {
				if !yield(v, struct{}{}) {
					return
				}
			}
		}),
		proxyToken: c.ProxyToken,
	}

	web := webapp.NewWebApp(c.Dev, c.DevServer, c.BaseURL)

	e := echo.NewWithConfig(echo.Config{
		Logger:    GetLogger("echo"),
		Renderer:  web,
		Validator: &gvalidator{validator: validator.New()},
		HTTPErrorHandler: func(c *echo.Context, err error) {
			app.logger.Error("panic", "error", err)
			echo.DefaultHTTPErrorHandler(false)(c, err)
		},
	})
	e.Pre(middleware.RemoveTrailingSlash())
	e.Use(
		middleware.RequestID(),
		echootel.NewMiddleware(""),
		basicSecureMiddleware(),
		app.newSessionMiddleware(),
		middleware.Recover(),
	)

	if c.LogRequest {
		e.Use(app.newRequstLoggerMiddleware(c.RequestLogPath))
	}

	{
		e.GET("/", func(c *echo.Context) error {
			return c.Redirect(http.StatusFound, "/dashboard")
		})
		if !c.Dev {
			e.GET("/public/*", echo.StaticDirectoryHandler(web.Public, true))
			e.GET("/assets/*", echo.StaticDirectoryHandler(web.Assets, true))
		}
	}

	{
		auth := e.Group("")
		auth.GET("/auth", app.NewHandleAuth(), app.checkAuthRequestMiddleware)
		auth.GET("/auth/callback", app.NewHandleOAuth2Callback(), middleware.CSRF())
		auth.POST("/signout", app.NewHandleLogout(), middleware.CSRF())
	}

	{
		api := e.Group("/api", app.loginGuardMiddleware)
		api.POST("/sign-url", app.NewHandleSignURL())
	}

	{
		webApp := e.Group("", app.loginGuardMiddleware, middleware.CSRF(), cspMiddleware(c.Dev))
		webApp.GET("/dashboard", app.renderWeb)
	}

	app.Echo = e
	return app, nil
}

func NewOIDCRP(c *Config) (rp.RelyingParty, error) {
	cookieHandler := httphelper.NewCookieHandler(c.OIDC.CookieSecureKey[0], c.OIDC.CookieSecureKey[1], httphelper.WithDomain(c.Session.CookieDomain))
	client := &http.Client{}
	if c.Dev {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	u, _ := url.Parse(c.BaseURL)
	u.Path = path.Join(u.Path, "/auth/callback")

	slog.Info("connecting to oidc idp", "issuer", c.OIDC.Issuer, "client_id", c.OIDC.ClientID)
	return rp.NewRelyingPartyOIDC(context.TODO(),
		c.OIDC.Issuer,
		c.OIDC.ClientID,
		c.OIDC.ClientSecret,
		u.String(),
		[]string{oidc.ScopeOpenID, oidc.ScopeProfile, oidc.ScopeEmail, oidc.ScopeOfflineAccess},
		rp.WithCookieHandler(cookieHandler),
		rp.WithVerifierOpts(rp.WithIssuedAtOffset(5*time.Second)),
		rp.WithLogger(GetLogger("OIDC")),
		rp.WithSigningAlgsFromDiscovery(),
		rp.WithPKCEFromDiscovery(cookieHandler),
		rp.WithHTTPClient(client),
	)
}

func (app *App) NewHandleLogout() func(c *echo.Context) error {
	return func(c *echo.Context) error {
		tokens := app.GetTokens(c)
		if tokens == nil {
			return c.Redirect(http.StatusFound, app.appBaseUrl)
		}

		// try cleanup the session anyway
		if err := app.session.Destroy(c.Request().Context()); err != nil {
			app.logger.Error("error destroy session", slog.Any("error", err))
			return echo.ErrInternalServerError.Wrap(err)
		}

		u, err := url.Parse(app.oidc.GetEndSessionEndpoint())
		if err != nil {
			app.logger.Error("error parse end session endpoint", slog.Any("error", err))
			return echo.ErrInternalServerError.Wrap(err)
		}
		q := u.Query()
		q.Set("id_token_hint", tokens.IDToken)
		q.Set("post_logout_redirect_uri", app.appBaseUrl)
		u.RawQuery = q.Encode()

		return c.JSON(http.StatusOK, map[string]any{
			"redirectUrl": u.String(),
		})
	}
}

func (app *App) SignUrl(rawURL string, expires time.Time) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	query := u.Query()
	query.Set(expiresQueryKey, strconv.FormatInt(expires.Unix(), 10))
	u.RawQuery = query.Encode()

	h := app.signer()
	h.Write([]byte(u.String()))
	query.Set(signatureQueryKey, base64.RawURLEncoding.EncodeToString(h.Sum(nil)))
	u.RawQuery = query.Encode()
	return u, nil
}

func (app *App) NewHandleAuth() echo.HandlerFunc {
	verifier := app.oidc.IDTokenVerifier()
	result := func(c *echo.Context) error {
		if err := app.checkDomain(c); err != nil {
			return echo.ErrBadRequest.Wrap(err)
		}

		// hotlinking, api request, etc. should not be redirected
		mode := c.Request().Header.Get("Sec-Fetch-Mode")
		if mode != "navigate" {
			return c.NoContent(http.StatusUnauthorized)
		}

		return app.redirectToLogin(c)
	}

	return func(c *echo.Context) error {
		ok, err := app.VerifySignature(c)
		if err != nil {
			return echo.ErrBadRequest
		}
		if ok {
			return c.NoContent(http.StatusOK)
		}

		claims, err := app.verifyAndRefreshTokens(c, verifier)
		if err != nil {
			app.logger.Error("verify and refresh tokens error", slog.Any("error", err))
		}
		if claims == nil || err != nil {
			return result(c)
		}

		setUserHeaders(c, claims)
		return c.NoContent(http.StatusOK)
	}
}

func (app *App) verifyAndRefreshTokens(c *echo.Context, verifier *rp.IDTokenVerifier) (*oidc.IDTokenClaims, error) {
	tokens := app.GetTokens(c)
	if tokens == nil {
		return nil, nil
	}

	claims, err := rp.VerifyIDToken[*oidc.IDTokenClaims](c.Request().Context(), tokens.IDToken, verifier)
	if err != nil && !errors.Is(err, oidc.ErrExpired) {
		return nil, fmt.Errorf("invalid id_token: %w", err)
	}

	if errors.Is(err, oidc.ErrExpired) {
		span := trace.SpanFromContext(c.Request().Context())
		span.AddEvent("request refresh")
		app.logger.Warn("id_token expired", slog.String("claims", string(mustJson(claims))))

		newClaims, err := rp.RefreshTokens[*oidc.IDTokenClaims](c.Request().Context(), app.oidc, tokens.RefreshToken, "", "")
		if err != nil {
			return nil, fmt.Errorf("refresh tokens error: %w", err)
		}
		claims = newClaims.IDTokenClaims

		if err := app.session.Destroy(c.Request().Context()); err != nil {
			return nil, fmt.Errorf("error destroy session: %w", err)
		}
		app.SetTokens(c, &Tokens{
			AccessToken:  newClaims.AccessToken,
			RefreshToken: newClaims.RefreshToken,
			IDToken:      newClaims.IDToken,
		})
		span.AddEvent("refresh ok")
		app.logger.Info("id_token refreshed", slog.String("claims", string(mustJson(newClaims.IDTokenClaims))))
	}

	return claims, nil
}

func setUserHeaders(c *echo.Context, claims *oidc.IDTokenClaims) {
	c.Response().Header().Set("x-user-issuer", claims.Issuer)
	c.Response().Header().Set("x-user-id", claims.Subject)
	c.Response().Header().Set("x-user-email", claims.Email)
	c.Response().Header().Set("x-user-name", claims.PreferredUsername)
}

func (app *App) NewHandleOAuth2Callback() func(c *echo.Context) error {
	return func(c *echo.Context) error {
		cb := func(w http.ResponseWriter, r *http.Request, tokens *oidc.Tokens[*oidc.IDTokenClaims], state string, rp rp.RelyingParty, info *oidc.UserInfo) {
			oidcState, err := app.parseState(state)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			// will oidc rp package handle the state expiration?
			if time.Since(oidcState.Timestamp) >= 3*time.Minute {
				app.logger.Warn("state has expired", slog.Any("state", state))
				http.Error(w, "expired", http.StatusBadRequest)
				return
			}

			err = app.session.Destroy(c.Request().Context())
			if err != nil {
				app.logger.Error("error destroy session", slog.Any("error", err))
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			app.SetTokens(c, &Tokens{
				AccessToken:  tokens.AccessToken,
				RefreshToken: tokens.RefreshToken,
				IDToken:      tokens.IDToken,
			})

			if oidcState.OriginUrl != "" {
				http.Redirect(w, r, oidcState.OriginUrl, http.StatusFound)
				return
			}

			trace.SpanFromContext(c.Request().Context()).AddEvent("new token")
			http.Redirect(w, r, app.appBaseUrl, http.StatusSeeOther)
		}

		h := rp.CodeExchangeHandler(rp.UserinfoCallback(cb), app.oidc)
		h(c.Response(), c.Request())
		return nil
	}
}

func (app *App) NewHandleSignURL() func(c *echo.Context) error {
	return func(c *echo.Context) error {
		var request struct {
			Url string `form:"url" validate:"required,url"`
			Exp int64  `form:"exp" validate:"required,oneof=3600 10800 43200 86400"`
		}
		if err := c.Bind(&request); err != nil {
			return err
		}
		if err := c.Validate(&request); err != nil {
			return err
		}

		expiresAt := time.Now().Add(time.Duration(request.Exp) * time.Second)
		result, err := app.SignUrl(request.Url, expiresAt)
		if err != nil {
			return echo.ErrBadRequest.Wrap(err)
		}

		return c.JSON(http.StatusOK, R{
			Result: map[string]string{
				"url": result.String(),
			},
		})
	}
}

func (app *App) VerifySignature(c *echo.Context) (bool, error) {
	u, err := url.Parse(app.originUrlOf(c))
	if err != nil {
		return false, fmt.Errorf("error parse url: %w", err)
	}

	query := u.Query()
	var (
		expiresStr = query.Get(expiresQueryKey)
		signature  = query.Get(signatureQueryKey)
	)
	if expiresStr == "" || signature == "" {
		return false, nil
	}
	expires, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil {
		return false, fmt.Errorf("error parse expires: %w", err)
	}
	if time.Now().After(time.Unix(expires, 0)) {
		return false, errors.New("expired")
	}

	sig, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return false, fmt.Errorf("error decoding signature: %w", err)
	}

	query.Del(signatureQueryKey)
	u.RawQuery = query.Encode()

	h := app.signer()
	h.Write([]byte(u.String()))
	return hmac.Equal(sig, h.Sum(nil)), nil
}

func (app *App) GetTokens(c *echo.Context) *Tokens {
	v := app.session.Get(c.Request().Context(), sessionKeySessionToken)
	if v == nil {
		return nil
	}
	return v.(*Tokens)
}

func (app *App) SetTokens(c *echo.Context, tokens *Tokens) {
	app.session.Put(c.Request().Context(), sessionKeySessionToken, tokens)
}

func (app *App) originUrlOf(c *echo.Context) string {
	// trust proxy
	originUrl := c.Request().Header.Get("X-Forwarded-Uri")
	return originUrl
}

func (app *App) parseState(state string) (*OIDCState, error) {
	b, err := base64.RawURLEncoding.DecodeString(state)
	if err != nil {
		return nil, fmt.Errorf("error decoding state: %w", err)
	}
	if len(b) < app.cipher.NonceSize() {
		return nil, errors.New("invalid state")
	}
	nonce := b[:app.cipher.NonceSize()]
	raw, err := app.cipher.Open(nil, nonce, b[app.cipher.NonceSize():], nil)
	if err != nil {
		return nil, fmt.Errorf("error decoding state: %w", err)
	}

	var oidcState = &OIDCState{}
	if err := gob.NewDecoder(bytes.NewBuffer(raw)).Decode(oidcState); err != nil {
		return nil, fmt.Errorf("error gob: %w", err)
	}

	return oidcState, nil
}

func (app *App) redirectToLogin(c *echo.Context) error {
	redirUrl := app.originUrlOf(c)
	nonce := make([]byte, app.cipher.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		panic(err)
	}

	b := &bytes.Buffer{}
	err := gob.NewEncoder(b).Encode(OIDCState{
		OriginUrl: redirUrl,
		Timestamp: time.Now(),
	})
	if err != nil {
		return echo.ErrInternalServerError.Wrap(err)
	}
	state := app.cipher.Seal(b.Bytes()[:0], nonce, b.Bytes(), nil)

	redirect := rp.AuthURLHandler(func() string {
		return base64.RawURLEncoding.EncodeToString(append(nonce, state...))
	}, app.oidc)
	redirect(c.Response(), c.Request())
	return nil
}

func (app *App) checkDomain(c *echo.Context) error {
	proto := c.Request().Header.Get("X-Forwarded-Proto")
	host := c.Request().Header.Get("X-Forwarded-Host")
	origin := fmt.Sprintf("%s://%s", proto, host)
	if _, ok := app.allowedOrigins[origin]; !ok {
		return echo.ErrUnauthorized.Wrap(fmt.Errorf("domain not allowed: %s", host))
	}
	return nil
}

func (app *App) newRequstLoggerMiddleware(path string) echo.MiddlewareFunc {
	logger, err := RequestLogger(path)
	if err != nil {
		panic(err)
	}
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogLatency:   true,
		LogRemoteIP:  true,
		LogHost:      true,
		LogMethod:    true,
		LogURI:       true,
		LogRequestID: true,
		LogUserAgent: true,
		LogStatus:    true,
		LogHeaders:   []string{"X-Forwarded-Uri", "X-Forwarded-For"},
		LogValuesFunc: func(c *echo.Context, v middleware.RequestLoggerValues) error {
			logger.LogAttrs(context.Background(), slog.LevelInfo, "request",
				slog.String("method", v.Method),
				slog.String("uri", v.URI),
				slog.Int("status", v.Status),
				slog.String("user_agent", v.UserAgent),
				slog.String("request_id", v.RequestID),
				slog.Any("forwarded", v.Headers),
			)
			return nil
		},
	})
}

func (app *App) newSessionMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Response().Header().Add("Vary", "Cookie")

			ctx := c.Request().Context()
			var token string
			cookie, err := c.Cookie(app.session.Cookie.Name)
			if err == nil {
				token = cookie.Value
			}

			ctx, err = app.session.Load(ctx, token)
			if err != nil {
				return err
			}

			c.SetRequest(c.Request().WithContext(ctx))

			cr, err := echo.UnwrapResponse(c.Response())
			if err != nil {
				panic(err)
			}

			cr.Before(func() {
				switch app.session.Status(ctx) {
				case scs.Modified:
					token, expiry, err := app.session.Commit(ctx)
					if err != nil {
						panic(err)
					}

					app.session.WriteSessionCookie(ctx, c.Response(), token, expiry)

				case scs.Destroyed:
					app.session.WriteSessionCookie(ctx, c.Response(), "", time.Time{})
				default:
					// session might not exist yet
				}
			})
			return next(c)

		}
	}
}

func (app *App) loginGuardMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	verifier := app.oidc.IDTokenVerifier()
	return func(c *echo.Context) error {
		claims, err := app.verifyAndRefreshTokens(c, verifier)
		if err != nil {
			app.logger.Error("verify and refresh tokens error", slog.Any("error", err))
		}
		if claims == nil || err != nil {
			return app.redirectToLogin(c)
		}

		setUserHeaders(c, claims)
		return next(c)
	}
}

func (app *App) checkAuthRequestMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		requiredHeaders := []string{"X-Forwarded-Proto", "X-Forwarded-Host", "X-Forwarded-Uri"}
		headers := c.Request().Header

		if headers.Get("X-Proxy-Token") != app.proxyToken {
			c.Logger().Warn("invalid proxy token")
			return echo.ErrUnauthorized
		}

		for _, h := range requiredHeaders {
			if headers.Get(h) == "" {
				c.Logger().Warn("missing header", slog.String("header", h))
				return echo.ErrUnauthorized
			}
		}
		return next(c)
	}
}

func (app *App) renderWeb(c *echo.Context) error {
	return c.Render(http.StatusOK, c.Request().URL.Path, app)
}

func basicSecureMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			res := c.Response()
			res.Header().Set(echo.HeaderReferrerPolicy, "strict-origin-when-cross-origin")
			res.Header().Set(echo.HeaderXContentTypeOptions, "nosniff")
			return next(c)
		}
	}
}

func cspMiddleware(dev bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			res := c.Response()

			nonce := rand.Text()
			c.SetRequest(c.Request().WithContext(templ.WithNonce(c.Request().Context(), nonce)))
			if !dev {
				res.Header().Set(echo.HeaderContentSecurityPolicy, fmt.Sprintf("default-src 'self'; style-src 'self' 'unsafe-inline' 'nonce-%s'; script-src 'nonce-%s' 'strict-dynamic'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'", nonce, nonce))
			} else {
				res.Header().Set(echo.HeaderContentSecurityPolicyReportOnly, fmt.Sprintf("default-src 'self'; style-src 'self' 'unsafe-inline' 'nonce-%s'; script-src 'nonce-%s' 'strict-dynamic'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'", nonce, nonce))
			}

			return next(c)
		}
	}
}

func mustJson(in any) []byte {
	o, err := json.Marshal(in)
	if err != nil {
		panic(err)
	}
	return o
}
