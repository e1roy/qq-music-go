package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"qqmusic/internal/player"
	"qqmusic/internal/playqueue"
	"qqmusic/internal/qqmusic"
	"qqmusic/internal/xiaoyuzhou"
)

// These tests pin down what the app does now, offline, so that moving its
// code around can be checked against them.

var errOffline = errors.New("offline")

// offlineTransport fails every request at once: a load a test sets off
// sees its failure, and no service is asked anything.
type offlineTransport struct{}

func (offlineTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, errOffline }

func newOfflineApp(t *testing.T, dir string) (*app, *fakePlatform) {
	t.Helper()
	f := &fakePlatform{posts: make(chan func(), 1024)}
	a := newApp(f, dir)
	a.api.HTTP = &http.Client{Transport: offlineTransport{}}
	a.xiaoyuzhou.api.HTTP = &http.Client{Transport: offlineTransport{}}
	a.images = newImages(func(context.Context, string) ([]byte, error) { return nil, errOffline }, f.Post, a.trim.soon)
	return a, f
}

// drain runs what was posted until cond holds.
func drain(t *testing.T, f *fakePlatform, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.After(d)
	for !cond() {
		select {
		case fn := <-f.posts:
			fn()
		case <-deadline:
			t.Fatal("timed out")
		}
	}
}

// episodes are songs of 小宇宙 whose sound is on a port nothing listens
// on: they need no login, load no lyrics, and fail to play at once.
func episodes(names ...string) []qqmusic.Song {
	songs := make([]qqmusic.Song, len(names))
	for i, n := range names {
		songs[i] = qqmusic.Song{Mid: n, Name: n, Source: xiaoyuzhouSource, URL: "http://127.0.0.1:1/" + n + ".m4a"}
	}
	return songs
}

func setQueue(a *app, songs []qqmusic.Song, cur int) {
	a.queue = playqueue.Restore(songs, cur, false)
}

// queueState is the mids of the queue, and where it is.
func queueState(a *app) (string, int) {
	mids := make([]string, len(a.queue.Songs()))
	for i, s := range a.queue.Songs() {
		mids[i] = s.Mid
	}
	return strings.Join(mids, " "), a.queue.Cur()
}

// TestSongWay checks that the bar slides the song in from the side the
// change came from: the next one from the right, the one before from the
// left.
func TestSongWay(t *testing.T) {
	a, _ := newOfflineApp(t, t.TempDir())
	setQueue(a, episodes("a", "b"), 1)
	at := a.songTrack
	a.prev()
	if a.songTrack != at-1 {
		t.Fatalf("track %d after 上一首, want %d", a.songTrack, at-1)
	}
	a.next(false)
	if a.songTrack != at {
		t.Fatalf("track %d after 下一首, want %d", a.songTrack, at)
	}
}

func TestQueueEdits(t *testing.T) {
	dir := t.TempDir()
	a, _ := newOfflineApp(t, dir)
	setQueue(a, episodes("a", "b", "c", "d"), 1)
	check := func(step, mids string, cur int) {
		t.Helper()
		if m, c := queueState(a); m != mids || c != cur {
			t.Fatalf("%s: queue %q at %d, want %q at %d", step, m, c, mids, cur)
		}
	}
	a.playNext(episodes("x")[0])
	check("playNext", "a b x c d", 1)
	a.enqueue(episodes("y")[0])
	check("enqueue", "a b x c d y", 1)
	if want := []string{"已添加到下一首播放", "已添加到播放队列"}; !reflect.DeepEqual(a.toasts, want) {
		t.Fatalf("toasts %q, want %q", a.toasts, want)
	}
	a.removeFromQueue(0)
	check("remove before", "b x c d y", 0)
	a.removeFromQueue(2)
	check("remove after", "b x d y", 0)
	a.removeFromQueue(0)
	check("remove the current", "x d y", 0)
	a.removeFromQueue(9)
	check("remove out of range", "x d y", 0)
	a.queue.Select(2)
	a.removeFromQueue(2)
	check("remove the current last", "x d", 1)

	var q savedQueue
	if !readJSON(t, filepath.Join(dir, "queue.json"), &q) || q.Cur != 1 || len(q.Songs) != 2 {
		t.Fatalf("queue.json: %+v", q)
	}
	a.removeFromQueue(0)
	a.removeFromQueue(0)
	check("remove all", "", -1)
	if _, err := os.Stat(filepath.Join(dir, "queue.json")); !os.IsNotExist(err) {
		t.Fatalf("queue.json is left: %v", err)
	}

	// Into an empty queue, a song plays at once, without a toast.
	a.toasts = nil
	a.enqueue(episodes("z")[0])
	check("enqueue into empty", "z", 0)
	if len(a.toasts) != 0 {
		t.Fatalf("toasts %q", a.toasts)
	}
	a.queue.SetRadio(true)
	a.clearQueue()
	check("clear", "", -1)
	if a.queue.Radio() {
		t.Fatal("clear kept the radio")
	}
}

