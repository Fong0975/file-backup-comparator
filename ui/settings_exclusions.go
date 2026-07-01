package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// buildGlobalExclusionsTab builds the "Exclusions" tab: an inline add/edit
// form above a list of glob patterns that are excluded from every project's
// comparisons, on top of whatever each path entry already excludes itself.
func buildGlobalExclusionsTab(exclusions *[]string) fyne.CanvasObject {
	patternEntry := widget.NewEntry()
	patternEntry.SetPlaceHolder("e.g. *.tmp, node_modules, docs/*")

	var exclusionList *widget.List
	editingIdx := -1

	var saveBtn *widget.Button
	resetForm := func() {
		editingIdx = -1
		patternEntry.SetText("")
		saveBtn.SetText("Add")
	}

	saveBtn = widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), nil)
	saveBtn.OnTapped = func() {
		text := strings.TrimSpace(patternEntry.Text)
		if text == "" {
			return
		}

		if editingIdx >= 0 {
			(*exclusions)[editingIdx] = text
		} else {
			*exclusions = append(*exclusions, text)
		}
		exclusionList.Refresh()
		resetForm()
	}
	cancelEditBtn := widget.NewButtonWithIcon("", theme.CancelIcon(), nil)
	cancelEditBtn.OnTapped = func() { resetForm() }

	editForm := widget.NewForm(widget.NewFormItem("Pattern", patternEntry))

	exclusionList = widget.NewList(
		func() int { return len(*exclusions) },
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

			text.SetText((*exclusions)[id])

			idx := int(id)
			btnBox.Objects[0].(*widget.Button).OnTapped = func() {
				editingIdx = idx
				patternEntry.SetText((*exclusions)[idx])
				saveBtn.SetText("Update")
			}
			btnBox.Objects[1].(*widget.Button).OnTapped = func() {
				*exclusions = append((*exclusions)[:idx], (*exclusions)[idx+1:]...)
				exclusionList.Refresh()
				if editingIdx == idx {
					resetForm()
				}
			}
		},
	)

	exclusionList.HideSeparators = true

	description := widget.NewLabel("These patterns are excluded from every project's comparisons, regardless of each path's own exclusion list.")
	description.Wrapping = fyne.TextWrapWord

	content := container.NewBorder(
		container.NewVBox(
			description,
			editForm,
			container.NewHBox(layout.NewSpacer(), cancelEditBtn, saveBtn),
			settingsFormListGap(),
			widget.NewLabelWithStyle("Global Exclusions", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		),
		nil, nil, nil,
		exclusionList,
	)
	return container.New(layout.NewCustomPaddedLayout(settingsTabTopPadding(), 0, 0, 0), content)
}
