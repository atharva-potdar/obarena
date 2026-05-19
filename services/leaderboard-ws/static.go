package main

import "embed"

// staticFS embeds the static frontend assets.
//go:embed static
var staticFS embed.FS