func TestNextPrev(t *testing.T) {
	for _, tc := range []struct {
		mode      playqueue.Mode
		cur       int
		auto, rev bool
		want      int
	}{
		{playqueue.Loop, 0, false, false, 1},
		{playqueue.Loop, 2, true, false, 0},
		{playqueue.Loop, 2, false, false, 0},
		{playqueue.Loop, 0, false, true, 2},
		{playqueue.Loop, 2, false, true, 1},
		{playqueue.One, 1, true, false, 1},
		{playqueue.One, 1, false, false, 2},
		{playqueue.One, 2, false, false, 0},
		{playqueue.Order, 0, true, false, 1},
		{playqueue.Order, 2, true, false, 2},
		{playqueue.Order, 2, false, false, 0},
		{playqueue.Order, 0, false, true, 2},
	} {
		a, _ := newOfflineApp(t, "")
		setQueue(a, episodes("a", "b", "c"), tc.cur)
		a.set.Mode = tc.mode
		if tc.rev {
			a.prev()
		} else {
			a.next(tc.auto)
		}
		if _, cur := queueState(a); cur != tc.want {
			t.Errorf("%v from %d (auto %v, prev %v): at %d, want %d", tc.mode, tc.cur, tc.auto, tc.rev, cur, tc.want)
		}
	}

	// Shuffled, next goes through the whole queue in some order, and prev
	// goes back the way it came.
	a, _ := newOfflineApp(t, "")
	setQueue(a, episodes("a", "b", "c", "d"), 0)
	a.set.Mode = playqueue.Shuffle
	seen := map[int]bool{0: true}
	for range 3 {
		before := a.queue.Cur()
		a.next(false)
		seen[a.queue.Cur()] = true
		a.prev()
		if a.queue.Cur() != before {
			t.Fatalf("prev went to %d, not back to %d", a.queue.Cur(), before)
		}
		a.next(true)
	}
	if len(seen) != 4 {
		t.Errorf("shuffle visited %v", seen)
	}
	// A queue that grew is shuffled again: next reaches the new song.
	a.enqueue(episodes("e")[0])
	seen = map[int]bool{}
	for range 5 {
		a.next(false)
		seen[a.queue.Cur()] = true
	}
	if len(seen) != 5 {
		t.Errorf("shuffle after enqueue visited %v", seen)
	}

	empty, _ := newOfflineApp(t, "")
	empty.next(true)
	empty.prev()
	if _, cur := queueState(empty); cur != -1 {
		t.Errorf("empty queue went to %d", cur)
	}
}

// TestPlayRadio: 猜你喜欢 asks to log in rather than play, and the queue
// left as it was is not made the radio's, to grow with its songs.
func TestPlayRadio(t *testing.T) {
	a, _ := newOfflineApp(t, "")
	setQueue(a, episodes("a", "b", "c"), 2)
	a.home.guess = []qqmusic.Song{{Mid: "g", Name: "猜你喜欢的歌"}}
	a.login.tab = 2 // the tab of the QQ account, which loads no QR code
	a.playRadio()
	if m, c := queueState(a); m != "a b c" || c != 2 || a.queue.Radio() {
		t.Fatalf("queue %q at %d, radio %v", m, c, a.queue.Radio())
	}
	if !a.login.open || len(a.toasts) != 1 || a.toasts[0] != "登录后即可播放歌曲" {
		t.Fatalf("login open %v, toasts %q", a.login.open, a.toasts)
	}

	// Of episodes, which need no login, the queue becomes the radio's.
	a.home.guess = episodes("x", "y")
	a.playRadio()
	if m, c := queueState(a); m != "x y" || c != 0 || !a.queue.Radio() {
		t.Fatalf("queue %q at %d, radio %v", m, c, a.queue.Radio())
	}
}

