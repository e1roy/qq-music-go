package app

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"qqmusic/internal/qqmusic"
	"qqmusic/internal/xiaoyuzhou"
)

// TestPodcasts shows 小宇宙, a podcast's page and a search of podcasts
// from what they would load, and keeps the shows listened to across runs.
func TestPodcasts(t *testing.T) {
	dir := t.TempDir()
	f := &fakePlatform{posts: make(chan func(), 1024)}
	a := newApp(f, dir)
	// No network: the covers do not come.
	a.images = newImages(func(context.Context, string) ([]byte, error) { return nil, errors.New("offline") }, f.Post, a.trim.soon)

	ep := func(mid, name, show string, secs int) qqmusic.Song {
		return qqmusic.Song{ID: 1, Mid: mid, Name: name, Title: name, Interval: secs,
			Album: qqmusic.Album{Mid: show, Name: "节目 " + show}, Singers: []qqmusic.Singer{{Name: "主播 " + show}}}
	}
	pc := &a.podcast
	pc.loaded = true
	pc.area = qqmusic.PodcastArea{Title: "播客专区", Desc: "听见闪闪发光的灵魂", Shelves: []qqmusic.PodcastShelf{
		{Title: "近期热点", Episodes: []qqmusic.Song{ep("e1", "第一集", "s1", 600)}},
		{Title: "明星播客", Hosts: true, Shows: []qqmusic.Show{{ID: 7, Title: "王晰"}}},
		{Title: "热门节目", Shows: []qqmusic.Show{{ID: 8, Mid: "s2", Title: "无乐不谈", Host: "王东", Episodes: 38}}},
	}}
	pc.picks = []qqmusic.Show{{ID: 9, Title: "喷嚏", Host: "喷嚏官方", Intro: "两位女创作者", PlayCount: 173540}}
	pc.known["s1"], pc.known["s2"] = true, true

	// Episodes that played make their shows heard, the last first, once
	// each; songs that are not episodes do not.
	a.addRecent(ep("e1", "第一集", "s1", 600))
	a.hearShow(ep("e1", "第一集", "s1", 600))
	a.addRecent(qqmusic.Song{Mid: "song", Name: "一首歌", Album: qqmusic.Album{Mid: "music"}})
	a.addRecent(ep("e2", "第二集", "s2", 7000))
	a.hearShow(ep("e2", "第二集", "s2", 7000))
	a.hearShow(ep("e1", "第一集", "s1", 600))
	if eps := a.recentEpisodes(8); len(eps) != 2 || eps[0].Mid != "e2" {
		t.Fatalf("recent episodes: %+v", eps)
	}
	b := newApp(f, dir)
	if len(b.podcast.heard) != 2 || b.podcast.heard[0].Mid != "s1" || b.podcast.heard[1].Mid != "s2" || !b.podcast.known["s2"] {
		t.Fatalf("heard: %+v", b.podcast.heard)
	}
	b.clearHeard()
	if c := newApp(f, dir); len(c.podcast.heard) != 0 {
		t.Fatalf("cleared: %+v", c.podcast.heard)
	}

	tt := ui.NewTester(a.view, 1280, 2400)
	// 小宇宙 opens 小宇宙's own podcasts unless QQ Music's are chosen.
	a.set.XiaoyuzhouOfficial = false
	if err := tt.Click("小宇宙"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, s := range []string{"听见闪闪发光的灵魂", "最近收听", "第二集", "推荐节目", "喷嚏", "两位女创作者", "我听过的节目", "近期热点", "明星播客", "王晰", "热门节目", "无乐不谈", "38 集 · 王东"} {
		if !tt.HasText(s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}
	shot(t, tt, "podcasts")

	// A podcast's page: its episodes, the next of which load at their end;
	// here they are all.
	show := page{kind: pageAlbum, mid: "s2", title: "无乐不谈"}
	eps := []qqmusic.Episode{
		{Song: ep("e3", "vol.02 第二期", "s2", 7000), Index: 2, ListenCount: 21148, UploadTime: "2022-06-14"},
		{Song: ep("e4", "vol.01 第一期", "s2", 946), Index: 1, UploadTime: "2022-06-13"},
	}
	a.lists[show.key()] = &songList{
		album: qqmusic.AlbumInfo{Mid: "s2", Name: "无乐不谈", Type: "人声音频"},
		info:  qqmusic.Playlist{Title: "无乐不谈"},
		eps:   eps, songs: qqmusic.EpisodeSongs(eps), total: len(eps),
	}
	a.navigate(show)
	tt.Frame()
	for _, s := range []string{"节目", "2 集", "vol.02 第二期", "2022-06-14", "2.1万", "1:56:40", "15:46"} {
		if !tt.HasText(s) {
			t.Errorf("show: no %q in %q", s, tt.Texts())
		}
	}
	shot(t, tt, "podcast-show")

	// What a search found, and a tab that found none.
	a.navigate(page{kind: pagePodcast, title: "小宇宙"})
	pc.search.query = "日谈"
	pc.search.shows = pager[qqmusic.Show]{items: []qqmusic.Show{{ID: 1, Mid: "rt", Title: "日谈公园", Host: "日谈公园", Episodes: 839}}, pages: 1}
	pc.search.episodes = pager[qqmusic.Song]{pages: 1}
	tt.Frame()
	if !tt.HasText("日谈公园") || !tt.HasText("839 集 · 日谈公园") || tt.HasText("近期热点") {
		t.Errorf("found shows: %q", tt.Texts())
	}
	pc.search.tab = 1
	tt.Frame()
	if !tt.HasText("没有找到相关单集") {
		t.Errorf("found episodes: %q", tt.Texts())
	}

	// Its 清除搜索 goes back to the shelves.
	if err := tt.Click("清除搜索"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if pc.search.query != "" || !tt.HasText("近期热点") {
		t.Fatalf("cleared: %q, %q", pc.search.query, tt.Texts())
	}

	// On 小宇宙 the title bar's box searches its podcasts only, with no
	// suggestions of songs; elsewhere it searches songs again.
	a.api.HTTP = &http.Client{Transport: offline{}}
	pc.search.tab = 0
	if !tt.HasText("搜索播客") {
		t.Errorf("placeholder: %q", tt.Texts())
	}
	a.search.input = "日谈"
	a.search.suggest, a.search.sugFor = []string{"日谈公园 - 歌"}, "日谈"
	if w := a.searchWords(); w != nil {
		t.Errorf("suggested %q on 小宇宙", w)
	}
	a.chooseSearch("日谈")
	if a.current().kind != pagePodcast || pc.search.query != "日谈" || a.search.query != "" || !pc.search.shows.loading {
		t.Fatalf("searched on %+v: podcasts %q, songs %q", a.current(), pc.search.query, a.search.query)
	}
	settle(tt, f, 5*time.Second, func() bool { return !pc.search.shows.loading })
	if pc.search.shows.err == "" || !tt.HasText("重试") {
		t.Errorf("offline search: %q, %q", pc.search.shows.err, tt.Texts())
	}
	a.navigate(page{kind: pageHome})
	if len(a.searchWords()) != 1 {
		t.Errorf("no suggestions off 小宇宙")
	}
}

// offline is a transport with no network, for what a test does not need
// to load.
type offline struct{}

func (offline) RoundTrip(*http.Request) (*http.Response, error) { return nil, errors.New("offline") }

// TestSearchPodcastsTab searches the podcasts beside the songs as a query
// is new, showing the songs first, and the podcasts as their tab.
func TestSearchPodcastsTab(t *testing.T) {
	a, tt, f := newTestApp(t)
	a.api.HTTP = &http.Client{Transport: offline{}}
	a.images = newImages(func(context.Context, string) ([]byte, error) { return nil, errors.New("offline") }, f.Post, a.trim.soon)
	s := &a.search
	a.runSearch("来都来了", 1)
	if s.tab != foundSongs || !s.loading || !s.podcasts.loading || s.singers.loading {
		t.Fatalf("searching: tab %d, songs %v, podcasts %v, singers %v", s.tab, s.loading, s.podcasts.loading, s.singers.loading)
	}
	settle(tt, f, 5*time.Second, func() bool { return !s.loading && !s.podcasts.loading })
	if s.podcasts.err == "" {
		t.Fatalf("offline podcasts: %+v", s.podcasts)
	}

	// The next page of songs keeps them; showing their tab after a failure
	// tries again.
	s.podcasts = pager[qqmusic.Show]{pages: 1, items: []qqmusic.Show{{Mid: "003DKRD82GxqiR", Title: "来都来了 | 听了再走", Episodes: 283}}}
	a.runSearch("来都来了", 2)
	if len(s.podcasts.items) != 1 || s.podcasts.loading {
		t.Fatalf("songs' page 2: %+v", s.podcasts)
	}
	settle(tt, f, 5*time.Second, func() bool { return !s.loading })
	if err := tt.Click("播客"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, want := range []string{"来都来了 | 听了再走", "283 集", "找到 1 个播客"} {
		if !tt.HasText(want) {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
	shot(t, tt, "search-podcasts")

	// A new query forgets them, and searches its own.
	a.runSearch("日谈公园", 1)
	if len(s.podcasts.items) != 0 || !s.podcasts.loading {
		t.Fatalf("new query: %+v", s.podcasts)
	}
}

// TestSearchPodcastsTabBesideXiaoyuzhou shows the podcasts of 小宇宙 first on the
// 播客 tab of a search, beside those of QQ Music's, and counted with them.
func TestSearchPodcastsTabBesideXiaoyuzhou(t *testing.T) {
	a, tt, f := newTestApp(t)
	a.api.HTTP = &http.Client{Transport: offline{}}
	a.images = newImages(func(context.Context, string) ([]byte, error) { return nil, errors.New("offline") }, f.Post, a.trim.soon)
	s := &a.search
	a.runSearch("大医来了", 1)
	settle(tt, f, 5*time.Second, func() bool { return !s.loading && !s.podcasts.loading })

	s.xiaoyuzhouShows = pager[xiaoyuzhou.Podcast]{pages: 1, items: []xiaoyuzhou.Podcast{
		{Pid: "64abcdef", Title: "日谈公园", Author: "李叔", EpisodeCount: 12},
	}}
	s.podcasts = pager[qqmusic.Show]{pages: 1, items: []qqmusic.Show{
		{Mid: "003DKRD82GxqiR", Title: "来都来了 | 听了再走", Episodes: 283},
	}}
	a.showSearchTab(foundPodcasts)
	tt.Frame()

	texts := tt.Texts()
	xzAt, qqAt := slices.Index(texts, "日谈公园"), slices.Index(texts, "来都来了 | 听了再走")
	if xzAt < 0 || qqAt < 0 || xzAt > qqAt {
		t.Fatalf("小宇宙's podcast not first: %q", texts)
	}
	for _, want := range []string{"12 集", "283 集", "找到 2 个播客"} {
		if !tt.HasText(want) {
			t.Errorf("no %q in %q", want, texts)
		}
	}
	shot(t, tt, "search-podcasts-xiaoyuzhou")
}

// TestSearchPodcastsTabEnd: a page of QQ Music's with none of its
// podcasts lets the search go on, and an error of 小宇宙's shows beside
// what QQ Music found.
func TestSearchPodcastsTabEnd(t *testing.T) {
	a, tt, f := newTestApp(t)
	a.api.HTTP = &http.Client{Transport: offline{}}
	a.images = newImages(func(context.Context, string) ([]byte, error) { return nil, errors.New("offline") }, f.Post, a.trim.soon)
	s := &a.search
	a.runSearch("大医来了", 1)
	settle(tt, f, 5*time.Second, func() bool { return !s.loading && !s.podcasts.loading })
	a.showSearchTab(foundPodcasts)

	s.podcasts = pager[qqmusic.Show]{pages: 1, more: true}
	s.xiaoyuzhouShows = pager[xiaoyuzhou.Podcast]{}
	tt.Frame()
	if !tt.HasText("前 1 页没有找到相关播客") || !tt.HasText("继续查找") || tt.HasText("暂无播客") {
		t.Fatalf("none on a page: %q", tt.Texts())
	}

	s.podcasts = pager[qqmusic.Show]{pages: 1, items: []qqmusic.Show{{Mid: "m", Title: "来都来了", Episodes: 3}}}
	s.xiaoyuzhouShows = pager[xiaoyuzhou.Podcast]{err: "小宇宙出错了"}
	tt.Frame()
	if !tt.HasText("来都来了") || !tt.HasText("小宇宙出错了") {
		t.Fatalf("小宇宙's error: %q", tt.Texts())
	}

	// QQ Music's error leaves 小宇宙's more to load.
	s.podcasts = pager[qqmusic.Show]{err: "QQ 音乐出错了"}
	s.xiaoyuzhouShows = pager[xiaoyuzhou.Podcast]{pages: 1, more: true, items: []xiaoyuzhou.Podcast{{Pid: "p", Title: "日谈公园"}}}
	tt.Frame()
	if !tt.HasText("QQ 音乐出错了") || !tt.HasText("加载更多播客") {
		t.Fatalf("QQ Music's error: %q", tt.Texts())
	}
}
