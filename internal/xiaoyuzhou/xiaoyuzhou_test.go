package xiaoyuzhou

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// service answers the client's requests by path, as the API would, and
// keeps what was asked.
type service struct {
	answer func(path string, h http.Header, body map[string]any) (int, string)
	paths  []string
}

func (s *service) RoundTrip(req *http.Request) (*http.Response, error) {
	var body map[string]any
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		json.Unmarshal(b, &body)
	}
	s.paths = append(s.paths, req.URL.RequestURI())
	status, out := s.answer(req.URL.Path, req.Header, body)
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(out)), Header: http.Header{}, Request: req}, nil
}

func offline(cr Credential, s *service) *Client {
	c := New(cr)
	c.HTTP = &http.Client{Transport: s}
	return c
}

func TestSearch(t *testing.T) {
	// A page of what was found mixes in headers and footers.
	page := `{"data":[{"type":"HEADER","title":"节目"},
		{"type":"PODCAST","pid":"p1","title":"知行小酒馆","author":"有知有行","episodeCount":274,"image":{"picUrl":"https://image.xyzcdn.net/a.jpg","smallPicUrl":"https://image.xyzcdn.net/a.jpg@small"}},
		{"type":"FOOTER"},
		{"type":"EPISODE","eid":"e1","pid":"p1","title":"E255","duration":4167,"pubDate":"2026-10-23T12:00:00.000Z",
		 "media":{"source":{"url":"https://media.xyzcdn.net/a.m4a"}},"enclosure":{"url":"https://media.xyzcdn.net/b.m4a"},"podcast":{"pid":"p1","title":"知行小酒馆"}},
		{"type":"EPISODE","eid":"e2","isPrivateMedia":true,"payType":"PAY_EPISODE","media":{"source":{"url":"https://pmedia.xyzcdn.net/c.m4a"}}}],
		"loadMoreKey":{"skip":20}}`
	var asked []map[string]any
	s := &service{answer: func(path string, h http.Header, body map[string]any) (int, string) {
		if path != "/v1/search/create" || h.Get("x-jike-access-token") != "access" || h.Get("x-jike-device-id") != "device" {
			return http.StatusBadRequest, `{"toast":"请求错误"}`
		}
		asked = append(asked, body)
		return http.StatusOK, page
	}}
	c := offline(Credential{AccessToken: "access", DeviceID: "device"}, s)
	ctx := context.Background()

	pods, next, err := c.SearchPodcasts(ctx, "知行", nil)
	if err != nil || len(pods) != 1 || pods[0].Title != "知行小酒馆" || pods[0].Image.URL(300) != "https://image.xyzcdn.net/a.jpg@small" {
		t.Fatalf("podcasts %+v %v", pods, err)
	}
	if string(next) != `{"skip":20}` {
		t.Fatalf("cursor %s", next)
	}
	eps, _, err := c.SearchEpisodes(ctx, "知行", next)
	if err != nil || len(eps) != 2 {
		t.Fatalf("episodes %+v %v", eps, err)
	}
	if eps[0].AudioURL() != "https://media.xyzcdn.net/a.m4a" || eps[0].Podcast.Title != "知行小酒馆" || eps[0].PubDate.Year() != 2026 || eps[0].Paid() {
		t.Errorf("episode %+v", eps[0])
	}
	if eps[1].AudioURL() != "" || !eps[1].Paid() {
		t.Error("an episode bought has its URL from PrivateMediaURL")
	}
	if asked[0]["type"] != "PODCAST" || asked[0]["loadMoreKey"] != nil || asked[1]["type"] != "EPISODE" || asked[1]["loadMoreKey"] == nil {
		t.Errorf("asked %v", asked)
	}
}

