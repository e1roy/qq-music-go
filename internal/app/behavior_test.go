package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
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

// TestHistoryKeys checks that the mouse's side buttons, which MyGo reports
// as KeyBack and KeyForward, walk the history as Alt with the arrows does.
func TestHistoryKeys(t *testing.T) {
	a, _ := newOfflineApp(t, t.TempDir())
	tt := ui.NewTester(a.view, 1280, 820)
	home := a.current()
	a.navigate(page{kind: pageSettings})
	tt.Frame()
	if a.current().kind != pageSettings {
		t.Fatalf("on %+v, want the settings", a.current())
	}
	tt.Key(0, ui.KeyBack)
	tt.Frame()
	if a.current().key() != home.key() {
		t.Fatalf("on %+v after the back button, want %+v", a.current(), home)
	}
	tt.Key(0, ui.KeyForward)
	tt.Frame()
	if a.current().kind != pageSettings {
		t.Fatalf("on %+v after the forward button, want the settings", a.current())
	}
}

// TestCommentsSlideIn checks that the comments panel comes in from the
// edge of its room rather than showing at its place.
func TestCommentsSlideIn(t *testing.T) {
	a, _ := newOfflineApp(t, t.TempDir())
	a.showLyrics = true
	tt := ui.NewTester(a.view, 1280, 820)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	a.toggleComments()
	tt.Frame()
	start, ok := tt.Find("还没有评论")
	if !ok {
		t.Fatalf("no comments panel: %q", tt.Texts())
	}
	tt.Advance(40 * time.Millisecond)
	tt.Frame()
	coming, _ := tt.Find("还没有评论")
	tt.Advance(time.Second)
	tt.Frame()
	rest, _ := tt.Find("还没有评论")
	if !(coming.X < start.X && rest.X < coming.X) {
		t.Fatalf("the panel does not come in: %.0f, %.0f, %.0f", start.X, coming.X, rest.X)
	}
}

// TestBackToLyrics checks that a page opened from the lyrics, as the album
// link of them does, goes back to them, and that the lyrics closed by hand
// do not come back.
func TestBackToLyrics(t *testing.T) {
	a, _ := newOfflineApp(t, t.TempDir())
	from := a.current()
	a.showLyrics = true
	a.navigate(page{kind: pageAlbum, mid: "m", title: "专辑"})
	if a.showLyrics {
		t.Fatal("the lyrics stay up over the page they go to")
	}
	a.back()
	if a.current().key() != from.key() {
		t.Fatalf("back on %+v, want %+v", a.current(), from)
	}
	if !a.showLyrics {
		t.Fatal("back does not bring the lyrics up again")
	}

	a.closeLyrics()
	a.navigate(page{kind: pageSinger, mid: "s", title: "歌手"})
	a.back()
	if a.showLyrics {
		t.Fatal("the lyrics come back after being closed")
	}
}

// TestHistoryKeyAnimates checks that going back by the mouse's side buttons
// brings the page in from below, as the top bar's button does, not at its
// place at once.
func TestHistoryKeyAnimates(t *testing.T) {
	a, _ := newOfflineApp(t, t.TempDir())
	tt := ui.NewTester(a.view, 1280, 820)
	// 设置 holds what a page shows without asking anything, and 最近播放
	// asks nothing either: nothing else moves as the page comes back.
	a.navigate(page{kind: pageSettings})
	tt.Frame()
	a.navigate(page{kind: pageRecent})
	tt.Frame()
	tt.Advance(time.Second)

	tt.Key(0, ui.KeyBack)
	coming := tt.Image()
	tt.Advance(time.Second)
	rest := tt.Image()
	moved := 0
	for i := 0; i+3 < len(coming.Pix); i += 4 {
		if coming.Pix[i] != rest.Pix[i] || coming.Pix[i+1] != rest.Pix[i+1] || coming.Pix[i+2] != rest.Pix[i+2] {
			moved++
		}
	}
	// A page 16 DIPs below its place moves the pixels of the text it shows,
	// a twentieth of the window; at its place at once it moves none.
	if pixels := len(coming.Pix) / 4; moved < pixels/20 {
		t.Errorf("the page shows at its place %d pixels into %d: it did not come in", moved, pixels)
	}
}

