package web

import "embed"

// StaticFiles embeds the entire web/static directory into the binary.
//
//go:embed static/*
var StaticFiles embed.FS
