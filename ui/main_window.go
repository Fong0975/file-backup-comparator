package ui

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"sort"
	"strings"
	"sync"
	"time"

	"filecompare/core"
	"filecompare/models"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	progressDialogWidth  = 440
	progressDialogHeight = 265
)

type mainWindow struct {
	win           fyne.Window
	cfg           *models.AppConfig
	versionInfo   models.VersionInfo
	sidebarVBox   *fyne.Container
	sidebarScroll *container.Scroll
	resultView    *ResultView
	selectedIdx   int
	// resultCache stores the most recent CompareResult per project ID so that
	// batch-compare results can be browsed from the sidebar after the progress
	// dialog closes. Keyed by Project.ID, value is *core.CompareResult.
	resultCache sync.Map
}

// NewMainWindow creates and returns the application's main window.
func NewMainWindow(a fyne.App, versionInfo models.VersionInfo) fyne.Window {
	mw := &mainWindow{selectedIdx: -1, versionInfo: versionInfo}
	mw.win = a.NewWindow("File Backup Comparator")
	mw.win.Resize(fyne.NewSize(1280, 720))
	mw.win.CenterOnScreen()
	mw.win.SetOnClosed(clearFTPSessionCache)

	cfg, err := models.LoadConfig()
	if err != nil {
		cfg = &models.AppConfig{}
	}
	mw.cfg = cfg
	mw.build()
	return mw.win
}

func (mw *mainWindow) build() {
	mw.sidebarVBox = container.NewVBox()
	mw.sidebarScroll = container.NewVScroll(mw.sidebarVBox)
	mw.buildSidebar()

	addBtn := widget.NewButtonWithIcon("New", theme.ContentAddIcon(), mw.onAdd)
	compareAllBtn := widget.NewButtonWithIcon("Compare", theme.MediaPlayIcon(), mw.onCompareAll)
	settingsBtn := widget.NewButtonWithIcon("Settings", theme.SettingsIcon(), mw.onSettings)

	toolbar := container.NewHBox(addBtn, widget.NewSeparator(), compareAllBtn, widget.NewSeparator(), settingsBtn)

	leftPanel := container.NewBorder(
		container.NewVBox(toolbar, widget.NewSeparator()),
		nil, nil, nil,
		mw.sidebarScroll,
	)

	mw.resultView = NewResultView(mw.win)
	mw.resultView.SetOnUpdateProject(func(updated models.Project) {
		for i, p := range mw.cfg.Projects {
			if p.ID == updated.ID {
				mw.cfg.Projects[i] = updated
				mw.saveAndRefresh()
				return
			}
		}
	})

	split := container.NewHSplit(leftPanel, mw.resultView.Container())
	split.SetOffset(0.22)

	mw.win.SetContent(split)
}

func (mw *mainWindow) onAdd() {
	p := models.Project{
		ID:   fmt.Sprintf("%d", time.Now().UnixNano()),
		Name: "New Project",
	}
	NewProjectEditor(mw.win, &p, mw.cfg.Settings.FTPServers, mw.cfg.Settings.Labels, func(saved *models.Project) {
		mw.cfg.Projects = append(mw.cfg.Projects, *saved)
		mw.saveAndRefresh()
	}).Show()
}

func (mw *mainWindow) onEdit() {
	if !mw.checkSelected() {
		return
	}
	p := mw.cfg.Projects[mw.selectedIdx]
	idx := mw.selectedIdx
	NewProjectEditor(mw.win, &p, mw.cfg.Settings.FTPServers, mw.cfg.Settings.Labels, func(saved *models.Project) {
		mw.cfg.Projects[idx] = *saved
		mw.saveAndRefresh()
	}).Show()
}

func (mw *mainWindow) onDelete() {
	if !mw.checkSelected() {
		return
	}
	name := mw.cfg.Projects[mw.selectedIdx].Name
	idx := mw.selectedIdx
	ShowConfirmPopup(mw.win, "Delete Project",
		fmt.Sprintf("Delete project '%s'? This cannot be undone.", name),
		"Delete", "Cancel",
		func(ok bool) {
			if !ok {
				return
			}
			mw.cfg.Projects = append(mw.cfg.Projects[:idx], mw.cfg.Projects[idx+1:]...)
			mw.selectedIdx = -1
			mw.saveAndRefresh()
		})
}

