package ui

import (
	"image/color"

	"filecompare/models"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// settingsTabTopPadding is the gap left between the tab bar and the first
// field of whichever tab is open, so the form doesn't sit flush against it.
func settingsTabTopPadding() float32 {
	return theme.Padding() * 3
}

// settingsFormListGap separates a tab's add/edit form from its list below,
// using blank space instead of a divider line.
func settingsFormListGap() fyne.CanvasObject {
	spacer := canvas.NewRectangle(color.Transparent)
	spacer.SetMinSize(fyne.NewSize(0, theme.Padding()*4))
	return spacer
}

// showSettingsDialog lets the user manage app-wide settings: saved FTP
// server profiles and saved path labels, each in their own tab. onSave is
// called after cfg.Settings has been updated, so the caller can persist it.
func showSettingsDialog(win fyne.Window, cfg *models.AppConfig, versionInfo models.VersionInfo, onSave func()) {
	projectOrder := append([]models.Project{}, cfg.Projects...)
	servers := append([]models.FTPServer{}, cfg.Settings.FTPServers...)
	labels := append([]string{}, cfg.Settings.Labels...)
	exclusions := append([]string{}, cfg.Settings.GlobalExclusions...)

	// closeDialog is assigned after d is created; the backup tab's callbacks
	// capture it by reference so they always call the real Hide when triggered.
	var closeDialog = func() {}

	orderTab := buildProjectOrderTab(&projectOrder)
	ftpTab := buildFTPSettingsTab(&servers)
	labelTab := buildLabelSettingsTab(&labels)
	exclusionsTab := buildGlobalExclusionsTab(&exclusions)
	backupTab := buildBackupTab(win, cfg, func() {
		closeDialog()
		onSave()
	})
	aboutTab := buildAboutTab(versionInfo)

	tabs := container.NewAppTabs(
		container.NewTabItem("Project Order", orderTab),
		container.NewTabItem("FTP", ftpTab),
		container.NewTabItem("Label", labelTab),
		container.NewTabItem("Exclusions", exclusionsTab),
		container.NewTabItem("Backup", backupTab),
		container.NewTabItem("About", aboutTab),
	)

	d := NewStyledPopup(win, "Settings", tabs, []PopupButton{
		{Text: "Cancel"},
		{Text: "Save", Importance: widget.HighImportance, OnTapped: func() {
			cfg.Projects = projectOrder
			cfg.Settings.FTPServers = servers
			cfg.Settings.Labels = labels
			cfg.Settings.GlobalExclusions = exclusions
			onSave()
		}},
	})
	closeDialog = d.Hide
	d.Resize(fyne.NewSize(540, 520))
	d.Show()
}
