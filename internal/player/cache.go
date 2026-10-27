package player

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// Cache is a directory the songs download to as they play, which keeps
// those that came whole, to play again at once without asking for their
// URLs. A download is a file of a random name, and of what Source.Name
// tells, as 2974105101-晴天-HQ 320k.audio; once whole, <key>.json
// names it. Written last, the JSON is there only for a file that is
// whole, and as it never is open it can be replaced on Windows, where an
// open file cannot be renamed. Once the songs kept take more than the
// limit, those used longest ago go first. Its methods are safe from any
// goroutine, and those exported do nothing on a nil Cache.
type Cache struct {
	dir string

	mu    sync.Mutex
	limit int64          // in bytes; 0 keeps nothing
	inUse map[string]int // the files open, by name
}

// cached is a <key>.json.
type cached struct {
	File string `json:"file"`
	Size int64  `json:"size"`
	Note string `json:"note"`
}

var errNotCached = errors.New("player: not in the cache")

// NewCache returns a cache in dir, made when a song first downloads.
func NewCache(dir string, limit int64) *Cache {
	return &Cache{dir: dir, limit: limit, inUse: map[string]int{}}
}

// validKey reports whether key can name a file of the cache: one of
// letters, digits, - and _ cannot name one outside it.
func validKey(key string) bool {
	for _, r := range key {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '-' || r == '_') {
			return false
		}
	}
	return key != ""
}

// Dir is the directory of the cache.
func (c *Cache) Dir() string { return c.dir }

// fileLabel makes name fit in a file's name on Windows: without the
// characters it forbids, nor the * that os.CreateTemp replaces, not
// ending in a dot or a space, which Windows drops, and not too long.
func fileLabel(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || strings.ContainsRune(`\/:*?"<>|`, r) {
			return '_'
		}
		return r
	}, name)
	if r := []rune(name); len(r) > 60 {
		name = string(r[:60])
	}
	return strings.TrimRight(strings.TrimSpace(name), ". ")
}

// SetLimit sets how many bytes the songs kept may take, dropping those
// past it.
func (c *Cache) SetLimit(limit int64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.limit = limit
	c.mu.Unlock()
	c.trim(limit)
}

// Clear drops the songs kept, but for the one playing.
func (c *Cache) Clear() {
	if c != nil {
		c.trim(0)
	}
}

// Size is how many bytes the cache takes, with the download playing.
func (c *Cache) Size() int64 {
	if c == nil {
		return 0
	}
	entries, _ := os.ReadDir(c.dir)
	var n int64
	for _, e := range entries {
		if info, err := e.Info(); err == nil {
			n += info.Size()
		}
	}
	return n
}

// Lookup reports whether the cache keeps key, with the note kept with it.
func (c *Cache) Lookup(key string) (note string, ok bool) {
	e, ok := c.entry(key)
	return e.Note, ok
}

// entry reads the JSON of key, if its file is whole. It counts as a use,
// for key to go last.
func (c *Cache) entry(key string) (cached, bool) {
	if c == nil || !validKey(key) {
		return cached{}, false
	}
	path := filepath.Join(c.dir, key+".json")
	var e cached
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &e) != nil || filepath.Base(e.File) != e.File {
		return cached{}, false
	}
	if info, err := os.Stat(filepath.Join(c.dir, e.File)); err != nil || info.Size() != e.Size {
		os.Remove(path)
		return cached{}, false
	}
	now := time.Now()
	os.Chtimes(path, now, now)
	return e, true
}

// open opens the file kept as key, and its size.
func (c *Cache) open(key string) (*os.File, int64, error) {
	e, ok := c.entry(key)
	if !ok {
		return nil, 0, errNotCached
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	f, err := os.Open(filepath.Join(c.dir, e.File))
	if err != nil {
		return nil, 0, err
	}
	c.inUse[e.File]++
	return f, e.Size, nil
}

// create makes the file of a download, named after its random part by
// label, a fileLabel: nil without a cache, or if the directory takes none.
func (c *Cache) create(label string) *os.File {
	if c == nil || os.MkdirAll(c.dir, 0o700) != nil {
		return nil
	}
	// Held while the file is made, for a trim that lists it to see it
	// in use.
	c.mu.Lock()
	defer c.mu.Unlock()
	pattern := "*.audio"
	if label != "" {
		pattern = "*-" + label + ".audio"
	}
	f, err := os.CreateTemp(c.dir, pattern)
	if err != nil {
		return nil
	}
	c.inUse[filepath.Base(f.Name())]++
	return f
}

// release notes that a file open closed.
func (c *Cache) release(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	name := filepath.Base(path)
	if c.inUse[name]--; c.inUse[name] <= 0 {
		delete(c.inUse, name)
	}
}

// fits reports whether a song of size bytes is kept: one past a quarter
// of the limit would push out the others.
func (c *Cache) fits(size int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return size <= c.limit/4
}

// keep names the whole file at path as key.
func (c *Cache) keep(key, note, path string, size int64) {
	data, _ := json.Marshal(cached{File: filepath.Base(path), Size: size, Note: note})
	tmp := filepath.Join(c.dir, key+".json.tmp")
	if os.WriteFile(tmp, data, 0o600) != nil || os.Rename(tmp, filepath.Join(c.dir, key+".json")) != nil {
		os.Remove(tmp) // the file, named by nothing, goes with the next trim
		return
	}
	c.mu.Lock()
	limit := c.limit
	c.mu.Unlock()
	c.trim(limit)
}

// trim drops the songs used longest ago until those kept take at most
// limit bytes, and the files no JSON names: of downloads cut short, the
// songs replaced, or a crash. It leaves the files open.
func (c *Cache) trim(limit int64) {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	c.mu.Lock()
	inUse := maps.Clone(c.inUse)
	c.mu.Unlock()
	type song struct {
		json string
		e    cached
		used time.Time
	}
	var songs []song
	named := map[string]bool{}
	var total int64
	for _, de := range entries {
		if filepath.Ext(de.Name()) != ".json" {
			continue
		}
		path := filepath.Join(c.dir, de.Name())
		var e cached
		data, err := os.ReadFile(path)
		info, ierr := de.Info()
		if err != nil || ierr != nil || json.Unmarshal(data, &e) != nil {
			os.Remove(path)
			continue
		}
		songs = append(songs, song{path, e, info.ModTime()})
		named[e.File] = true
		total += e.Size
	}
	for _, de := range entries {
		if n := de.Name(); !named[n] && inUse[n] == 0 && (filepath.Ext(n) == ".audio" || filepath.Ext(n) == ".tmp") {
			os.Remove(filepath.Join(c.dir, n))
		}
	}
	slices.SortFunc(songs, func(a, b song) int { return a.used.Compare(b.used) })
	for _, s := range songs {
		if total <= limit {
			break
		}
		if inUse[s.e.File] > 0 {
			continue
		}
		os.Remove(s.json)
		os.Remove(filepath.Join(c.dir, s.e.File))
		total -= s.e.Size
	}
}
