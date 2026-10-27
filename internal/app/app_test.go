package app

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"qqmusic/internal/playqueue"
	"qqmusic/internal/qqmusic"
)

// fakePlatform queues what goroutines post, for the test to run on its
// own goroutine as the main thread.
type fakePlatform struct{ posts chan func() }

func (f *fakePlatform) Post(fn func())                          { f.posts <- fn }
func (f *fakePlatform) WebViewAvailable() bool                  { return true }
func (f *fakePlatform) OpenQQLogin(done func([]*cookie, error)) { go done(nil, errLoginClosed) }
func (f *fakePlatform) OpenXiaoyuzhouLogin(done func(access, refresh string, err error)) {
	go done("", "", errLoginClosed)
}
func (f *fakePlatform) Minimize()         {}
func (f *fakePlatform) ToggleMaximize()   {}
func (f *fakePlatform) Maximized() bool   { return false }
func (f *fakePlatform) Close()            {}
func (f *fakePlatform) OpenFolder(string) {}

func newTestApp(t *testing.T) (*app, *ui.Tester, *fakePlatform) {
	f := &fakePlatform{posts: make(chan func(), 1024)}
	a := newApp(f, "")
	tt := ui.NewTester(a.view, 1280, 820)
	return a, tt, f
}

// settle runs what was posted until cond holds or the time is up.
func settle(tt *ui.Tester, f *fakePlatform, d time.Duration, cond func() bool) bool {
	deadline := time.After(d)
	for {
		tt.Frame()
		if cond() {
			// Let images that started arrive.
			for end := time.After(1500 * time.Millisecond); ; {
				select {
				case fn := <-f.posts:
					fn()
					continue
				case <-end:
				}
				break
			}
			tt.Frame()
			tt.Frame()
			return true
		}
		select {
		case fn := <-f.posts:
			fn()
		case <-time.After(50 * time.Millisecond):
		case <-deadline:
			return false
		}
	}
}

func shot(t *testing.T, tt *ui.Tester, name string) {
	dir := os.Getenv("SHOTS")
	if dir == "" {
		return
	}
	fh, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	// Let what comes in arrive.
	tt.Advance(time.Second)
	tt.Frame()
	png.Encode(fh, tt.Image())
}

