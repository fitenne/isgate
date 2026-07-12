package main

import (
	"context"
	"fmt"
	"isgate/app"
	"os"

	"github.com/labstack/echo/v5"
	"github.com/urfave/cli/v3"
)

func runAction(ctx context.Context, c *cli.Command) error {
	config, err := app.LoadConfig(c.String("config"))
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	app.InitLogger(config)
	app.InitOtel()

	app, err := app.NewApp(config)
	if err != nil {
		return err
	}

	sc := echo.StartConfig{
		Address: config.Listen,
	}
	return sc.Start(context.TODO(), app.Echo)
}

func main() {
	root := &cli.Command{
		Name:  "isgate",
		Usage: "InSecure GATE",
		Commands: []*cli.Command{
			{
				Name: "run",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:        "config",
						Aliases:     []string{"c"},
						DefaultText: "config.yaml",
						Usage:       "path to config file",
						TakesFile:   true,
						Required:    true,
						Sources:     cli.EnvVars("CONFIG"),
					},
				},
				Action: runAction,
			},
		},
	}

	if err := root.Run(context.TODO(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
