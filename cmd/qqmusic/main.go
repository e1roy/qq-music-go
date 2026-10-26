// Command qqmusic is the QQ Music desktop client.
package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime/debug"

	"qqmusic/internal/app"
	"qqmusic/internal/diag"
)

func main() {
	// The exe has no console: a crash writes its trace to crash.log beside
	// the settings, or it would be lost.
	if base, err := os.UserConfigDir(); err == nil {
		dir := filepath.Join(base, "QQMusicGo")
		os.MkdirAll(dir, 0o700)
		if f, err := os.OpenFile(filepath.Join(dir, "crash.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600); err == nil {
			debug.SetCrashOutput(f, debug.CrashOptions{})
			f.Close()
		}
	}
	// QQMUSIC_PPROF=127.0.0.1:6060 serves a page of the app's memory over
	// time at /mem, and net/http/pprof.
	if addr := os.Getenv("QQMUSIC_PPROF"); addr != "" {
		diag.Start(addr)
	}
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