// TestPlayFailures skips a song that fails while the queue has others,
// up to the third failure in a row.
func TestPlayFailures(t *testing.T) {
	a, f := newOfflineApp(t, "")
	setQueue(a, episodes("a", "b", "c"), 0)
	a.playAt(0)
	drain(t, f, 15*time.Second, func() bool { return a.failStreak == 3 })
	if _, cur := queueState(a); cur != 2 || a.playErr == "" {
		t.Fatalf("at %d, error %q", cur, a.playErr)
	}
	if len(a.toasts) != 3 || !strings.HasSuffix(a.toasts[0], "，已跳过") || !strings.HasSuffix(a.toasts[1], "，已跳过") ||
		strings.Contains(a.toasts[2], "已跳过") || !strings.HasPrefix(a.toasts[2], "《c》无法播放：") {
		t.Fatalf("toasts %q", a.toasts)
	}
	if len(a.recent) != 0 {
		t.Fatalf("a song that failed is recent: %v", a.recent)
	}

	one, f := newOfflineApp(t, "")
	setQueue(one, episodes("a"), 0)
	one.playAt(0)
	drain(t, f, 15*time.Second, func() bool { return one.failStreak == 1 })
	if len(one.toasts) != 1 || strings.Contains(one.toasts[0], "已跳过") {
		t.Fatalf("toasts %q", one.toasts)
	}
}

// TestQualityLabel is what the player bar says of the quality of the
// song playing, as against what was asked for.
func TestQualityLabel(t *testing.T) {
	for _, tc := range []struct {
		set, asked, got qqmusic.Quality
		known, episode  bool
		want            string
	}{
		{set: qqmusic.High, want: "HQ 320k"},
		{set: qqmusic.Best, want: "最高音质"},
		{known: true, asked: qqmusic.Best, got: qqmusic.AAC, want: "AAC（无 MP3 音源）"},
		{known: true, asked: qqmusic.Best, got: qqmusic.Lossless, want: "无损 FLAC"},
		{known: true, asked: qqmusic.High, got: qqmusic.Lossless, want: "无损 FLAC"},
		{known: true, asked: qqmusic.Lossless, got: qqmusic.High, want: "HQ 320k（无损不可用）"},
		{known: true, asked: qqmusic.High, got: qqmusic.Standard, want: "标准 128k（无高品质）"},
		{known: true, asked: qqmusic.Standard, got: qqmusic.High, want: "HQ 320k"},
		{known: true, asked: qqmusic.High, got: qqmusic.Standard, episode: true, want: "小宇宙"},
		{set: qqmusic.Best, episode: true, want: "小宇宙"},
	} {
		if got := qualityLabel(tc.set, tc.known, tc.asked, tc.got, tc.episode); got != tc.want {
			t.Errorf("%+v: %q", tc, got)
		}
	}
}

func readJSON(t *testing.T, path string, v any) bool {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
	return true
}