// TestSetSubscription: subscribing and unsubscribing ask the service for
// the mode wanted.
func TestSetSubscription(t *testing.T) {
	var asked []map[string]any
	s := &service{answer: func(path string, h http.Header, body map[string]any) (int, string) {
		if path != "/v1/subscription/update" || h.Get("x-jike-access-token") != "access" {
			return http.StatusBadRequest, `{"toast":"请求错误"}`
		}
		asked = append(asked, body)
		return http.StatusOK, `{"success":true}`
	}}
	c := offline(Credential{AccessToken: "access", DeviceID: "device"}, s)
	ctx := context.Background()
	if err := c.SetSubscription(ctx, "p1", true); err != nil {
		t.Fatal(err)
	}
	if err := c.SetSubscription(ctx, "p1", false); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 2 || asked[0]["pid"] != "p1" || asked[0]["mode"] != "ON" || asked[1]["mode"] != "OFF" {
		t.Fatalf("asked %v", asked)
	}
}

// TestUser: a host's profile, counts and podcasts are read, and following
// them asks for FOLLOWING, unfollowing for STRANGE.
func TestUser(t *testing.T) {
	var asked []map[string]any
	s := &service{answer: func(path string, h http.Header, body map[string]any) (int, string) {
		switch path {
		case "/v1/profile/get":
			return http.StatusOK, `{"success":true,"data":{"uid":"u1","nickname":"张三","bio":"长期主义","ipLoc":"北京","relation":"STRANGE"}}`
		case "/v1/user-stats/get":
			return http.StatusOK, `{"success":true,"data":{"followingCount":1,"followerCount":2,"subscriptionCount":3}}`
		case "/v1/podcaster/owned-podcasts":
			if body["uid"] != "u1" {
				return http.StatusBadRequest, `{"toast":"请求错误"}`
			}
			return http.StatusOK, `{"success":true,"data":[{"pid":"p1","title":"知行小酒馆"}]}`
		case "/v1/user-relation/update":
			asked = append(asked, body)
			return http.StatusOK, `{"success":true}`
		}
		return http.StatusBadRequest, `{"toast":"请求错误"}`
	}}
	c := offline(Credential{AccessToken: "access"}, s)
	ctx := context.Background()
	u, err := c.UserProfile(ctx, "u1")
	if err != nil || u.Nickname != "张三" || u.Bio != "长期主义" || u.IPLoc != "北京" || u.Following() {
		t.Fatalf("profile %+v %v", u, err)
	}
	stats, err := c.UserStats(ctx, "u1")
	if err != nil || stats.FollowingCount != 1 || stats.FollowerCount != 2 || stats.SubscriptionCount != 3 {
		t.Fatalf("stats %+v %v", stats, err)
	}
	pods, err := c.UserPodcasts(ctx, "u1")
	if err != nil || len(pods) != 1 || pods[0].Title != "知行小酒馆" {
		t.Fatalf("podcasts %+v %v", pods, err)
	}
	if err := c.SetFollowing(ctx, "u1", true); err != nil {
		t.Fatal(err)
	}
	if err := c.SetFollowing(ctx, "u1", false); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 2 || asked[0]["uid"] != "u1" || asked[0]["relation"] != "FOLLOWING" || asked[1]["relation"] != "STRANGE" {
		t.Fatalf("asked %v", asked)
	}
	if len(s.paths) != 5 || s.paths[0] != "/v1/profile/get?uid=u1" || s.paths[1] != "/v1/user-stats/get?uid=u1" {
		t.Errorf("paths %v", s.paths)
	}
}

