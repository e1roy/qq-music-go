// Package diag serves what helps look into the running app, on a local
// address: a page of its memory over time at /mem, and net/http/pprof.
package diag

import (
	_ "embed"
	"encoding/json"
	"log"
	"net/http"
	_ "net/http/pprof"
	"runtime/debug"
	"runtime/metrics"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

//go:embed mem.html
var memPage []byte

// sample is the memory of the app at a moment, in bytes.
type sample struct {
	t          int64 // Unix milliseconds
	privateWS  uint64
	commit     uint64
	heapLive   uint64 // objects, live or not swept yet
	heapFree   uint64 // free and kept from the system
	goroutines uint64
}

// event is something that changes the memory: a song starting, or the
// heap given back.
type event struct {
	t          int64
	kind, text string
}

const (
	keepSamples = 3600 // one a second: an hour
	keepEvents  = 1000
)

var (
	on      atomic.Bool
	mu      sync.Mutex
	samples []sample
	events  []event
)

// Start serves the page and pprof at addr, and samples the memory each
// second, until the app exits.
func Start(addr string) {
	on.Store(true)
	http.HandleFunc("/{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/mem", http.StatusFound)
	})
	http.HandleFunc("/mem", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(memPage)
	})
	http.HandleFunc("/mem.json", serveData)
	http.HandleFunc("POST /mem/gc", func(w http.ResponseWriter, r *http.Request) {
		freeOSMemory("gc")
		w.WriteHeader(http.StatusNoContent)
	})
	go func() {
		for {
			take()
			time.Sleep(time.Second)
		}
	}()
	go func() { log.Println(http.ListenAndServe(addr, nil)) }()
}

// Event notes something of kind, as "song", at this moment.
func Event(kind, text string) {
	if !on.Load() {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if len(events) >= keepEvents {
		events = append(events[:0], events[1:]...)
	}
	events = append(events, event{time.Now().UnixMilli(), kind, text})
}

// FreeOSMemory is debug.FreeOSMemory, noted as an event with how much it
// gave back.
func FreeOSMemory() { freeOSMemory("trim") }

func freeOSMemory(kind string) {
	if !on.Load() {
		debug.FreeOSMemory()
		return
	}
	before := goMemory()
	debug.FreeOSMemory()
	after := goMemory()
	Event(kind, strconv.FormatFloat(float64(int64(before.heapFree+before.heapLive)-int64(after.heapFree+after.heapLive))/(1<<20), 'f', 1, 64)+" MB")
	take()
}

var metricNames = []string{
	"/memory/classes/heap/objects:bytes",
	"/memory/classes/heap/free:bytes",
	"/sched/goroutines:goroutines",
}

// goMemory reads the Go heap, without stopping the world as
// runtime.ReadMemStats does.
func goMemory() sample {
	ms := make([]metrics.Sample, len(metricNames))
	for i, n := range metricNames {
		ms[i].Name = n
	}
	metrics.Read(ms)
	return sample{heapLive: ms[0].Value.Uint64(), heapFree: ms[1].Value.Uint64(), goroutines: ms[2].Value.Uint64()}
}

func take() {
	s := goMemory()
	s.t = time.Now().UnixMilli()
	s.privateWS, s.commit = processMemory()
	mu.Lock()
	defer mu.Unlock()
	if len(samples) >= keepSamples {
		samples = append(samples[:0], samples[1:]...)
	}
	samples = append(samples, s)
}

// serveData answers the samples and events after ?since=, in Unix
// milliseconds: [t, privateWS, commit, heapLive, heapFree, goroutines]
// and [t, kind, text].
func serveData(w http.ResponseWriter, r *http.Request) {
	since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	mu.Lock()
	out := struct {
		S [][6]int64 `json:"s"`
		E [][3]any   `json:"e"`
	}{S: [][6]int64{}, E: [][3]any{}}
	for _, s := range samples {
		if s.t > since {
			out.S = append(out.S, [6]int64{s.t, int64(s.privateWS), int64(s.commit), int64(s.heapLive), int64(s.heapFree), int64(s.goroutines)})
		}
	}
	for _, e := range events {
		if e.t > since {
			out.E = append(out.E, [3]any{e.t, e.kind, e.text})
		}
	}
	mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(out)
}
