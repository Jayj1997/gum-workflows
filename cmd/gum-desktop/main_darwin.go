//go:build darwin

package main

import (
	"context"
	"log"

	"github.com/Jayj1997/gum-workflows/internal/history"
	"github.com/Jayj1997/gum-workflows/internal/product"
	"github.com/Jayj1997/gum-workflows/internal/product/nodecatalog"
	"github.com/Jayj1997/gum-workflows/internal/runtimepath"
	"github.com/Jayj1997/gum-workflows/internal/secret"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

func main() {
	paths, err := runtimepath.Resolve("")
	if err != nil {
		log.Fatal(err)
	}
	store, err := history.Open(context.Background(), paths.Database())
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("close product database: %v", err)
		}
	}()

	catalog, err := nodecatalog.NewBuiltinRegistry()
	if err != nil {
		log.Fatal(err)
	}
	adapter := newDesktopAdapter(product.NewApplication(store, catalog, product.WithRunPaths(paths), product.WithSecretAdapter(secret.NewKeychainAdapter(nil))))
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
