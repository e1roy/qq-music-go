package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"qqmusic/internal/qqmusic"
)

func TestZZFeed(t *testing.T) {
	f := &fakePlatform{posts: make(chan func(), 1024)}
	a := newApp(f, "")
	b, _ := os.ReadFile(filepath.Join(os.Getenv("APPDATA"), "QQMusicGo", "credential.json"))
	var cr qqmusic.Credential
	json.Unmarshal(b, &cr)
	a.api.SetCredential(cr)
	tt := ui.NewTester(a.view, 1280, 4200)
	a.enter(a.current())
	if !settle(tt, f, 30*time.Second, func() bool { return a.home.loaded }) {
		t.Fatal("home did not load")
	}
	settle(tt, f, 6*time.Second, func() bool { return false })
	t.Logf("feed %d err %q", len(a.home.feed), a.home.err)
	for _, sh := range a.home.feed {
		t.Logf("%q %d %d", sh.Title, len(sh.Songs), len(sh.Playlists))
	}
	if err := tt.Click("换一批"); err != nil {
		t.Log(err)
	}
	shot(t, tt, "feed")
}