// TestPersistedFiles reads the files v1.0.1 wrote, as a run after an
// update does, and reads back what it saves of them. It does not pin the
// bytes written: a field added to a model writes more, and the files of
// earlier runs still read.
func TestPersistedFiles(t *testing.T) {
	dir := t.TempDir()
	fixtures := filepath.Join("testdata", "v1.0.1")
	entries, err := os.ReadDir(fixtures)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(fixtures, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(dir, e.Name()), b, 0o600)
	}
	a, f := newOfflineApp(t, dir)
	want := settings{Volume: 0.5, Quality: qqmusic.High, Mode: playqueue.Shuffle, MineClosed: true, Searches: []string{"晴天"},
		WinX: 10, WinY: 20, WinWidth: 1100, WinHeight: 780, XiaoyuzhouOfficial: true, CacheGB: 2}
	if !reflect.DeepEqual(a.set, want) || a.volume != 0.5 {
		t.Errorf("settings %+v, want %+v", a.set, want)
	}
	if m, c := queueState(a); m != "0039MnYb0qxYhV e" || c != 1 || !a.queue.Radio() {
		t.Errorf("queue %q at %d, radio %v", m, c, a.queue.Radio())
	}
	if s := a.queue.Songs()[1]; !isXiaoyuzhou(s) || s.URL != "http://127.0.0.1:1/e.m4a" {
		t.Errorf("the episode of the queue: %+v", s)
	}
	if len(a.recent) != 1 || a.recent[0].Name != "晴天" || a.recent[0].Singers[0].Name != "周杰伦" ||
		a.recent[0].Album.Name != "叶惠美" || a.recent[0].Interval != 269 || a.recent[0].File.Size128 != 4317292 {
		t.Errorf("recent %+v", a.recent)
	}
	if len(a.podcast.heard) != 1 || a.podcast.heard[0].Title != "节目" || a.podcast.heard[0].Host != "主播" || !a.podcast.known["show1"] {
		t.Errorf("heard %+v", a.podcast.heard)
	}
	if cr := a.xiaoyuzhou.api.Credential(); cr.AccessToken != "access" || cr.RefreshToken != "refresh" || cr.DeviceID != "device" || a.xiaoyuzhou.user.Nickname != "听众" {
		t.Errorf("小宇宙 login %+v %+v", cr, a.xiaoyuzhou.user)
	}
	if cr := a.api.Credential(); cr.MusicID != 12345 || cr.MusicKey != "key" || cr.LoginType != 2 {
		t.Errorf("QQ login %+v", cr)
	}
	// The login of 小宇宙 moved to the file of its new name.
	if _, err := os.Stat(filepath.Join(dir, "xyz.json")); !os.IsNotExist(err) {
		t.Errorf("xyz.json is left: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "xiaoyuzhou.json")); err != nil {
		t.Errorf("xiaoyuzhou.json: %v", err)
	}

	// What it saves, the next run reads.
	a.set.FavClosed = true
	a.setVolume(0.3)
	a.saveSettings()
	a.enqueue(episodes("f")[0])
	a.addRecent(episodes("e")[0])
	a.hearShow(qqmusic.Song{Mid: "ep2", Album: qqmusic.Album{Mid: "show2", Name: "另一个节目"}})
	a.saveXiaoyuzhou()
	a.saveCredential()
	b := newApp(f, dir)
	if !reflect.DeepEqual(b.set, a.set) || b.volume != 0.3 {
		t.Errorf("settings %+v, want %+v", b.set, a.set)
	}
	if !reflect.DeepEqual(b.queue.Songs(), a.queue.Songs()) || b.queue.Cur() != a.queue.Cur() || !b.queue.Radio() {
		t.Errorf("queue %v at %d", b.queue.Songs(), b.queue.Cur())
	}
	if !reflect.DeepEqual(b.recent, a.recent) || !reflect.DeepEqual(b.podcast.heard, a.podcast.heard) {
		t.Errorf("recent %v, heard %v", b.recent, b.podcast.heard)
	}
	if b.xiaoyuzhou.api.Credential() != a.xiaoyuzhou.api.Credential() || b.xiaoyuzhou.user != a.xiaoyuzhou.user || b.api.Credential() != a.api.Credential() {
		t.Error("the logins did not come back")
	}

	// What is empty is removed.
	a.clearQueue()
	a.clearHeard()
	a.api.Logout()
	a.saveCredential()
	for _, name := range []string{"queue.json", "podcasts.json", "credential.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s is left: %v", name, err)
		}
	}

	// A mode it does not know, as a later version may write, is the
	// default: the view indexes its icons by it.
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"volume": 0.5, "mode": 4}`), 0o600)
	c, _ := newOfflineApp(t, dir)
	if c.set.Mode != playqueue.Loop {
		t.Fatalf("mode %d", c.set.Mode)
	}
	ui.NewTester(c.view, 1280, 820).Frame()

	// QQ Music's 小宇宙, as v1.0.1 wrote it, is kept against the default.
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"volume": 0.5, "xyz_official": false}`), 0o600)
	if c, _ := newOfflineApp(t, dir); c.set.XiaoyuzhouOfficial || c.set.OldOfficial != nil {
		t.Fatalf("official %v, old %v", c.set.XiaoyuzhouOfficial, c.set.OldOfficial)
	}

	// Without a folder, nothing is written anywhere.
	mem, _ := newOfflineApp(t, "")
	mem.saveSettings()
	mem.addRecent(episodes("e")[0])
	mem.saveXiaoyuzhou()
}

