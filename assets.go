// Package french75 exposes the embedded static assets for use by cmd/server.
package french75

import "embed"

//go:embed all:static
var StaticFiles embed.FS
