package ui

import (
	"fmt"
	"path"
	"strings"
	"time"

	"filecompare/core"
	"filecompare/models"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type ResultView struct {
	outer           *fyne.Container
	header          *fyne.Container
	projectLabel    *widget.Label
	tabs            *container.AppTabs
	result          *core.CompareResult
	win             fyne.Window
	project         models.Project
	onUpdateProject func(models.Project)
}

// SetOnUpdateProject registers a callback invoked after a path entry's
// Exclusions or ExpectedGaps slice is modified from the Missing tab's item menu.
func (rv *ResultView) SetOnUpdateProject(f func(models.Project)) {
	rv.onUpdateProject = f
}

// NewResultView creates and returns a new ResultView bound to win for popup display.
func NewResultView(win fyne.Window) *ResultView {
	rv := &ResultView{win: win}
	placeholder := container.NewCenter(widget.NewLabel("Select a project and click Run Compare."))
	rv.tabs = container.NewAppTabs(container.NewTabItem("Results", placeholder))

	rv.projectLabel = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	rv.header = container.NewVBox(rv.projectLabel, widget.NewSeparator())
	rv.header.Hide()

	rv.outer = container.NewBorder(rv.header, nil, nil, nil, rv.tabs)
	return rv
}

func (rv *ResultView) Container() fyne.CanvasObject {
	return rv.outer
}

// Update replaces the displayed result and the project context used by item
// actions (Open Folder).
func (rv *ResultView) Update(result *core.CompareResult, project models.Project) {
	rv.result = result
	rv.project = project
	rv.projectLabel.SetText(project.Name)
	rv.header.Show()
	rv.tabs.SetItems([]*container.TabItem{
		container.NewTabItem(
			fmt.Sprintf("Missing (%d)", len(result.Missing)),
			rv.buildMissingTab(result),
		),
		container.NewTabItem(
			fmt.Sprintf("Different (%d)", len(result.Differ)),
			rv.buildDifferTab(result),
		),
		container.NewTabItem(
			fmt.Sprintf("Same (%d)", len(result.Same)),
			rv.buildSameTab(result),
		),
	})
}

func (rv *ResultView) buildMissingTab(result *core.CompareResult) fyne.CanvasObject {
	if len(result.Missing) == 0 {
		return container.NewCenter(widget.NewLabel("No missing files — all paths have the same file set."))
	}

	n := len(result.Labels)

	list := widget.NewList(
		func() int { return len(result.Missing) },
		func() fyne.CanvasObject {
			pathLabel := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

			// status row: groups of [HBox(label, btn)] separated by vertical separators.
			// Object indices in the outer HBox: group i is at i*2, separator between i and i+1 at i*2+1.
			statusItems := make([]fyne.CanvasObject, 0, max(0, n*2-1))
			for i := 0; i < n; i++ {
				if i > 0 {
					statusItems = append(statusItems, widget.NewSeparator())
				}
				lbl := widget.NewLabel("")
				btn := widget.NewButtonWithIcon("", theme.MenuDropDownIcon(), func() {})
				btn.Importance = widget.LowImportance
				statusItems = append(statusItems, container.NewHBox(lbl, btn))
			}

			return container.NewVBox(pathLabel, container.NewHBox(statusItems...))
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			vbox := obj.(*fyne.Container)
			pathLabel := vbox.Objects[0].(*widget.Label)
			statusRow := vbox.Objects[1].(*fyne.Container)

			status := result.Missing[id]
			availWidth := obj.Size().Width - 2*theme.InnerPadding()
			if availWidth <= 0 {
				availWidth = 600
			}
			pathLabel.SetText(truncateRelPath(status.RelPath, availWidth, theme.TextSize(), fyne.TextStyle{Bold: true}))

			for i, label := range result.Labels {
				capturedLabel := label
				// group i is at statusRow.Objects[i*2]; separators occupy odd indices.
				group := statusRow.Objects[i*2].(*fyne.Container)
				lbl := group.Objects[0].(*widget.Label)
				btn := group.Objects[1].(*widget.Button)

				if status.Entries[label] != nil {
					lbl.SetText("✓ " + label)
				} else {
					lbl.SetText("✗ " + label)
				}

				btn.OnTapped = func() {
					rv.showLabelMenu(btn, status, capturedLabel)
				}
			}
		},
	)

	return list
}

func (rv *ResultView) showLabelMenu(btn *widget.Button, status *core.FileStatus, label string) {
	var pe *models.PathEntry
	for i := range rv.project.Paths {
		if rv.project.Paths[i].Label == label {
			pe = &rv.project.Paths[i]
			break
		}
	}

	dir := path.Dir(status.RelPath)
	var dirPattern string
	if dir != "." {
		dirPattern = dir + "/**"
	}
	filePattern := status.RelPath
	noPE := pe == nil

	openItem := fyne.NewMenuItem("Open Folder", func() {
		if pe != nil {
			rv.openFolderForLabel(status, *pe)
		}
	})
	openItem.Disabled = noPE || pe.Type.IsFTP()

	excludeDirItem := fyne.NewMenuItem("Exclude Directory...", func() {
		rv.showPatternDialog("Exclude Directory", dirPattern, func(pattern string) {
			pe.Exclusions = append(pe.Exclusions, pattern)
			rv.notifyProjectUpdate()
			rv.refilterMissing()
		})
	})
	excludeDirItem.Disabled = noPE || dirPattern == ""

	excludeFileItem := fyne.NewMenuItem("Exclude File...", func() {
		rv.showPatternDialog("Exclude File", filePattern, func(pattern string) {
			pe.Exclusions = append(pe.Exclusions, pattern)
			rv.notifyProjectUpdate()
			rv.refilterMissing()
		})
	})
	excludeFileItem.Disabled = noPE

	gapDirItem := fyne.NewMenuItem("Expected Gap (Directory)...", func() {
		rv.showPatternDialog("Expected Gap (Directory)", dirPattern, func(pattern string) {
			pe.ExpectedGaps = append(pe.ExpectedGaps, pattern)
			rv.notifyProjectUpdate()
			rv.refilterMissing()
		})
	})
	gapDirItem.Disabled = noPE || dirPattern == ""

	gapFileItem := fyne.NewMenuItem("Expected Gap (File)...", func() {
		rv.showPatternDialog("Expected Gap (File)", filePattern, func(pattern string) {
			pe.ExpectedGaps = append(pe.ExpectedGaps, pattern)
			rv.notifyProjectUpdate()
			rv.refilterMissing()
		})
	})
	gapFileItem.Disabled = noPE

	menuItems := []*fyne.MenuItem{
		openItem,
		fyne.NewMenuItemSeparator(),
		excludeDirItem,
		excludeFileItem,
		fyne.NewMenuItemSeparator(),
		gapDirItem,
		gapFileItem,
	}

	if status.Entries[label] == nil {
		var sourcePE *models.PathEntry
		for i := range rv.project.Paths {
			src := &rv.project.Paths[i]
			if src.Label != label && status.Entries[src.Label] != nil && !src.Type.IsFTP() {
				sourcePE = src
				break
			}
		}

		copyItem := fyne.NewMenuItem("Copy", func() {
			if pe != nil && sourcePE != nil {
				rv.copyFileToPath(status, *sourcePE, *pe)
			}
		})
		copyItem.Disabled = noPE || pe.Type.IsFTP() || sourcePE == nil

		menuItems = append(menuItems, fyne.NewMenuItemSeparator(), copyItem)
	}

	menu := fyne.NewMenu("", menuItems...)
	absPos := fyne.CurrentApp().Driver().AbsolutePositionForObject(btn)
	widget.ShowPopUpMenuAtPosition(menu, rv.win.Canvas(), fyne.NewPos(absPos.X, absPos.Y+btn.Size().Height))
}

func (rv *ResultView) showPatternDialog(title, suggestedPattern string, onConfirm func(pattern string)) {
	entry := widget.NewEntry()
	entry.SetText(suggestedPattern)

	form := widget.NewForm(widget.NewFormItem("Pattern", entry))

	d := NewStyledPopup(rv.win, title, form, []PopupButton{
		{Text: "Cancel"},
		{Text: "Add", Importance: widget.HighImportance, OnTapped: func() {
			pattern := strings.TrimSpace(entry.Text)
			if pattern != "" {
				onConfirm(pattern)
			}
		}},
	})
	d.Resize(fyne.NewSize(400, 180))
	d.Show()
}

func (rv *ResultView) notifyProjectUpdate() {
	if rv.onUpdateProject != nil {
		rv.onUpdateProject(rv.project)
	}
}

func checkAllSame(entries map[string]*core.FileInfo, labels []string) bool {
	first := ""
	for _, label := range labels {
		e := entries[label]
		if e == nil {
			continue
		}
		if first == "" {
			first = e.Hash
		} else if e.Hash != first {
			return false
		}
	}
	return true
}

// reclassifyStatus re-evaluates where status belongs after its Entries map has
// been updated. If the status is no longer missing from any label it is removed
// from result.Missing and appended to result.Same or result.Differ.
func (rv *ResultView) reclassifyStatus(status *core.FileStatus) {
	hasMissing := false
	for _, label := range rv.result.Labels {
		if status.Entries[label] == nil {
			hasMissing = true
			break
		}
	}

	if hasMissing {
		return
	}

	for i, ms := range rv.result.Missing {
		if ms == status {
			rv.result.Missing = append(rv.result.Missing[:i], rv.result.Missing[i+1:]...)
			break
		}
	}

	allSame := checkAllSame(status.Entries, rv.result.Labels)

	status.AllSame = allSame
	if allSame {
		rv.result.Same = append(rv.result.Same, status)
	} else {
		rv.result.Differ = append(rv.result.Differ, status)
	}
}

// refilterMissing re-evaluates every item in result.Missing against the
// current project's Exclusions and ExpectedGaps. Items whose every missing
// entry is now covered by a pattern are moved to Same or Differ and the
// tabs are rebuilt.
func (rv *ResultView) refilterMissing() {
	if rv.result == nil {
		return
	}

	pathMap := make(map[string]*models.PathEntry, len(rv.project.Paths))
	for i := range rv.project.Paths {
		pathMap[rv.project.Paths[i].Label] = &rv.project.Paths[i]
	}

	for i := len(rv.result.Missing) - 1; i >= 0; i-- {
		status := rv.result.Missing[i]

		hasMissing := false
		for _, label := range rv.result.Labels {
			if status.Entries[label] != nil {
				continue
			}
			pe := pathMap[label]
			if pe == nil {
				hasMissing = true
				continue
			}
			if core.MatchesAnyPattern(status.RelPath, pe.ExpectedGaps) {
				continue
			}
			if core.MatchesAnyPattern(status.RelPath, pe.Exclusions) {
				continue
			}
			hasMissing = true
		}

		if hasMissing {
			continue
		}

		rv.result.Missing = append(rv.result.Missing[:i], rv.result.Missing[i+1:]...)

		status.AllSame = checkAllSame(status.Entries, rv.result.Labels)
		if status.AllSame {
			rv.result.Same = append(rv.result.Same, status)
		} else {
			rv.result.Differ = append(rv.result.Differ, status)
		}
	}

	rv.Update(rv.result, rv.project)
}

func (rv *ResultView) buildDifferTab(result *core.CompareResult) fyne.CanvasObject {
	if len(result.Differ) == 0 {
		return container.NewCenter(widget.NewLabel("No content differences — all shared files are identical."))
	}

	n := len(result.Labels)

	list := widget.NewList(
		func() int { return len(result.Differ) },
		func() fyne.CanvasObject {
			children := make([]fyne.CanvasObject, 0, n+1)
			children = append(children,
				widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			)
			for i := 0; i < n; i++ {
				detailLabel := widget.NewLabel("")
				btn := widget.NewButtonWithIcon("", theme.MenuDropDownIcon(), func() {})
				btn.Importance = widget.LowImportance
				children = append(children, container.NewBorder(nil, nil, nil, btn, detailLabel))
			}
			return container.NewVBox(children...)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			vbox := obj.(*fyne.Container)
			status := result.Differ[id]

			availWidth := obj.Size().Width - 2*theme.InnerPadding()
			if availWidth <= 0 {
				availWidth = 600
			}
			vbox.Objects[0].(*widget.Label).SetText(truncateRelPath(status.RelPath, availWidth, theme.TextSize(), fyne.TextStyle{Bold: true}))
			for i, label := range result.Labels {
				capturedLabel := label
				row := vbox.Objects[i+1].(*fyne.Container)
				detailLabel := row.Objects[0].(*widget.Label)
				btn := row.Objects[1].(*widget.Button)

				info := status.Entries[label]
				var text string
				if info != nil {
					text = fmt.Sprintf("  %-20s  %8s  %s",
						label,
						formatSize(info.Size),
						info.ModTime.Format(time.DateTime),
					)
				} else {
					text = fmt.Sprintf("  %-20s  (missing)", label)
				}
				detailLabel.SetText(text)
				btn.OnTapped = func() {
					rv.showDifferLabelMenu(btn, status, capturedLabel)
				}
			}
		},
	)

	return list
}

func (rv *ResultView) buildSameTab(result *core.CompareResult) fyne.CanvasObject {
	if len(result.Same) == 0 {
		return container.NewCenter(widget.NewLabel("No files found in all paths simultaneously."))
	}

	n := len(result.Labels)

	list := widget.NewList(
		func() int { return len(result.Same) },
		func() fyne.CanvasObject {
			pathLabel := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

			statusItems := make([]fyne.CanvasObject, 0, max(0, n*2-1))
			for i := 0; i < n; i++ {
				if i > 0 {
					statusItems = append(statusItems, widget.NewSeparator())
				}
				lbl := widget.NewLabel("")
				btn := widget.NewButtonWithIcon("", theme.MenuDropDownIcon(), func() {})
				btn.Importance = widget.LowImportance
				statusItems = append(statusItems, container.NewHBox(lbl, btn))
			}

			return container.NewVBox(pathLabel, container.NewHBox(statusItems...))
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			vbox := obj.(*fyne.Container)
			pathLabel := vbox.Objects[0].(*widget.Label)
			statusRow := vbox.Objects[1].(*fyne.Container)

			status := result.Same[id]
			availWidth := obj.Size().Width - 2*theme.InnerPadding()
			if availWidth <= 0 {
				availWidth = 600
			}
			pathLabel.SetText(truncateRelPath(status.RelPath, availWidth, theme.TextSize(), fyne.TextStyle{Bold: true}))

			for i, label := range result.Labels {
				capturedLabel := label
				group := statusRow.Objects[i*2].(*fyne.Container)
				lbl := group.Objects[0].(*widget.Label)
				btn := group.Objects[1].(*widget.Button)

				lbl.SetText("✓ " + label)
				btn.OnTapped = func() {
					rv.showSameLabelMenu(btn, status, capturedLabel)
				}
			}
		},
	)

	return list
}

func (rv *ResultView) showDifferLabelMenu(btn *widget.Button, status *core.FileStatus, label string) {
	var pe *models.PathEntry
	for i := range rv.project.Paths {
		if rv.project.Paths[i].Label == label {
			pe = &rv.project.Paths[i]
			break
		}
	}

	openItem := fyne.NewMenuItem("Open Folder", func() {
		if pe != nil {
			rv.openFolderForLabel(status, *pe)
		}
	})
	openItem.Disabled = pe == nil || pe.Type.IsFTP()

	replaceItem := fyne.NewMenuItem("Replace...", func() {
		rv.showReplaceDialog(status, label)
	})
	replaceItem.Disabled = pe == nil || pe.Type.IsFTP()

	menu := fyne.NewMenu("", openItem, fyne.NewMenuItemSeparator(), replaceItem)
	absPos := fyne.CurrentApp().Driver().AbsolutePositionForObject(btn)
	widget.ShowPopUpMenuAtPosition(menu, rv.win.Canvas(), fyne.NewPos(absPos.X, absPos.Y+btn.Size().Height))
}

func (rv *ResultView) showReplaceDialog(status *core.FileStatus, srcLabel string) {
	var srcPE *models.PathEntry
	for i := range rv.project.Paths {
		if rv.project.Paths[i].Label == srcLabel {
			srcPE = &rv.project.Paths[i]
			break
		}
	}

	type targetRow struct {
		pe    models.PathEntry
		check *widget.Check
	}

	var rows []targetRow
	for _, pe := range rv.project.Paths {
		if pe.Label == srcLabel {
			continue
		}
		var checkLabel string
		if pe.Type.IsFTP() {
			checkLabel = pe.Label + "  (FTP — not supported)"
		} else {
			checkLabel = pe.Label
		}
		ch := widget.NewCheck(checkLabel, nil)
		if pe.Type.IsFTP() {
			ch.Disable()
		}
		rows = append(rows, targetRow{pe: pe, check: ch})
	}

	descLabel := widget.NewLabel(fmt.Sprintf("Replace with the version from  \"%s\"  in:", srcLabel))
	checkboxes := make([]fyne.CanvasObject, 0, len(rows))
	for _, r := range rows {
		checkboxes = append(checkboxes, r.check)
	}
	body := container.NewVBox(append([]fyne.CanvasObject{descLabel}, checkboxes...)...)

	var popup *StyledPopup
	popup = NewStyledPopup(rv.win, "Replace File", body, []PopupButton{
		{Text: "Cancel"},
		{Text: "Replace", Importance: widget.HighImportance, KeepOpen: true, OnTapped: func() {
			var targets []models.PathEntry
			for _, r := range rows {
				if r.check.Checked {
					targets = append(targets, r.pe)
				}
			}
			popup.Hide()
			if len(targets) > 0 && srcPE != nil {
				rv.replaceFileInPaths(status, *srcPE, targets)
			}
		}},
	})
	popup.Resize(fyne.NewSize(380, float32(120+len(rows)*40)))
	popup.Show()
}

// reclassifyDifferStatus moves status from result.Differ to result.Same when
// all labels now share the same hash after a replace operation.
func (rv *ResultView) reclassifyDifferStatus(status *core.FileStatus) {
	if !checkAllSame(status.Entries, rv.result.Labels) {
		return
	}

	for i, ds := range rv.result.Differ {
		if ds == status {
			rv.result.Differ = append(rv.result.Differ[:i], rv.result.Differ[i+1:]...)
			break
		}
	}

	status.AllSame = true
	rv.result.Same = append(rv.result.Same, status)
}

func (rv *ResultView) showSameLabelMenu(btn *widget.Button, status *core.FileStatus, label string) {
	var pe *models.PathEntry
	for i := range rv.project.Paths {
		if rv.project.Paths[i].Label == label {
			pe = &rv.project.Paths[i]
			break
		}
	}

	openItem := fyne.NewMenuItem("Open Folder", func() {
		if pe != nil {
			rv.openFolderForLabel(status, *pe)
		}
	})
	openItem.Disabled = pe == nil || pe.Type.IsFTP()

	menu := fyne.NewMenu("", openItem)
	absPos := fyne.CurrentApp().Driver().AbsolutePositionForObject(btn)
	widget.ShowPopUpMenuAtPosition(menu, rv.win.Canvas(), fyne.NewPos(absPos.X, absPos.Y+btn.Size().Height))
}
