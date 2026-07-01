package core

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"filecompare/models"

	"github.com/jlaffaye/ftp"
)

type FileInfo struct {
	Hash    string
	Size    int64
	ModTime time.Time
}

type FileStatus struct {
	RelPath string
	Entries map[string]*FileInfo // label -> info (nil means missing from that path)
	AllSame bool
}

type CompareResult struct {
	Labels  []string
	Missing []*FileStatus
	Differ  []*FileStatus
	Same    []*FileStatus
	// Warnings describes path entries that were skipped because they
	// couldn't be scanned (e.g. a missing local folder or an FTP login
	// failure). Their files are still reported as Missing for that label.
	Warnings []string
}

type hashTask struct {
	label    string
	relPath  string
	file     RemoteFile
	cacheKey string
	size     int64
	modTime  time.Time
}

type hashDone struct {
	label   string
	relPath string
	info    *FileInfo
}

// ProgressPhase identifies the current phase of a Compare operation.
type ProgressPhase int

const (
	PhaseScanning ProgressPhase = iota // directory walk / FTP list
	PhaseHashing                       // file content hashing
)

// ProgressEvent is delivered to the onProgress callback at meaningful
// milestones during a Compare call.
type ProgressEvent struct {
	Phase ProgressPhase

	// PhaseScanning: cumulative file count discovered across all labels so far.
	ScannedFiles int

	// PhaseHashing fields.
	BytesTotal int64  // total bytes to process (stable once hashing begins)
	BytesDone  int64  // bytes processed so far (includes cache hits and skipped files)
	FilePath   string // relative path of the file currently being hashed
}

// countingReader wraps an io.Reader and calls onBytes after each Read with
// the number of bytes read in that call.
type countingReader struct {
	r       io.Reader
	onBytes func(int64)
}

func (cr *countingReader) Read(p []byte) (int, error) {
	n, err := cr.r.Read(p)
	if n > 0 {
		cr.onBytes(int64(n))
	}
	return n, err
}

type filePreInfo struct {
	size    int64
	modTime time.Time
}

// buildCacheKey returns the stable hash-cache key for relPath under the
// source described by pe. The key is independent of which project or label
// references pe, so two projects pointing at the same physical folder or FTP
// sub-path share cache entries.
//
// Local keys are filepath.Clean'd before joining so trivial path spelling
// differences (trailing slash, mixed separators) don't fragment the cache;
// this does not resolve symlinks or fold case, so a folder reached via a
// different but equivalent path is simply treated as a fresh location --
// that only ever costs a slower run, never an incorrect result.
func buildCacheKey(pe models.PathEntry, relPath string) string {
	if pe.Type.IsFTP() {
		root := strings.TrimSuffix(pe.Path, "/")
		return fmt.Sprintf("ftp:%s:%d:%s:%s/%s", pe.FTPDomain, pe.Port(), pe.FTPUsername, root, relPath)
	}
	abs := filepath.Clean(filepath.Join(pe.Path, relPath))
	return "local:" + filepath.ToSlash(abs)
}

// buildCacheKeysFor builds the relPath -> cache-key map for every file just
// scanned under pe.
func buildCacheKeysFor(pe models.PathEntry, files map[string]RemoteFile) map[string]string {
	keys := make(map[string]string, len(files))
	for rel := range files {
		keys[rel] = buildCacheKey(pe, rel)
	}
	return keys
}

// cacheKeyPrefix returns the prefix shared by every cache key buildCacheKey
// can produce for files under pe, for use with HashCache.PruneExcept.
func cacheKeyPrefix(pe models.PathEntry) string {
	if pe.Type.IsFTP() {
		root := strings.TrimSuffix(pe.Path, "/")
		return fmt.Sprintf("ftp:%s:%d:%s:%s/", pe.FTPDomain, pe.Port(), pe.FTPUsername, root)
	}
	abs := filepath.Clean(pe.Path)
	return "local:" + filepath.ToSlash(abs) + "/"
}