// TestPageSwitchAnimates checks that a page switched to comes in on its own:
// the page is drawn a frame into it, none of the page before
// drawn under it, and it is not at its place at once either.
func TestPageSwitchAnimates(t *testing.T) {
	a, _ := newOfflineApp(t, t.TempDir())
	// 最近播放 of songs and 设置: two pages that show plenty, one of rows and
	// one of text, both without asking anything.
	for i := range 40 {
		a.recent = append(a.recent, qqmusic.Song{Mid: fmt.Sprint(i), Name: fmt.Sprintf("歌曲 %d", i+1), Interval: 240})
	}
	tt := ui.NewTester(a.view, 1280, 820)
	a.navigate(page{kind: pageRecent, title: "最近播放"})
	tt.Frame()
	tt.Advance(time.Second)
	before := tt.Image()

	a.navigate(page{kind: pageSettings})
	tt.Frame()
	tt.Advance(50 * time.Millisecond)
	coming := tt.Image()
	tt.Advance(time.Second)
	rest := tt.Image()

	// Drawn a frame into the switch, and on its way: it is not the page it
	// settles in, and were the page before drawn under it, it would be.
	area := pageArea(rest)
	if shown, drawn := litPixels(coming, area), litPixels(rest, area); drawn == 0 || shown < drawn*3/4 {
		t.Errorf("the page is not drawn a frame into the switch: %d lit pixels, against %d", shown, drawn)
	}
	moved, settled := changedPixels(coming, rest), changedPixels(before, rest)
	if settled == 0 {
		t.Fatal("the pages show alike")
	}
	if moved < settled/10 {
		t.Errorf("the page is at its place at once: %d pixels of %d", moved, settled)
	}
	if changedPixels(coming, before) < settled/2 {
		t.Error("the page before is drawn under the one arriving")
	}
}

