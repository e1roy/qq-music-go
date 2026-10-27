package app

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"qqmusic/internal/xyz"
)

// TestXyz shows 小宇宙 beta from what it would load, logged out and in, a
// podcast's page, and plays an episode, which needs no login of QQ Music.
func TestXyz(t *testing.T) {
	dir := t.TempDir()
	f := &fakePlatform{posts: make(chan func(), 1024)}
	a := newApp(f, dir)
	a.images = newImages(func(context.Context, string) ([]byte, error) { return nil, errors.New("offline") }, f.Post, a.trim.soon)
	tt := ui.NewTester(a.view, 1280, 1600)

	// Logged out, it asks for the QR code; failing, it says so.
	x := &a.xyz
	x.qrErr = "获取二维码失败：离线"
	if err := tt.Click("小宇宙"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("切换到小宇宙官方"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.current().kind != pageXyz || !a.set.XyzOfficial {
		t.Fatalf("switched to %v, official %v", a.current().kind, a.set.XyzOfficial)
	}
	for _, s := range []string{"官方", "登录小宇宙", "二维码获取失败", "刷新二维码", "获取二维码失败：离线", "手机号登录"} {
		if !tt.HasText(s) {
			t.Errorf("logged out: no %q in %q", s, tt.Texts())
		}
	}

	// Logged in, what it loaded shows: nothing more loads.
	x.api = xyz.New(xyz.Credential{AccessToken: "token", RefreshToken: "refresh"})
	x.user = xyz.User{Nickname: "听众"}
	pod := xyz.Podcast{Pid: "p1", Title: "知行小酒馆", Author: "有知有行", EpisodeCount: 274, SubscriptionCount: 2159868}
	eps := []xyz.Episode{
		{Eid: "e1", Pid: "p1", Title: "E255 越努力越幸运", Duration: 4167, PlayCount: 202684, PayType: "FREE",
			PubDate: time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC), Podcast: &pod},
		{Eid: "e2", Pid: "p1", Title: "E254 意外之财", Duration: 7466, IsPrivateMedia: true, PayType: "PAY_EPISODE", Podcast: &pod},
	}
	eps[0].Media.Source.URL = "http://127.0.0.1:1/e1.m4a"
	x.tops[0] = xyzTop{list: xyz.TopList{Title: "最热榜", Information: "每日 8 点更新", Episodes: eps}, loaded: true}
	x.inbox = xyzList[xyz.Episode]{items: eps[:1], loaded: true}
	x.subs = xyzList[xyz.Podcast]{items: []xyz.Podcast{pod}, loaded: true}

	// 推荐 first: the categories and the sections of the app's front page.
	x.feed.loaded = true
	x.feed.cats = []xyz.Category{{ID: "c1", Name: "商业"}, {ID: "c2", Name: "科技"}}
	for _, n := range strings.Fields("自我成长 社会 故事奇谈 人文 阅读 趣味闲谈 医疗健康 影视娱乐 文旅 治愈陪伴 职业探索 饮食 艺术 体育运动 兴趣生活 音乐 动漫游戏") {
		x.feed.cats = append(x.feed.cats, xyz.Category{ID: n, Name: n})
	}
	long := strings.Repeat("听这期最有感触的还是「时间贫困」。作为 80 后，我们这代人一直在努力养家。", 3)
	x.feed.sections = []xyz.FeedSection{
		{Title: "为你推荐", Items: []xyz.FeedItem{{Episode: &eps[0], Note: "小米造车背后的人事江湖"}, {Episode: &eps[1], Note: "评论数 99+"}, {Episode: &eps[0], Note: "进入 26 年 8 月锋芒榜"}}},
		{Title: "为你精选的节目", Items: []xyz.FeedItem{{Podcast: &pod, Note: "因为你喜欢三五环"}}},
		{Title: "TA们的喜欢", Items: []xyz.FeedItem{{Episode: &eps[1], Note: "太有趣的一期", By: "本鹤"}, {Episode: &eps[0], Note: long, By: "-Rhaenyra"}}},
	}
	tt.Frame()
	for _, s := range []string{"推荐", "分类探索", "商业", "科技", "为你推荐", "小米造车背后的人事江湖", "为你精选的节目", "因为你喜欢三五环", "TA们的喜欢", "“太有趣的一期” —— 本鹤", "动漫游戏"} {
		if !tt.HasText(s) {
			t.Errorf("feed: no %q in %q", s, tt.Texts())
		}
	}
	shot(t, tt, "xyz-feed")

	// A category's page: its tabs, its podcasts with their newest episode.
	x.cats["c1"] = &xyzCategory{
		tabs:  []xyz.CategoryTab{{ID: "ALL", Name: "全部"}, {ID: "HOT", Name: "近期热门"}},
		lists: []xyzList[xyz.FeedItem]{{items: []xyz.FeedItem{{Podcast: &pod, Episode: &eps[0]}}, loaded: true}, {}},
	}
	if err := tt.Click("商业"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, s := range []string{"商业", "分类", "全部", "近期热门", "知行小酒馆", "最新：E255 越努力越幸运"} {
		if !tt.HasText(s) {
			t.Errorf("category: no %q in %q", s, tt.Texts())
		}
	}
	shot(t, tt, "xyz-category")
	a.back()
	tt.Frame()

	if err := tt.Click("发现"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, s := range []string{"听众", "退出", "发现", "最热榜", "锋芒榜", "每日 8 点更新", "E255 越努力越幸运", "知行小酒馆", "2026-10-23", "20.3万", "1:09:27", "付费"} {
		if !tt.HasText(s) {
			t.Errorf("charts: no %q in %q", s, tt.Texts())
		}
	}
	shot(t, tt, "xyz")
	if err := tt.Click("我的订阅"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("有知有行 · 274 集") {
		t.Errorf("subscriptions: %q", tt.Texts())
	}

	// 编辑精选: the picks of each day, with what a listener said.
	pick := xyz.Pick{Episode: eps[0]}
	pick.Comment.Text, pick.Comment.Author.Nickname = "我一个70岁的老太太听了这一期，\n想了很多。", "子期听琴"
	x.picks = xyzList[xyz.PickDay]{items: []xyz.PickDay{{Date: "2026-10-26", Picks: []xyz.Pick{pick}}}, loaded: true}
	if err := tt.Click("发现"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("编辑精选"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, s := range []string{"10月26日", "E255 越努力越幸运", "“我一个70岁的老太太听了这一期， 想了很多。” —— 子期听琴"} {
		if !tt.HasText(s) {
			t.Errorf("picks: no %q in %q", s, tt.Texts())
		}
	}
	shot(t, tt, "xyz-picks")

	// An episode plays without a login of QQ Music, from its own URL.
	if a.api.LoggedIn() {
		t.Fatal("QQ Music is logged in")
	}
	a.playList(episodeSongs(eps, nil), 0)
	s, ok := a.nowPlaying()
	if !ok || s.Source != xyzSource || s.URL != "http://127.0.0.1:1/e1.m4a" || s.Album.Mid != "p1" || s.Artists() != "有知有行" {
		t.Fatalf("playing %+v", s)
	}
	if a.lyrics[0].Text != "播客单集暂无歌词" {
		t.Errorf("lyrics %+v", a.lyrics)
	}
	if p := albumOf(s); p.kind != pageXyzShow || p.mid != "p1" {
		t.Errorf("album page %+v", p)
	}
	// One bought has no URL: it asks 小宇宙 when it plays.
	if s := episodeSong(eps[1], nil); s.URL != "" || s.Source != xyzSource {
		t.Errorf("private %+v", s)
	}

	// A podcast's page, from what it loaded.
	x.shows["p1"] = &xyzShow{info: pod, eps: eps, loaded: true, web: true}
	a.navigate(albumOf(s))
	tt.Frame()
	for _, s := range []string{"节目", "知行小酒馆", "有知有行 · 274 集 · 216.0万 人订阅", "播放全部", "E254 意外之财", "网页版只提供最新 2 集，登录小宇宙后可查看全部单集"} {
		if !tt.HasText(s) {
			t.Errorf("show: no %q in %q", s, tt.Texts())
		}
	}
	shot(t, tt, "xyz-show")

	// With more to come, the end loads them once it shows, unasked.
	sh := x.shows["p1"]
	sh.web, sh.next = false, xyz.More(`{"id":"e2"}`)
	x.api.HTTP = &http.Client{Transport: offline{}}
	for i := 0; i < 3 && !sh.loading; i++ {
		tt.Frame()
	}
	if !sh.loading {
		t.Error("the end of the episodes did not load more")
	}

	// The login is kept, with its device, and forgotten on logout but for
	// the device.
	a.saveXyz()
	b := newApp(f, dir)
	if cr := b.xyz.api.Credential(); cr.AccessToken != "token" || cr.DeviceID != x.api.Credential().DeviceID || b.xyz.user.Nickname != "听众" {
		t.Fatalf("saved %+v %+v", cr, b.xyz.user)
	}
	b.xyzLogout()
	c := newApp(f, dir)
	if c.xyz.api.LoggedIn() || c.xyz.api.Credential().DeviceID != x.api.Credential().DeviceID {
		t.Fatalf("logged out %+v", c.xyz.api.Credential())
	}

	// The queue of episodes comes back as it was.
	d := newApp(f, dir)
	if s, ok := d.nowPlaying(); !ok || s.Source != xyzSource || s.URL == "" {
		t.Fatalf("queue %+v", s)
	}
}