func scanPaths(
	ctx context.Context,
	project models.Project,
	creds FTPCredentials,
	globalExclusions []string,
	onProgress func(ProgressEvent),
) (
	labels []string,
	scanResults map[string]map[string]RemoteFile,
	ftpLabels map[string]bool,
	expectedGaps map[string][]string,
	cacheKeys map[string]map[string]string,
	ftpConns []*ftp.ServerConn,
	warnings []string,
) {
	labels = make([]string, 0, len(project.Paths))
	scanResults = make(map[string]map[string]RemoteFile)
	ftpLabels = make(map[string]bool)
	expectedGaps = make(map[string][]string, len(project.Paths))
	cacheKeys = make(map[string]map[string]string)

	var scannedFiles int

	for _, pe := range project.Paths {
		if ctx.Err() != nil {
			return
		}
		labels = append(labels, pe.Label)
		expectedGaps[pe.Label] = pe.ExpectedGaps
		if len(globalExclusions) > 0 {
			pe.Exclusions = append(append([]string{}, pe.Exclusions...), globalExclusions...)
		}

		if pe.Type.IsFTP() {
			ftpLabels[pe.Label] = true

			conn, err := DialFTP(pe, creds[FTPKey(pe)])
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s (FTP): could not connect or log in: %v", pe.Label, err))
				continue
			}
			ftpConns = append(ftpConns, conn)

			files, err := ScanFTPPath(conn, pe)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s (FTP): %v", pe.Label, err))
				continue
			}
			scanResults[pe.Label] = files
			cacheKeys[pe.Label] = buildCacheKeysFor(pe, files)
			scannedFiles += len(files)
			if onProgress != nil {
				onProgress(ProgressEvent{Phase: PhaseScanning, ScannedFiles: scannedFiles})
			}
			continue
		}

		files, err := ScanPath(pe)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", pe.Label, err))
			continue
		}
		scanResults[pe.Label] = files
		cacheKeys[pe.Label] = buildCacheKeysFor(pe, files)
		scannedFiles += len(files)
		if onProgress != nil {
			onProgress(ProgressEvent{Phase: PhaseScanning, ScannedFiles: scannedFiles})
		}
	}
	return
}

func buildPreStats(
	scanResults map[string]map[string]RemoteFile,
) (union map[string]bool, preStats map[string]map[string]filePreInfo) {
	union = make(map[string]bool)
	for _, files := range scanResults {
		for rel := range files {
			union[rel] = true
		}
	}

	preStats = make(map[string]map[string]filePreInfo, len(union))
	for label, files := range scanResults {
		for rel, file := range files {
			size, mod := file.Stat()
			if preStats[rel] == nil {
				preStats[rel] = make(map[string]filePreInfo)
			}
			preStats[rel][label] = filePreInfo{size, mod}
		}
	}
	return
}

func detectSkipped(
	preStats map[string]map[string]filePreInfo,
	labels []string,
) (skipped map[string]bool, hashMap map[string]map[string]*FileInfo) {
	hashMap = make(map[string]map[string]*FileInfo)
	skipped = make(map[string]bool)

	for rel, stats := range preStats {
		if len(stats) != len(labels) {
			continue
		}
		var first int64 = -1
		differs := false
		for _, s := range stats {
			if first < 0 {
				first = s.size
			} else if s.size != first {
				differs = true
				break
			}
		}
		if !differs {
			continue
		}
		skipped[rel] = true
		hashMap[rel] = make(map[string]*FileInfo, len(stats))
		for label, s := range stats {
			hashMap[rel][label] = &FileInfo{
				Size:    s.size,
				ModTime: s.modTime,
				Hash:    fmt.Sprintf("size:%d", s.size),
			}
		}
	}
	return
}

// detectMetadataOnly returns a hash map for files that should be compared by
// metadata only, using "size:N" as the hash for every label that has the
// file. When allFiles is true every file is included; otherwise only files
// whose relative path matches one of patterns are included. Callers should
// merge the result into the main hash map and add each key to the skipped set
// so hashAllFiles does not read those files' contents.
func detectMetadataOnly(
	preStats map[string]map[string]filePreInfo,
	patterns []string,
	allFiles bool,
) map[string]map[string]*FileInfo {
	moHashMap := make(map[string]map[string]*FileInfo)
	if !allFiles && len(patterns) == 0 {
		return moHashMap
	}
	for rel, stats := range preStats {
		if !allFiles && !shouldExclude(rel, patterns) {
			continue
		}
		moHashMap[rel] = make(map[string]*FileInfo, len(stats))
		for label, s := range stats {
			moHashMap[rel][label] = &FileInfo{
				Size:    s.size,
				ModTime: s.modTime,
				Hash:    fmt.Sprintf("size:%d", s.size),
			}
		}
	}
	return moHashMap
}

