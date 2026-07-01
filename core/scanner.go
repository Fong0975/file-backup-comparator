package core

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"filecompare/models"

	"github.com/bmatcuk/doublestar/v4"
)

// ScanPath walks a local directory and returns a map of relative path ->
// RemoteFile, applying the path entry's exclusion patterns before adding
// files.
func ScanPath(entry models.PathEntry) (map[string]RemoteFile, error) {
	if _, err := os.Stat(entry.Path); err != nil {
		return nil, err
	}

	result := make(map[string]RemoteFile)

	err := filepath.Walk(entry.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		rel, err := filepath.Rel(entry.Path, path)
		if err != nil || rel == "." {
			return nil
		}

		relSlash := filepath.ToSlash(rel)

		if info.IsDir() {
			if shouldSkipDir(relSlash, entry.Exclusions) {
				return filepath.SkipDir
			}
			return nil
		}

		if shouldExclude(relSlash, entry.Exclusions) {
			return nil
		}

		result[relSlash] = newLocalFile(path)
		return nil
	})

	return result, err
}

// shouldExclude reports whether relSlash (a file's path, relative to the
// scan root) matches any of the given patterns.
//
// A pattern with no "/" matches anywhere in the tree -- against the
// basename, or against any individual path segment -- the same as a plain
// .gitignore entry with no slash (e.g. "node_modules" or "*.tmp" matches
// regardless of where it shows up). A pattern containing "/" is instead
// anchored to the scan root and matched as a path glob, following the same
// convention as .gitignore / GitHub Actions "paths:" filters: "*" matches
// within a single path segment, "**" matches any number of segments. So
// "docs/*" matches only files directly under docs/, while "docs/**" matches
// anything anywhere under docs/ no matter how deep.
func shouldExclude(relSlash string, patterns []string) bool {
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if matchesPattern(relSlash, pattern) {
			return true
		}
	}
	return false
}

func matchesPattern(relSlash, pattern string) bool {
	pattern = strings.TrimPrefix(pattern, "/")

	if !strings.Contains(pattern, "/") {
		base := filepath.Base(relSlash)
		if matched, _ := filepath.Match(pattern, base); matched {
			return true
		}
		for _, part := range strings.Split(relSlash, "/") {
			if matched, _ := filepath.Match(pattern, part); matched {
				return true
			}
		}
		if matched, _ := filepath.Match(pattern, relSlash); matched {
			return true
		}
		// Literal fallback: handles names containing glob metacharacters such
		// as '[' and ']' that filepath.Match treats as character-class syntax.
		// On Windows '\' cannot escape them, so we fall back to a plain string
		// comparison after the glob attempt above.
		if base == pattern || relSlash == pattern {
			return true
		}
		return slices.Contains(strings.Split(relSlash, "/"), pattern)
	}

	if matched, _ := doublestar.Match(pattern, relSlash); matched {
		return true
	}
	// Literal fallback for path patterns whose directory or file names contain
	// glob metacharacters (e.g. "[tag] folder/file.txt"). doublestar interprets
	// '[...]' as a character class; on Windows '\' is a path separator so
	// escaping is unavailable. We cover the two practical pattern shapes:
	//   "some/literal/path"       → exact match
	//   "some/literal/prefix/**"  → everything under that prefix
	if prefix, ok := strings.CutSuffix(pattern, "/**"); ok {
		return relSlash == prefix || strings.HasPrefix(relSlash, prefix+"/")
	}
	return relSlash == pattern
}

// shouldSkipDir reports whether every possible file under the directory
// dirRelSlash is guaranteed to match one of patterns, so a scan can skip
// recursing into it entirely instead of matching each descendant
// individually -- the same optimisation .gitignore-aware tools apply once a
// whole directory is known to be ignored.
//
// This must never report true for a pattern that only excludes SOME of a
// directory's descendants (e.g. "docs/*", which covers only docs' direct
// children, not anything nested deeper) -- only a no-slash pattern matching
// the directory's own name, or a path-anchored pattern ending in "/**", can
// safely skip the whole subtree.
func shouldSkipDir(dirRelSlash string, patterns []string) bool {
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		pattern = strings.TrimPrefix(pattern, "/")

		if !strings.Contains(pattern, "/") {
			if matchesPattern(dirRelSlash, pattern) {
				return true
			}
			continue
		}

		if rest, ok := strings.CutSuffix(pattern, "/**"); ok {
			if matched, _ := doublestar.Match(rest, dirRelSlash); matched {
				return true
			}
			// Literal fallback: same metacharacter issue as in matchesPattern.
			if dirRelSlash == rest || strings.HasPrefix(dirRelSlash, rest+"/") {
				return true
			}
		}
	}
	return false
}

// MatchesAnyPattern reports whether relPath matches any of the given glob
// patterns using the same semantics as Exclusions and ExpectedGaps.
func MatchesAnyPattern(relPath string, patterns []string) bool {
	return shouldExclude(relPath, patterns)
}
