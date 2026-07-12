//go:build !dev

package webapp

import "embed"

//go:embed dist/*
var dist embed.FS

//go:embed dist/.vite/manifest.json
var manifestBytes []byte
