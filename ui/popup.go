package ui

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// StyledPopup is a modal popup built directly on widget.PopUp instead of the
// dialog package. dialog's own internal layout always reserves space for a
// title label and a button row above/below the content, even when both are
// left empty (an empty Label still has a non-zero line height) — so a
// header/footer built from dialog.NewCustomWithoutButtons can never sit
// truly flush against the popup's edges. StyledPopup has no such hidden
// reservation: the margins applied in NewStyledPopup are the only padding
// there is, plus the small fixed inset widget.PopUp itself always applies.
type StyledPopup struct {
	popup    *widget.PopUp
	onClosed func()
}

// Show displays the popup.
func (p *StyledPopup) Show() { p.popup.Show() }

// Hide dismisses the popup and runs any callback set via SetOnClosed.
func (p *StyledPopup) Hide() {
	p.popup.Hide()
	if p.onClosed != nil {
		p.onClosed()
	}
}

// Resize sets the popup's overall size.
func (p *StyledPopup) Resize(size fyne.Size) { p.popup.Resize(size) }

// SetOnClosed registers a callback invoked whenever this popup is hidden.
func (p *StyledPopup) SetOnClosed(f func()) { p.onClosed = f }

// popupSideMargin is the left/right space reserved for every section of a
// styled popup, so the header's title, the body, and the footer's buttons
// all share the same left edge and never touch the window's sides.
func popupSideMargin() float32 {
	return theme.Padding() * 2
}

// popupEdgeMargin is the top/bottom space left above the header and below
// the footer when they're present. It's intentionally small: the header
// should sit close to the window's top edge and the footer close to its
// bottom edge, rather than floating in the middle of an oversized margin.
func popupEdgeMargin() float32 {
	return theme.Padding()
}

// popupSectionPadding is the vertical space reserved above and below the
// header's title text and, symmetrically, above and below the footer's
// button row. It's applied explicitly (rather than left to each widget's
// own internal padding) because a Label and a Button don't reserve the same
// amount of internal padding, so without this the header and footer would
// look unevenly spaced even though the surrounding layout is identical.
func popupSectionPadding() float32 {
	return theme.Padding() * 2
}

// popupGroove renders a thin horizontal divider with a subtle "etched" look
// — a dark line immediately above a light one — instead of a single flat
// line, marking the boundary between sections more like a carved-in seam.
func popupGroove() fyne.CanvasObject {
	dark := canvas.NewRectangle(theme.Color(theme.ColorNameSeparator))
	dark.SetMinSize(fyne.NewSize(1, 1))

	light := canvas.NewRectangle(color.NRGBA{R: 255, G: 255, B: 255, A: 20})
	light.SetMinSize(fyne.NewSize(1, 1))

	return container.New(layout.NewCustomPaddedVBoxLayout(0), dark, light)
}

// PopupButton describes one footer button on a styled popup.
type PopupButton struct {
	Text       string
	Importance widget.Importance
	// OnTapped runs when the button is pressed, before the popup closes.
	OnTapped func()
	// KeepOpen prevents the popup from closing automatically after this
	// button is tapped. Use this when OnTapped manages the popup's
	// lifecycle itself, e.g. by showing its own confirmation dialog first.
	KeepOpen bool
}

