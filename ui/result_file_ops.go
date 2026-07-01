package ui

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"filecompare/core"
	"filecompare/models"
)

// openFolderForLabel opens the parent directory of status.RelPath under pe.
// If the target directory does not exist, it walks up toward pe.Path until it
// finds an existing ancestor and opens that instead.
func (rv *ResultView) openFolderForLabel(status *core.FileStatus, pe models.PathEntry) {
	target := filepath.Join(pe.Path, filepath.FromSlash(path.Dir(status.RelPath)))

	dir := target
	for {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// reached the filesystem root without finding a directory;
			// fall back to the path entry's own root
			dir = pe.Path
			break
		}
		dir = parent
	}

	if err := openDirectory(dir); err != nil {
		ShowInfoPopup(rv.win, "Error", fmt.Sprintf("Could not open folder: %v", err))
	}
}

func openDirectory(dir string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer.exe", dir).Start()
	case "darwin":
		return exec.Command("open", dir).Start()
	default:
		return exec.Command("xdg-open", dir).Start()
	}
}

// copyFileToPath copies status.RelPath from src's local folder to dst's local folder,
// then immediately updates the in-memory result so the view reflects the change
// without requiring a full re-run of Compare.
func (rv *ResultView) copyFileToPath(status *core.FileStatus, src, dst models.PathEntry) {
	srcPath := filepath.Join(src.Path, filepath.FromSlash(status.RelPath))
	dstPath := filepath.Join(dst.Path, filepath.FromSlash(status.RelPath))

	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		ShowInfoPopup(rv.win, "Copy Failed", fmt.Sprintf("Could not create directory: %v", err))
		return
	}

	if err := copyLocalFile(srcPath, dstPath); err != nil {
		ShowInfoPopup(rv.win, "Copy Failed", fmt.Sprintf("Could not copy file: %v", err))
		return
	}

	// The copied file has identical content, so it shares the source's FileInfo.
	status.Entries[dst.Label] = status.Entries[src.Label]
	rv.reclassifyStatus(status)
	rv.Update(rv.result, rv.project)

	ShowInfoPopup(rv.win, "Copy Complete", fmt.Sprintf("Copied to:\n%s", dstPath))
}

// copyLocalFile copies src to dst atomically via a temporary file in the same directory.
func copyLocalFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	tmp, err := os.CreateTemp(filepath.Dir(dst), ".copy-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	_, copyErr := io.Copy(tmp, in)
	closeErr := tmp.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return closeErr
	}

	if err := os.Rename(tmpPath, dst); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return nil
}

func (rv *ResultView) replaceFileInPaths(status *core.FileStatus, src models.PathEntry, targets []models.PathEntry) {
	var errMsgs []string
	var replaced []string

	for _, dst := range targets {
		srcPath := filepath.Join(src.Path, filepath.FromSlash(status.RelPath))
		dstPath := filepath.Join(dst.Path, filepath.FromSlash(status.RelPath))

		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			errMsgs = append(errMsgs, fmt.Sprintf("%s: %v", dst.Label, err))
			continue
		}
		if err := copyLocalFile(srcPath, dstPath); err != nil {
			errMsgs = append(errMsgs, fmt.Sprintf("%s: %v", dst.Label, err))
			continue
		}

		status.Entries[dst.Label] = status.Entries[src.Label]
		replaced = append(replaced, dst.Label)
	}

	if len(replaced) > 0 {
		rv.reclassifyDifferStatus(status)
		rv.Update(rv.result, rv.project)
	}

	if len(errMsgs) > 0 {
		ShowInfoPopup(rv.win, "Replace Errors", strings.Join(errMsgs, "\n"))
	} else {
		ShowInfoPopup(rv.win, "Replace Complete", fmt.Sprintf("Replaced in: %s", strings.Join(replaced, ", ")))
	}
}
