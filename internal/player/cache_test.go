package player

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// download opens data from a local server into cache, as key.
func download(t *testing.T, cache *Cache, data []byte, key string) *buffer {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(srv.Close)
	b, err := openBuffer(Source{URL: srv.URL, Key: key, Note: "note of " + key}, cache)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// files lists the cache's directory.
func files(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// readAll reads b through ReadAt, as the decoders do.
func readAll(t *testing.T, b *buffer) []byte {
	t.Helper()
	var out []byte
	p := make([]byte, 5000)
	for off := int64(0); ; {
		n, err := b.ReadAt(p, off)
		out = append(out, p[:n]...)
		off += int64(n)
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

// A song that came whole plays again from the cache, the same bytes, and
// the MP3 decoder reads it from the file as it does from memory.
func TestCacheKeeps(t *testing.T) {
	data, err := os.ReadFile("testdata/sine.mp3")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	cache := NewCache(dir, 1<<30)
	b := download(t, cache, data, "song-1")
	if got := readAll(t, b); !bytes.Equal(got, data) {
		t.Fatalf("read %d bytes of %d", len(got), len(data))
	}
	b.Close()
	if note, ok := cache.Lookup("song-1"); !ok || note != "note of song-1" {
		t.Fatalf("lookup: %q, %v; files %v", note, ok, files(t, dir))
	}
	if n := len(files(t, dir)); n != 2 {
		t.Fatalf("files %v, want the song and its JSON", files(t, dir))
	}
	b, err = openCached(cache, "song-1")
	if err != nil {
		t.Fatal(err)
	}
	if got := readAll(t, b); !bytes.Equal(got, data) {
		t.Fatalf("read %d bytes of %d from the cache", len(got), len(data))
	}
	d, err := openDecoder(b, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]float32, 4096)
	for {
		if _, err := d.Read(buf); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
	}
	b.Close()
	if n := len(files(t, dir)); n != 2 {
		t.Fatalf("closing what the cache keeps removed it: %v", files(t, dir))
	}
}

// What is not kept, or cut short, leaves no file behind; a key that is
// not a file name is not looked up.
func TestCacheDrops(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 300<<10)
	dir := t.TempDir()
	cache := NewCache(dir, 1<<30)
	b := download(t, cache, data, "")
	readAll(t, b)
	b.Close()
	if names := files(t, dir); len(names) != 0 {
		t.Fatalf("a song not to keep left %v", names)
	}

	hold := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000000")
		w.Write(data[:1000])
		w.(http.Flusher).Flush()
		select {
		case <-hold:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(hold)
	b, err := openBuffer(Source{URL: srv.URL, Key: "cut"}, cache)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := b.ReadAt(make([]byte, 1000), 0); n != 1000 || err != nil {
		t.Fatalf("read %d, %v", n, err)
	}
	b.Close()
	if names := files(t, dir); len(names) != 0 {
		t.Fatalf("a download cut short left %v", names)
	}
	if _, ok := cache.Lookup("cut"); ok {
		t.Fatal("a download cut short is kept")
	}
	if validKey("../x") || validKey("") || !validKey("0039MnYb0qxYhV-2") {
		t.Fatal("validKey")
	}
}

// Past its limit the cache drops the songs used longest ago, but not one
// open; one past a quarter of the limit is not kept at all.
func TestCacheLimit(t *testing.T) {
	data := bytes.Repeat([]byte("y"), 100<<10)
	dir := t.TempDir()
	cache := NewCache(dir, 4*int64(len(data))+1000)
	keep := func(key string) {
		b := download(t, cache, data, key)
		readAll(t, b)
		b.Close()
	}
	for _, k := range []string{"a", "b", "c", "d"} {
		keep(k)
		time.Sleep(20 * time.Millisecond) // for the times of use to differ
	}
	cache.Lookup("a") // used again: "b" is the oldest now
	time.Sleep(20 * time.Millisecond)
	keep("e")
	var kept []string
	for _, k := range []string{"a", "b", "c", "d", "e"} {
		if _, ok := cache.Lookup(k); ok {
			kept = append(kept, k)
		}
	}
	if !slices.Equal(kept, []string{"a", "c", "d", "e"}) {
		t.Fatalf("kept %v", kept)
	}

	b := download(t, cache, bytes.Repeat([]byte("z"), 2*len(data)), "big")
	readAll(t, b)
	b.Close()
	if _, ok := cache.Lookup("big"); ok {
		t.Fatal("a song of half the limit is kept")
	}

	open, err := openCached(cache, "c")
	if err != nil {
		t.Fatal(err)
	}
	cache.Clear()
	if _, ok := cache.Lookup("c"); !ok {
		t.Fatal("clearing dropped the song open")
	}
	open.Close()
	cache.Clear()
	if names := files(t, dir); len(names) != 0 {
		t.Fatalf("cleared, the cache has %v", names)
	}
	if _, err := os.Stat(filepath.Join(dir, "c.json")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

// A download is named after the song as well, made fit for a file name.
func TestCacheNames(t *testing.T) {
	for in, want := range map[string]string{
		"晴天-HQ 320k":             "晴天-HQ 320k",
		`AC/DC: Back?<"In">*|\.`: "AC_DC_ Back___In_____",
		"../../x":                ".._.._x",
		" 走 . ":                  "走",
		"":                       "",
	} {
		if got := fileLabel(in); got != want {
			t.Errorf("fileLabel(%q) = %q, want %q", in, got, want)
		}
	}
	dir := t.TempDir()
	cache := NewCache(dir, 1<<30)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("x")) }))
	defer srv.Close()
	b, err := openBuffer(Source{URL: srv.URL, Key: "k", Name: fileLabel("晴天-HQ 320k")}, cache)
	if err != nil {
		t.Fatal(err)
	}
	readAll(t, b)
	b.Close()
	names := files(t, dir)
	if len(names) != 2 || !strings.HasSuffix(names[0], "-晴天-HQ 320k.audio") {
		t.Fatalf("files %v", names)
	}
	if _, ok := cache.Lookup("k"); !ok {
		t.Fatal("not kept")
	}
}