// NewStyledPopup builds a StyledPopup with the header/body/footer layout
// shared by every non-main-window popup in this app:
//   - header (shown only if title != ""), body, and footer (shown only if
//     len(buttons) > 0) all share the same left/right margin, so the body's
//     left edge lines up under the header's title text
//   - the header (if present) is pinned flush to the popup's top edge (after
//     a small margin) and the footer (if present) flush to its bottom edge;
//     the body fills all remaining space in between, so resizing the popup
//     larger than its natural content size never leaves dead space floating
//     beneath the footer
//   - a groove divider marks the header/body boundary; the body/footer
//     boundary instead gets double the usual whitespace above the footer's
//     buttons, with no line at all
//   - the header is bold and left-aligned
//   - the body keeps its natural top/left alignment unless the caller wraps
//     it in something else (e.g. container.NewCenter) before passing it in
//   - footer buttons are right-aligned; tapping one hides the popup unless
//     that button's KeepOpen is set
func NewStyledPopup(parent fyne.Window, title string, body fyne.CanvasObject, buttons []PopupButton) *StyledPopup {
	hasHeader := title != ""
	hasFooter := len(buttons) > 0

	sectionPad := popupSectionPadding()

	var top fyne.CanvasObject
	if hasHeader {
		headerLabel := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		paddedHeader := container.New(layout.NewCustomPaddedLayout(sectionPad, sectionPad, 0, 0), headerLabel)
		top = container.New(layout.NewCustomPaddedVBoxLayout(0), paddedHeader, popupGroove())
	}

	sp := &StyledPopup{}

	var bottom fyne.CanvasObject
	if hasFooter {
		footerRow := make([]fyne.CanvasObject, 0, len(buttons)+1)
		footerRow = append(footerRow, layout.NewSpacer())
		for _, b := range buttons {
			cb := b.OnTapped
			keepOpen := b.KeepOpen

			btn := widget.NewButton(b.Text, nil)
			btn.Importance = b.Importance
			btn.OnTapped = func() {
				if cb != nil {
					cb()
				}
				if !keepOpen {
					sp.Hide()
				}
			}
			footerRow = append(footerRow, btn)
		}
		// No divider between body and footer here: the separation comes from
		// doubling the footer's top whitespace instead of drawing a line.
		bottom = container.New(layout.NewCustomPaddedLayout(2*sectionPad, sectionPad, 0, 0), container.NewHBox(footerRow...))
	}

	// Border (not VBox) is essential here: it pins top/bottom to the actual
	// edges of whatever size the popup ends up at and stretches the center
	// (body) to fill the rest. A VBox would just stack everything at its
	// natural height and leave any extra space as a dead gap below the
	// footer whenever the popup is resized larger than that natural size.
	inner := container.NewBorder(top, bottom, nil, nil, body)

	sideMargin := popupSideMargin()
	topMargin, bottomMargin := sideMargin, sideMargin
	if hasHeader {
		topMargin = popupEdgeMargin()
	}
	if hasFooter {
		bottomMargin = popupEdgeMargin()
	}
	padded := container.New(layout.NewCustomPaddedLayout(topMargin, bottomMargin, sideMargin, sideMargin), inner)

	sp.popup = widget.NewModalPopUp(padded, parent.Canvas())
	return sp
}

const (
	textPopupMinWidth  = 360
	textPopupMaxWidth  = 560
	textPopupMinHeight = 160
	textPopupMaxHeight = 420
)

// sizeForMessage picks a popup size that fits message without wrapping every
// word onto its own line, capped so it doesn't look absurdly large for long
// messages (e.g. a list of warnings). Plain word-wrapped widget.Label has no
// reliable natural width to size a dialog from, so callers showing a single
// text message should size the popup explicitly using this.
func sizeForMessage(message string) fyne.Size {
	maxLineWidth := float32(0)
	lines := strings.Split(message, "\n")
	for _, line := range lines {
		if w := fyne.MeasureText(line, theme.TextSize(), fyne.TextStyle{}).Width; w > maxLineWidth {
			maxLineWidth = w
		}
	}

	width := maxLineWidth + 2*popupSideMargin() + 4*theme.Padding()
	width = fyne.Max(textPopupMinWidth, fyne.Min(width, textPopupMaxWidth))

	lineHeight := fyne.MeasureText("M", theme.TextSize(), fyne.TextStyle{}).Height
	// Header + separator + footer + separator + margins, plus the message
	// lines themselves with slack for any wrapping the width cap forces.
	height := float32(len(lines))*lineHeight*1.6 + 120
	height = fyne.Max(textPopupMinHeight, fyne.Min(height, textPopupMaxHeight))

	return fyne.NewSize(width, height)
}

// ShowInfoPopup shows a single-button styled popup carrying a plain text
// message. It returns immediately; the popup itself is non-blocking. Use
// this in place of dialog.ShowInformation/ShowError so every popup in the
// app shares the same look.
func ShowInfoPopup(parent fyne.Window, title, message string) {
	body := widget.NewLabel(message)
	body.Wrapping = fyne.TextWrapWord

	d := NewStyledPopup(parent, title, body, []PopupButton{
		{Text: "OK", Importance: widget.HighImportance},
	})
	d.Resize(sizeForMessage(message))
	d.Show()
}

// ShowConfirmPopup shows a two-button styled popup carrying a plain text
// message. callback is invoked with true if confirmText is pressed, or
// false if cancelText is pressed. Use this in place of dialog.ShowConfirm.
func ShowConfirmPopup(parent fyne.Window, title, message, confirmText, cancelText string, callback func(bool)) {
	body := widget.NewLabel(message)
	body.Wrapping = fyne.TextWrapWord

	d := NewStyledPopup(parent, title, body, []PopupButton{
		{Text: cancelText, OnTapped: func() { callback(false) }},
		{Text: confirmText, Importance: widget.HighImportance, OnTapped: func() { callback(true) }},
	})
	d.Resize(sizeForMessage(message))
	d.Show()
}
