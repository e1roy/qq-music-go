package app

import (
	"sync"
	"time"

	"qqmusic/internal/diag"
)

// trimmer gives the memory the Go heap no longer uses back to the system,
// a moment after the last call to soon. A song downloads into a block of
// some 10 MB, and pictures come in bursts: once they are dropped, the
// runtime would keep their pages for minutes, and they count as the app's.
type trimmer struct {
	mu sync.Mutex
	t  *time.Timer
}

const trimDelay = 3 * time.Second

func (tr *trimmer) soon() {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if tr.t == nil {
		tr.t = time.AfterFunc(trimDelay, diag.FreeOSMemory)
		return
	}
	tr.t.Reset(trimDelay)
}