// TestSongCardMenu checks that the item chosen from the menu of a song card
// runs: the menu opens from the card's … button, which the pointer leaves as
// it opens, so the button has to be built still for the choice to arrive.
func TestSongCardMenu(t *testing.T) {
	a, _ := newOfflineApp(t, t.TempDir())
	a.home.newSongs = []qqmusic.Song{{Mid: "a", Name: "歌曲 1", Interval: 240, Album: qqmusic.Album{Name: "专辑", Mid: "am"}}}
	a.home.loaded = true
	setQueue(a, episodes("x"), 0)
	tt := ui.NewTester(a.view, 1280, 820)
	tt.Frame()
	tt.Advance(time.Second)
	title, ok := tt.Find("歌曲 1")
	if !ok {
		t.Fatal("no 歌曲 1")
	}
	// The buttons of a card are icons, which no label finds: the one that
	// opens the menu is the last of them, to the right of the title.
	opened := false
	for dx := float32(60); dx >= 8 && !opened; dx -= 4 {
		tt.ClickAt(title.X+title.W+dx, title.Y+title.H/2)
		if opened = len(tt.Menu()) > 0; !opened && a.login.open {
			a.closeLogin() // the heart, not the menu
			tt.Frame()
		}
	}
	if !opened {
		t.Fatal("the menu of the card never opened")
	}
	if err := tt.ChooseMenuItem("添加到播放队列"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if mids, _ := queueState(a); mids != "x a" {
		t.Errorf("queue %q after 添加到播放队列, want %q", mids, "x a")
	}
}

// TestLikedRowSlides checks that the rows below one taken out of a song list
// slide up into their places: they start where they were, and are a row
// higher once it is done, rather than jumping there.
func TestLikedRowSlides(t *testing.T) {
	a, _ := newOfflineApp(t, t.TempDir())
	p := page{kind: pagePlaylist, dirid: 201, title: "我喜欢"}
	l := &songList{}
	a.lists[p.key()] = l
	for i := range 20 {
		l.songs = append(l.songs, qqmusic.Song{Mid: fmt.Sprint(i), Name: fmt.Sprintf("歌曲 %d", i+1), Interval: 240})
	}
	tt := ui.NewTester(a.view, 1280, 820)
	a.navigate(p)
	tt.Frame()
	tt.Advance(time.Second)
	was, ok := tt.Find("歌曲 6")
	if !ok {
		t.Fatal("no 歌曲 6")
	}

	// 歌曲 3 取消喜欢: it went, and the rows below come from where it was.
	l.rows.shifted(2, songRowHeight)
	l.songs = append(l.songs[:2], l.songs[3:]...)
	tt.Frame()
	if now, ok := tt.Find("歌曲 6"); !ok || now.Y < was.Y-2 {
		t.Errorf("the rows jumped up at once: 歌曲 6 at %v, from %v", now.Y, was.Y)
	}
	for range 12 {
		tt.Frame()
		tt.Advance(30 * time.Millisecond)
	}
	if now, ok := tt.Find("歌曲 6"); !ok || now.Y > was.Y-songRowHeight+2 || now.Y < was.Y-songRowHeight-2 {
		t.Errorf("the rows did not slide a row up: 歌曲 6 at %v, from %v", now.Y, was.Y)
	}
}

// TestRowGone checks which row of a list went as it loaded again.
func TestRowGone(t *testing.T) {
	songs := func(n int) []qqmusic.Song {
		s := make([]qqmusic.Song, n)
		for i := range s {
			s[i] = qqmusic.Song{Mid: fmt.Sprint(i)}
		}
		return s
	}
	old := songs(4)
	for _, c := range []struct {
		name string
		now  []qqmusic.Song
		want int
		gone bool
	}{
		{"the first", old[1:], 0, true},
		{"one in the middle", append(append([]qqmusic.Song{}, old[:1]...), old[2:]...), 1, true},
		{"the last", old[:3], 3, true},
		{"none", old, 0, false},
		{"two", old[2:], 0, false},
		{"another list", songs(4), 0, false},
	} {
		if got, gone := rowGone(old, c.now); got != c.want || gone != c.gone {
			t.Errorf("%s: %d, %v, want %d, %v", c.name, got, gone, c.want, c.gone)
		}
	}
}

// TestRowsComeInOneByOne checks that the rows of a song list come in one
// after another, from the top: the bottom rows are still to come while the
// top ones are in, and the page above moves meanwhile.
func TestRowsComeInOneByOne(t *testing.T) {
	a, _ := newOfflineApp(t, t.TempDir())
	for i := range 40 {
		a.recent = append(a.recent, qqmusic.Song{Mid: fmt.Sprint(i), Name: fmt.Sprintf("歌曲 %d", i+1), Interval: 240})
	}
	tt := ui.NewTester(a.view, 1280, 820)
	a.navigate(page{kind: pageSettings})
	tt.Frame()
	tt.Advance(time.Second)

	a.navigate(page{kind: pageRecent, title: "最近播放"})
	tt.Frame()
	tt.Advance(60 * time.Millisecond)
	early := tt.Image()
	tt.Advance(time.Second)
	rest := tt.Image()

	// The rows of the top of the list, and those of its bottom.
	top, bottom := image.Rect(270, 260, 1240, 400), image.Rect(270, 540, 1240, 680)
	if lit := litPixels(rest, top); lit == 0 {
		t.Fatal("the top rows show nothing")
	}
	if lit := litPixels(early, top); lit < litPixels(rest, top)/2 {
		t.Errorf("the top rows are not in at 60 ms: %d lit pixels, against %d", lit, litPixels(rest, top))
	}
	if lit := litPixels(early, bottom); lit > litPixels(rest, bottom)/4 {
		t.Errorf("the bottom rows came in with the top ones: %d lit pixels, against %d", lit, litPixels(rest, bottom))
	}
}

// pageArea is the page area of a frame: between the sidebar and the edge,
// under the title bar, where the pages draw.
func pageArea(img *image.RGBA) image.Rectangle {
	bounds := img.Bounds()
	return image.Rect(bounds.Min.X+260, bounds.Min.Y+topHeight+8, bounds.Max.X-30, bounds.Max.Y-100)
}

// litPixels counts the lit pixels of a frame within r: text and pictures
// stand out of the panel.
func litPixels(img *image.RGBA, r image.Rectangle) int {
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y += 2 {
		for x := r.Min.X; x < r.Max.X; x += 2 {
			c := img.RGBAAt(x, y)
			if 0.299*float64(c.R)+0.587*float64(c.G)+0.114*float64(c.B) > 60 {
				n++
			}
		}
	}
	return n
}

// changedPixels counts the pixels two frames of the page area tell apart.
func changedPixels(a, b *image.RGBA) int {
	r := pageArea(a)
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y += 2 {
		for x := r.Min.X; x < r.Max.X; x += 2 {
			if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
				n++
			}
		}
	}
	return n
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
	// xyz_official, which v1.0.1 wrote, is a source already shown: it counts
	// as chosen, so the run after the update does not ask again.
	want := settings{Volume: 0.5, Quality: qqmusic.High, Mode: playqueue.Shuffle, MineClosed: true, Searches: []string{"晴天"},
		WinX: 10, WinY: 20, WinWidth: 1100, WinHeight: 780, XiaoyuzhouOfficial: true, XiaoyuzhouChosen: true, CacheGB: 2}
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
	if c, _ := newOfflineApp(t, dir); c.set.XiaoyuzhouOfficial || c.set.OldOfficial != nil || !c.set.XiaoyuzhouChosen {
		t.Fatalf("official %v, old %v, chosen %v", c.set.XiaoyuzhouOfficial, c.set.OldOfficial, c.set.XiaoyuzhouChosen)
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
		known       bool
		want, other string
	}{
		{false, "专辑", "节目"},
		{true, "节目", "专辑"},
	} {
		a.podcast.known["show"] = tc.known
		setQueue(a, []qqmusic.Song{{Mid: "e", Name: "第一集", Album: qqmusic.Album{Mid: "show", Name: "日谈公园"}}}, 0)
		tt.Frame()
		tt.Advance(time.Second)
		tt.Frame()
		if !tt.HasText(tc.want) || tt.HasText(tc.other) {
			t.Errorf("known %v: no %q but %q in %q", tc.known, tc.want, tc.other, tt.Texts())
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

// TestSettings opens 设置 from the gear beside the user, which closes the
// user's popup, and sets the song cache there: its limit, kept, and
// clearing it.
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
	tt.Frame()
	if !a.userMenu || !tt.HasText("退出登录") {
		t.Fatalf("the user's popup, open %v", a.userMenu)
	}
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	drain(t, f, 5*time.Second, func() bool { return a.cacheUsed >= 0 && !a.cacheBusy })
	tt.Frame()
	if a.current().kind != pageSettings || a.userMenu {
		t.Fatalf("at %v, the user's popup open %v", a.current().kind, a.userMenu)
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