// TestRenew: a request turned away for its tokens renews them once, and
// is made again with the new ones.
func TestRenew(t *testing.T) {
	s := &service{answer: func(path string, h http.Header, _ map[string]any) (int, string) {
		switch {
		case path == "/app_auth_tokens.refresh" && h.Get("x-jike-refresh-token") == "refresh":
			return http.StatusOK, `{"success":true,"x-jike-access-token":"new"}`
		case h.Get("x-jike-access-token") == "new":
			return http.StatusOK, `{"data":{"nickname":"听众"}}`
		}
		return http.StatusUnauthorized, `{}`
	}}
	c := offline(Credential{AccessToken: "old", RefreshToken: "refresh"}, s)
	var saved Credential
	c.OnRefresh = func(cr Credential) { saved = cr }
	u, err := c.Profile(context.Background())
	if err != nil || u.Nickname != "听众" {
		t.Fatalf("profile %+v %v", u, err)
	}
	if saved.AccessToken != "new" || saved.RefreshToken != "refresh" || len(s.paths) != 3 {
		t.Errorf("saved %+v after %v", saved, s.paths)
	}

	// Tokens that do not renew are a login gone.
	c = offline(Credential{AccessToken: "old", RefreshToken: "gone"}, s)
	if _, err := c.Profile(context.Background()); err != ErrNotLoggedIn {
		t.Errorf("err %v", err)
	}
	if _, err := New(Credential{}).Profile(context.Background()); err != ErrNotLoggedIn {
		t.Errorf("no login: %v", err)
	}
}

// TestEpisodeComments: an episode's notes come from its details, and its
// comments from the comments path, a page at a time in the order asked.
func TestEpisodeComments(t *testing.T) {
	var asked []map[string]any
	s := &service{answer: func(path string, _ http.Header, body map[string]any) (int, string) {
		switch path {
		case "/v1/episode/get":
			return http.StatusOK, `{"data":{"eid":"e1","title":"E1","description":"简介","shownotes":"<p>第一段</p><p>第二段</p>","commentCount":34}}`
		case "/v1/comment/list-primary":
			asked = append(asked, body)
			return http.StatusOK, `{"data":[{"id":"c1","text":"好听","likeCount":7,"replyCount":2,"createdAt":"2026-10-26T04:00:00.000Z","ipLoc":"上海","author":{"nickname":"听众","avatar":{"picture":{"smallPicUrl":"https://image.xyzcdn.net/a.jpg@small"}}},"replies":[{"id":"r1","text":"同感","author":{"nickname":"朋友"}}]}],"loadMoreKey":{"skip":20}}`
		}
		return http.StatusBadRequest, `{"toast":"请求错误"}`
	}}
	c := offline(Credential{AccessToken: "access", DeviceID: "device"}, s)
	ctx := context.Background()
	e, err := c.Episode(ctx, "e1")
	if err != nil || e.Shownotes != "<p>第一段</p><p>第二段</p>" || e.CommentCount != 34 {
		t.Fatalf("episode %+v %v", e, err)
	}
	comments, next, err := c.Comments(ctx, "e1", LatestComments, nil)
	if err != nil || len(comments) != 1 || comments[0].Text != "好听" || comments[0].Author.Nickname != "听众" || comments[0].LikeCount != 7 {
		t.Fatalf("comments %+v %v", comments, err)
	}
	if comments[0].Replies[0].Text != "同感" || string(next) != `{"skip":20}` {
		t.Errorf("reply %+v cursor %s", comments[0].Replies, next)
	}
	if len(asked) != 1 || asked[0]["order"] != "TIME" || asked[0]["limit"] != "20" {
		t.Fatalf("asked %v", asked)
	}
	owner, _ := asked[0]["owner"].(map[string]any)
	if owner["id"] != "e1" || owner["type"] != "EPISODE" {
		t.Errorf("owner %v", owner)
	}
}

// TestWebEpisode: the web page has the notes too, for no login.
func TestWebEpisode(t *testing.T) {
	page := `<html><body><script id="__NEXT_DATA__" type="application/json">
		{"props":{"pageProps":{"episode":{"eid":"e1","title":"E1","shownotes":"<p>简介</p>"}}}}
	</script></body></html>`
	s := &service{answer: func(path string, h http.Header, _ map[string]any) (int, string) {
		if path != "/episode/e1" || h.Get("User-Agent") == "" {
			return http.StatusForbidden, `<html></html>`
		}
		return http.StatusOK, page
	}}
	e, err := offline(Credential{}, s).WebEpisode(context.Background(), "e1")
	if err != nil || e.Eid != "e1" || e.Shownotes != "<p>简介</p>" {
		t.Fatalf("episode %+v %v", e, err)
	}
}

