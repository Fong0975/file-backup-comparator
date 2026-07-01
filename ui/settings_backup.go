package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"filecompare/models"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	nativedialog "github.com/sqweek/dialog"
)

// settingsBackup is the envelope written by Export and read back by Import.
// The Marker field distinguishes FileCompare backup files from arbitrary JSON.
type settingsBackup struct {
	Marker string           `json:"_filecompare_backup"`
	Config models.AppConfig `json:"config"`
}

const backupMarker = "v1"

// buildBackupTab builds the "Backup" tab with Export, Import, and Reset
// sections. onApply is called after Import or Reset; it is expected to hide
// the containing dialog and persist the modified cfg to disk.
func buildBackupTab(win fyne.Window, cfg *models.AppConfig, onApply func()) fyne.CanvasObject {
	suggestedName := "FileCompare_" + time.Now().Format("2006-01-02_15-04-05") + ".json"
	suggestedLabel := widget.NewLabel(suggestedName)
	copyBtn := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		win.Clipboard().SetContent(suggestedName)
	})
	suggestedRow := container.NewBorder(nil, nil, nil, copyBtn, suggestedLabel)

	exportBtn := widget.NewButtonWithIcon("Export Settings...", theme.DocumentSaveIcon(), func() {
		bringNativeDialogToFront()
		path, err := nativedialog.File().
			Title("Export Settings").
			Filter("JSON Backup", "json").
			Save()
		if err != nil {
			return
		}
		if !strings.HasSuffix(strings.ToLower(path), ".json") {
			path += ".json"
		}
		data, err := json.MarshalIndent(settingsBackup{Marker: backupMarker, Config: *cfg}, "", "  ")
		if err != nil {
			ShowInfoPopup(win, "Export Failed", err.Error())
			return
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			ShowInfoPopup(win, "Export Failed", err.Error())
			return
		}
		ShowInfoPopup(win, "Export Complete", "Settings exported to:\n"+path)
	})

	importBtn := widget.NewButtonWithIcon("Import Settings...", theme.FolderOpenIcon(), func() {
		bringNativeDialogToFront()
		path, err := nativedialog.File().
			Title("Import Settings").
			Filter("JSON Backup", "json").
			Load()
		if err != nil {
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			ShowInfoPopup(win, "Import Failed", "Could not read file:\n"+err.Error())
			return
		}
		var backup settingsBackup
		if json.Unmarshal(data, &backup) != nil || backup.Marker != backupMarker {
			ShowInfoPopup(win, "Invalid Backup File",
				"The selected file is not a valid FileCompare settings backup.")
			return
		}
		ShowConfirmPopup(win, "Import Settings",
			"This will replace all projects and settings with the backup's contents.\nThis cannot be undone.",
			"Import", "Cancel",
			func(ok bool) {
				if !ok {
					return
				}
				*cfg = backup.Config
				onApply()
			})
	})

	clearBtn := widget.NewButtonWithIcon("Clear All Settings and Cache", theme.DeleteIcon(), func() {
		ShowConfirmPopup(win, "Clear All Data",
			"This will permanently delete all projects, settings, and the file hash cache.\nThis cannot be undone.",
			"Clear All", "Cancel",
			func(ok bool) {
				if !ok {
					return
				}
				*cfg = models.AppConfig{}
				_ = os.Remove(filepath.Join(models.ConfigDir(), "hash_cache.json"))
				onApply()
			})
	})
	clearBtn.Importance = widget.DangerImportance

	exportDesc := widget.NewLabel("Save all projects and settings to a backup file.")
	exportDesc.Wrapping = fyne.TextWrapWord
	importDesc := widget.NewLabel("Restore projects and settings from a previously exported backup.")
	importDesc.Wrapping = fyne.TextWrapWord
	resetDesc := widget.NewLabel("Delete all projects, settings, and the file hash cache.")
	resetDesc.Wrapping = fyne.TextWrapWord

	content := container.NewVBox(
		widget.NewLabelWithStyle("Export", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		exportDesc,
		suggestedRow,
		exportBtn,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Import", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		importDesc,
		importBtn,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Reset", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		resetDesc,
		clearBtn,
	)
	return container.New(layout.NewCustomPaddedLayout(settingsTabTopPadding(), 0, 0, 0), content)
}