func hashAllFiles(
	ctx context.Context,
	scanResults map[string]map[string]RemoteFile,
	ftpLabels map[string]bool,
	preStats map[string]map[string]filePreInfo,
	skipped map[string]bool,
	hashMap map[string]map[string]*FileInfo,
	cacheKeys map[string]map[string]string,
	cache *HashCache,
	onProgress func(ProgressEvent),
) map[string]map[string]*FileInfo {
	var totalBytes int64
	for _, stats := range preStats {
		for _, s := range stats {
			totalBytes += s.size
		}
	}
	var bytesDone atomic.Int64
	for rel := range skipped {
		for _, s := range preStats[rel] {
			bytesDone.Add(s.size)
		}
	}

	done := make(chan hashDone, 256)
	var wg sync.WaitGroup

	processTask := func(t hashTask) {
		if ctx.Err() != nil {
			return
		}

		info := &FileInfo{Size: t.size, ModTime: t.modTime}
		if hash, ok := cache.Lookup(t.cacheKey, t.size, t.modTime); ok {
			info.Hash = hash
			bytesDone.Add(t.size)
			if onProgress != nil {
				onProgress(ProgressEvent{
					Phase:      PhaseHashing,
					BytesTotal: totalBytes,
					BytesDone:  bytesDone.Load(),
					FilePath:   t.relPath,
				})
			}
		} else if rc, err := t.file.Open(); err == nil {
			if onProgress != nil {
				onProgress(ProgressEvent{
					Phase:      PhaseHashing,
					BytesTotal: totalBytes,
					BytesDone:  bytesDone.Load(),
					FilePath:   t.relPath,
				})
			}
			cr := &countingReader{r: rc, onBytes: func(n int64) {
				bytesDone.Add(n)
				if onProgress != nil {
					onProgress(ProgressEvent{
						Phase:      PhaseHashing,
						BytesTotal: totalBytes,
						BytesDone:  bytesDone.Load(),
						FilePath:   t.relPath,
					})
				}
			}}
			info.Hash, _ = HashReader(cr)
			_ = rc.Close()
			if info.Hash != "" {
				cache.Store(t.cacheKey, CacheEntry{Size: t.size, ModTime: t.modTime, Hash: info.Hash})
			}
		}
		done <- hashDone{label: t.label, relPath: t.relPath, info: info}
	}

	// Local files can be hashed concurrently by a shared worker pool. Each
	// FTP source gets its own single-goroutine lane instead, because an FTP
	// control connection can't process more than one command at a time.
	workers := max(runtime.NumCPU(), 8)
	localTasks := make(chan hashTask, 256)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range localTasks {
				processTask(t)
			}
		}()
	}

	ftpTaskChans := make(map[string]chan hashTask, len(ftpLabels))
	for label := range ftpLabels {
		ch := make(chan hashTask, 256)
		ftpTaskChans[label] = ch
		wg.Add(1)
		go func(ch chan hashTask) {
			defer wg.Done()
			for t := range ch {
				processTask(t)
			}
		}(ch)
	}

	go func() {
		for label, files := range scanResults {
			for rel, file := range files {
				if skipped[rel] {
					continue
				}
				ps := preStats[rel][label]
				t := hashTask{
					label:    label,
					relPath:  rel,
					file:     file,
					cacheKey: cacheKeys[label][rel],
					size:     ps.size,
					modTime:  ps.modTime,
				}
				if ch, ok := ftpTaskChans[label]; ok {
					ch <- t
				} else {
					localTasks <- t
				}
			}
		}
		close(localTasks)
		for _, ch := range ftpTaskChans {
			close(ch)
		}
		wg.Wait()
		close(done)
	}()

	for d := range done {
		if hashMap[d.relPath] == nil {
			hashMap[d.relPath] = make(map[string]*FileInfo)
		}
		hashMap[d.relPath][d.label] = d.info
	}
	return hashMap
}

