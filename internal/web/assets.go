package web

import "embed"

// Public contains the generated Nuxt SPA. Build with make build or Docker.
//
//go:embed all:public
var Public embed.FS