// TestSearchScope: on 小宇宙, the box searches its podcasts, with no
// suggestions of songs.
func TestSearchScope(t *testing.T) {
	for _, tc := range []struct {
		kind         pageKind
		label, menu  string
		suggests     bool
		searched     func(a *app) string
		otherQueries func(a *app) string
	}{
		{pageHome, "搜索歌曲", "搜索 “q”", true,
			func(a *app) string { return a.search.query },
			func(a *app) string { return a.podcast.search.query + a.xiaoyuzhou.search.query }},
		{pagePodcast, "搜索播客", "在小宇宙中搜索 “q”", false,
			func(a *app) string { return a.podcast.search.query },
			func(a *app) string { return a.search.query + a.xiaoyuzhou.search.query }},
		{pageXiaoyuzhou, "搜索小宇宙", "在小宇宙官方中搜索 “q”", false,
			func(a *app) string { return a.xiaoyuzhou.search.query },
			func(a *app) string { return a.search.query + a.podcast.search.query }},
	} {
		a, _ := newOfflineApp(t, "")
		a.history = []page{{kind: tc.kind}}
		tt := ui.NewTester(a.view, 1280, 820)
		if err := tt.Click(tc.label); err != nil {
			t.Fatalf("%v: %v", tc.kind, err)
		}
		a.search.input = "q"
		a.search.suggest, a.search.sugFor = []string{"q 的联想"}, "q"
		tt.Frame()
		if !tt.HasText(tc.menu) {
			t.Errorf("%v: no %q in %q", tc.kind, tc.menu, tt.Texts())
		}
		if got := tt.HasText("q 的联想"); got != tc.suggests {
			t.Errorf("%v: suggestions shown %v", tc.kind, got)
		}
		a.chooseSearch(" q ")
		if tc.searched(a) != "q" || tc.otherQueries(a) != "" {
			t.Errorf("%v: searched %q, others %q", tc.kind, tc.searched(a), tc.otherQueries(a))
		}
		if a.set.Searches[0] != "q" {
			t.Errorf("%v: history %q", tc.kind, a.set.Searches)
		}
	}
}

func (k pageKind) String() string { return fmt.Sprint("page ", int(k)) }

// TestLogoutRadio: logged out, the queue is no longer the radio's, in the
// next run too.
func TestLogoutRadio(t *testing.T) {
	dir := t.TempDir()
	a, f := newOfflineApp(t, dir)
	a.api.SetCredential(qqmusic.Credential{MusicID: 1, MusicKey: "key"})
	setQueue(a, episodes("a", "b"), 0)
	a.queue.SetRadio(true)
	a.saveQueue()
	a.logout()
	if b := newApp(f, dir); b.queue.Radio() || len(b.queue.Songs()) != 2 {
		t.Fatalf("after logout: radio %v, %d songs", b.queue.Radio(), len(b.queue.Songs()))
	}
}

// fakeLikes is QQ Music's 我喜欢 as the service keeps it: likes change it,
// and a read gives it as it was when the read came. Each read waits for
// the test to open its gate, for the test to order the answers.
type fakeLikes struct {
	mu    sync.Mutex
	liked []qqmusic.Song
	reads chan chan struct{}
}

