package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// buildLabelSettingsTab builds the "Label" tab: an inline add/edit form
// above a list of saved label strings, each with edit/delete buttons.
func buildLabelSettingsTab(labels *[]string) fyne.CanvasObject {
	labelEntry := widget.NewEntry()
	labelEntry.SetPlaceHolder("e.g. Main PC")

	var labelList *widget.List
	editingIdx := -1

	var saveBtn *widget.Button
	resetForm := func() {
		editingIdx = -1
		labelEntry.SetText("")
		saveBtn.SetText("Add")
	}

	saveBtn = widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), nil)
	saveBtn.OnTapped = func() {
		text := strings.TrimSpace(labelEntry.Text)
		if text == "" {
			return
		}

		if editingIdx >= 0 {
			(*labels)[editingIdx] = text
		} else {
			*labels = append(*labels, text)
		}
		labelList.Refresh()
		resetForm()
	}
	cancelEditBtn := widget.NewButtonWithIcon("", theme.CancelIcon(), nil)
	cancelEditBtn.OnTapped = func() { resetForm() }

	editForm := widget.NewForm(widget.NewFormItem("Label", labelEntry))

	labelList = widget.NewList(
		func() int { return len(*labels) },
		func() fyne.CanvasObject {
			text := widget.NewLabel("")
			text.Truncation = fyne.TextTruncateEllipsis

			return container.NewBorder(
				nil, nil, nil,
				container.NewHBox(
					widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), func() {}),
					widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {}),
				),
				text,
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			border := obj.(*fyne.Container)
			text := border.Objects[0].(*widget.Label)
			btnBox := border.Objects[1].(*fyne.Container)

			text.SetText((*labels)[id])

			idx := int(id)
			btnBox.Objects[0].(*widget.Button).OnTapped = func() {
				editingIdx = idx
				labelEntry.SetText((*labels)[idx])
				saveBtn.SetText("Update")
			}
			btnBox.Objects[1].(*widget.Button).OnTapped = func() {
				*labels = append((*labels)[:idx], (*labels)[idx+1:]...)
				labelList.Refresh()
				if editingIdx == idx {
					resetForm()
				}
			}
		},
	)

	labelList.HideSeparators = true

	content := container.NewBorder(
		container.NewVBox(
			editForm,
			container.NewHBox(layout.NewSpacer(), cancelEditBtn, saveBtn),
			settingsFormListGap(),
			widget.NewLabelWithStyle("Saved Labels", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		),
		nil, nil, nil,
		labelList,
	)
	return container.New(layout.NewCustomPaddedLayout(settingsTabTopPadding(), 0, 0, 0), content)
}
