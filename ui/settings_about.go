package ui

import (
	"filecompare/models"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// buildAboutTab builds the read-only "About" tab showing the build metadata
// from versioninfo.json (the same file goversioninfo reads to build
// icon_windows_amd64.syso), so this always matches the .exe's own
// Properties > Details tab in Windows Explorer.
func buildAboutTab(vi models.VersionInfo) fyne.CanvasObject {
	descLabel := widget.NewLabel(vi.FileDescription)
	descLabel.Wrapping = fyne.TextWrapWord

	form := widget.NewForm(
		widget.NewFormItem("Product", widget.NewLabel(vi.ProductName)),
		widget.NewFormItem("Version", widget.NewLabel(vi.ProductVersion)),
		widget.NewFormItem("Description", descLabel),
		widget.NewFormItem("Author", widget.NewLabel(vi.CompanyName)),
		widget.NewFormItem("Copyright", widget.NewLabel(vi.LegalCopyright)),
	)

	return container.New(layout.NewCustomPaddedLayout(settingsTabTopPadding(), 0, 0, 0), form)
}
