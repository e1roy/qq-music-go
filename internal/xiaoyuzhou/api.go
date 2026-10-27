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

// UserProfile is another user's profile, as the account sees it.
func (c *Client) UserProfile(ctx context.Context, uid string) (User, error) {
	var u User
	_, err := c.request(ctx, "/v1/profile/get?uid="+url.QueryEscape(uid), nil, &u)
	return u, err
}

// UserStats is a user's counts.
func (c *Client) UserStats(ctx context.Context, uid string) (UserStats, error) {
	var s UserStats
	_, err := c.request(ctx, "/v1/user-stats/get?uid="+url.QueryEscape(uid), nil, &s)
	return s, err
}

// UserPodcasts is the podcasts a user hosts.
func (c *Client) UserPodcasts(ctx context.Context, uid string) ([]Podcast, error) {
	var pods []Podcast
	_, err := c.request(ctx, "/v1/podcaster/owned-podcasts", map[string]any{"uid": uid}, &pods)
	return pods, err
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

// SetSubscription subscribes to a podcast, or unsubscribes from it.
func (c *Client) SetSubscription(ctx context.Context, pid string, on bool) error {
	mode := "OFF"
	if on {
		mode = "ON"
	}
	var v struct{}
	_, err := c.request(ctx, "/v1/subscription/update", map[string]any{"pid": pid, "mode": mode}, &v)
	return err
}

// SetFollowing follows a user, or unfollows them.
func (c *Client) SetFollowing(ctx context.Context, uid string, on bool) error {
	relation := "STRANGE"
	if on {
		relation = "FOLLOWING"
	}
	var v struct{}
	_, err := c.request(ctx, "/v1/user-relation/update", map[string]any{"uid": uid, "relation": relation}, &v)
	return err
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

// Episode is an episode's details, its shownotes among them, as the app
// shows them on its page. It needs a login; WebEpisode is the same page
// without one.
func (c *Client) Episode(ctx context.Context, eid string) (Episode, error) {
	var e Episode
	_, err := c.request(ctx, "/v1/episode/get?eid="+url.QueryEscape(eid), nil, &e)
	return e, err
}

// Comments is a page of an episode's comments. They need a login.
func (c *Client) Comments(ctx context.Context, eid string, order CommentOrder, after Cursor) ([]Comment, Cursor, error) {
	var comments []Comment
	body := after.into(map[string]any{
		"limit": pageSize,
		"owner": map[string]any{"id": eid, "type": "EPISODE"},
		"order": string(order),
	})
	next, err := c.request(ctx, "/v1/comment/list-primary", body, &comments)
	return comments, next, err
}

// WebEpisode is an episode's page of the web site, which needs no login.
// It has what the app's page does, the shownotes among it.
func (c *Client) WebEpisode(ctx context.Context, eid string) (Episode, error) {
	html, err := c.webPage(ctx, siteBase+"/episode/"+url.PathEscape(eid))
	if err != nil {
		return Episode{}, err
	}
	m := nextData.FindSubmatch(html)
	if m == nil {
		return Episode{}, fmt.Errorf("xiaoyuzhou: 单集页没有数据")
	}
	var v struct {
		Props struct {
			PageProps struct {
				Episode Episode `json:"episode"`
			} `json:"pageProps"`
		} `json:"props"`
	}
	if err := json.Unmarshal(m[1], &v); err != nil {
		return Episode{}, fmt.Errorf("xiaoyuzhou: 单集页: %w", err)
	}
	e := v.Props.PageProps.Episode
	if e.Eid == "" {
		return Episode{}, fmt.Errorf("xiaoyuzhou: 单集不存在")
	}
	return e, nil
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
