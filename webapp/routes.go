package webapp

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/url"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v5"
)

type ManifestEntry struct {
	File           string   `json:"file"`
	Src            string   `json:"src"`
	IsEntry        bool     `json:"isEntry"`
	CSS            []string `json:"css"`
	Imports        []string `json:"imports"`
	DynamicImports []string `json:"dynamicImports"`
}

type Manifest map[string]ManifestEntry

func initManifest() Manifest {
	manifest := Manifest{}
	err := json.Unmarshal(manifestBytes, &manifest)
	if err != nil {
		panic(err)
	}
	return manifest
}

type WebApp struct {
	dev       bool
	devServer string
	baseUrl   *url.URL
	manifest  Manifest
	routes    map[string]func(data any) templ.Component

	Assets fs.FS
	Public fs.FS
}

func NewWebApp(dev bool, devServer string, baseUrl string) *WebApp {
	u, err := url.Parse(baseUrl)
	if err != nil {
		panic(err)
	}

	app := &WebApp{
		dev:       dev,
		devServer: devServer,
		baseUrl:   u,
	}

	if !dev {
		app.manifest = initManifest()
		assets, err := fs.Sub(dist, "dist/assets")
		if err != nil {
			panic(err)
		}
		public, err := fs.Sub(dist, "dist/public")
		if err != nil {
			panic(err)
		}
		app.Assets = assets
		app.Public = public
	}

	app.routes = map[string]func(data any) templ.Component{
		"/dashboard": func(_ any) templ.Component {
			return app.dashboard()
		},
	}
	return app
}

func (t *WebApp) Render(c *echo.Context, w io.Writer, name string, data any) error {
	component, ok := t.routes[name]
	if !ok {
		return fmt.Errorf("no routes for %s", name)
	}
	return component(data).Render(c.Request().Context(), w)
}
