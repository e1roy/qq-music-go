package player

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
)

// buffer downloads a file on a goroutine of its own; reads wait for the
// bytes they ask for. With a cache, it downloads into a file of the cache
// and reads it back: the system keeps what of it was read lately, rather
// than the app's heap all of it. Without one, or once writing the file
// failed, it keeps the bytes in memory.
type buffer struct {
	cancel context.CancelFunc
	cache  *Cache
	// What the cache keeps the file as once it is whole: "" keeps it not.
	key, note string

	mu     sync.Mutex
	cond   sync.Cond
	file   *os.File // nil: the bytes are in data
	data   []byte
	n      int64 // the bytes that came
	size   int64 // -1 until known
	done   bool
	err    error
	kept   bool // the cache keeps the file: closing leaves it
	closed bool
}

var errClosed = errors.New("player: closed")

// maxPrealloc is the most a buffer takes at once for the length a server
// tells: past it, it grows as the bytes come, so that a wrong length does
// not take the memory before any of it does.
const maxPrealloc = 32 << 20

func openBuffer(src Source, cache *Cache) (*buffer, error) {
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		cancel()
		return nil, err
	}
	req.Header.Set("Referer", "https://y.qq.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("player: HTTP %s", resp.Status)
	}
	b := &buffer{cancel: cancel, cache: cache, key: src.Key, note: src.Note, size: resp.ContentLength}
	b.cond.L = &b.mu
	b.file = cache.create(src.Name)
	if b.file == nil && b.size > 0 {
		b.data = make([]byte, 0, min(b.size, maxPrealloc))
	}
	go b.fill(resp.Body)
	return b, nil
}

// openCached opens the file cache keeps as key, whole.
func openCached(cache *Cache, key string) (*buffer, error) {
	f, size, err := cache.open(key)
	if err != nil {
		return nil, err
	}
	b := &buffer{cancel: func() {}, cache: cache, file: f, n: size, size: size, done: true, kept: true}
	b.cond.L = &b.mu
	return b, nil
}

func (b *buffer) fill(body io.ReadCloser) {
	defer body.Close()
	chunk := make([]byte, 64<<10)
	for {
		n, err := body.Read(chunk)
		b.mu.Lock()
		f, at := b.file, b.n
		b.mu.Unlock()
		// Written outside the lock, for reads not to wait on the disk:
		// only fill writes past n.
		if f != nil && n > 0 {
			if _, werr := f.WriteAt(chunk[:n], at); werr != nil {
				b.toMemory()
			}
		}
		b.mu.Lock()
		if b.done { // closed, or the file failed to read back
			b.mu.Unlock()
			return
		}
		if b.file == nil {
			b.data = append(b.data, chunk[:n]...)
		}
		b.n += int64(n)
		if err != nil {
			b.done = true
			if err != io.EOF {
				b.err = err
			} else {
				if b.size < 0 {
					b.size = b.n
				}
				// Kept before it reads as whole, for the next lookup to
				// find it.
				if b.file != nil && b.key != "" && b.cache.fits(b.n) {
					b.kept = true
					b.cache.keep(b.key, b.note, b.file.Name(), b.n)
				}
			}
		}
		b.cond.Broadcast()
		b.mu.Unlock()
		if err != nil {
			return
		}
	}
}

// toMemory goes on in memory with what came, after writing the file
// failed, as with a full disk: the song plays, but is not kept.
func (b *buffer) toMemory() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.file == nil {
		return
	}
	b.data = make([]byte, b.n, max(b.n, min(b.size, maxPrealloc)))
	if _, err := b.file.ReadAt(b.data, 0); err != nil {
		b.done, b.err = true, err
	}
	b.dropFile()
}

// dropFile closes the file, and removes it unless the cache keeps it.
func (b *buffer) dropFile() {
	name := b.file.Name()
	b.file.Close()
	if !b.kept {
		os.Remove(name)
	}
	b.cache.release(name)
	b.file = nil
}

// ReadAt reads at off, waiting for the download to get there.
func (b *buffer) ReadAt(p []byte, off int64) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for b.n < off+int64(len(p)) && !b.done {
		b.cond.Wait()
	}
	if b.closed {
		return 0, errClosed
	}
	if off >= b.n {
		if b.err != nil {
			return 0, b.err
		}
		return 0, io.EOF
	}
	want := min(int64(len(p)), b.n-off)
	n := 0
	if b.file != nil {
		var err error
		if n, err = b.file.ReadAt(p[:want], off); err != nil {
			return n, err
		}
	} else {
		n = copy(p, b.data[off:b.n])
	}
	if n < len(p) {
		if b.err != nil {
			return n, b.err
		}
		return n, io.EOF
	}
	return n, nil
}

// whole waits for the whole file and returns its length.
func (b *buffer) whole() (int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for !b.done {
		b.cond.Wait()
	}
	return b.n, b.err
}

// known is the length of the file the server told, or -1.
func (b *buffer) known() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.size > 0 {
		return b.size
	}
	return -1
}

// Size is the length of the file, or what was downloaded if unknown.
func (b *buffer) Size() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.size > 0 {
		return b.size
	}
	return b.n
}

// Progress is how much of the file is downloaded, from 0 to 1.
func (b *buffer) Progress() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.done {
		return 1
	}
	if b.size <= 0 {
		return 0
	}
	return float64(b.n) / float64(b.size)
}

func (b *buffer) Close() {
	b.cancel()
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	if !b.done {
		b.done, b.err = true, errClosed
	}
	if b.file != nil {
		b.dropFile()
	}
	b.cond.Broadcast()
}

// reader reads a buffer from an offset, as an io.Reader only, so that
// go-mp3 does not scan the whole file for its length first.
type reader struct {
	b   *buffer
	off int64
}

func (r *reader) Read(p []byte) (int, error) {
	n, err := r.b.ReadAt(p, r.off)
	r.off += int64(n)
	if n > 0 && err == io.EOF {
		err = nil
	}
	return n, err
}
