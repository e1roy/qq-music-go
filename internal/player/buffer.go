package player

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// buffer downloads a file into memory on a goroutine of its own; reads
// wait for the bytes they ask for.
type buffer struct {
	cancel context.CancelFunc

	mu   sync.Mutex
	cond sync.Cond
	data []byte
	size int64 // -1 until known
	done bool
	err  error
}

var errClosed = errors.New("player: closed")

func openBuffer(url string) (*buffer, error) {
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
	b := &buffer{cancel: cancel, size: resp.ContentLength}
	b.cond.L = &b.mu
	if b.size > 0 {
		b.data = make([]byte, 0, b.size)
	}
	go b.fill(resp.Body)
	return b, nil
}

func (b *buffer) fill(body io.ReadCloser) {
	defer body.Close()
	chunk := make([]byte, 64<<10)
	for {
		n, err := body.Read(chunk)
		b.mu.Lock()
		b.data = append(b.data, chunk[:n]...)
		if err != nil {
			b.done = true
			if err != io.EOF {
				b.err = err
			} else if b.size < 0 {
				b.size = int64(len(b.data))
			}
		}
		b.cond.Broadcast()
		b.mu.Unlock()
		if err != nil {
			return
		}
	}
}

// ReadAt reads at off, waiting for the download to get there.
func (b *buffer) ReadAt(p []byte, off int64) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for int64(len(b.data)) < off+int64(len(p)) && !b.done {
		b.cond.Wait()
	}
	if off >= int64(len(b.data)) {
		if b.err != nil {
			return 0, b.err
		}
		return 0, io.EOF
	}
	n := copy(p, b.data[off:])
	if n < len(p) {
		if b.err != nil {
			return n, b.err
		}
		return n, io.EOF
	}
	return n, nil
}

// all waits for the whole file and returns it; it must not be changed.
func (b *buffer) all() ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for !b.done {
		b.cond.Wait()
	}
	return b.data, b.err
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
	return int64(len(b.data))
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
	return float64(len(b.data)) / float64(b.size)
}

func (b *buffer) Close() {
	b.cancel()
	b.mu.Lock()
	if !b.done {
		b.done, b.err = true, errClosed
	}
	b.cond.Broadcast()
	b.mu.Unlock()
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