// TestPages goes through the pages against the live service.
func TestPages(t *testing.T) {
	if testing.Short() {
		t.Skip("uses the network")
	}
	a, tt, f := newTestApp(t)
	a.enter(a.current())
	if !settle(tt, f, 20*time.Second, func() bool { return a.home.loaded }) {
		t.Fatal("home did not load")
	}
	if a.home.err != "" || len(a.home.recs) == 0 {
		t.Fatalf("home: %q, %d playlists", a.home.err, len(a.home.recs))
	}
	if !tt.HasText("推荐歌单") {
		t.Errorf("home texts: %q", tt.Texts())
	}
	shot(t, tt, "1-home")

	if err := tt.Click("音乐馆"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("排行榜"); err != nil {
		t.Fatal(err)
	}
	settle(tt, f, 20*time.Second, func() bool { return a.charts.loaded })
	if !tt.HasText("巅峰榜") {
		t.Errorf("charts texts: %q", tt.Texts())
	}
	shot(t, tt, "2-charts")

	if err := tt.Click("热歌榜"); err != nil {
		t.Fatal(err)
	}
	pg := a.current()
	if pg.kind != pageToplist {
		t.Fatalf("on %+v after clicking a chart", pg)
	}
	settle(tt, f, 20*time.Second, func() bool { l := a.lists[pg.key()]; return l != nil && !l.loading })
	if l := a.lists[pg.key()]; len(l.songs) == 0 {
		t.Fatalf("chart: %q", l.err)
	}
	shot(t, tt, "3-chart")

	// Back on 音乐馆, the tab is the one left: 排行榜.
	if err := tt.Click("音乐馆"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("歌单广场"); err != nil {
		t.Fatal(err)
	}
	settle(tt, f, 20*time.Second, func() bool { return len(a.square.pls) > 0 })
	shot(t, tt, "4-square")

	// The search box, not 搜索 in the sidebar before it.
	if err := tt.Click("搜索歌曲"); err != nil {
		t.Fatal(err)
	}
	tt.Type("周杰伦")
	tt.Key(0, ui.KeyEnter)
	settle(tt, f, 20*time.Second, func() bool { return a.search.query != "" && !a.search.loading })
	if len(a.search.songs) == 0 {
		t.Fatalf("search %q: %q", a.search.query, a.search.err)
	}
	if !tt.HasText("“周杰伦” 的搜索结果") {
		t.Errorf("search texts: %q", tt.Texts())
	}
	// It goes straight to the singer, above the tabs.
	if d := a.search.direct; d == nil || d.Kind != qqmusic.DirectSinger || d.Name != "周杰伦" {
		t.Errorf("search direct: %+v", a.search.direct)
	} else if !tt.HasText("歌手: 周杰伦") {
		t.Errorf("search direct texts: %q", tt.Texts())
	}
	shot(t, tt, "5-search")

	// The playlists found are in a tab of their own, which loads as it
	// shows.
	if a.search.pls.pages != 0 || a.search.pls.loading {
		t.Error("playlists load before their tab shows")
	}
	if err := tt.Click("歌单"); err != nil {
		t.Fatal(err)
	}
	settle(tt, f, 20*time.Second, func() bool { return a.search.pls.pages > 0 || a.search.pls.err != "" })
	if len(a.search.pls.items) == 0 {
		t.Fatalf("search playlists: %q", a.search.pls.err)
	}
	settle(tt, f, 5*time.Second, func() bool { return tt.HasText(a.search.pls.items[0].Title) })
	if !tt.HasText(a.search.pls.items[0].Title) {
		t.Errorf("search playlists texts: %q", tt.Texts())
	}
	shot(t, tt, "5b-search-playlists")
	found := a.search.pls.items[0]
	if err := tt.Click(found.Title); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if pg := a.current(); pg.kind != pagePlaylist || pg.id != found.ID {
		t.Fatalf("on %+v after clicking playlist %d %q", pg, found.ID, found.Title)
	}
	settle(tt, f, 20*time.Second, func() bool { l := a.lists[a.current().key()]; return l != nil && !l.loading })
	if l := a.lists[a.current().key()]; len(l.songs) == 0 {
		t.Fatalf("searched playlist: %q", l.err)
	}
	a.back()
	tt.Frame()

	// The singers found are in a tab of their own, and open their pages.
	if err := tt.Click("歌手"); err != nil {
		t.Fatal(err)
	}
	settle(tt, f, 20*time.Second, func() bool { return a.search.singers.pages > 0 || a.search.singers.err != "" })
	if len(a.search.singers.items) == 0 {
		t.Fatalf("search singers: %q", a.search.singers.err)
	}
	sg := a.search.singers.items[0]
	settle(tt, f, 5*time.Second, func() bool { return tt.HasText(sg.Name) })
	if !tt.HasText(sg.Name) || !tt.HasText(fmt.Sprintf("共找到 %d 位歌手", a.search.singers.total)) {
		t.Errorf("search singers texts: %q", tt.Texts())
	}
	shot(t, tt, "5c-search-singers")
	if err := tt.Click(sg.Name); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if pg := a.current(); pg.kind != pageSinger || pg.mid != sg.Mid {
		t.Fatalf("on %+v after clicking singer %+v", pg, sg)
	}
	a.back()
	tt.Frame()

	// Playing without a login asks for one, with a QR code.
	a.playList(a.search.songs, 0)
	if !a.login.open {
		t.Fatal("no login dialog when playing logged out")
	}
	settle(tt, f, 20*time.Second, func() bool { return a.login.bmp != nil || a.login.err != "" })
	if a.login.err != "" {
		t.Fatal(a.login.err)
	}
	shot(t, tt, "6-login-qq")
	if err := tt.Click("微信扫码"); err != nil {
		t.Fatal(err)
	}
	settle(tt, f, 20*time.Second, func() bool { return a.login.bmp != nil || a.login.err != "" })
	if a.login.err != "" {
		t.Fatal(a.login.err)
	}
	shot(t, tt, "7-login-wx")
	if err := tt.Click("QQ 账号"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("打开 QQ 登录窗口") {
		t.Errorf("account tab texts: %q", tt.Texts())
	}
	shot(t, tt, "8-login-account")
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.login.open {
		t.Error("Escape left the login dialog open")
	}

	// The now-playing page and the queue, with a song in the queue.
	a.queue = playqueue.Restore(a.search.songs, 0, false)
	a.loadLyrics(a.search.songs[0])
	a.showLyrics, a.showQueue = true, true
	settle(tt, f, 20*time.Second, func() bool { return a.lyrics != nil })
	if len(a.lyrics) < 5 {
		t.Errorf("lyrics: %d lines", len(a.lyrics))
	}
	shot(t, tt, "9-now-playing")
}

func TestLRC(t *testing.T) {
	lines := qqmusic.ParseLRC("[ti:x]\n[00:01.50]一\n[00:03.00][00:10.00]二\n[00:05.123]\n", "[00:01.50]one\n[00:03.00]//\n")
	if len(lines) != 3 || lines[0].Text != "一" || lines[0].Trans != "one" || lines[1].Trans != "" || lines[2].At != 10*time.Second {
		t.Fatalf("%+v", lines)
	}
	if qqmusic.LineAt(lines, 4*time.Second) != 1 || qqmusic.LineAt(lines, 0) != -1 {
		t.Error("LineAt")
	}
}

// TestModeMenu chooses a play mode from the menu above its button.
func TestModeMenu(t *testing.T) {
	a, tt, _ := newTestApp(t)
	a.set.Mode = playqueue.Loop
	tt.Frame()
	if tt.HasText("单曲循环") {
		t.Fatal("the menu shows before a click")
	}
	if err := tt.Click("播放模式"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for m := range playqueue.Mode(4) {
		if !tt.HasText(m.String()) {
			t.Errorf("no %s in %q", m, tt.Texts())
		}
	}
	shot(t, tt, "10-mode-menu")
	if err := tt.Click("单曲循环"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.set.Mode != playqueue.One || a.modeOpen {
		t.Errorf("mode %s, open %v", a.set.Mode, a.modeOpen)
	}
}

// TestQualityMenu chooses a quality from the panel above its badge.
func TestQualityMenu(t *testing.T) {
	a, tt, _ := newTestApp(t)
	if a.set.Quality != qqmusic.Best {
		t.Fatalf("default quality %v", a.set.Quality)
	}
	a.queue = playqueue.Restore([]qqmusic.Song{{Mid: "x", Name: "晴天", File: qqmusic.File{Size128: 4317292, Size320: 10792943, SizeFlac: 55397039}}}, 0, false)
	tt.Frame()
	if tt.HasText("HQ 320k") {
		t.Fatal("the menu shows before a click")
	}
	if err := tt.Click("音质"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, want := range []string{"最高音质", "无损 FLAC", "HQ 320k", "标准 128k", "52.8 MB", "10.3 MB", "4.1 MB"} {
		if !tt.HasText(want) {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
	shot(t, tt, "11-quality-menu")
	if err := tt.Click("HQ 320k"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.set.Quality != qqmusic.High || a.qualityOpen {
		t.Errorf("quality %v after choosing HQ, open %v", a.set.Quality, a.qualityOpen)
	}
}

// TestSearchSinger goes from a song found to its singer's page, against
// the live service.
func TestSearchSinger(t *testing.T) {
	if testing.Short() {
		t.Skip("uses the network")
	}
	a, tt, f := newTestApp(t)
	a.search.input = "晴天"
	a.search.tab = foundPlaylists
	a.runSearch("晴天", 1)
	settle(tt, f, 20*time.Second, func() bool { return !a.search.loading && !a.search.pls.loading })
	if len(a.search.songs) == 0 {
		t.Fatalf("search: %q", a.search.err)
	}

	// Pages of playlists may be short or overlap: none shows twice.
	for range 3 {
		a.searchPlaylists()
		settle(tt, f, 20*time.Second, func() bool { return !a.search.pls.loading })
	}
	seen := map[int64]bool{}
	for _, p := range a.search.pls.items {
		if seen[p.ID] {
			t.Fatalf("playlist %d %q twice", p.ID, p.Title)
		}
		seen[p.ID] = true
	}
	if a.search.pls.pages != 4 {
		t.Errorf("playlists at page %d", a.search.pls.pages)
	}

	// Turning the pages of the songs, back to the first, keeps the
	// playlists found and how many songs there are.
	pls, total := len(a.search.pls.items), a.search.total
	for _, pg := range []int{2, 1} {
		a.runSearch("晴天", pg)
		if a.search.total != total || len(a.search.pls.items) != pls || a.search.pls.loading {
			t.Fatalf("page %d: %d songs, %d playlists, loading %v", pg, a.search.total, len(a.search.pls.items), a.search.pls.loading)
		}
		settle(tt, f, 20*time.Second, func() bool { return !a.search.loading })
	}

	// The VIP mark is for those who are no members.
	a.showSearchTab(foundSongs)
	a.user.VIP = false
	tt.Frame()
	if !tt.HasText("VIP") {
		t.Error("no VIP mark for a non-member")
	}
	a.user.VIP = true
	tt.Frame()
	if tt.HasText("VIP") {
		t.Error("VIP mark for a member")
	}

	sg := a.search.songs[0].Singers[0]
	if err := tt.Click(sg.Name); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	pg := a.current()
	if pg.kind != pageSinger || pg.mid != sg.Mid {
		t.Fatalf("on %+v after clicking singer %+v", pg, sg)
	}
	settle(tt, f, 20*time.Second, func() bool { l := a.lists[pg.key()]; return l != nil && !l.loading })
	l := a.lists[pg.key()]
	if len(l.songs) != singerPage || l.total <= singerPage || l.info.Title != sg.Name {
		t.Fatalf("singer: %d of %d songs, %q, err %q", len(l.songs), l.total, l.info.Title, l.err)
	}
	shot(t, tt, "10-singer")

	// Reaching the end loads the next songs.
	l.list.ScrollToEnd()
	settle(tt, f, 20*time.Second, func() bool { return len(l.songs) > singerPage && !l.loading })
	if len(l.songs) != 2*singerPage {
		t.Fatalf("after the end: %d songs, err %q", len(l.songs), l.err)
	}

	// 最新 loads the songs again, newest first.
	popular := l.songs[0].Mid
	l.list.ScrollTo(0, ui.Start)
	tt.Frame()
	tt.Frame()
	if err := tt.Click("最新"); err != nil {
		t.Fatal(err)
	}
	// As they load again, how many songs there are still shows.
	tt.Frame()
	if songs := fmt.Sprintf("歌曲 %d", l.total); !l.loading || !tt.HasText(songs) {
		t.Fatalf("loading %v, no %q in %q", l.loading, songs, tt.Texts())
	}
	settle(tt, f, 20*time.Second, func() bool { return len(l.songs) > 0 && !l.loading })
	if l.order != qqmusic.Latest || len(l.songs) != singerPage || l.songs[0].Mid == popular {
		t.Fatalf("latest: order %d, %d songs, first %q, err %q", l.order, len(l.songs), l.songs[0].Name, l.err)
	}
	shot(t, tt, "11-singer-latest")

	// The albums, a tab away, and an album's page.
	settle(tt, f, 20*time.Second, func() bool { return l.albums.loaded })
	if len(l.albums.items) == 0 || l.albums.total < len(l.albums.items) {
		t.Fatalf("albums: %d of %d, err %q", len(l.albums.items), l.albums.total, l.albums.err)
	}
	if err := tt.Click(fmt.Sprintf("专辑 %d", l.albums.total)); err != nil {
		t.Fatal(err)
	}
	// Only the rows below the header change, as they come in.
	for range 3 {
		tt.Frame()
		if !tt.HasText(l.info.Title) || !tt.HasText("最新") {
			t.Fatalf("the header went with the tab: %q", tt.Texts())
		}
	}
	settle(tt, f, 5*time.Second, func() bool { return tt.HasText(l.albums.items[0].Name) })
	shot(t, tt, "12-singer-albums")

	// Sorting the albums shown loads them again, without the rows built
	// for the albums of before.
	latest := l.albums.items[0].Mid
	if err := tt.Click("热门"); err != nil {
		t.Fatal(err)
	}
	settle(tt, f, 20*time.Second, func() bool { return l.albums.loaded && len(l.songs) > 0 && !l.loading })
	if l.order != qqmusic.Popular || len(l.albums.items) == 0 || l.albums.items[0].Mid == latest {
		t.Fatalf("popular albums: order %d, %d albums, err %q", l.order, len(l.albums.items), l.albums.err)
	}
	alb := l.albums.items[0]
	if err := tt.Click(alb.Name); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	pg = a.current()
	if pg.kind != pageAlbum || pg.mid != alb.Mid {
		t.Fatalf("on %+v after clicking album %+v", pg, alb)
	}
	settle(tt, f, 20*time.Second, func() bool { l := a.lists[pg.key()]; return l != nil && !l.loading })
	al := a.lists[pg.key()]
	if len(al.songs) == 0 || al.album.Name != alb.Name || len(al.album.Singers) == 0 {
		t.Fatalf("album: %d songs, %+v, err %q", len(al.songs), al.album, al.err)
	}
	shot(t, tt, "13-album")

	// Back on the singer's page, it is as it was left.
	a.back()
	tt.Frame()
	if l.tab != tabAlbums || l.order != qqmusic.Popular {
		t.Fatalf("back on the singer: tab %d, order %d", l.tab, l.order)
	}
}

func TestDisplayName(t *testing.T) {
	for _, c := range []struct {
		s    qqmusic.Song
		want string
	}{
		{qqmusic.Song{Name: "余红", Title: "余红", Subtitle: "《余红旧事》影视剧主题曲"}, "余红 (《余红旧事》影视剧主题曲)"},
		{qqmusic.Song{Name: "余红", Title: "余红 (伴奏)"}, "余红 (伴奏)"},
		{qqmusic.Song{Name: "晴天", Title: "其他"}, "晴天"},
		{qqmusic.Song{Name: "晴天"}, "晴天"},
	} {
		if got := c.s.DisplayName(); got != c.want {
			t.Errorf("%+v: %q, want %q", c.s, got, c.want)
		}
	}
}

// TestRecent keeps the songs played last, once each, across runs, with
// the folded sidebar sections.
func TestRecent(t *testing.T) {
	dir := t.TempDir()
	f := &fakePlatform{posts: make(chan func(), 1024)}
	a := newApp(f, dir)
	for i := range recentMax + 10 {
		a.addRecent(qqmusic.Song{ID: 1, Mid: fmt.Sprint(i), Name: fmt.Sprint("song ", i)})
	}
	a.addRecent(qqmusic.Song{Mid: "50"})
	a.set.FavClosed = true
	a.saveSettings()
	a.rememberWindow(120, 90, 1100, 780, true)

	b := newApp(f, dir)
	if len(b.recent) != recentMax {
		t.Fatalf("got %d songs, want %d", len(b.recent), recentMax)
	}
	if b.recent[0].Mid != "50" || b.recent[1].Mid != fmt.Sprint(recentMax+9) {
		t.Fatalf("order: %s, %s", b.recent[0].Mid, b.recent[1].Mid)
	}
	seen := map[string]bool{}
	for _, s := range b.recent {
		if seen[s.Mid] {
			t.Fatalf("%s twice", s.Mid)
		}
		seen[s.Mid] = true
	}
	if !b.set.FavClosed || b.set.MineClosed {
		t.Fatalf("sidebar: %+v", b.set)
	}
	if b.set.WinX != 120 || b.set.WinY != 90 || b.set.WinWidth != 1100 ||
		b.set.WinHeight != 780 || !b.set.WinMax {
		t.Fatalf("window: %+v", b.set)
	}
	if s, ok := b.nowPlaying(); !ok || s.Mid != "50" || b.player.Playing() {
		t.Fatalf("now playing: %+v %v", s, ok)
	}

	// The queue comes back whole, at the song it was on.
	b.queue = playqueue.Restore([]qqmusic.Song{{Mid: "a"}, {Mid: "b"}, {Mid: "c"}}, 1, false)
	b.enqueue(qqmusic.Song{Mid: "d"})
	c := newApp(f, dir)
	if songs := c.queue.Songs(); len(songs) != 4 || songs[3].Mid != "d" || c.queue.Cur() != 1 {
		t.Fatalf("queue: %d songs at %d", len(songs), c.queue.Cur())
	}
	c.clearQueue()
	if d := newApp(f, dir); len(d.queue.Songs()) != 1 || d.queue.Songs()[0].Mid != "50" {
		t.Fatalf("cleared queue: %d songs", len(d.queue.Songs()))
	}

	tt := ui.NewTester(b.view, 1280, 820)
	b.navigate(page{kind: pageRecent, title: "最近播放"})
	tt.Frame()
	shot(t, tt, "recent")
}

// TestSearchHistory keeps the searches made last, once each, newest first,
// across runs, and shows them below the box while it is focused.
func TestSearchHistory(t *testing.T) {
	dir := t.TempDir()
	f := &fakePlatform{posts: make(chan func(), 1024)}
	a := newApp(f, dir)
	a.rememberSearch("晴天")
	a.rememberSearch("周杰伦")
	a.rememberSearch("晴天")
	if len(a.set.Searches) != 2 || a.set.Searches[0] != "晴天" {
		t.Fatalf("history: %q", a.set.Searches)
	}
	for i := range searchHistoryMax + 5 {
		a.rememberSearch(fmt.Sprint("query ", i))
	}
	if len(a.set.Searches) != searchHistoryMax {
		t.Fatalf("got %d searches, want %d", len(a.set.Searches), searchHistoryMax)
	}
	newest := fmt.Sprint("query ", searchHistoryMax+4)
	if a.set.Searches[0] != newest {
		t.Fatalf("newest first: %q", a.set.Searches[0])
	}

	b := newApp(f, dir)
	if len(b.set.Searches) != searchHistoryMax || b.set.Searches[0] != newest {
		t.Fatalf("history after a run: %q", b.set.Searches)
	}
	b.clearSearches()
	if c := newApp(f, dir); len(c.set.Searches) != 0 {
		t.Fatalf("cleared history: %q", c.set.Searches)
	}

	b.rememberSearch("晴天")
	b.rememberSearch("周杰伦")
	tt := ui.NewTester(b.view, 1280, 820)
	if err := tt.Click("搜索歌曲"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !b.search.open {
		t.Error("focusing the box did not open the history")
	}
	for _, want := range []string{"搜索历史", "周杰伦", "晴天"} {
		if !tt.HasText(want) {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
	shot(t, tt, "search-history")

	// Typed text shows the search of it and what the service says it may
	// become, not the searches made before.
	b.rememberSearch("稻香")
	b.search.input = "晴"
	b.search.suggest, b.search.sugFor = []string{"晴空", "晴天 - 周杰伦"}, "晴"
	tt.Frame()
	for _, want := range []string{"搜索 “晴”", "晴空", "晴天 - 周杰伦"} {
		if !tt.HasText(want) {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
	for _, no := range []string{"搜索历史", "稻香"} {
		if tt.HasText(no) {
			t.Errorf("%q while the box is typed in: %q", no, tt.Texts())
		}
	}
	shot(t, tt, "search-typing")

	// The arrows walk what the text may become, and stop at either end.
	for _, step := range []struct {
		key  ui.Key
		want int
	}{
		{ui.KeyDown, 0},
		{ui.KeyDown, 1},
		{ui.KeyDown, 1},
		{ui.KeyUp, 0},
		{ui.KeyUp, 0},
	} {
		tt.Key(0, step.key)
		tt.Frame()
		if b.search.sel != step.want {
			t.Fatalf("%v: chose %d, want %d", step.key, b.search.sel, step.want)
		}
	}
	shot(t, tt, "search-typing-sel")

	// Escape closes the menu, and a press in the box opens it again.
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if b.search.open {
		t.Error("Escape did not close the menu")
	}
	if err := tt.Click("搜索歌曲"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !b.search.open {
		t.Error("a press in the box did not open the menu")
	}

	// A press on nothing outside takes the focus from the box for good,
	// though MyGo gives it back as the menu goes.
	if err := tt.Click("推荐歌单"); err != nil {
		t.Fatal(err)
	}
	for range 4 {
		tt.Frame()
	}
	if b.search.open || b.search.focused {
		t.Errorf("a press outside left the box focused (%v) or the menu open (%v)", b.search.focused, b.search.open)
	}
	if err := tt.Click("搜索歌曲"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !b.search.open || !b.search.focused {
		t.Error("a press in the box after did not focus it")
	}
}

func TestQueueClose(t *testing.T) {
	a, tt, _ := newTestApp(t)
	a.showQueue = true
	tt.Frame()
	if !tt.HasText("队列是空的") {
		t.Fatal("the queue did not open")
	}
	// A press outside closes it, and it slides out before it is gone.
	if err := tt.Click("推荐歌单"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.showQueue {
		t.Fatal("a click outside did not close the queue")
	}
	tt.Advance(time.Second)
	tt.Frame()
	if tt.HasText("队列是空的") {
		t.Error("the queue is still built after it went")
	}
}

// TestCleanShownotes: the HTML of the shownotes becomes paragraphs and
// list items, with the entities decoded.
func TestCleanShownotes(t *testing.T) {
	in := `<p>第一段<br>换行</p><ul><li>甲</li><li>乙</li></ul>&amp;`
	want := "第一段\n换行\n\n· 甲\n· 乙\n\n&"
	if got := cleanShownotes(in); got != want {
		t.Fatalf("cleanShownotes:\n%q\nwant\n%q", got, want)
	}
}

// TestEpisodeNotes draws a 小宇宙 episode's notes in place of the lyrics,
// which it has none of, and its comments beside them.
func TestEpisodeNotes(t *testing.T) {
	a, tt, _ := newTestApp(t)
	a.queue = playqueue.Restore([]qqmusic.Song{{
		Mid: "e1", Name: "想当纯过日子人", Source: xiaoyuzhouSource,
	}}, 0, false)
	a.notes = episodeNotes{eid: "e1", loaded: true, paragraphs: []string{
		"第一段：这是单集的简介。",
		"第二段：还有 shownotes。",
	}}
	a.comments.eid, a.comments.total = "e1", 120
	a.comments.lists[qqmusic.HotComments] = commentList{loaded: true, items: []qqmusic.Comment{
		{ID: "c1", Nick: "听众", Content: "讲得真好", Likes: 7, Time: time.Now().Add(-time.Hour)},
	}}
	a.showLyrics = true
	tt.Frame()
	for _, want := range []string{"单集简介", "第一段：这是单集的简介。", "第二段：还有 shownotes。", "评论", "120"} {
		if !tt.HasText(want) {
			t.Fatalf("no %q: %q", want, tt.Texts())
		}
	}
	shot(t, tt, "episode-notes")
	if err := tt.Click("评论"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if !tt.HasText("讲得真好") || !tt.HasText("120 条评论") {
		t.Fatalf("no episode comments: %q", tt.Texts())
	}
}

// TestCopyNowPlaying takes the name and the lyrics of the song playing to
// the clipboard: the name by selecting it or from its menu, a line or all of
// them from the lyrics' menus, which leave a press on a line to seeking.
func TestCopyNowPlaying(t *testing.T) {
	a, tt, _ := newTestApp(t)
	const name = "Training Season (London Sessions)"
	const line, trans = "And baby I don't need to learn my lesson twice", "宝贝 我不需要重蹈覆辙"
	a.queue = playqueue.Restore([]qqmusic.Song{{Mid: "copy", Name: name}}, 0, false)
	a.lyrics, a.lyricsFor = []qqmusic.LyricLine{
		{At: time.Second, Text: "Play fair", Trans: "彼此坦诚吧"},
		{At: 2 * time.Second, Text: line, Trans: trans},
	}, "copy"
	a.showLyrics = true
	tt.Frame()

	// The name takes a selection, which Ctrl+C copies.
	if err := tt.Click(name); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(ui.Cmd, ui.KeyC)
	if got := tt.Clipboard(); got != name {
		t.Errorf("the selected name copied %q", got)
	}

	// Its menu copies it whole, in one go.
	if err := tt.RightClick(name); err != nil {
		t.Fatal(err)
	}
	if got := tt.Menu(); !slices.Contains(got, "复制歌名") {
		t.Fatalf("the name's menu is %q", got)
	}
	if err := tt.ChooseMenuItem("复制歌名"); err != nil {
		t.Fatal(err)
	}
	if got := tt.Clipboard(); got != name {
		t.Errorf("the name's menu copied %q", got)
	}

	// A line, with its translation under it, and all the lyrics.
	for _, step := range []struct {
		item, want string
	}{
		{"复制此句", line + "\n" + trans},
		{"复制全部歌词", "Play fair\n彼此坦诚吧\n" + line + "\n" + trans},
	} {
		if err := tt.RightClick(line); err != nil {
			t.Fatal(err)
		}
		if got := tt.Menu(); !slices.Contains(got, step.item) {
			t.Fatalf("the lyrics' menu is %q", got)
		}
		if err := tt.ChooseMenuItem(step.item); err != nil {
			t.Fatal(err)
		}
		if got := tt.Clipboard(); got != step.want {
			t.Errorf("%s copied %q, want %q", step.item, got, step.want)
		}
	}

	// A left click still seeks, which the copy took no room from.
	a.lyricHold = time.Now()
	if err := tt.Click(line); err != nil {
		t.Fatal(err)
	}
	if !a.lyricHold.IsZero() {
		t.Error("a press on a line no longer seeks")
	}
}

// TestComments opens the comments beside the lyrics, in place of the
// queue, without the network: the comments are there already.
func TestComments(t *testing.T) {
	a, tt, _ := newTestApp(t)
	a.queue = playqueue.Restore([]qqmusic.Song{{ID: 97773, Mid: "0039MnYb0qxYhV", Name: "晴天"}}, 0, false)
	a.lyrics, a.lyricsFor = []qqmusic.LyricLine{{At: -1, Text: "故事的小黄花"}}, "0039MnYb0qxYhV"
	a.comments.song, a.comments.total = 97773, 339387
	a.comments.lists[qqmusic.HotComments] = commentList{loaded: true, items: []qqmusic.Comment{
		{ID: "1", Nick: "十九八七", Content: "等到专辑的那天也许我会比较好一点", Likes: 22362, Replies: 165,
			Time: time.Now().Add(-3 * time.Hour), Location: "广东", Reply: &qqmusic.CommentReply{Nick: "张宝珠", Content: "冷落了你"}},
	}}
	a.toggleLyrics()
	tt.Frame()
	if !tt.HasText("评论") || !tt.HasText("33.9万") {
		t.Fatalf("no comments tab: %q", tt.Texts())
	}
	name, _ := tt.Find("晴天")
	if err := tt.Click("评论"); err != nil {
		t.Fatal(err)
	}
	// The panel slides in, as the click brings it, not at its place at once.
	start, _ := tt.Find("33.9万 条评论")
	tt.Advance(40 * time.Millisecond)
	tt.Frame()
	coming, _ := tt.Find("33.9万 条评论")
	tt.Advance(time.Second)
	tt.Frame()
	rest, _ := tt.Find("33.9万 条评论")
	if !(coming.X < start.X && rest.X < coming.X) {
		t.Errorf("the panel does not come in on a click: %.0f, %.0f, %.0f", start.X, coming.X, rest.X)
	}
	// The cover and its name stay where they were.
	if moved, _ := tt.Find("晴天"); moved != name {
		t.Errorf("the name moved from %+v to %+v as the comments opened", name, moved)
	}
	for _, s := range []string{"33.9万 条评论", "等到专辑的那天也许我会比较好一点", "张宝珠：冷落了你", "3小时前 · 广东", "2.2万"} {
		if !tt.HasText(s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}
	shot(t, tt, "comments")
	// They take the lyrics' place, which come back as they close.
	if tt.HasText("故事的小黄花") {
		t.Error("the lyrics show beside the comments")
	}
	a.toggleComments()
	tt.Frame()
	if !tt.HasText("故事的小黄花") {
		t.Error("the lyrics did not come back as the comments closed")
	}
	a.toggleComments()

	// The queue takes its place.
	a.toggleQueue()
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if a.comments.open || tt.HasText("33.9万 条评论") {
		t.Error("the queue did not close the comments")
	}
	if tt.HasText("故事的小黄花") {
		t.Error("the lyrics show beside the queue")
	}
	a.toggleComments()
	if a.showQueue {
		t.Error("the comments did not close the queue")
	}
	// They go with the lyrics.
	a.toggleLyrics()
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if tt.HasText("33.9万 条评论") {
		t.Error("the comments stayed without the lyrics")
	}
	// The lyrics open again on themselves, not on the comments left open.
	a.toggleLyrics()
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if a.comments.open || tt.HasText("33.9万 条评论") || !tt.HasText("故事的小黄花") {
		t.Error("the lyrics opened again on the comments")
	}
}
