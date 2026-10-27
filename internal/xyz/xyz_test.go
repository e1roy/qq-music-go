package xyz

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSearchItems(t *testing.T) {
	// A page of a search mixes headers and footers in with what it found.
	raw := `[{"type":"HEADER","title":"节目"},
		{"type":"PODCAST","pid":"p1","title":"知行小酒馆","author":"有知有行","episodeCount":274,"image":{"picUrl":"https://image.xyzcdn.net/a.jpg","smallPicUrl":"https://image.xyzcdn.net/a.jpg@small"}},
		{"type":"FOOTER"},
		{"type":"EPISODE","eid":"e1","pid":"p1","title":"E255","duration":4167,"pubDate":"2026-10-23T12:00:00.000Z",
		 "media":{"source":{"url":"https://media.xyzcdn.net/a.m4a"}},"enclosure":{"url":"https://media.xyzcdn.net/b.m4a"},"podcast":{"pid":"p1","title":"知行小酒馆"}},
		{"type":"EPISODE","eid":"e2","isPrivateMedia":true,"media":{"source":{"url":"https://pmedia.xyzcdn.net/c.m4a"}}}]`
	var items []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		t.Fatal(err)
	}
	var eps []Episode
	for _, it := range items {
		var e Episode
		if json.Unmarshal(it, &e) == nil && e.Eid != "" {
			eps = append(eps, e)
		}
	}
	if len(eps) != 2 || eps[0].Audio() != "https://media.xyzcdn.net/a.m4a" || eps[0].Podcast.Title != "知行小酒馆" {
		t.Fatalf("episodes %+v", eps)
	}
	if eps[1].Audio() != "" {
		t.Error("a private episode has its URL from PrivateMedia")
	}
	if eps[0].PubDate.Year() != 2026 {
		t.Error("pubDate", eps[0].PubDate)
	}
	if more(json.RawMessage("null")) != nil || more(nil) != nil || more(json.RawMessage(`{"skip":20}`)) == nil {
		t.Error("more")
	}
	im := Image{PicURL: "p", SmallPicURL: "s"}
	if im.URL(300) != "s" || im.URL(800) != "p" {
		t.Error("image", im.URL(300), im.URL(800))
	}
}

func TestDeviceID(t *testing.T) {
	id := New(Credential{}).Credential().DeviceID
	if len(id) != 36 || strings.ToUpper(id) != id || id[14] != '4' {
		t.Error(id)
	}
	if New(Credential{DeviceID: "X"}).Credential().DeviceID != "X" {
		t.Error("the device is kept")
	}
}

// TestWeb fetches one podcast's web page and makes one QR code: two
// requests, neither of which needs a login.
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
	if p.Title == "" || len(eps) == 0 || eps[0].Audio() == "" || eps[0].Duration == 0 {
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
	if _, err := c.Profile(ctx); err != ErrNotLoggedIn {
		t.Error("no login:", err)
	}
}
