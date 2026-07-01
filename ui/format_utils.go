package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
)

// truncateToWidth shortens s, if needed, so that it renders no wider than
// maxWidth at the given text size/style, keeping the head and tail and
// replacing the middle portion with an ellipsis. Character widths vary, so a
// fixed rune count alone can't guarantee the text fits a fixed-width area.
func truncateToWidth(s string, maxWidth float32, textSize float32, style fyne.TextStyle) string {
	if fyne.MeasureText(s, textSize, style).Width <= maxWidth {
		return s
	}

	const ellipsis = "…"
	r := []rune(s)

	lo, hi := 0, len(r)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		head := mid / 2
		tail := mid - head
		candidate := string(r[:head]) + ellipsis + string(r[len(r)-tail:])
		if fyne.MeasureText(candidate, textSize, style).Width <= maxWidth {
			lo = mid
		} else {
			hi = mid - 1
		}
	}

	if lo <= 0 {
		return ellipsis
	}
	head := lo / 2
	tail := lo - head
	return string(r[:head]) + ellipsis + string(r[len(r)-tail:])
}

// truncateRelPath shortens a slash-separated relative path to fit within
// maxWidth by removing middle directory segments, always preserving the
// filename (last segment) and as many leading segments as possible. Falls back
// to character-level mid-truncation when even the shortest segment form is too
// wide.
func truncateRelPath(p string, maxWidth float32, textSize float32, style fyne.TextStyle) string {
	if fyne.MeasureText(p, textSize, style).Width <= maxWidth {
		return p
	}

	parts := strings.Split(p, "/")
	n := len(parts)
	if n <= 2 {
		return truncateToWidth(p, maxWidth, textSize, style)
	}

	filename := parts[n-1]
	for leading := n - 2; leading >= 1; leading-- {
		candidate := strings.Join(parts[:leading], "/") + "/…/" + filename
		if fyne.MeasureText(candidate, textSize, style).Width <= maxWidth {
			return candidate
		}
	}

	return truncateToWidth("…/"+filename, maxWidth, textSize, style)
}

func formatSize(size int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case size >= gb:
		return fmt.Sprintf("%.2f GB", float64(size)/gb)
	case size >= mb:
		return fmt.Sprintf("%.2f MB", float64(size)/mb)
	case size >= kb:
		return fmt.Sprintf("%.2f KB", float64(size)/kb)
	default:
		return fmt.Sprintf("%d B", size)
	}
}