func classifyResults(
	union map[string]bool,
	labels []string,
	hashMap map[string]map[string]*FileInfo,
	expectedGaps map[string][]string,
) (missing, differ, same []*FileStatus) {
	for rel := range union {
		status := &FileStatus{
			RelPath: rel,
			Entries: make(map[string]*FileInfo),
		}

		hasMissing := false
		for _, label := range labels {
			if info, ok := hashMap[rel][label]; ok {
				status.Entries[label] = info
			} else {
				status.Entries[label] = nil
				if !shouldExclude(rel, expectedGaps[label]) {
					hasMissing = true
				}
			}
		}

		if hasMissing {
			missing = append(missing, status)
			continue
		}

		firstHash := ""
		allSame := true
		for _, label := range labels {
			entry := status.Entries[label]
			if entry == nil {
				continue // expected gap for this label; excluded from the comparison
			}
			if firstHash == "" {
				firstHash = entry.Hash
			} else if entry.Hash != firstHash {
				allSame = false
				break
			}
		}

		status.AllSame = allSame
		if allSame {
			same = append(same, status)
		} else {
			differ = append(differ, status)
		}
	}
	return
}

// Compare scans all paths in the project and returns a categorized result.
// creds supplies passwords for any FTP path entries, keyed by FTPKey(entry);
// it is read-only and never modified or persisted by this function.
// onProgress, if non-nil, is called at meaningful milestones: once per label
// after its directory scan completes (PhaseScanning), and once per file plus
// once per 4 MB chunk during hashing (PhaseHashing). It may be called from
// multiple goroutines concurrently and must be safe for concurrent use.
// If ctx is canceled, Compare stops as soon as possible and returns ctx.Err().
//
// A path entry that cannot be scanned (missing local folder, FTP connection
// or login failure) does not abort the whole comparison: it is skipped, a
// message is added to the result's Warnings, and its files are reported as
// Missing for that label.
//
// globalExclusions are glob patterns applied on top of every path entry's
// own exclusions, regardless of source type.
func Compare(ctx context.Context, project models.Project, creds FTPCredentials, globalExclusions []string, onProgress func(ProgressEvent)) (*CompareResult, error) {
	cache := LoadHashCache()

	labels, scanResults, ftpLabels, expectedGaps, cacheKeys, ftpConns, warnings := scanPaths(ctx, project, creds, globalExclusions, onProgress)
	defer func() {
		for _, conn := range ftpConns {
			_ = conn.Quit()
		}
	}()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	union, preStats := buildPreStats(scanResults)
	skipped, hashMap := detectSkipped(preStats, labels)

	for rel, entries := range detectMetadataOnly(preStats, project.MetadataOnlyPatterns, project.MetadataOnlyAll) {
		if hashMap[rel] == nil {
			hashMap[rel] = entries
		} else {
			for label, info := range entries {
				hashMap[rel][label] = info
			}
		}
		skipped[rel] = true
	}

	hashMap = hashAllFiles(ctx, scanResults, ftpLabels, preStats, skipped, hashMap, cacheKeys, cache, onProgress)

	// Drop cached rows for files that no longer exist under a source this
	// run found (deleted/renamed since the cache was last written), but only
	// for sources that were fully and successfully scanned+hashed this run
	// -- a canceled run's key set is incomplete, so pruning against it would
	// wrongly evict still-valid entries for files that simply hadn't been
	// reached yet.
	if ctx.Err() == nil {
		for _, pe := range project.Paths {
			keys, scanned := cacheKeys[pe.Label]
			if !scanned {
				continue
			}
			keep := make(map[string]bool, len(keys))
			for _, key := range keys {
				keep[key] = true
			}
			cache.PruneExcept(cacheKeyPrefix(pe), keep)
		}
	}
	_ = cache.Save()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	missing, differ, same := classifyResults(union, labels, hashMap, expectedGaps)
	return &CompareResult{
		Labels:   labels,
		Missing:  missing,
		Differ:   differ,
		Same:     same,
		Warnings: warnings,
	}, nil
}
