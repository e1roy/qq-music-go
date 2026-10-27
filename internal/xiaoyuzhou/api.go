package xiaoyuzhou

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
)

// Profile is the user logged in.
func (c *Client) Profile(ctx context.Context) (User, error) {
	var u User
	_, err := c.request(ctx, "/v1/profile/get", nil, &u)
	return u, err
}

// Chart is a chart of the day's episodes.
func (c *Client) Chart(ctx context.Context, kind ChartKind) (Chart, error) {
	var v struct {
		Title       string `json:"title"`
		Information string `json:"information"`
		Items       []struct {
			Item Episode `json:"item"`
		} `json:"items"`
	}
	_, err := c.request(ctx, "/v1/top-list/get?category="+string(kind), nil, &v)
	ch := Chart{Title: v.Title, About: v.Information}
	for _, it := range v.Items {
		if it.Item.Eid != "" {
			ch.Episodes = append(ch.Episodes, it.Item)
		}
	}
	return ch, err
}

// EditorPicks is the episodes the editors picked, a few a day, the latest
// day first.
func (c *Client) EditorPicks(ctx context.Context, after Cursor) ([]DayPicks, Cursor, error) {
	var days []DayPicks
	next, err := c.request(ctx, "/v1/editor-pick/list", after.into(map[string]any{}), &days)
	return days, next, err
}

// Inbox is what the podcasts subscribed to put out, the newest first.
func (c *Client) Inbox(ctx context.Context, after Cursor) ([]Episode, Cursor, error) {
	var eps []Episode
	next, err := c.request(ctx, "/v1/inbox/list", after.into(map[string]any{"limit": pageSize}), &eps)
	return eps, next, err
}

// Subscriptions is the podcasts subscribed to, the latest first.
func (c *Client) Subscriptions(ctx context.Context, after Cursor) ([]Podcast, Cursor, error) {
	var pods []Podcast
	body := map[string]any{"limit": pageSize, "sortBy": "subscribedAt", "sortOrder": "desc"}
	next, err := c.request(ctx, "/v1/subscription/list", after.into(body), &pods)
	return pods, next, err
}

// Podcast is a podcast's details.
func (c *Client) Podcast(ctx context.Context, pid string) (Podcast, error) {
	var p Podcast
	_, err := c.request(ctx, "/v1/podcast/get?pid="+url.QueryEscape(pid), nil, &p)
	return p, err
}

// Episodes is a podcast's episodes, the newest first.
func (c *Client) Episodes(ctx context.Context, pid string, after Cursor) ([]Episode, Cursor, error) {
	var eps []Episode
	body := map[string]any{"limit": pageSize, "pid": pid, "order": "desc"}
	next, err := c.request(ctx, "/v1/episode/list", after.into(body), &eps)
	return eps, next, err
}

// SearchPodcasts looks for podcasts.
func (c *Client) SearchPodcasts(ctx context.Context, keyword string, after Cursor) ([]Podcast, Cursor, error) {
	return search[Podcast](ctx, c, "PODCAST", keyword, after)
}

// SearchEpisodes looks for episodes.
func (c *Client) SearchEpisodes(ctx context.Context, keyword string, after Cursor) ([]Episode, Cursor, error) {
	return search[Episode](ctx, c, "EPISODE", keyword, after)
}

// search looks for items of a kind. A page of what it finds mixes in
// headers and footers, of other types, which are dropped.
func search[T any](ctx context.Context, c *Client, kind, keyword string, after Cursor) ([]T, Cursor, error) {
	var raws []json.RawMessage
	body := map[string]any{"limit": pageSize, "type": kind, "keyword": keyword, "sourcePageName": "4", "currentPageName": "4"}
	next, err := c.request(ctx, "/v1/search/create", after.into(body), &raws)
	var found []T
	for _, raw := range raws {
		var head struct {
			Type string `json:"type"`
		}
		var item T
		if json.Unmarshal(raw, &head) == nil && head.Type == kind && json.Unmarshal(raw, &item) == nil {
			found = append(found, item)
		}
	}
	return found, next, err
}

// PrivateMediaURL is where the sound of an episode bought is, for a few
// hours.
func (c *Client) PrivateMediaURL(ctx context.Context, eid string) (string, error) {
	var v struct {
		URL string `json:"url"`
	}
	_, err := c.request(ctx, "/v1/private-media/get?dubbing=false&eid="+url.QueryEscape(eid), nil, &v)
	if err == nil && v.URL == "" {
		err = fmt.Errorf("xiaoyuzhou: 付费单集没有音频地址，可能尚未购买")
	}
	return v.URL, err
}

// nextData is the data a page of the web site is rendered from.
var nextData = regexp.MustCompile(`(?s)<script id="__NEXT_DATA__" type="application/json">(.*?)</script>`)

// WebPodcast is a podcast and its newest episodes, about 15, from its
// page of the web site, which needs no login.
func (c *Client) WebPodcast(ctx context.Context, pid string) (Podcast, []Episode, error) {
	html, err := c.webPage(ctx, siteBase+"/podcast/"+url.PathEscape(pid))
	if err != nil {
		return Podcast{}, nil, err
	}
	m := nextData.FindSubmatch(html)
	if m == nil {
		return Podcast{}, nil, fmt.Errorf("xiaoyuzhou: 节目页没有数据")
	}
	var v struct {
		Props struct {
			PageProps struct {
				Podcast struct {
					Podcast
					Episodes []Episode `json:"episodes"`
				} `json:"podcast"`
			} `json:"pageProps"`
		} `json:"props"`
	}
	if err := json.Unmarshal(m[1], &v); err != nil {
		return Podcast{}, nil, fmt.Errorf("xiaoyuzhou: 节目页: %w", err)
	}
	p := v.Props.PageProps.Podcast
	if p.Pid == "" {
		return Podcast{}, nil, fmt.Errorf("xiaoyuzhou: 节目不存在")
	}
	return p.Podcast, p.Episodes, nil
}
