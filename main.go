package main

// Regenerates icon_windows_amd64.syso (app icon + Windows file-properties
// metadata) from versioninfo.json. Run `go generate ./...` after editing
// that file, then build as normal -- go build does not run this on its own.
//go:generate go run github.com/josephspurrier/goversioninfo/cmd/goversioninfo@v1.4.0 -64 -o icon_windows_amd64.syso

import (
	_ "embed"

	"filecompare/models"
	"filecompare/ui"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

//go:embed assets/Icon.png
var iconData []byte

// versionInfoData backs the app's About tab, so it always matches the same
// file that goversioninfo reads to build icon_windows_amd64.syso.
//
//go:embed versioninfo.json
var versionInfoData []byte

func main() {
	a := app.NewWithID("io.filecompare.app")
	a.SetIcon(&fyne.StaticResource{StaticName: "Icon.png", StaticContent: iconData})
	versionInfo := models.ParseVersionInfo(versionInfoData)
	w := ui.NewMainWindow(a, versionInfo)
	w.ShowAndRun()
}
