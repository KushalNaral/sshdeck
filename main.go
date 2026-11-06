package main

import (
	"embed"
	"log"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/KushalNaral/sshdeck/internal/vpn"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// On Windows the app starts a copy of itself with administrator rights to run VPN clients (see internal/vpn).
	if len(os.Args) == 4 && os.Args[1] == "--vpn-helper" {
		os.Exit(vpn.RunHelper(os.Args[2], os.Args[3]))
	}
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
		// Files dragged in from the desktop are reported to the file panel (see App.startup).
		DragAndDrop:   &options.DragAndDrop{EnableFileDrop: true},
		OnStartup:     app.startup,
		OnShutdown:    app.shutdown,
		OnBeforeClose: app.beforeClose,
		Bind:          []interface{}{app},
	})
	if err != nil {
		log.Fatal(err)
	}
}