func (mw *mainWindow) onSettings() {
	showSettingsDialog(mw.win, mw.cfg, mw.versionInfo, mw.saveAndRefresh)
}

func (mw *mainWindow) onCompareAll() {
	if len(mw.cfg.Projects) == 0 {
		ShowInfoPopup(mw.win, "No Projects", "Add at least one project first.")
		return
	}
	showProjectSelectDialog(mw.win, mw.cfg.Projects, func(indices []int) {
		go func() {
			entries := make([]*multiCompareEntry, 0, len(indices))
			for _, idx := range indices {
				rp, creds, ok := resolveFTPCredentials(mw.win, mw.cfg.Projects[idx])
				if !ok {
					return
				}
				entries = append(entries, &multiCompareEntry{
					project:   rp,
					creds:     creds,
					modeLabel: projectModeLabel(rp),
				})
			}
			showMultiCompareProgress(mw.win, entries, mw.cfg.Settings.GlobalExclusions,
				func(project models.Project, result *core.CompareResult) {
					mw.resultCache.Store(project.ID, result)
					mw.buildSidebar()
				},
			)
		}()
	})
}

func (mw *mainWindow) onCompare() {
	if !mw.checkSelected() {
		return
	}

	project := mw.cfg.Projects[mw.selectedIdx]
	if len(project.Paths) < 2 {
		ShowInfoPopup(mw.win, "Info", "Please add at least 2 paths to compare.")
		return
	}
	// Credential resolution blocks on dialog responses, so the whole flow
	// must run off the Fyne UI/driver goroutine (see resolveFTPCredentials).
	go func() {
		resolvedProject, creds, ok := resolveFTPCredentials(mw.win, project)
		if !ok {
			return
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// canvas.Text has no implicit padding, so it lines up flush-left with
		// the progress bar and current-file row below it (unlike
		// widget.Label, which reserves its own padding around the text).
		descText := canvas.NewText("Scanning files...", theme.Color(theme.ColorNameForeground))
		descText.TextSize = theme.TextSize()
		progressBar := widget.NewProgressBar()

		currentFileText := canvas.NewText("", theme.Color(theme.ColorNamePlaceHolder))
		currentFileText.TextSize = theme.CaptionTextSize()
		// Leave generous breathing room on the right edge so the truncated
		// path never sits flush against the dialog border.
		currentFileMaxWidth := progressDialogWidth - 2*popupSideMargin() - 4*theme.Padding()
		// Pin this row to a fixed cell size so the dialog's width never
		// grows or shrinks to fit the current file path being displayed.
		currentFileRow := container.New(
			layout.NewGridWrapLayout(fyne.NewSize(currentFileMaxWidth, currentFileText.MinSize().Height)),
			currentFileText,
		)

		var modeLabel string
		switch {
		case resolvedProject.MetadataOnlyAll:
			modeLabel = "Full metadata comparison"
		case len(resolvedProject.MetadataOnlyPatterns) > 0:
			modeLabel = "Mixed comparison"
		default:
			modeLabel = "Full comparison"
		}
		modeText := canvas.NewText(modeLabel, theme.Color(theme.ColorNamePlaceHolder))
		modeText.TextSize = theme.CaptionTextSize()

		body := container.NewVBox(descText, modeText, progressBar, currentFileRow)
		prog := NewStyledPopup(mw.win, "Comparing", body, []PopupButton{
			{
				Text:     "Cancel",
				KeepOpen: true,
				OnTapped: func() {
					ShowConfirmPopup(mw.win, "Cancel Compare", "Are you sure you want to stop the current scan?",
						"Stop", "Keep Going",
						func(ok bool) {
							if ok {
								cancel()
							}
						})
				},
			},
		})
		prog.Resize(fyne.NewSize(progressDialogWidth, progressDialogHeight))
		prog.Show()

		var statusMu sync.Mutex
		onProgress := func(e core.ProgressEvent) {
			statusMu.Lock()
			defer statusMu.Unlock()
			switch e.Phase {
			case core.PhaseScanning:
				descText.Text = fmt.Sprintf("Scanning files... (%d found)", e.ScannedFiles)
				descText.Refresh()
			case core.PhaseHashing:
				var pct float64
				if e.BytesTotal > 0 {
					pct = float64(e.BytesDone) / float64(e.BytesTotal)
				}
				progressBar.SetValue(pct)
				descText.Text = fmt.Sprintf("Comparing files... %.0f%%", pct*100)
				descText.Refresh()
				currentFileText.Text = truncateToWidth(e.FilePath, currentFileMaxWidth, currentFileText.TextSize, currentFileText.TextStyle)
				currentFileText.Refresh()
			}
		}

		result, err := core.Compare(ctx, resolvedProject, creds, mw.cfg.Settings.GlobalExclusions, onProgress)
		prog.Hide()

		if err != nil {
			if !errors.Is(err, context.Canceled) {
				ShowInfoPopup(mw.win, "Error", err.Error())
			}
			return
		}

		sort.Slice(result.Missing, func(i, j int) bool { return result.Missing[i].RelPath < result.Missing[j].RelPath })
		sort.Slice(result.Differ, func(i, j int) bool { return result.Differ[i].RelPath < result.Differ[j].RelPath })
		sort.Slice(result.Same, func(i, j int) bool { return result.Same[i].RelPath < result.Same[j].RelPath })

		mw.resultCache.Store(project.ID, result)
		mw.buildSidebar()
		mw.resultView.Update(result, project)

		if len(result.Warnings) > 0 {
			ShowInfoPopup(mw.win, "Some Paths Were Skipped", strings.Join(result.Warnings, "\n"))
		}
	}()
}

func (mw *mainWindow) checkSelected() bool {
	if mw.selectedIdx < 0 || mw.selectedIdx >= len(mw.cfg.Projects) {
		ShowInfoPopup(mw.win, "Info", "Please select a project first.")
		return false
	}
	return true
}

// buildSidebar recreates all sidebar rows from mw.cfg.Projects and replaces
// the VBox content. Safe to call any time projects are added, removed, or
// reordered.
func (mw *mainWindow) buildSidebar() {
	rows := make([]fyne.CanvasObject, len(mw.cfg.Projects))

	for i := range mw.cfg.Projects {
		capturedIdx := i
		p := mw.cfg.Projects[i]

		// Status dot: green = all identical, red = differences found,
		// transparent = not yet compared.
		var dotClr color.Color = color.Transparent
		if val, ok := mw.resultCache.Load(p.ID); ok {
			r := val.(*core.CompareResult)
			if len(r.Missing) > 0 || len(r.Differ) > 0 {
				dotClr = color.NRGBA{R: 220, G: 53, B: 69, A: 255}
			} else {
				dotClr = color.NRGBA{R: 40, G: 167, B: 69, A: 255}
			}
		}
		dot := canvas.NewCircle(dotClr)
		const dotDiam float32 = 10
		dotFixed := container.New(layout.NewGridWrapLayout(fyne.NewSize(dotDiam, dotDiam)), dot)

		// Clicking the project name shows its cached result (if available).
		nameBtn := widget.NewButtonWithIcon(p.Name, theme.FolderIcon(), func() {
			mw.selectedIdx = capturedIdx
			if val, ok := mw.resultCache.Load(mw.cfg.Projects[capturedIdx].ID); ok {
				mw.resultView.Update(val.(*core.CompareResult), mw.cfg.Projects[capturedIdx])
			}
		})
		nameBtn.Alignment = widget.ButtonAlignLeading
		nameBtn.Importance = widget.LowImportance
		if _, ok := mw.resultCache.Load(p.ID); !ok {
			nameBtn.Disable()
		}

		btnBox := container.NewHBox(
			widget.NewButtonWithIcon("", theme.MediaPlayIcon(), func() {
				mw.selectedIdx = capturedIdx
				mw.onCompare()
			}),
			widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), func() {
				mw.selectedIdx = capturedIdx
				mw.onEdit()
			}),
			widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
				mw.selectedIdx = capturedIdx
				mw.onDelete()
			}),
		)
		border := container.NewBorder(nil, nil, dotFixed, btnBox, nameBtn)
		rows[i] = container.New(layout.NewCustomPaddedLayout(0, 0, theme.Padding(), 0), border)
	}

	mw.sidebarVBox.Objects = rows
	mw.sidebarVBox.Refresh()
	mw.sidebarScroll.Refresh()
}

func (mw *mainWindow) saveAndRefresh() {
	if err := models.SaveConfig(mw.cfg); err != nil {
		ShowInfoPopup(mw.win, "Error", err.Error())
	}
	mw.buildSidebar()
}
