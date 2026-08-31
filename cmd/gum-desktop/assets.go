package main

import "embed"

// desktopAssets contains the dependency-free browser UI used by Wails.
//
//go:embed all:frontend/dist
var desktopAssets embed.FS
