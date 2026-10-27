package xyz

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// TestLive calls each API of the app once, in turn, with the login saved
// at XYZ_CRED (the app's xyz.json).
func TestLive(t *testing.T) {
	path := os.Getenv("XYZ_CRED")
	if path == "" {
		t.Skip("set XYZ_CRED to the app's xyz.json")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var sv struct {
		Credential Credential `json:"credential"`
	}
	json.Unmarshal(b, &sv)
	c := New(sv.Credential)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	u, err := c.Profile(ctx)
	t.Logf("profile %q %v", u.Nickname, err)
	top, err := c.Top(ctx, TopHot)
	t.Logf("top %q %d %v", top.Title, len(top.Episodes), err)
	if len(top.Episodes) > 0 {
		e := top.Episodes[0]
		t.Logf("  #1 %s · %s · %ds · %s", e.Title, e.Podcast.Title, e.Duration, e.Audio())
	}
	feed, err := c.Feed(ctx)
	t.Logf("feed %d sections %v", len(feed), err)
	for _, s := range feed {
		it := s.Items[0]
		name := ""
		if it.Episode != nil {
			name = "单集 " + it.Episode.Title
		} else {
			name = "节目 " + it.Podcast.Title
		}
		t.Logf("  %s (%d): %s · %q %s", s.Title, len(s.Items), name, it.Note[:min(24, len(it.Note))], it.By)
	}
	cats, err := c.Categories(ctx)
	t.Logf("categories %d %v", len(cats), err)
	if len(cats) > 0 {
		tabs, err := c.CategoryTabs(ctx, cats[0].ID)
		t.Logf("  %s tabs %+v %v", cats[0].Name, tabs, err)
		pods, more, err := c.CategoryPodcasts(ctx, cats[0].ID, "ALL", nil)
		t.Logf("  %s 全部 %d more=%v %v", cats[0].Name, len(pods), more != nil, err)
		if more != nil {
			pods2, _, err := c.CategoryPodcasts(ctx, cats[0].ID, "ALL", more)
			t.Logf("  page 2 %d %v", len(pods2), err)
			if len(pods2) > 0 && len(pods) > 0 && pods2[0].Podcast.Pid == pods[0].Podcast.Pid {
				t.Error("category page 2 is page 1")
			}
		}
	}
	days, more, err := c.EditorPicks(ctx, nil)
	t.Logf("editor picks %d days more=%v %v", len(days), more != nil, err)
	if len(days) > 0 && len(days[0].Picks) > 0 {
		p := days[0].Picks[0]
		t.Logf("  %s %s · %q —— %s", days[0].Date, p.Episode.Title, p.Comment.Text[:min(30, len(p.Comment.Text))], p.Comment.Author.Nickname)
	}
	if more != nil {
		days2, _, err := c.EditorPicks(ctx, more)
		t.Logf("editor picks page 2 %d days, from %s %v", len(days2), func() string {
			if len(days2) > 0 {
				return days2[0].Date
			}
			return ""
		}(), err)
	}
	inbox, more, err := c.Inbox(ctx, nil)
	t.Logf("inbox %d more=%v %v", len(inbox), more != nil, err)
	subs, more, err := c.Subscriptions(ctx, nil)
	t.Logf("subs %d more=%v %v", len(subs), more != nil, err)
	for _, p := range subs[:min(3, len(subs))] {
		t.Logf("  %s · %s · %d 集", p.Title, p.Author, p.EpisodeCount)
	}
	pods, _, more, err := c.Search(ctx, SearchPodcasts, "知行小酒馆", nil)
	t.Logf("search podcasts %d more=%v %v", len(pods), more != nil, err)
	_, eps, more, err := c.Search(ctx, SearchEpisodes, "知行小酒馆", nil)
	t.Logf("search episodes %d more=%v %v", len(eps), more != nil, err)
	p, err := c.Podcast(ctx, "6013f9f58e2f7ee375cf4216")
	t.Logf("podcast %q %d 集 %v", p.Title, p.EpisodeCount, err)
	page1, more, err := c.Episodes(ctx, "6013f9f58e2f7ee375cf4216", nil)
	t.Logf("episodes %d more=%v %v", len(page1), more != nil, err)
	if more != nil {
		page2, more2, err := c.Episodes(ctx, "6013f9f58e2f7ee375cf4216", more)
		t.Logf("episodes page 2 %d more=%v %v", len(page2), more2 != nil, err)
		if len(page2) > 0 && len(page1) > 0 && page2[0].Eid == page1[0].Eid {
			t.Error("page 2 is page 1")
		}
	}
}
