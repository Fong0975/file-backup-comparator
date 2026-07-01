package ui

import (
	"filecompare/models"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// buildProjectOrderTab builds the "Project Order" tab: a list of all projects
// with Up and Down buttons to reorder them. Changes take effect when the
// caller's Save button writes projectOrder back to cfg.Projects.
func buildProjectOrderTab(projects *[]models.Project) fyne.CanvasObject {
	var projectList *widget.List
	projectList = widget.NewList(
		func() int { return len(*projects) },
		func() fyne.CanvasObject {
			nameLabel := widget.NewLabel("")
			nameLabel.Truncation = fyne.TextTruncateEllipsis
			upBtn := widget.NewButton("↑", func() {})
			downBtn := widget.NewButton("↓", func() {})
			return container.NewBorder(nil, nil, nil,
				container.NewHBox(upBtn, downBtn),
				nameLabel,
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			border := obj.(*fyne.Container)
			nameLabel := border.Objects[0].(*widget.Label)
			btnBox := border.Objects[1].(*fyne.Container)
			upBtn := btnBox.Objects[0].(*widget.Button)
			downBtn := btnBox.Objects[1].(*widget.Button)

			idx := int(id)
			nameLabel.SetText((*projects)[idx].Name)

			if idx == 0 {
				upBtn.Disable()
			} else {
				upBtn.Enable()
				upBtn.OnTapped = func() {
					(*projects)[idx], (*projects)[idx-1] = (*projects)[idx-1], (*projects)[idx]
					projectList.Refresh()
				}
			}
			if idx == len(*projects)-1 {
				downBtn.Disable()
			} else {
				downBtn.Enable()
				downBtn.OnTapped = func() {
					(*projects)[idx], (*projects)[idx+1] = (*projects)[idx+1], (*projects)[idx]
					projectList.Refresh()
				}
			}
		},
	)
	projectList.HideSeparators = true

	return container.New(
		layout.NewCustomPaddedLayout(settingsTabTopPadding(), 0, 0, 0),
		projectList,
	)
}
