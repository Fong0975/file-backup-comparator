package core

import (
	"fmt"
	"io"
	"strings"
	"time"

	"filecompare/models"

	"github.com/jlaffaye/ftp"
)

// FTPCredentialKey identifies a distinct FTP account. The same domain and
// port with a different username is treated as a separate account.
type FTPCredentialKey struct {
	Domain   string
	Port     int
	Username string
}

// FTPCredentials resolves an FTP account to its password for a single
// Compare call. Passwords are supplied by the caller and are never persisted
// or logged by this package.
type FTPCredentials map[FTPCredentialKey]string

// FTPKey returns the credential key for the entry's configured FTP account.
func FTPKey(pe models.PathEntry) FTPCredentialKey {
	return FTPCredentialKey{Domain: pe.FTPDomain, Port: pe.Port(), Username: pe.FTPUsername}
}

// ftpFile is a RemoteFile backed by a file on an FTP server. It shares a
// single connection with the other files from the same PathEntry, so it must
// not be used concurrently with other files from that same connection.
//
// dir/name are kept apart (rather than a single combined path) so Open can
// CWD into dir and RETR just the bare name. See the comment on ScanFTPPath
// for why: some FTP servers shell-glob-expand multi-segment path arguments
// to LIST/RETR, so a literal directory or file name containing characters
// like '[' ']' '*' '?' can silently fail to match anything when passed as
// part of a path argument, even though it works fine as a CWD target or a
// bare name within the current directory.
type ftpFile struct {
	conn    *ftp.ServerConn
	dir     string
	name    string
	size    int64
	modTime time.Time
}

func (f *ftpFile) Stat() (int64, time.Time) {
	return f.size, f.modTime
}

func (f *ftpFile) Open() (io.ReadCloser, error) {
	if err := f.conn.ChangeDir(f.dir); err != nil {
		return nil, err
	}
	resp, err := f.conn.Retr(f.name)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// DialFTP connects and authenticates to the FTP server described by entry,
// using the supplied password. The password is never written back onto
// entry, and the caller is responsible for closing the returned connection.
func DialFTP(entry models.PathEntry, password string) (*ftp.ServerConn, error) {
	addr := fmt.Sprintf("%s:%d", entry.FTPDomain, entry.Port())

	conn, err := ftp.Dial(addr, ftp.DialWithTimeout(15*time.Second))
	if err != nil {
		return nil, err
	}

	if err := conn.Login(entry.FTPUsername, password); err != nil {
		_ = conn.Quit()
		return nil, err
	}

	return conn, nil
}

// ScanFTPPath recursively lists the files under entry.Path on conn, applying
// the same exclusion patterns as a local scan.
//
// This deliberately avoids the underlying library's conn.Walk(), which lists
// a directory by passing its full accumulated path straight to LIST/MLSD
// (e.g. `LIST /a/[b] c/d`). Several common FTP server implementations apply
// shell-style glob expansion to that argument server-side, so a literal
// directory name containing glob metacharacters -- '[' and ']' are not
// unusual in real-world folder names -- can come back with zero matches
// even though the directory exists and is non-empty. CWD takes its argument
// as a literal pathname on every server we're aware of (glob-expanding "cd"
// would make no sense), so changing into each directory first and listing
// with no argument (bare LIST/MLSD has nothing to glob) sidesteps the
// ambiguity entirely.
func ScanFTPPath(conn *ftp.ServerConn, entry models.PathEntry) (map[string]RemoteFile, error) {
	root := entry.Path
	if root == "" {
		root = "/"
	}
	root = strings.TrimSuffix(root, "/")
	if root == "" {
		root = "/"
	}

	result := make(map[string]RemoteFile)
	if err := walkFTPDir(conn, root, "", entry.Exclusions, result); err != nil {
		return nil, err
	}

	return result, nil
}

func walkFTPDir(conn *ftp.ServerConn, absDir, relDir string, exclusions []string, result map[string]RemoteFile) error {
	if err := conn.ChangeDir(absDir); err != nil {
		return fmt.Errorf("%w\n  → CWD %s", err, absDir)
	}

	entries, err := conn.List("")
	if err != nil {
		return fmt.Errorf("%w\n  → LIST %s", err, absDir)
	}

	for _, e := range entries {
		if e.Name == "" || e.Name == "." || e.Name == ".." {
			continue
		}

		rel := e.Name
		if relDir != "" {
			rel = relDir + "/" + e.Name
		}

		switch e.Type {
		case ftp.EntryTypeFolder:
			if shouldSkipDir(rel, exclusions) {
				continue
			}
			// An unreadable subdirectory is skipped rather than aborting
			// the whole scan; whatever else was already found is kept.
			_ = walkFTPDir(conn, absDir+"/"+e.Name, rel, exclusions, result)
		case ftp.EntryTypeFile:
			if shouldExclude(rel, exclusions) {
				continue
			}
			result[rel] = &ftpFile{
				conn:    conn,
				dir:     absDir,
				name:    e.Name,
				size:    int64(e.Size),
				modTime: e.Time,
			}
		}
	}

	return nil
}
