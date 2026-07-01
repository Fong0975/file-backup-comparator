package models

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// PathSourceType identifies where a PathEntry's files come from.
type PathSourceType string

const (
	// SourceLocal reads files from the local filesystem. This is also the
	// zero value, so PathEntry records saved before FTP support was added
	// are still interpreted as local paths.
	SourceLocal PathSourceType = "local"
	SourceFTP   PathSourceType = "ftp"
)

// IsFTP reports whether the entry sources its files from an FTP server.
func (t PathSourceType) IsFTP() bool {
	return t == SourceFTP
}

type PathEntry struct {
	Label      string         `json:"label"`
	Type       PathSourceType `json:"type"` // "" is treated as SourceLocal
	Path       string         `json:"path"` // local: folder path; ftp: sub-path on the server
	Exclusions []string       `json:"exclusions"`
	// ExpectedGaps are glob patterns (matched the same way as Exclusions,
	// including against each path segment, so a folder name also covers
	// everything under it) for files or folders this path is known to
	// intentionally not have -- e.g. a path that deliberately skips backing
	// up a cache folder present on every other path.
	//
	// Unlike Exclusions, which are applied while scanning this path (so a
	// matching file is simply never read from it), ExpectedGaps only affects
	// comparison: a file matching one of these patterns is never reported as
	// Missing for this path, even when other paths in the project have it.
	// It still participates in Same/Differ comparisons among whichever other
	// paths do have it.
	ExpectedGaps []string `json:"expectedGaps,omitempty"`

	// FTP-only fields. The password is intentionally never stored here.
	// FTPServerID references Settings.FTPServers, so the project editor can
	// re-select the right entry in its server dropdown; Domain/Port/Username
	// are a snapshot of that server taken when the path was last saved, and
	// are what Compare actually dials with. Editing the server in Settings
	// later does not retroactively update path entries that already
	// reference it -- re-saving the path entry refreshes the snapshot.
	FTPServerID string `json:"ftpServerId,omitempty"`
	FTPDomain   string `json:"ftpDomain,omitempty"`
	FTPPort     int    `json:"ftpPort,omitempty"` // 0 means the default FTP port (21)
	FTPUsername string `json:"ftpUsername,omitempty"`
}

// Port returns the FTP port to connect to, defaulting to 21 when unset.
func (p PathEntry) Port() int {
	if p.FTPPort <= 0 {
		return 21
	}
	return p.FTPPort
}

type Project struct {
	ID    string      `json:"id"`
	Name  string      `json:"name"`
	Paths []PathEntry `json:"paths"`
	// MetadataOnlyAll, when true, compares every file in this project by
	// metadata only (name and size), regardless of MetadataOnlyPatterns.
	// Useful when the entire project consists of large, rarely-changing files.
	MetadataOnlyAll bool `json:"metadataOnlyAll,omitempty"`
	// MetadataOnlyPatterns are glob patterns (matched the same way as
	// Exclusions) for files that should be compared by metadata only (name and
	// size) rather than by content. Files matching these patterns are never read
	// for hashing; two copies are considered identical when their sizes match.
	// Intended for large, rarely-changing files such as videos or disk images
	// where content hashing would be prohibitively slow.
	// Ignored when MetadataOnlyAll is true.
	MetadataOnlyPatterns []string `json:"metadataOnlyPatterns,omitempty"`
}

func (p *Project) Clone() *Project {
	cp := *p
	cp.Paths = make([]PathEntry, len(p.Paths))
	for i, pe := range p.Paths {
		cp.Paths[i] = pe
		excl := make([]string, len(pe.Exclusions))
		copy(excl, pe.Exclusions)
		cp.Paths[i].Exclusions = excl
		gaps := make([]string, len(pe.ExpectedGaps))
		copy(gaps, pe.ExpectedGaps)
		cp.Paths[i].ExpectedGaps = gaps
	}
	moPatterns := make([]string, len(p.MetadataOnlyPatterns))
	copy(moPatterns, p.MetadataOnlyPatterns)
	cp.MetadataOnlyPatterns = moPatterns
	return &cp
}

// FTPServer is a saved FTP connection profile that path entries can pick
// from, so the same server's domain/port/account only has to be entered
// once instead of on every path entry that uses it.
type FTPServer struct {
	ID     string `json:"id"`
	Name   string `json:"name,omitempty"` // shown in dropdowns/lists; falls back to domain:port when blank
	Domain string `json:"domain"`
	Port   int    `json:"port"` // 0 means the default FTP port (21)
	// Username is optional. When blank, the user is prompted for both an
	// account and a password the first time this server is used in a
	// Compare run.
	Username string `json:"username,omitempty"`
}

// PortOrDefault returns the port to connect to, defaulting to 21 when unset.
func (s FTPServer) PortOrDefault() int {
	if s.Port <= 0 {
		return 21
	}
	return s.Port
}

// DisplayName is how this server is labeled in dropdowns and lists: its
// Name if one was given, otherwise a domain:port fallback.
func (s FTPServer) DisplayName() string {
	if s.Name != "" {
		return s.Name
	}
	return fmt.Sprintf("%s:%d", s.Domain, s.PortOrDefault())
}

// Settings holds app-wide preferences that are not specific to any project.
type Settings struct {
	FTPServers []FTPServer `json:"ftpServers"`
	// Labels are saved path-entry labels the user can pick from when adding
	// or editing a path, instead of typing the same label out every time.
	Labels []string `json:"labels"`
	// GlobalExclusions are glob patterns excluded from every project's
	// comparisons, in addition to whatever a path entry excludes itself.
	GlobalExclusions []string `json:"globalExclusions"`
}

type AppConfig struct {
	Projects []Project `json:"projects"`
	Settings Settings  `json:"settings"`
}

func ConfigDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), "FileCompare")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "FileCompare")
}

func LoadConfig() (*AppConfig, error) {
	data, err := os.ReadFile(filepath.Join(ConfigDir(), "config.json"))
	if os.IsNotExist(err) {
		return &AppConfig{Projects: []Project{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg AppConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func SaveConfig(cfg *AppConfig) error {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "config.json"), data, 0644)
}
