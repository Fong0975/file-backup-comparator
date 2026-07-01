package ui

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"

	"filecompare/core"
	"filecompare/models"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const multiCompareDialogWidth float32 = 520

// multiCompareEntry holds the UI widgets for one project's progress row.
// status and file are canvas.Text (not widget.Label) because onProgress is
// called from hashAllFiles worker goroutines concurrently, and widget.Label
// internally uses widget.RichText whose textRenderer holds an RWMutex that
// is also accessed by the Fyne render goroutine — concurrent access causes
// "RUnlock of unlocked RWMutex" and a fatal crash. canvas.Text has a simpler
// refresh path (a plain invalidation notification) that is safe to trigger
// from any goroutine.
type multiCompareEntry struct {
	project   models.Project
	creds     core.FTPCredentials
	modeLabel string
	bar       *widget.ProgressBar
	status    *canvas.Text
	file      *canvas.Text
	// block is the visual unit for this project (row content + optional bottom
	// separator). Hiding it collapses the row from the scroll list.
	block fyne.CanvasObject
	mu    sync.Mutex
}

func projectModeLabel(p models.Project) string {
	switch {
	case p.MetadataOnlyAll:
		return "Full metadata comparison"
	case len(p.MetadataOnlyPatterns) > 0:
		return "Mixed comparison"
	default:
		return "Full comparison"
	}
}

// showProjectSelectDialog shows a checkbox list for picking which projects to
// compare. Projects with fewer than 2 paths are shown disabled. onConfirm
// receives the slice of selected indices into the original projects slice.
func showProjectSelectDialog(win fyne.Window, projects []models.Project, onConfirm func([]int)) {
	checks := make([]*widget.Check, len(projects))
	rows := make([]fyne.CanvasObject, len(projects))
	for i, p := range projects {
		c := widget.NewCheck(p.Name, nil)
		if len(p.Paths) < 2 {
			c.Disable()
		} else {
			c.SetChecked(true)
		}
		checks[i] = c
		rows[i] = c
	}

	allSelected := true
	toggleBtn := widget.NewButton("Deselect All", nil)
	toggleBtn.OnTapped = func() {
		allSelected = !allSelected
		for _, c := range checks {
			if !c.Disabled() {
				c.SetChecked(allSelected)
			}
		}
		if allSelected {
			toggleBtn.SetText("Deselect All")
		} else {
			toggleBtn.SetText("Select All")
		}
	}

	listHeader := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle("Select Projects", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		toggleBtn,
	)

	const perRowH float32 = 36
	listH := float32(len(projects)) * perRowH
	if listH > 300 {
		listH = 300
	}
	scroll := container.NewVScroll(container.NewVBox(rows...))
	scroll.SetMinSize(fyne.NewSize(400, listH))

	body := container.NewVBox(listHeader, widget.NewSeparator(), scroll)

	var d *StyledPopup
	d = NewStyledPopup(win, "Compare Projects", body, []PopupButton{
		{Text: "Cancel"},
		{Text: "Compare", Importance: widget.HighImportance, KeepOpen: true, OnTapped: func() {
			var sel []int
			for i, c := range checks {
				if c.Checked {
					sel = append(sel, i)
				}
			}
			if len(sel) == 0 {
				ShowInfoPopup(win, "Nothing Selected", "Please select at least one project.")
				return
			}
			d.Hide()
			onConfirm(sel)
		}},
	})
	dialogH := listH + 160
	if dialogH < 240 {
		dialogH = 240
	}
	if dialogH > 500 {
		dialogH = 500
	}
	d.Resize(fyne.NewSize(460, dialogH))
	d.Show()
}

