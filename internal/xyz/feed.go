package xyz

import (
	"context"
	"encoding/json"
	"fmt"
)

// FeedItem is an episode or a podcast a section of the front page shows,
// with a line of why: what it is about, why it is recommended, or what a
// listener said of it, By whom.
type FeedItem struct {
	Episode *Episode
	Podcast *Podcast
	Note    string
	By      string
}

// FeedSection is a section of the app's front page (发现).
type FeedSection struct {
	Title string
	Items []FeedItem
}

// feedTarget is an item of the sections the recommender makes.
type feedTarget struct {
	Recommendation string   `json:"recommendation"`
	Episode        *Episode `json:"episode"`
	Podcast        *Podcast `json:"podcast"`
}

type feedModule struct {
	Title  string       `json:"title"`
	Target []feedTarget `json:"target"`
}

func (m feedModule) section(title string) FeedSection {
	if m.Title != "" {
		title = m.Title
	}
	s := FeedSection{Title: title}
	for _, t := range m.Target {
		if t.Episode != nil || t.Podcast != nil {
			s.Items = append(s.Items, FeedItem{Episode: t.Episode, Podcast: t.Podcast, Note: t.Recommendation})
		}
	}
	return s
}

// feedPages is how many pages of the front page are read at most: the
// app has four, its loadMoreKey naming the next.
const feedPages = 6

// Feed is the app's front page: what is recommended to the user, the
// editors' picks, new podcasts and what other listeners liked, in the
// app's order. The charts, banners and links to pages of the app are
// left out.
func (c *Client) Feed(ctx context.Context) ([]FeedSection, error) {
	var out []FeedSection
	var after json.RawMessage
	for range feedPages {
		body := map[string]any{"returnAll": "false"}
		if after != nil {
			body["loadMoreKey"] = after
		}
		env, err := c.call(ctx, "/v1/discovery-feed/list", body)
		if err != nil {
			return out, err
		}
		mods, err := decode[[]struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}]("/v1/discovery-feed/list", env.Data)
		if err != nil {
			return out, err
		}
		for _, m := range mods {
			// Each kind is read alone: one that changes drops itself only.
			switch m.Type {
			case "DISCOVERY_EPISODE_RECOMMEND", "PILOT":
				var v feedModule
				if json.Unmarshal(m.Data, &v) == nil {
					out = append(out, v.section("为你推荐"))
				}
			case "DISCOVERY_COLLECTION":
				var v []feedModule
				json.Unmarshal(m.Data, &v)
				for _, mod := range v {
					out = append(out, mod.section("为你精选"))
				}
			case "EDITOR_PICK":
				var v PickDay
				if json.Unmarshal(m.Data, &v) == nil {
					s := FeedSection{Title: "编辑精选"}
					for _, p := range v.Picks {
						s.Items = append(s.Items, FeedItem{Episode: &p.Episode, Note: p.Comment.Text, By: p.Comment.Author.Nickname})
					}
					out = append(out, s)
				}
			case "NEW_POWER":
				var v struct {
					Serial string `json:"serialNumber"`
					Items  []struct {
						Podcast
						Recommendation string `json:"recommendation"`
					} `json:"items"`
				}
				if json.Unmarshal(m.Data, &v) == nil {
					s := FeedSection{Title: "宝藏新节目"}
					if v.Serial != "" {
						s.Title += fmt.Sprintf(" · 第 %s 期", v.Serial)
					}
					for _, it := range v.Items {
						s.Items = append(s.Items, FeedItem{Podcast: &it.Podcast, Note: it.Recommendation})
					}
					out = append(out, s)
				}
			case "DISCOVERY_PICK":
				var v []struct {
					Pick struct {
						Story struct {
							Text string `json:"text"`
						} `json:"story"`
						Episode *Episode `json:"episode"`
						User    User     `json:"user"`
					} `json:"pick"`
				}
				if json.Unmarshal(m.Data, &v) == nil {
					s := FeedSection{Title: "TA们的喜欢"}
					for _, p := range v {
						if p.Pick.Episode != nil {
							s.Items = append(s.Items, FeedItem{Episode: p.Pick.Episode, Note: p.Pick.Story.Text, By: p.Pick.User.Nickname})
						}
					}
					out = append(out, s)
				}
			}
		}
		if after = json.RawMessage(more(env.LoadMoreKey)); after == nil {
			break
		}
	}
	// A section of nothing, of a kind that changed, is not shown.
	kept := out[:0]
	for _, s := range out {
		if len(s.Items) > 0 {
			kept = append(kept, s)
		}
	}
	return kept, nil
}

// Category is a category of podcasts (分类).
type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CategoryTab is a list of a category: 全部, 近期热门, 为你推荐.
type CategoryTab struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Categories is the categories of podcasts.
func (c *Client) Categories(ctx context.Context) ([]Category, error) {
	env, err := c.call(ctx, "/v1/category/list-all", map[string]any{})
	if err != nil {
		return nil, err
	}
	return decode[[]Category]("/v1/category/list-all", env.Data)
}

// CategoryTabs is the lists of a category.
func (c *Client) CategoryTabs(ctx context.Context, id string) ([]CategoryTab, error) {
	env, err := c.call(ctx, "/v1/category/podcast/list-tabs", map[string]any{"categoryId": id})
	if err != nil {
		return nil, err
	}
	return decode[[]CategoryTab]("/v1/category/podcast/list-tabs", env.Data)
}

// CategoryPodcasts is a list of a category: its podcasts, each with an
// episode of it, ten at a time.
func (c *Client) CategoryPodcasts(ctx context.Context, id, tab string, after More) ([]FeedItem, More, error) {
	body := map[string]any{"categoryId": id, "tab": tab, "omitSubscribed": false}
	if after != nil {
		body["loadMoreKey"] = json.RawMessage(after)
	}
	items, next, err := list[struct {
		Podcast *Podcast `json:"podcast"`
		Episode *Episode `json:"episode"`
	}](ctx, c, "/v1/category/podcast/list-by-tab", body)
	var out []FeedItem
	for _, it := range items {
		if it.Podcast != nil {
			out = append(out, FeedItem{Podcast: it.Podcast, Episode: it.Episode})
		}
	}
	return out, next, err
}
