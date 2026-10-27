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
