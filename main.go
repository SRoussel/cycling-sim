package main

import (
	"embed"

	"cycling_sim/internal/app"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:gui/dist
var assets embed.FS

// main starts the desktop app; the backend Wails binds is internal/app.
func main() {
	// Create application with options
	a := app.NewApp()

	err := wails.Run(&options.App{
		Title:     "Cycling Simulator",
		Width:     1280,
		Height:    800,
		OnStartup: a.Startup,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 18, G: 18, B: 18, A: 1},
		Bind: []interface{}{
			a,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
