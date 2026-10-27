package xiaoyuzhou

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// TestLive calls each API once, in turn, with the login the app saved at
// XIAOYUZHOU_CRED (its xiaoyuzhou.json).
func TestLive(t *testing.T) {
	path := os.Getenv("XIAOYUZHOU_CRED")
	if path == "" {
		t.Skip("set XIAOYUZHOU_CRED to the app's xiaoyuzhou.json")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Credential Credential `json:"credential"`
	}
	json.Unmarshal(b, &saved)
	c := New(saved.Credential)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	u, err := c.Profile(ctx)
	t.Logf("profile %q %v", u.Nickname, err)

	ch, err := c.Chart(ctx, ChartHot)
	t.Logf("chart %q %d %v", ch.Title, len(ch.Episodes), err)
	if len(ch.Episodes) > 0 {
		e := ch.Episodes[0]
		t.Logf("  #1 %s · %ds · %s", e.Title, e.Duration, e.AudioURL())
	}

	sections, err := c.Discovery(ctx)
	t.Logf("discovery %d sections %v", len(sections), err)
	for _, s := range sections {
		e := s.Entries[0]
		name := ""
		if e.Episode != nil {
			name = "单集 " + e.Episode.Title
		} else {
			name = "节目 " + e.Podcast.Title
		}
		t.Logf("  %s (%d): %s · %q %s", s.Title, len(s.Entries), name, []rune(e.Note)[:min(16, len([]rune(e.Note)))], e.By)
	}

	cats, err := c.Categories(ctx)
	t.Logf("categories %d %v", len(cats), err)
	if len(cats) > 0 {
		tabs, err := c.CategoryTabs(ctx, cats[0].ID)
		t.Logf("  %s tabs %+v %v", cats[0].Name, tabs, err)
		first, next, err := c.CategoryPodcasts(ctx, cats[0].ID, "ALL", nil)
		t.Logf("  %s 全部 %d next=%v %v", cats[0].Name, len(first), next != nil, err)
		if next != nil {
			second, _, err := c.CategoryPodcasts(ctx, cats[0].ID, "ALL", next)
			t.Logf("  page 2 %d %v", len(second), err)
			if len(first) > 0 && len(second) > 0 && first[0].Podcast.Pid == second[0].Podcast.Pid {
				t.Error("page 2 of the category is page 1")
			}
		}
	}

	days, next, err := c.EditorPicks(ctx, nil)
	t.Logf("editor picks %d days next=%v %v", len(days), next != nil, err)
	if next != nil {
		more, _, err := c.EditorPicks(ctx, next)
		t.Logf("  page 2 %d days %v", len(more), err)
	}

	inbox, next, err := c.Inbox(ctx, nil)
	t.Logf("inbox %d next=%v %v", len(inbox), next != nil, err)
	subs, next, err := c.Subscriptions(ctx, nil)
	t.Logf("subscriptions %d next=%v %v", len(subs), next != nil, err)

	pods, next, err := c.SearchPodcasts(ctx, "知行小酒馆", nil)
	t.Logf("search podcasts %d next=%v %v", len(pods), next != nil, err)
	eps, next, err := c.SearchEpisodes(ctx, "知行小酒馆", nil)
	t.Logf("search episodes %d next=%v %v", len(eps), next != nil, err)

	const pid = "6013f9f58e2f7ee375cf4216"
	p, err := c.Podcast(ctx, pid)
	t.Logf("podcast %q %d 集 %v", p.Title, p.EpisodeCount, err)
	first, next, err := c.Episodes(ctx, pid, nil)
	t.Logf("episodes %d next=%v %v", len(first), next != nil, err)
	if next != nil {
		second, _, err := c.Episodes(ctx, pid, next)
		t.Logf("  page 2 %d %v", len(second), err)
		if len(first) > 0 && len(second) > 0 && first[0].Eid == second[0].Eid {
			t.Error("page 2 of the episodes is page 1")
		}
	}
}
