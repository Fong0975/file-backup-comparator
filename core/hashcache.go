package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"filecompare/models"
)

// cacheFileName is the JSON file holding the persistent hash cache, stored
// alongside config.json in models.ConfigDir().
const cacheFileName = "hash_cache.json"

// CacheEntry is one cached file's last-known identity and hash.
type CacheEntry struct {
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
	Hash    string    `json:"hash"`
}

// HashCache is a persistent map from a cache key (see buildCacheKey in
// comparator.go) to a file's last-known size/modTime/hash, letting Compare
// skip re-reading and re-hashing files that have not changed since the last
// run. Safe for concurrent use by Compare's hashing workers.
//
// Trust model: a cached entry is considered valid only if both the size and
// modTime match exactly what's freshly observed via Stat(). This is the same
// "quick check" heuristic rsync, git and restic use by default -- not a
// byte-for-byte guarantee, just a (size, mtime) tuple that's extremely
// unlikely to collide for a genuinely-changed file. The window is wider for
// FTP sources, since many FTP servers report modification times with only
// minute-level granularity. This is an accepted, documented tradeoff.
type HashCache struct {
	mu      sync.Mutex
	entries map[string]CacheEntry
	dirty   bool
}

// LoadHashCache reads the persisted cache from disk. Any failure to find,
// read or parse the file is treated as "start with an empty cache" -- this
// is a performance-only feature, so a corrupt cache file must never surface
// as an error to the caller. It only ever degrades to slower, uncached
// behavior.
func LoadHashCache() *HashCache {
	c := &HashCache{entries: make(map[string]CacheEntry)}

	data, err := os.ReadFile(filepath.Join(models.ConfigDir(), cacheFileName))
	if err != nil {
		return c
	}

	var entries map[string]CacheEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return c
	}

	c.entries = entries
	return c
}

// Lookup reports whether key has a cached entry whose size and modTime
// exactly match the freshly-observed values. On a hit it returns the cached
// hash and true; on a miss it returns ("", false).
func (c *HashCache) Lookup(key string, size int64, modTime time.Time) (hash string, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, found := c.entries[key]
	if !found || entry.Size != size || !entry.ModTime.Equal(modTime) {
		return "", false
	}
	return entry.Hash, true
}

// Store records or updates the cached entry for key.
func (c *HashCache) Store(key string, entry CacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = entry
	c.dirty = true
}

// PruneExcept removes every cached entry whose key has the given prefix and
// is not in keep. Call this once a path entry has been fully and
// successfully scanned+hashed this run, to drop stale rows for files that
// have since been deleted or renamed under that same source. A path entry
// that failed to scan this run must not have PruneExcept called for its
// prefix, so a transient failure doesn't wipe out otherwise-valid cached
// hashes for that source.
func (c *HashCache) PruneExcept(prefix string, keep map[string]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key := range c.entries {
		if len(key) < len(prefix) || key[:len(prefix)] != prefix {
			continue
		}
		if !keep[key] {
			delete(c.entries, key)
			c.dirty = true
		}
	}
}

// Save persists the cache to disk if it has changed since it was loaded (or
// since the last Save); otherwise it's a no-op. As with LoadHashCache, a
// non-nil error here is purely informational -- this is a performance-only
// side effect and should never fail or warn the user about a Compare run.
func (c *HashCache) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.dirty {
		return nil
	}

	dir := models.ConfigDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.Marshal(c.entries)
	if err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(dir, cacheFileName), data, 0644); err != nil {
		return err
	}

	c.dirty = false
	return nil
}
