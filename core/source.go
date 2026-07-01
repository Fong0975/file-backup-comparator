package core

import (
	"io"
	"os"
	"time"
)

// RemoteFile abstracts a single file that can be hashed, regardless of
// whether it lives on the local filesystem or on a remote FTP server.
type RemoteFile interface {
	// Stat returns the file's size and last-modified time.
	Stat() (size int64, modTime time.Time)
	// Open returns a reader for the file's contents. The caller must Close it.
	Open() (io.ReadCloser, error)
}

// localFile is a RemoteFile backed by a path on the local filesystem.
type localFile struct {
	absPath string
}

func newLocalFile(absPath string) RemoteFile {
	return &localFile{absPath: absPath}
}

func (f *localFile) Stat() (int64, time.Time) {
	info, err := os.Stat(f.absPath)
	if err != nil {
		return 0, time.Time{}
	}
	return info.Size(), info.ModTime()
}

func (f *localFile) Open() (io.ReadCloser, error) {
	return os.Open(f.absPath)
}