func TestDiscovery(t *testing.T) {
	pages := map[string]string{
		"": `{"data":[{"type":"DISCOVERY_EPISODE_RECOMMEND","data":{"target":[{"recommendation":"为什么","episode":{"eid":"e1"}}]}},
			{"type":"DISCOVERY_BANNER","data":[]},{"type":"EDITOR_PICK","data":"changed"}],"loadMoreKey":"topList"}`,
		"topList": `{"data":[{"type":"DISCOVERY_COLLECTION","data":[{"title":"为你精选的节目","target":[{"podcast":{"pid":"p1"}}]},{"target":[]}]},
			{"type":"NEW_POWER","data":{"serialNumber":"12","items":[{"pid":"p2","recommendation":"新"}]}}],"loadMoreKey":"pick"}`,
		"pick": `{"data":[{"type":"DISCOVERY_PICK","data":[{"pick":{"story":{"text":"好听"},"episode":{"eid":"e2"},"user":{"nickname":"本鹤"}}}]}]}`,
	}
	s := &service{answer: func(_ string, _ http.Header, body map[string]any) (int, string) {
		key, _ := body["loadMoreKey"].(string)
		return http.StatusOK, pages[key]
	}}
	sections, err := offline(Credential{AccessToken: "access"}, s).Discovery(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, sec := range sections {
		got = append(got, sec.Title)
	}
	if strings.Join(got, "|") != "为你推荐|为你精选的节目|宝藏新节目 · 第 12 期|TA们的喜欢" {
		t.Fatalf("sections %q", got)
	}
	if e := sections[3].Entries[0]; e.Episode.Eid != "e2" || e.Note != "好听" || e.By != "本鹤" {
		t.Errorf("pick %+v", e)
	}
}

func TestTypes(t *testing.T) {
	p := Picture{PicURL: "p", SmallPicURL: "s"}
	if p.URL(120) != "p" || p.URL(300) != "s" || p.URL(800) != "p" {
		t.Error("picture", p.URL(120), p.URL(300), p.URL(800))
	}
	if cursor(json.RawMessage("null")) != nil || cursor(nil) != nil || cursor(json.RawMessage(`{}`)) != nil || cursor(json.RawMessage(`"2026-10-26"`)) == nil {
		t.Error("cursor")
	}
	id := New(Credential{}).Credential().DeviceID
	if len(id) != 36 || strings.ToUpper(id) != id || id[14] != '4' {
		t.Error("device", id)
	}
	if New(Credential{DeviceID: "X"}).Credential().DeviceID != "X" {
		t.Error("the device is kept")
	}
}

// TestWeb fetches a podcast's web page and makes a QR code: two requests,
// neither of which needs a login.
func TestWeb(t *testing.T) {
	if testing.Short() {
		t.Skip("uses the network")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := New(Credential{})
	p, eps, err := c.WebPodcast(ctx, "6013f9f58e2f7ee375cf4216")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s · %s · %d 集，网页 %d 集", p.Title, p.Author, p.EpisodeCount, len(eps))
	if p.Title == "" || len(eps) == 0 || eps[0].AudioURL() == "" || eps[0].Duration == 0 {
		t.Fatalf("podcast %+v", p)
	}
	q, err := c.NewQR(ctx)
	if err != nil {
		t.Fatal(err)
	}
	st, err := c.PollQR(ctx, q)
	t.Logf("QR %s %s", q.URL, st)
	if err != nil || st != QRWaiting || len(q.Image) == 0 {
		t.Fatal(st, err)
	}
}
