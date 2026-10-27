// Package store keeps the app's data as JSON files in a folder only the
// user may read. A store of no folder keeps nothing, as tests want.
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Dir is the folder of the files.
type Dir struct{ path string }

// Open is the store in the folder at path, made as a file is first saved;
// "" for one that keeps nothing.
func Open(path string) Dir { return Dir{path: path} }

// Load reads the file name into v, reporting whether it was there and
// whole. A file cut short may have set some fields of v.
func (d Dir) Load(name string, v any) bool {
	if d.path == "" {
		return false
	}
	b, err := os.ReadFile(filepath.Join(d.path, name))
	return err == nil && json.Unmarshal(b, v) == nil
}

// Save writes v to the file name, indented for a person to read or as
// small as it gets. It writes a file beside it and renames that over it:
// a crash halfway leaves the one before whole, rather than a login or a
// queue cut short. It does not wait for the disk (no Sync): it runs on the
// UI thread at every change of song, and a flush can take a slow disk
// hundreds of milliseconds. A failure goes unnoticed: what is kept is a
// convenience, and the next save tries again.
func (d Dir) Save(name string, v any, indent bool) {
	if d.path == "" {
		return
	}
	var b []byte
	if indent {
		b, _ = json.MarshalIndent(v, "", "  ")
	} else {
		b, _ = json.Marshal(v)
	}
	os.MkdirAll(d.path, 0o700)
	path := filepath.Join(d.path, name)
	tmp := path + ".tmp"
	if os.WriteFile(tmp, b, 0o600) != nil || os.Rename(tmp, path) != nil {
		os.Remove(tmp)
	}
}

// Remove deletes the file name: what it kept is empty.
func (d Dir) Remove(name string) {
	if d.path == "" {
		return
	}
	os.Remove(filepath.Join(d.path, name))
}
