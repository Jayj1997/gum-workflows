//go:build darwin

package main

import (
	"log"

	"github.com/Jayj1997/gum-workflows/internal/product"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

func main() {
	adapter := newDesktopAdapter(product.NewFakeApplication())
	if err := wails.Run(&options.App{
		Title:     "Gum Workflows",
		Width:     1080,
		Height:    720,
		MinWidth:  820,
		MinHeight: 560,
		AssetServer: &assetserver.Options{
			Assets: desktopAssets,
		},
		BackgroundColour: &options.RGBA{R: 245, G: 243, B: 237, A: 1},
		OnStartup:        adapter.startup,
		Bind:             []any{adapter},
	}); err != nil {
		log.Fatal(err)
	}
}
