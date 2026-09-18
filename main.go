package main

import (
	"embed"

	wa "whatsapp-desktop/internal/whatsapp"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Create the WhatsApp service
	waService := wa.NewWhatsAppService()

	// Create an instance of the app structure
	app := NewApp(waService)

	// Create application with options
	err := wails.Run(&options.App{
		Title:     "Wamio",
		Width:     1100,
		Height:    750,
		MinWidth:  800,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 17, G: 27, B: 33, A: 1},
		DragAndDrop:      &options.DragAndDrop{EnableFileDrop: true},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			Theme:                windows.Dark,
		},
		Bind: []interface{}{
			app,
			waService,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
