package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"qqmusic/internal/xiaoyuzhou"
)

// apiAnswer answers the calls a podcast and a host page make, keeping the
// modes and relations asked for. Playing a paid episode asks for its URL
// too, which is not answered here.
type apiAnswer struct {
	modes     []string
	relations []string
}

func (s *apiAnswer) RoundTrip(req *http.Request) (*http.Response, error) {
	reply := func(body string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	}
	switch req.URL.Path {
	case "/v1/subscription/update":
		var body struct {
			Mode string `json:"mode"`
		}
		json.NewDecoder(req.Body).Decode(&body)
		s.modes = append(s.modes, body.Mode)
		return reply(`{"success":true,"data":null}`)
	case "/v1/user-relation/update":
		var body struct {
			Relation string `json:"relation"`
		}
		json.NewDecoder(req.Body).Decode(&body)
		s.relations = append(s.relations, body.Relation)
		return reply(`{"success":true,"data":null}`)
	case "/v1/profile/get":
		return reply(`{"success":true,"data":{"uid":"u1","nickname":"张三","bio":"长期主义","ipLoc":"北京","relation":"STRANGE"}}`)
	case "/v1/user-stats/get":
		return reply(`{"success":true,"data":{"followingCount":1,"followerCount":10,"subscriptionCount":2}}`)
	case "/v1/podcaster/owned-podcasts":
		return reply(`{"success":true,"data":[{"pid":"p1","title":"知行小酒馆","author":"有知有行","episodeCount":274,"image":{"smallPicUrl":"https://image.xyzcdn.net/a.jpg@small"}}]}`)
	}
	return nil, errors.New("offline")
}

