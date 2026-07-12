//go:build dev

package webapp

import "embed"

var (
	dist          embed.FS
	manifestBytes []byte
)
