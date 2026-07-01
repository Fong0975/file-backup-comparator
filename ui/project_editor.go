package ui

import (
	"fmt"
	"strings"

	"filecompare/models"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	nativedialog "github.com/sqweek/dialog"
)

type ProjectEditor struct {
	parent     fyne.Window
	project    *models.Project
	ftpServers []models.FTPServer
	labels     []string
	onSave     func(*models.Project)

	d                    *StyledPopup
	nameEntry            *widget.Entry
	pathList             *widget.List
	metadataOnlyAllCheck *widget.Check
	metadataOnlyEntry    *widget.Entry
}

func NewProjectEditor(parent fyne.Window, project *models.Project, ftpServers []models.FTPServer, labels []string, onSave func(*models.Project)) *ProjectEditor {
	pe := &ProjectEditor{
		parent:     parent,
		project:    project.Clone(),
		ftpServers: ftpServers,
		labels:     labels,
		onSave:     onSave,
	}
	pe.build()
	return pe
}

func (pe *ProjectEditor) Show() {
	pe.d.Show()
}

func (pe *ProjectEditor) build() {
	pe.nameEntry = widget.NewEntry()
	pe.nameEntry.SetText(pe.project.Name)
	pe.nameEntry.SetPlaceHolder("Project name")

	const pathRowLabelWidth = 120

	pe.pathList = widget.NewList(
		func() int { return len(pe.project.Paths) },
		func() fyne.CanvasObject {
			labelLabel := widget.NewLabel("")
			labelLabel.Truncation = fyne.TextTruncateEllipsis
			fixedLabelBox := container.New(
				layout.NewGridWrapLayout(fyne.NewSize(pathRowLabelWidth, labelLabel.MinSize().Height)),
				labelLabel,
			)

			// Truncation is done manually in the update callback (keeping
			// both the head and tail of the path). Leaving the widget's own
			// Truncation on would let it append a second, trailing "…" of
			// its own whenever the manual estimate runs a little wide,
			// producing a confusing double-ellipsis instead of a clean
			// head…tail result.
			pathLabel := widget.NewLabel("")

			return container.NewBorder(
				nil, nil,
				container.NewHBox(widget.NewIcon(theme.FolderIcon()), fixedLabelBox),
				container.NewHBox(
					widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), func() {}),
					widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {}),
				),
				pathLabel,
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			border := obj.(*fyne.Container)
			pathLabel := border.Objects[0].(*widget.Label)
			leftBox := border.Objects[1].(*fyne.Container)
			icon := leftBox.Objects[0].(*widget.Icon)
			fixedLabelBox := leftBox.Objects[1].(*fyne.Container)
			labelLabel := fixedLabelBox.Objects[0].(*widget.Label)
			btnBox := border.Objects[2].(*fyne.Container)

			entry := pe.project.Paths[id]
			labelLabel.SetText(entry.Label)

			pathDisplay := entry.Path
			if entry.Type.IsFTP() {
				icon.SetResource(theme.StorageIcon())
				pathDisplay = fmt.Sprintf("ftp://%s:%d%s", entry.FTPDomain, entry.Port(), entry.Path)
			} else {
				icon.SetResource(theme.FolderIcon())
			}

			listWidth := pe.pathList.Size().Width
			if listWidth <= 0 {
				listWidth = 720
			}
			// Subtract the scrollbar's width too (it isn't part of any
			// child's MinSize but still eats into the row's actual space
			// once enough rows are added to need one), plus a little extra
			// slack so the estimate errs narrow rather than wide -- if the
			// manual truncation here ran even a pixel too wide, the label's
			// own truncation would kick in on top of it and produce a
			// confusing double "…".
			pathMaxWidth := listWidth - leftBox.MinSize().Width - btnBox.MinSize().Width - theme.ScrollBarSize() - 6*theme.Padding()
			pathLabel.SetText(truncateToWidth(pathDisplay, pathMaxWidth, theme.TextSize(), fyne.TextStyle{}))

			idx := int(id)
			btnBox.Objects[0].(*widget.Button).OnTapped = func() { pe.showPathDialog(idx) }
			btnBox.Objects[1].(*widget.Button).OnTapped = func() { pe.deletePath(idx) }
		},
	)

	addPathBtn := widget.NewButtonWithIcon("Add Path", theme.ContentAddIcon(), func() {
		pe.showPathDialog(-1)
	})

	// Indent the path list by the same amount a Label reserves internally,
	// so its rows line up under (rather than left of) the "Comparison
	// Paths" header, reading as the header's sub-items.
	indentedPathList := container.New(
		layout.NewCustomPaddedLayout(0, 0, theme.InnerPadding(), 0),
		pe.pathList,
	)

	pe.metadataOnlyEntry = widget.NewMultiLineEntry()
	pe.metadataOnlyEntry.SetPlaceHolder("One glob pattern per line, e.g.\n*.mp4\n*.mkv\n*.iso\nvideos/**")
	pe.metadataOnlyEntry.SetMinRowsVisible(3)
	pe.metadataOnlyEntry.SetText(strings.Join(pe.project.MetadataOnlyPatterns, "\n"))

	pe.metadataOnlyAllCheck = widget.NewCheck("All files (ignore patterns below)", func(checked bool) {
		if checked {
			pe.metadataOnlyEntry.Disable()
		} else {
			pe.metadataOnlyEntry.Enable()
		}
	})
	pe.metadataOnlyAllCheck.SetChecked(pe.project.MetadataOnlyAll)
	if pe.project.MetadataOnlyAll {
		pe.metadataOnlyEntry.Disable()
	}

	metadataOnlySection := container.NewVBox(
		widget.NewSeparator(),
		container.NewBorder(nil, nil,
			widget.NewLabelWithStyle("Large File Patterns", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			nil,
		),
		widget.NewLabel("Files matching these patterns are compared by name and size only — their contents are never read. Use for large, rarely-changing files such as videos."),
		pe.metadataOnlyAllCheck,
		pe.metadataOnlyEntry,
	)

	content := container.NewBorder(
		container.NewVBox(
			widget.NewForm(widget.NewFormItem("Project Name", pe.nameEntry)),
			container.NewBorder(nil, nil,
				widget.NewLabelWithStyle("Comparison Paths", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
				addPathBtn,
			),
		),
		metadataOnlySection,
		nil, nil,
		indentedPathList,
	)

	pe.d = NewStyledPopup(pe.parent, "Edit Project", content, []PopupButton{
		{Text: "Cancel"},
		{Text: "Save", Importance: widget.HighImportance, OnTapped: func() {
			pe.project.Name = pe.nameEntry.Text
			pe.project.MetadataOnlyAll = pe.metadataOnlyAllCheck.Checked
			pe.project.MetadataOnlyPatterns = splitLines(pe.metadataOnlyEntry.Text)
			pe.onSave(pe.project)
		}},
	})
	pe.d.Resize(fyne.NewSize(720, 580))
}

func (pe *ProjectEditor) showPathDialog(editIdx int) {
	var current models.PathEntry
	if editIdx >= 0 {
		current = pe.project.Paths[editIdx]
	}

	// SelectEntry offers the saved labels as suggestions but still accepts
	// arbitrary typed text, so it works the same as a plain entry when no
	// labels have been saved yet.
	labelEntry := widget.NewSelectEntry(pe.labels)
	labelEntry.SetText(current.Label)
	labelEntry.SetPlaceHolder("e.g. Main PC")

	// Local-folder fields.
	localPathEntry := widget.NewEntry()
	localPathEntry.SetText(current.Path)
	localPathEntry.SetPlaceHolder("e.g. D:\\Backup\\Photos")
	browseBtn := widget.NewButton("Browse...", func() {
		bringNativeDialogToFront()
		selected, err := nativedialog.Directory().Title("Select Folder").SetStartDir(localPathEntry.Text).Browse()
		if err != nil {
			return
		}
		localPathEntry.SetText(selected)
	})
	localPathRow := container.NewBorder(nil, nil, nil, browseBtn, localPathEntry)
	localFields := widget.NewForm(widget.NewFormItem("Path", localPathRow))

	// FTP fields: pick a saved server (configured in Settings) and just the
	// sub-path on it. Domain/port/account live on the server profile, not
	// on the path entry's own form, so the same server can be reused across
	// many path entries without re-entering its connection details.
	serverNames := make([]string, len(pe.ftpServers))
	for i, s := range pe.ftpServers {
		serverNames[i] = s.DisplayName()
	}

	serverSelect := widget.NewSelect(serverNames, nil)
	if len(pe.ftpServers) == 0 {
		serverSelect.PlaceHolder = "No FTP servers configured — add one in Settings"
	}
	for i, s := range pe.ftpServers {
		if s.ID == current.FTPServerID {
			serverSelect.SetSelectedIndex(i)
			break
		}
	}

	ftpPathEntry := widget.NewEntry()
	if current.Type.IsFTP() {
		ftpPathEntry.SetText(current.Path)
	}
	ftpPathEntry.SetPlaceHolder("e.g. /backup/photos")

	ftpFields := widget.NewForm(
		widget.NewFormItem("FTP Server", serverSelect),
		widget.NewFormItem("Sub-path", ftpPathEntry),
	)

	const (
		optionLocal = "Local Folder"
		optionFTP   = "FTP Server"
	)

	sourceFields := container.NewStack(localFields, ftpFields)
	sourceType := widget.NewRadioGroup([]string{optionLocal, optionFTP}, func(selected string) {
		if selected == optionFTP {
			localFields.Hide()
			ftpFields.Show()
		} else {
			ftpFields.Hide()
			localFields.Show()
		}
	})
	sourceType.Horizontal = true
	if current.Type.IsFTP() {
		sourceType.SetSelected(optionFTP)
	} else {
		sourceType.SetSelected(optionLocal)
	}

	exclusionEntry := widget.NewMultiLineEntry()
	exclusionEntry.SetPlaceHolder("One glob pattern per line, e.g.\n*.tmp\nnode_modules\ndocs/*")
	exclusionEntry.SetMinRowsVisible(5)
	exclusionEntry.SetText(strings.Join(current.Exclusions, "\n"))

	expectedGapsEntry := widget.NewMultiLineEntry()
	expectedGapsEntry.SetPlaceHolder("Files/folders this path intentionally doesn't back up.\nOne glob pattern per line, e.g.\ncache\narchive/**")
	expectedGapsEntry.SetMinRowsVisible(5)
	expectedGapsEntry.SetText(strings.Join(current.ExpectedGaps, "\n"))

	form := widget.NewForm(
		widget.NewFormItem("Label", labelEntry),
		widget.NewFormItem("Source", sourceType),
		widget.NewFormItem("", sourceFields),
		widget.NewFormItem("Exclusions", exclusionEntry),
		widget.NewFormItem("Known Gaps", expectedGapsEntry),
	)

	title := "Add Path"
	if editIdx >= 0 {
		title = "Edit Path"
	}

	var d *StyledPopup
	save := func() {
		label := strings.TrimSpace(labelEntry.Text)
		if label == "" {
			ShowInfoPopup(pe.parent, "No Name Entered", "Please enter a name for this path before saving.")
			return
		}

		patterns := splitLines(exclusionEntry.Text)
		gaps := splitLines(expectedGapsEntry.Text)

		saved := models.PathEntry{
			Label:        label,
			Exclusions:   patterns,
			ExpectedGaps: gaps,
		}

		if sourceType.Selected == optionFTP {
			idx := serverSelect.SelectedIndex()
			if idx < 0 || idx >= len(pe.ftpServers) {
				ShowInfoPopup(pe.parent, "No FTP Server Selected",
					"Please select an FTP server before saving. If none are listed, add one in Settings first.")
				return
			}

			server := pe.ftpServers[idx]
			saved.Type = models.SourceFTP
			saved.FTPServerID = server.ID
			saved.FTPDomain = server.Domain
			saved.FTPPort = server.Port
			saved.FTPUsername = server.Username
			saved.Path = strings.TrimSpace(ftpPathEntry.Text)
		} else {
			path := strings.TrimSpace(localPathEntry.Text)
			if path == "" {
				ShowInfoPopup(pe.parent, "No Folder Selected",
					"Please choose a local folder before saving.")
				return
			}

			saved.Type = models.SourceLocal
			saved.Path = path
		}

		if editIdx < 0 {
			pe.project.Paths = append(pe.project.Paths, saved)
		} else {
			pe.project.Paths[editIdx] = saved
		}
		pe.pathList.Refresh()
		d.Hide()
	}

	d = NewStyledPopup(pe.parent, title, form, []PopupButton{
		{Text: "Cancel"},
		{Text: "Save", Importance: widget.HighImportance, KeepOpen: true, OnTapped: save},
	})
	// Fix the width but size height to the form's own natural content height
	// (rather than a guessed constant), so the popup never leaves dead space
	// below the last field -- see NewStyledPopup's comment on why its body
	// stretches to fill whatever height it's given.
	d.Resize(fyne.NewSize(620, d.popup.MinSize().Height))
	d.Show()
}

func (pe *ProjectEditor) deletePath(idx int) {
	ShowConfirmPopup(pe.parent, "Delete Path",
		"Remove '"+pe.project.Paths[idx].Label+"' from this project?",
		"Delete", "Cancel",
		func(ok bool) {
			if ok {
				pe.project.Paths = append(pe.project.Paths[:idx], pe.project.Paths[idx+1:]...)
				pe.pathList.Refresh()
			}
		})
}

func splitLines(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if p := strings.TrimSpace(line); p != "" {
			out = append(out, p)
		}
	}
	return out
}
