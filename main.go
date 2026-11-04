package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app, err := NewApp()
	if err != nil {
		log.Fatal(err)
	}
	err = wails.Run(&options.App{
		Title:       "sshdeck",
		Width:       1440,
		Height:      900,
		MinWidth:    960,
		MinHeight:   600,
		AssetServer: &assetserver.Options{Assets: assets},
		// No browser context menu (no "Inspect" or "Reload"), and one running copy: two would race on the
		// vault, known_hosts and the VPN lock file. A second launch brings the first to the front.
		EnableDefaultContextMenu: false,
		SingleInstanceLock:       &options.SingleInstanceLock{UniqueId: "dev.kushalnaral.sshdeck", OnSecondInstanceLaunch: app.secondLaunch},
		OnStartup:                app.startup,
		OnShutdown:               app.shutdown,
		OnBeforeClose:            app.beforeClose,
		Bind:                     []interface{}{app},
	})
	if err != nil {
		log.Fatal(err)
	}
}
