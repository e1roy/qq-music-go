package xyz

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
)

// More is where a list goes on, given back to the call that made it for
// its next page; nil at its end.
type More json.RawMessage

func more(raw json.RawMessage) More {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return nil
	}
	return More(raw)
}

// listBody is the body of a call of a list: its page size, as a string as
// the app sends, and where it goes on.
func listBody(after More, kv ...string) map[string]any {
	b := map[string]any{"limit": "20"}
	for i := 0; i+1 < len(kv); i += 2 {
		b[kv[i]] = kv[i+1]
	}
	if after != nil {
		b["loadMoreKey"] = json.RawMessage(after)
	}
	return b
}

func decode[T any](path string, raw json.RawMessage) (T, error) {
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, fmt.Errorf("xyz: %s: %w", path, err)
	}
	return v, nil
}

// list calls a list of the API and decodes its page.
func list[T any](ctx context.Context, c *Client, path string, body any) ([]T, More, error) {
	env, err := c.call(ctx, path, body)
	if err != nil {
		return nil, nil, err
	}
	items, err := decode[[]T](path, env.Data)
	return items, more(env.LoadMoreKey), err
}

// Profile is the user logged in.
func (c *Client) Profile(ctx context.Context) (User, error) {
	env, err := c.call(ctx, "/v1/profile/get", nil)
	if err != nil {
		return User{}, err
	}
	return decode[User]("/v1/profile/get", env.Data)
}

// Top is a chart of the episodes of the day.
func (c *Client) Top(ctx context.Context, cat TopCategory) (TopList, error) {
	path := "/v1/top-list/get?category=" + string(cat)
	env, err := c.call(ctx, path, nil)
	if err != nil {
		return TopList{}, err
	}
	v, err := decode[struct {
		Title       string `json:"title"`
		Information string `json:"information"`
		Items       []struct {
			Item Episode `json:"item"`
		} `json:"items"`
	}](path, env.Data)
	t := TopList{Title: v.Title, Information: v.Information}
	for _, it := range v.Items {
		if it.Item.Eid != "" {
			t.Episodes = append(t.Episodes, it.Item)
		}
	}
	return t, err
}

// EditorPicks is the episodes the editors picked, a few a day, the last
// day first.
func (c *Client) EditorPicks(ctx context.Context, after More) ([]PickDay, More, error) {
	body := map[string]any{}
	if after != nil {
		body["loadMoreKey"] = json.RawMessage(after)
	}
	return list[PickDay](ctx, c, "/v1/editor-pick/list", body)
}

// Inbox is the episodes the podcasts subscribed to put out, the newest
// first, a page at a time.
func (c *Client) Inbox(ctx context.Context, after More) ([]Episode, More, error) {
	return list[Episode](ctx, c, "/v1/inbox/list", listBody(after))
}

// Subscriptions is the podcasts subscribed to, the last first.
func (c *Client) Subscriptions(ctx context.Context, after More) ([]Podcast, More, error) {
	return list[Podcast](ctx, c, "/v1/subscription/list", listBody(after, "sortOrder", "desc", "sortBy", "subscribedAt"))
}

// Podcast is a podcast's details.
func (c *Client) Podcast(ctx context.Context, pid string) (Podcast, error) {
	path := "/v1/podcast/get?pid=" + url.QueryEscape(pid)
	env, err := c.call(ctx, path, nil)
	if err != nil {
		return Podcast{}, err
	}
	return decode[Podcast](path, env.Data)
}

// Episodes is a podcast's episodes, the newest first.
func (c *Client) Episodes(ctx context.Context, pid string, after More) ([]Episode, More, error) {
	return list[Episode](ctx, c, "/v1/episode/list", listBody(after, "pid", pid, "order", "desc"))
}

// Search looks for podcasts or episodes. Its answer mixes in headers and
// footers, which are dropped.
func (c *Client) Search(ctx context.Context, kind SearchKind, keyword string, after More) ([]Podcast, []Episode, More, error) {
	items, next, err := list[json.RawMessage](ctx, c, "/v1/search/create",
		listBody(after, "type", string(kind), "keyword", keyword, "sourcePageName", "4", "currentPageName", "4"))
	var pods []Podcast
	var eps []Episode
	for _, raw := range items {
		var t struct {
			Type string `json:"type"`
		}
		json.Unmarshal(raw, &t)
		switch t.Type {
		case "PODCAST":
			var p Podcast
			if json.Unmarshal(raw, &p) == nil && p.Pid != "" {
				pods = append(pods, p)
			}
		case "EPISODE":
			var e Episode
			if json.Unmarshal(raw, &e) == nil && e.Eid != "" {
				eps = append(eps, e)
			}
		}
	}
	return pods, eps, next, err
}

// PrivateMedia is the URL of the sound of an episode bought, good for a
// few hours.
func (c *Client) PrivateMedia(ctx context.Context, eid string) (string, error) {
	path := "/v1/private-media/get?dubbing=false&eid=" + url.QueryEscape(eid)
	env, err := c.call(ctx, path, nil)
	if err != nil {
		return "", err
	}
	v, err := decode[struct {
		URL string `json:"url"`
	}](path, env.Data)
	if err == nil && v.URL == "" {
		err = fmt.Errorf("xyz: 付费单集没有音频地址，可能尚未购买")
	}
	return v.URL, err
}

var nextData = regexp.MustCompile(`(?s)<script id="__NEXT_DATA__" type="application/json">(.*?)</script>`)

// WebPodcast is a podcast and its newest episodes, about 15, from its web
// page, which needs no login.
func (c *Client) WebPodcast(ctx context.Context, pid string) (Podcast, []Episode, error) {
	html, err := c.page(ctx, siteURL+"/podcast/"+url.PathEscape(pid))
	if err != nil {
		return Podcast{}, nil, err
	}
	m := nextData.FindSubmatch(html)
	if m == nil {
		return Podcast{}, nil, fmt.Errorf("xyz: 节目页没有数据")
	}
	v, err := decode[struct {
		Props struct {
			PageProps struct {
				Podcast struct {
					Podcast
					Episodes []Episode `json:"episodes"`
				} `json:"podcast"`
			} `json:"pageProps"`
		} `json:"props"`
	}]("/podcast/"+pid, m[1])
	p := v.Props.PageProps.Podcast
	if err == nil && p.Pid == "" {
		err = fmt.Errorf("xyz: 节目不存在")
	}
	return p.Podcast, p.Episodes, err
}