// TestXiaoyuzhou shows 小宇宙 官方 from what it would load, logged out and
// in, a category's and a podcast's page, and plays an episode, which needs
// no login of QQ Music.
func TestXiaoyuzhou(t *testing.T) {
	dir := t.TempDir()
	f := &fakePlatform{posts: make(chan func(), 1024)}
	a := newApp(f, dir)
	a.images = newImages(func(context.Context, string) ([]byte, error) { return nil, errors.New("offline") }, f.Post, a.trim.soon)
	tt := ui.NewTester(a.view, 1280, 1600)

	// The first time, it asks where its podcasts come from. 小宇宙's own,
	// chosen here, logged out, asks for the QR code; failing, it says so.
	x := &a.xiaoyuzhou
	x.qrErr = "获取二维码失败：离线"
	if err := tt.Click("小宇宙"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.current().kind != pagePodcastStart || a.set.XiaoyuzhouChosen {
		t.Fatalf("opened %v, chosen %v", a.current().kind, a.set.XiaoyuzhouChosen)
	}
	if err := tt.Click("使用官方小宇宙"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.current().kind != pageXiaoyuzhou || !a.set.XiaoyuzhouOfficial || !a.set.XiaoyuzhouChosen {
		t.Fatalf("opened %v, official %v, chosen %v", a.current().kind, a.set.XiaoyuzhouOfficial, a.set.XiaoyuzhouChosen)
	}
	for _, s := range []string{"官方", "登录小宇宙", "二维码获取失败", "刷新二维码", "获取二维码失败：离线", "手机号登录"} {
		if !tt.HasText(s) {
			t.Errorf("logged out: no %q in %q", s, tt.Texts())
		}
	}

	// The button beside the title shows the other source in place, and
	// keeps which one the sidebar opens next time.
	if err := tt.Click("切换到 QQ 音乐的小宇宙"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.current().kind != pagePodcast || a.set.XiaoyuzhouOfficial {
		t.Fatalf("switched to %v, official %v", a.current().kind, a.set.XiaoyuzhouOfficial)
	}
	if err := tt.Click("切换到小宇宙官方"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.current().kind != pageXiaoyuzhou || !a.set.XiaoyuzhouOfficial {
		t.Fatalf("switched to %v, official %v", a.current().kind, a.set.XiaoyuzhouOfficial)
	}

	// Logged in, what it loaded shows, and nothing more loads.
	x.api = xiaoyuzhou.New(xiaoyuzhou.Credential{AccessToken: "token", RefreshToken: "refresh"})
	x.user = xiaoyuzhou.User{UID: "me", Nickname: "听众"}
	pod := xiaoyuzhou.Podcast{Pid: "p1", Title: "知行小酒馆", Author: "有知有行", EpisodeCount: 274, SubscriptionCount: 2159868}
	eps := []xiaoyuzhou.Episode{
		{Eid: "e1", Pid: "p1", Title: "E255 越努力越幸运", Duration: 4167, PlayCount: 202684, PayType: "FREE",
			PubDate: time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC), Podcast: &pod},
		{Eid: "e2", Pid: "p1", Title: "E254 意外之财", Duration: 7466, IsPrivateMedia: true, PayType: "PAY_EPISODE", Podcast: &pod},
	}
	eps[0].Media.Source.URL = "http://127.0.0.1:1/e1.m4a"
	x.charts[0] = xiaoyuzhouChart{chart: xiaoyuzhou.Chart{Title: "最热榜", About: "每日 8 点更新", Episodes: eps}, loaded: true}
	x.inbox = pager[xiaoyuzhou.Episode]{items: eps[:1], pages: 1}
	x.subs = pager[xiaoyuzhou.Podcast]{items: []xiaoyuzhou.Podcast{pod}, pages: 1}

	// 推荐 first: the categories, and the sections of the app's front page.
	x.feed.loaded = true
	x.feed.cats = []xiaoyuzhou.Category{{ID: "c1", Name: "商业"}, {ID: "c2", Name: "科技"}}
	for _, n := range strings.Fields("自我成长 社会 故事奇谈 人文 阅读 趣味闲谈 医疗健康 影视娱乐 文旅 治愈陪伴 职业探索 饮食 艺术 体育运动 兴趣生活 音乐 动漫游戏") {
		x.feed.cats = append(x.feed.cats, xiaoyuzhou.Category{ID: n, Name: n})
	}
	long := strings.Repeat("听这期最有感触的还是「时间贫困」。作为 80 后，我们这代人一直在努力养家。", 3)
	x.feed.sections = []xiaoyuzhou.Section{
		{Title: "为你推荐", Entries: []xiaoyuzhou.Entry{{Episode: &eps[0], Note: "小米造车背后的人事江湖"}, {Episode: &eps[1], Note: "评论数 99+"}, {Episode: &eps[0], Note: "进入 26 年 8 月锋芒榜"}}},
		{Title: "为你精选的节目", Entries: []xiaoyuzhou.Entry{{Podcast: &pod, Note: "因为你喜欢三五环"}}},
		{Title: "TA们的喜欢", Entries: []xiaoyuzhou.Entry{{Episode: &eps[1], Note: "太有趣的一期", By: "本鹤"}, {Episode: &eps[0], Note: long, By: "-Rhaenyra"}}},
	}
	tt.Frame()
	for _, s := range []string{"推荐", "分类探索", "商业", "科技", "为你推荐", "小米造车背后的人事江湖", "为你精选的节目", "因为你喜欢三五环", "TA们的喜欢", "“太有趣的一期” —— 本鹤", "动漫游戏"} {
		if !tt.HasText(s) {
			t.Errorf("feed: no %q in %q", s, tt.Texts())
		}
	}
	shot(t, tt, "xiaoyuzhou-feed")

	// A category's page: its tabs, its podcasts with their newest episode.
	x.cats["c1"] = &xiaoyuzhouCategory{
		tabs:  []xiaoyuzhou.Category{{ID: "ALL", Name: "全部"}, {ID: "HOT", Name: "近期热门"}},
		lists: []pager[xiaoyuzhou.Entry]{{items: []xiaoyuzhou.Entry{{Podcast: &pod, Episode: &eps[0]}}, pages: 1}, {}},
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
	shot(t, tt, "xiaoyuzhou-category")
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
	shot(t, tt, "xiaoyuzhou")
	if err := tt.Click("我的订阅"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("有知有行 · 274 集") {
		t.Errorf("subscriptions: %q", tt.Texts())
	}

	// 编辑精选: the picks of each day, with what a listener said.
	pick := xiaoyuzhou.Pick{Episode: eps[0]}
	pick.Comment.Text, pick.Comment.Author.Nickname = "我一个70岁的老太太听了这一期，\n想了很多。", "子期听琴"
	x.picks = pager[xiaoyuzhou.DayPicks]{items: []xiaoyuzhou.DayPicks{{Date: "2026-10-26", Picks: []xiaoyuzhou.Pick{pick}}}, pages: 1}
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
	shot(t, tt, "xiaoyuzhou-picks")

	// An episode plays without a login of QQ Music, from its own URL.
	if a.api.LoggedIn() {
		t.Fatal("QQ Music is logged in")
	}
	a.playList(episodeSongs(eps, nil), 0)
	s, ok := a.nowPlaying()
	if !ok || s.Source != xiaoyuzhouSource || s.URL != "http://127.0.0.1:1/e1.m4a" || s.Album.Mid != "p1" || s.Artists() != "有知有行" {
		t.Fatalf("playing %+v", s)
	}
	if a.lyrics[0].Text != "播客单集暂无歌词" {
		t.Errorf("lyrics %+v", a.lyrics)
	}
	if p := albumOf(s); p.kind != pageXiaoyuzhouShow || p.mid != "p1" {
		t.Errorf("album page %+v", p)
	}
	// One bought has no URL: it asks 小宇宙 when it plays.
	if s := episodeSong(eps[1], nil); s.URL != "" || s.Source != xiaoyuzhouSource {
		t.Errorf("bought %+v", s)
	}

	// A podcast's page, from what it loaded.
	x.shows["p1"] = &xiaoyuzhouShow{info: pod, eps: eps, loaded: true, web: true}
	a.navigate(albumOf(s))
	tt.Frame()
	for _, s := range []string{"节目", "知行小酒馆", "有知有行 · 274 集 · 216.0万 人订阅", "播放全部", "E254 意外之财", "网页版只提供最新 2 集，登录小宇宙后可查看全部单集"} {
		if !tt.HasText(s) {
			t.Errorf("show: no %q in %q", s, tt.Texts())
		}
	}
	shot(t, tt, "xiaoyuzhou-show")

	// A page of the API knows whether the account subscribes, and says so
	// once it changes: the lists it makes stale load again.
	sh := x.shows["p1"]
	sh.web, sh.info.Podcasters = false, []xiaoyuzhou.User{{UID: "u1", Nickname: "张三"}}
	answer := &apiAnswer{}
	x.api.HTTP = &http.Client{Transport: answer}
	tt.Frame()
	shot(t, tt, "xiaoyuzhou-show-api")
	if err := tt.Click("订阅"); err != nil {
		t.Fatal(err)
	}
	if !settle(tt, f, 5*time.Second, func() bool { return !sh.subscribing }) {
		t.Fatal("subscribing did not finish")
	}
	if !sh.info.Subscribed() || sh.info.SubscriptionCount != pod.SubscriptionCount+1 || x.subs.pages != 0 || x.inbox.pages != 0 {
		t.Errorf("subscribed %q %d, pages %d %d", sh.info.SubscriptionStatus, sh.info.SubscriptionCount, x.subs.pages, x.inbox.pages)
	}
	tt.Frame()
	if _, ok := tt.Find("已订阅"); !ok {
		t.Errorf("subscribed: %q", tt.Texts())
	}
	if err := tt.Click("已订阅"); err != nil {
		t.Fatal(err)
	}
	if !settle(tt, f, 5*time.Second, func() bool { return !sh.subscribing }) {
		t.Fatal("unsubscribing did not finish")
	}
	if sh.info.Subscribed() || sh.info.SubscriptionCount != pod.SubscriptionCount || len(answer.modes) != 2 || answer.modes[0] != "ON" || answer.modes[1] != "OFF" {
		t.Errorf("unsubscribed %q %d, asked %v", sh.info.SubscriptionStatus, sh.info.SubscriptionCount, answer.modes)
	}

	// A host has a page of their own: who they are, and 关注, which asks the
	// service for the relation.
	tt.Frame()
	if err := tt.Click("张三"); err != nil {
		t.Fatal(err)
	}
	if p := a.current(); p.kind != pageXiaoyuzhouUser || p.mid != "u1" {
		t.Fatalf("host page %+v", p)
	}
	us := x.users["u1"]
	if !settle(tt, f, 5*time.Second, func() bool { return us != nil && us.loaded }) {
		t.Fatal("the host did not load")
	}
	if us.user.Bio != "长期主义" || us.user.IPLoc != "北京" || us.stats.FollowerCount != 10 || len(us.shows) != 1 || us.shows[0].Title != "知行小酒馆" {
		t.Errorf("host %+v", us)
	}
	tt.Frame()
	if _, ok := tt.Find("关注"); !ok {
		t.Errorf("host: %q", tt.Texts())
	}
	if err := tt.Click("关注"); err != nil {
		t.Fatal(err)
	}
	if !settle(tt, f, 5*time.Second, func() bool { return !us.followBusy }) {
		t.Fatal("following did not finish")
	}
	if !us.following || us.user.Relation != "FOLLOWING" || us.stats.FollowerCount != 11 || len(answer.relations) != 1 || answer.relations[0] != "FOLLOWING" {
		t.Errorf("followed %v, %d fans, asked %v", us.following, us.stats.FollowerCount, answer.relations)
	}
	tt.Frame()
	if _, ok := tt.Find("已关注"); !ok {
		t.Errorf("followed: %q", tt.Texts())
	}
	shot(t, tt, "xiaoyuzhou-user")
	if err := tt.Click("已关注"); err != nil {
		t.Fatal(err)
	}
	if !settle(tt, f, 5*time.Second, func() bool { return !us.followBusy }) {
		t.Fatal("unfollowing did not finish")
	}
	if us.following || us.stats.FollowerCount != 10 || len(answer.relations) != 2 || answer.relations[1] != "STRANGE" {
		t.Errorf("unfollowed %v, %d fans, asked %v", us.following, us.stats.FollowerCount, answer.relations)
	}

	// Opened without a uid the page is no host's: there is no one to follow.
	x.users[""] = &xiaoyuzhouUser{loaded: true}
	a.navigate(page{kind: pageXiaoyuzhouUser})
	tt.Frame()
	if _, ok := tt.Find("关注"); ok {
		t.Errorf("no host, but 关注: %q", tt.Texts())
	}
	a.back()
	tt.Frame()
	a.back()
	tt.Frame()

	// With more to come, the end loads them once it shows, unasked.
	sh.web, sh.next = false, xiaoyuzhou.Cursor(`{"id":"e2"}`)
	x.api.HTTP = &http.Client{Transport: offline{}}
	for i := 0; i < 3 && !sh.loading; i++ {
		tt.Frame()
	}
	if !sh.loading {
		t.Error("the end of the episodes did not load more")
	}

	// The login is kept, with its device, and forgotten on logout but for
	// the device.
	a.saveXiaoyuzhou()
	b := newApp(f, dir)
	if cr := b.xiaoyuzhou.api.Credential(); cr.AccessToken != "token" || cr.DeviceID != x.api.Credential().DeviceID || b.xiaoyuzhou.user.Nickname != "听众" {
		t.Fatalf("saved %+v %+v", cr, b.xiaoyuzhou.user)
	}
	b.logoutXiaoyuzhou()
	c := newApp(f, dir)
	if c.xiaoyuzhou.api.LoggedIn() || c.xiaoyuzhou.api.Credential().DeviceID != x.api.Credential().DeviceID {
		t.Fatalf("logged out %+v", c.xiaoyuzhou.api.Credential())
	}

	// The queue of episodes comes back as it was.
	d := newApp(f, dir)
	if s, ok := d.nowPlaying(); !ok || s.Source != xiaoyuzhouSource || s.URL == "" {
		t.Fatalf("queue %+v", s)
	}
}
