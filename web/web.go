package web

import "embed"

// Files contains the browser UI served by the local LunarBridge process.
//
//go:embed *
var Files embed.FS
