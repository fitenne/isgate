package app

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
	"go.yaml.in/yaml/v4"

	"go.uber.org/zap/zapcore"
)

type Config struct {
	Dev       bool   `koanf:"dev"`
	DevServer string `koanf:"dev_server" validate:"omitempty,url"`

	Listen         string   `koanf:"listen" validate:"required,hostname_port"`
	BaseURL        string   `koanf:"base_url" validate:"required,url"`
	SecretKey      B64Key   `koanf:"secret_key" validate:"required"`
	ProxyToken     string   `koanf:"proxy_token" validate:"required"`
	AllowedOrigins []string `koanf:"allowed_origins" validate:"required,dive,origin"`

	LogLevel       LogLevel `koanf:"log_level"`
	LogRequest     bool     `koanf:"log_request"`
	RequestLogPath string   `koanf:"request_log_path" validate:"omitempty,filepath"`

	// session
	Session struct {
		DataDir      string `koanf:"data_dir" validate:"required,dirpath"`
		CookieDomain string `koanf:"cookie_domain" validate:"required,hostname"`
	} `koanf:"session" validate:"required"`

	// oidc
	OIDC struct {
		Issuer          string    `koanf:"issuer" validate:"required,url"`
		ClientID        string    `koanf:"client_id" validate:"required"`
		ClientSecret    string    `koanf:"client_secret" validate:"required"`
		CookieSecureKey [2]B64Key `koanf:"cookie_secure_key" validate:"required,dive"`
	} `koanf:"oidc" validate:"required"`
}

var defaultConfigFields = Config{
	LogLevel:   LogLevel(zapcore.InfoLevel),
	LogRequest: true,
}

type B64Key []byte

var _ mapstructure.Unmarshaler = (*B64Key)(nil)

// UnmarshalMapstructure implements [mapstructure.Unmarshaler].
func (k *B64Key) UnmarshalMapstructure(v any) error {
	vs, ok := v.(string)
	if !ok {
		return fmt.Errorf("expected string, got %T", v)
	}

	b, err := base64.StdEncoding.DecodeString(vs)
	if err != nil {
		return fmt.Errorf("decoding %v: %w", vs, err)
	}
	if len(b) != 32 {
		return fmt.Errorf("key must be 32 bytes long, got %d", len(b))
	}
	*k = b
	return nil
}

type LogLevel zapcore.Level

var _ mapstructure.Unmarshaler = (*LogLevel)(nil)

// UnmarshalMapstructure implements [mapstructure.Unmarshaler].
func (l *LogLevel) UnmarshalMapstructure(v any) error {
	vs, ok := v.(string)
	if !ok {
		return fmt.Errorf("expected string, got %T", v)
	}

	level, err := zapcore.ParseLevel(vs)
	if err != nil {
		return fmt.Errorf("parsing %v: %w", vs, err)
	}

	*l = LogLevel(level)
	return nil
}

func (c *Config) Validate() error {
	return validator.New(validator.WithRequiredStructEnabled()).Struct(c)
}

type parser struct{}

var _ koanf.Parser = &parser{}

// Marshal implements [koanf.Parser].
func (p *parser) Marshal(v map[string]any) ([]byte, error) {
	return yaml.Dump(v, yaml.WithV4Defaults())
}

// Unmarshal implements [koanf.Parser].
func (p *parser) Unmarshal(b []byte) (map[string]any, error) {
	var v map[string]any
	if err := yaml.Load(b, &v, yaml.WithV4Defaults()); err != nil {
		return nil, err
	}
	return v, nil
}

func LoadConfig(path string) (*Config, error) {
	k := koanf.New(".")

	if err := k.Load(structs.Provider(defaultConfigFields, "koanf"), nil); err != nil {
		panic(err)
	}

	if err := k.Load(file.Provider(path), &parser{}); err != nil {
		return nil, err
	}

	if err := k.Load(env.Provider(".", env.Opt{
		Prefix: "ISGATE_",
		TransformFunc: func(k, v string) (string, any) {
			// Transform the key.
			k = strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(k, "ISGATE_")), "_", ".")

			return k, v
		},
	}), nil); err != nil {
		return nil, err
	}

	var c = &Config{}
	if err := k.Unmarshal("", c); err != nil {
		return nil, err
	}

	if err := c.Validate(); err != nil {
		return nil, err
	}

	return c, nil
}