func buildProgressRow(e *multiCompareEntry) fyne.CanvasObject {
	nameLabel := widget.NewLabelWithStyle(e.project.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	nameLabel.Truncation = fyne.TextTruncateEllipsis

	modeTxt := canvas.NewText(e.modeLabel, theme.Color(theme.ColorNamePlaceHolder))
	modeTxt.TextSize = theme.CaptionTextSize()

	e.bar = widget.NewProgressBar()

	e.status = canvas.NewText("Waiting...", theme.Color(theme.ColorNameForeground))
	e.status.TextSize = theme.TextSize()

	e.file = canvas.NewText("", theme.Color(theme.ColorNamePlaceHolder))
	e.file.TextSize = theme.CaptionTextSize()

	headerRow := container.NewBorder(nil, nil, nameLabel, modeTxt)

	return container.NewVBox(headerRow, e.bar, e.status, e.file)
}

// showMultiCompareProgress shows a progress dialog and starts a goroutine per
// entry. All comparisons run concurrently. Each project's row disappears from
// the list as soon as it completes; when the last project finishes the dialog
// closes automatically. onProjectDone is called for each successfully
// completed project.
func showMultiCompareProgress(
	win fyne.Window,
	entries []*multiCompareEntry,
	globalExclusions []string,
	onProjectDone func(models.Project, *core.CompareResult),
) {
	n := len(entries)

	summaryLabel := canvas.NewText(fmt.Sprintf("0 / %d completed", n), theme.Color(theme.ColorNameForeground))
	summaryLabel.TextSize = theme.TextSize()

	// Build one visual block per entry. Each block is the row content plus a
	// bottom separator — except the last entry which has no trailing separator.
	// Hiding a block collapses the whole entry (content + separator) cleanly.
	blocks := make([]fyne.CanvasObject, n)
	for i, e := range entries {
		rowContent := buildProgressRow(e)
		if i < n-1 {
			blk := container.NewVBox(rowContent, widget.NewSeparator())
			e.block = blk
			blocks[i] = blk
		} else {
			e.block = rowContent
			blocks[i] = rowContent
		}
	}

	const perRowH float32 = 85
	listH := float32(n) * perRowH
	if listH > 400 {
		listH = 400
	}
	rowsVBox := container.NewVBox(blocks...)
	scroll := container.NewVScroll(rowsVBox)
	scroll.SetMinSize(fyne.NewSize(multiCompareDialogWidth-4*theme.Padding(), listH))

	body := container.NewVBox(summaryLabel, widget.NewSeparator(), scroll)

	ctx, cancel := context.WithCancel(context.Background())

	var d *StyledPopup
	d = NewStyledPopup(win, fmt.Sprintf("Comparing %d Projects", n), body, []PopupButton{
		{Text: "Cancel", KeepOpen: true, OnTapped: func() {
			ShowConfirmPopup(win, "Cancel Comparisons", "Stop all running comparisons?",
				"Stop", "Keep Going",
				func(ok bool) {
					if ok {
						cancel()
						d.Hide()
					}
				})
		}},
	})
	dialogH := listH + 140
	if dialogH < 260 {
		dialogH = 260
	}
	d.Resize(fyne.NewSize(multiCompareDialogWidth, dialogH))
	d.Show()

	fileMaxWidth := multiCompareDialogWidth - 2*popupSideMargin() - 4*theme.Padding()

	var completedCount atomic.Int32

	for _, e := range entries {
		e := e
		go func() {
			onProgress := func(ev core.ProgressEvent) {
				e.mu.Lock()
				defer e.mu.Unlock()
				switch ev.Phase {
				case core.PhaseScanning:
					e.status.Text = fmt.Sprintf("Scanning... (%d found)", ev.ScannedFiles)
					e.status.Refresh()
				case core.PhaseHashing:
					var pct float64
					if ev.BytesTotal > 0 {
						pct = float64(ev.BytesDone) / float64(ev.BytesTotal)
					}
					e.bar.SetValue(pct)
					e.status.Text = fmt.Sprintf("Comparing... %.0f%%", pct*100)
					e.status.Refresh()
					e.file.Text = truncateToWidth(ev.FilePath, fileMaxWidth, e.file.TextSize, e.file.TextStyle)
					e.file.Refresh()
				}
			}

			result, err := core.Compare(ctx, e.project, e.creds, globalExclusions, onProgress)

			if err == nil {
				e.bar.SetValue(1)
				e.status.Text = "Done"
				e.status.Refresh()
				if len(result.Warnings) > 0 {
					e.file.Text = truncateToWidth(result.Warnings[0], fileMaxWidth, e.file.TextSize, e.file.TextStyle)
					e.file.Refresh()
				}

				sort.Slice(result.Missing, func(i, j int) bool { return result.Missing[i].RelPath < result.Missing[j].RelPath })
				sort.Slice(result.Differ, func(i, j int) bool { return result.Differ[i].RelPath < result.Differ[j].RelPath })
				sort.Slice(result.Same, func(i, j int) bool { return result.Same[i].RelPath < result.Same[j].RelPath })

				if onProjectDone != nil {
					onProjectDone(e.project, result)
				}
			} else if errors.Is(err, context.Canceled) {
				e.status.Text = "Canceled"
				e.status.Refresh()
				e.file.Text = ""
				e.file.Refresh()
			} else {
				e.status.Text = "Error: " + err.Error()
				e.status.Refresh()
				e.file.Text = ""
				e.file.Refresh()
			}

			count := int(completedCount.Add(1))
			if count < n {
				summaryLabel.Text = fmt.Sprintf("%d / %d completed", count, n)
				summaryLabel.Refresh()
				e.block.Hide()
				rowsVBox.Refresh()
			} else {
				cancel()
				d.Hide()
			}
		}()
	}
}