func (fl *fakeLikes) RoundTrip(r *http.Request) (*http.Response, error) {
	var body struct {
		Req struct {
			Method string         `json:"method"`
			Param  map[string]any `json:"param"`
		} `json:"req"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, err
	}
	var data any
	switch body.Req.Method {
	case "AddSonglist", "DelSonglist":
		id := int(body.Req.Param["v_songInfo"].([]any)[0].(map[string]any)["songId"].(float64))
		fl.mu.Lock()
		fl.liked = slices.DeleteFunc(fl.liked, func(s qqmusic.Song) bool { return int(s.ID) == id })
		if body.Req.Method == "AddSonglist" {
			var s qqmusic.Song
			json.Unmarshal([]byte(fmt.Sprintf(`{"id":%d,"mid":"m%d"}`, id, id)), &s)
			fl.liked = append(fl.liked, s)
		}
		fl.mu.Unlock()
		data = map[string]any{"retCode": 0}
	case "CgiGetDiss":
		fl.mu.Lock()
		songs := slices.Clone(fl.liked)
		fl.mu.Unlock()
		gate := make(chan struct{})
		fl.reads <- gate
		<-gate
		data = map[string]any{"dirinfo": map[string]any{"dirid": 201, "songnum": len(songs)}, "songlist": songs, "total_song_num": len(songs)}
	default:
		return nil, errOffline
	}
	b, _ := json.Marshal(map[string]any{"code": 0, "req": map[string]any{"code": 0, "data": data}})
	return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(b))}, nil
}

// TestLikeRace likes a song and takes the like back at once: the read of
// 我喜欢 after the like, answered last, does not bring its heart back.
func TestLikeRace(t *testing.T) {
	a, f := newOfflineApp(t, "")
	fl := &fakeLikes{reads: make(chan chan struct{}, 4)}
	a.api.HTTP = &http.Client{Transport: fl}
	a.api.SetCredential(qqmusic.Credential{MusicID: 1, MusicKey: "key"})
	liked := page{kind: pagePlaylist, dirid: 201}
	a.lists[liked] = &songList{} // its page was opened
	l := a.lists[liked]
	var x qqmusic.Song
	json.Unmarshal([]byte(`{"id":7,"mid":"m7","name":"晴天"}`), &x)
	// read runs what was posted until a read of 我喜欢 waits.
	read := func() chan struct{} {
		t.Helper()
		deadline := time.After(10 * time.Second)
		for {
			select {
			case gate := <-fl.reads:
				return gate
			case fn := <-f.posts:
				fn()
			case <-deadline:
				t.Fatal("no read of 我喜欢")
			}
		}
	}

	a.toggleLike(x)
	afterLike := read() // the service has the like
	a.toggleLike(x)
	if a.liked[7] {
		t.Fatal("taking the like back did not show at once")
	}
	afterUnlike := read() // and no more
	close(afterUnlike)
	drain(t, f, 10*time.Second, func() bool { return !l.loading })
	if a.liked[7] {
		t.Fatal("the list without the like shows its heart")
	}
	close(afterLike)
	for end := time.After(500 * time.Millisecond); ; {
		select {
		case fn := <-f.posts:
			fn()
			continue
		case <-end:
		}
		break
	}
	if a.liked[7] || l.loading || len(l.songs) != 0 {
		t.Fatalf("the read before the like was taken back: liked %v, loading %v, songs %d", a.liked[7], l.loading, len(l.songs))
	}
	if want := []string{"已添加到我喜欢", "已取消喜欢"}; !reflect.DeepEqual(a.toasts, want) {
		t.Fatalf("toasts %q", a.toasts)
	}
}

// TestShowLabel: the page of an episode playing calls its album a show,
// as the song's menu does.
func TestShowLabel(t *testing.T) {
	a, _ := newOfflineApp(t, "")
	tt := ui.NewTester(a.view, 1280, 820)
	a.showLyrics = true
	for _, tc := range []struct {
		known bool
		want  string
	}{{false, "专辑："}, {true, "节目："}} {
		a.podcast.known["show"] = tc.known
		setQueue(a, []qqmusic.Song{{Mid: "e", Name: "第一集", Album: qqmusic.Album{Mid: "show", Name: "日谈公园"}}}, 0)
		tt.Frame()
		tt.Advance(time.Second)
		tt.Frame()
		if !tt.HasText(tc.want) {
			t.Errorf("known %v: no %q in %q", tc.known, tc.want, tt.Texts())
		}
	}
}

// TestLikedKeepsLikesOnTheWay: a read of 我喜欢 done before a like is
// does not take its heart away.
func TestLikedKeepsLikesOnTheWay(t *testing.T) {
	a, _ := newOfflineApp(t, "")
	a.liking[7] = true
	a.setLiked([]qqmusic.Song{{ID: 8}})
	if !a.liked[7] || !a.liked[8] {
		t.Fatalf("liked %v", a.liked)
	}
}

// TestPagerFailReset: an error that resets its list, as a login of
// 小宇宙 gone, is not shown on the list reset.
func TestPagerFailReset(t *testing.T) {
	a, f := newOfflineApp(t, "")
	p := &pager[int]{}
	loadPage(a, p, func(err error) string { p.reset(); return "gone" }, func(context.Context, int, xiaoyuzhou.Cursor) (func(), error) {
		return nil, errors.New("offline")
	})
	drain(t, f, 5*time.Second, func() bool { return !p.loading })
	if p.err != "" {
		t.Fatalf("err %q on the list reset", p.err)
	}
}

// TestXiaoyuzhouLoginSearchesWhenShown: logging in to 小宇宙 does not search it
// for a query made before, off the search page; the 播客 tab does, once
// the search page is shown again.
func TestXiaoyuzhouLoginSearchesWhenShown(t *testing.T) {
	a, _ := newOfflineApp(t, "")
	s := &a.search
	s.query, s.tab = "日谈公园", foundPodcasts
	s.podcasts.pages = 1
	a.xiaoyuzhou.api.SetTokens("access", "refresh")
	a.xiaoyuzhouLoggedIn()
	if s.xiaoyuzhouShows.loading || s.podcasts.loading {
		t.Fatalf("searched off the search page: 小宇宙 %v, QQ %v", s.xiaoyuzhouShows.loading, s.podcasts.loading)
	}
	a.navigate(page{kind: pageSearch, title: "搜索"})
	if !s.xiaoyuzhouShows.loading || s.podcasts.loading {
		t.Fatalf("search page shown: 小宇宙 %v, QQ %v", s.xiaoyuzhouShows.loading, s.podcasts.loading)
	}
}

// TestFrameBudget is how often the view is built while nothing it shows
// changes: never on a still page, and only at the player bar's pace while
// a song plays. An animation left running would cost CPU unnoticed.
func TestFrameBudget(t *testing.T) {
	a, f := newOfflineApp(t, t.TempDir())
	builds := 0
	tt := ui.NewTester(func(c *ui.Context) { builds++; a.view(c) }, 1280, 820)
	perSecond := func() int {
		for end := time.After(300 * time.Millisecond); ; {
			select {
			case fn := <-f.posts:
				fn()
				tt.Frame()
				continue
			case <-end:
			}
			break
		}
		tt.Frame()
		tt.Advance(2 * time.Second) // for what moves in to settle
		builds = 0
		tt.Advance(time.Second)
		return builds
	}
	if n := perSecond(); n > 0 {
		t.Errorf("a still page: %d builds a second", n)
	}

	// A song whose sound never comes: it loads, as one playing does.
	hold := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-hold:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(hold) })
	t.Cleanup(a.player.Stop)
	song := episodes("a")[0]
	song.URL = srv.URL
	setQueue(a, []qqmusic.Song{song}, 0)
	a.playAt(0)
	// Loading is set on the worker, which posts nothing until the sound
	// comes.
	for deadline := time.Now().Add(5 * time.Second); !a.player.Loading(); {
		if time.Now().After(deadline) {
			t.Fatal("the song never started loading")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if n := perSecond(); n < 3 || n > 5 {
		t.Errorf("a song playing: %d builds a second, want 4", n)
	}
	a.showLyrics = true
	if n := perSecond(); n < 9 || n > 11 {
		t.Errorf("its lyrics: %d builds a second, want 10", n)
	}
}

// TestPlayFromCache plays a song the cache keeps without asking for its
// URL, which offline would fail, and says the quality it got then; as
// SongURL, it needs the login.
func TestPlayFromCache(t *testing.T) {
	a, f := newOfflineApp(t, t.TempDir())
	a.player.Cache = player.NewCache(t.TempDir(), 1<<30)
	a.player.SetVolume(0)
	t.Cleanup(a.player.Stop)
	data, err := os.ReadFile("../player/testdata/sine.mp3")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data) }))
	t.Cleanup(srv.Close)
	key := fmt.Sprintf("m1-%d", qqmusic.Best)
	if _, err := a.player.Load(player.Source{URL: srv.URL, Key: key, Note: fmt.Sprint(int(qqmusic.High))}, 0); err != nil {
		t.Fatal(err)
	}
	for a.player.Buffered() < 1 {
		time.Sleep(10 * time.Millisecond)
	}
	a.player.Stop()

	a.api.SetCredential(qqmusic.Credential{MusicID: 1, MusicKey: "k"})
	setQueue(a, []qqmusic.Song{{Mid: "m1", Name: "m1"}}, 0)
	a.playAt(0)
	drain(t, f, 5*time.Second, func() bool { return a.qualityKnown || a.playErr != "" })
	if a.playErr != "" || a.gotQuality != qqmusic.High || !a.player.Loaded() {
		t.Fatalf("error %q, got %v, loaded %v", a.playErr, a.gotQuality, a.player.Loaded())
	}

	a.api.SetCredential(qqmusic.Credential{})
	a.qualityKnown = false
	a.playAt(0)
	drain(t, f, 5*time.Second, func() bool { return a.playErr != "" })
	if a.player.Loaded() {
		t.Fatal("played from the cache logged out")
	}
}

// TestSettings opens 设置 from the gear beside the user, not the user's
// menu, and sets the song cache there: its limit, kept, and clearing it.
func TestSettings(t *testing.T) {
	data := t.TempDir()
	a, f := newOfflineApp(t, data)
	dir := t.TempDir()
	a.player.Cache = player.NewCache(dir, 2<<30)
	os.WriteFile(filepath.Join(dir, "x.audio"), make([]byte, 1000), 0o600)
	a.api.SetCredential(qqmusic.Credential{MusicID: 1, MusicKey: "k"})
	a.user.Nick = "e1roy"
	tt := ui.NewTester(a.view, 1280, 820)
	tt.Frame()
	if err := tt.Click("e1roy"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(tt.Menu(), "退出登录") {
		t.Fatalf("the user's menu: %q", tt.Menu())
	}
	tt.CloseMenu()
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	drain(t, f, 5*time.Second, func() bool { return a.cacheUsed >= 0 && !a.cacheBusy })
	tt.Frame()
	if a.current().kind != pageSettings || len(tt.Menu()) != 0 {
		t.Fatalf("at %v, menu %q", a.current().kind, tt.Menu())
	}
	if a.cacheUsed != 1000 || !tt.HasText("空间上限") {
		t.Fatalf("used %d", a.cacheUsed)
	}
	if err := tt.Click("5 GB"); err != nil {
		t.Fatal(err)
	}
	if a.set.CacheGB != 5 {
		t.Fatalf("limit %d GB", a.set.CacheGB)
	}
	drain(t, f, 5*time.Second, func() bool { return !a.cacheBusy })
	tt.Frame()
	if err := tt.Click("清空缓存"); err != nil {
		t.Fatal(err)
	}
	drain(t, f, 5*time.Second, func() bool { return !a.cacheBusy })
	if a.cacheUsed != 0 {
		t.Fatalf("cleared, the cache takes %d", a.cacheUsed)
	}
	var saved settings
	if !readJSON(t, filepath.Join(data, fileSettings), &saved) || saved.CacheGB != 5 {
		t.Fatalf("saved %+v", saved)
	}
}

// TestDownloadName is the name of the file a song downloads to: its own,
// and the type its URL ends in.
func TestDownloadName(t *testing.T) {
	a, f := newOfflineApp(t, t.TempDir())
	dir := t.TempDir()
	a.player.Cache = player.NewCache(dir, 1<<30)
	a.player.SetVolume(0)
	t.Cleanup(a.player.Stop)
	data, err := os.ReadFile("../player/testdata/sine.mp3")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data) }))
	t.Cleanup(srv.Close)
	ep := episodes("医生")[0]
	ep.URL = srv.URL + "/ep/1.mp3?t=1"
	setQueue(a, []qqmusic.Song{ep}, 0)
	a.playAt(0)
	drain(t, f, 5*time.Second, func() bool { return a.player.Loaded() || a.playErr != "" })
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || !strings.HasSuffix(entries[0].Name(), "-医生-mp3.audio") {
		t.Fatalf("files %v, error %q", entries, a.playErr)
	}
}
