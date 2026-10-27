package xiaoyuzhou

import (
	"context"
	"encoding/json"
)

// discoveryPages is how many pages of the front page are read at most:
// the app's has four, each naming the next.
const discoveryPages = 6

// Discovery is the app's front page, in its order: what is recommended to
// the user, the editors' picks, new podcasts and what other listeners
// liked. Its charts, banners and links into the app are left out, and so
// is a section of nothing.
func (c *Client) Discovery(ctx context.Context) ([]Section, error) {
	var sections []Section
	var after Cursor
	for range discoveryPages {
		var modules []struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		next, err := c.request(ctx, "/v1/discovery-feed/list", after.into(map[string]any{"returnAll": "false"}), &modules)
		if err != nil {
			return nil, err
		}
		for _, m := range modules {
			for _, s := range moduleSections(m.Type, m.Data) {
				if len(s.Entries) > 0 {
					sections = append(sections, s)
				}
			}
		}
		if after = next; after == nil {
			break
		}
	}
	return sections, nil
}

// recommended is a list the recommender made: a title, and podcasts or
// episodes, each with why.
type recommended struct {
	Title  string `json:"title"`
	Target []struct {
		Recommendation string   `json:"recommendation"`
		Podcast        *Podcast `json:"podcast"`
		Episode        *Episode `json:"episode"`
	} `json:"target"`
}

func (r recommended) section(title string) Section {
	if r.Title != "" {
		title = r.Title
	}
	s := Section{Title: title}
	for _, t := range r.Target {
		if t.Podcast != nil || t.Episode != nil {
			s.Entries = append(s.Entries, Entry{Podcast: t.Podcast, Episode: t.Episode, Note: t.Recommendation})
		}
	}
	return s
}

// moduleSections is the sections of a module of the front page. Each kind
// is read alone: one whose data changed is dropped, not the page.
func moduleSections(kind string, data json.RawMessage) []Section {
	switch kind {
	case "DISCOVERY_EPISODE_RECOMMEND", "PILOT":
		var r recommended
		if json.Unmarshal(data, &r) == nil {
			return []Section{r.section("为你推荐")}
		}
	case "DISCOVERY_COLLECTION":
		var rs []recommended
		json.Unmarshal(data, &rs)
		var out []Section
		for _, r := range rs {
			out = append(out, r.section("为你精选"))
		}
		return out
	case "EDITOR_PICK":
		var day DayPicks
		if json.Unmarshal(data, &day) == nil {
			s := Section{Title: "编辑精选"}
			for _, p := range day.Picks {
				s.Entries = append(s.Entries, Entry{Episode: &p.Episode, Note: p.Comment.Text, By: p.Comment.Author.Nickname})
			}
			return []Section{s}
		}
	case "NEW_POWER":
		var v struct {
			Serial string `json:"serialNumber"`
			Items  []struct {
				Podcast
				Recommendation string `json:"recommendation"`
			} `json:"items"`
		}
		if json.Unmarshal(data, &v) == nil {
			s := Section{Title: "宝藏新节目"}
			if v.Serial != "" {
				s.Title += " · 第 " + v.Serial + " 期"
			}
			for _, it := range v.Items {
				s.Entries = append(s.Entries, Entry{Podcast: &it.Podcast, Note: it.Recommendation})
			}
			return []Section{s}
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
		if json.Unmarshal(data, &v) == nil {
			s := Section{Title: "TA们的喜欢"}
			for _, it := range v {
				if p := it.Pick; p.Episode != nil {
					s.Entries = append(s.Entries, Entry{Episode: p.Episode, Note: p.Story.Text, By: p.User.Nickname})
				}
			}
			return []Section{s}
		}
	}
	return nil
}

// Categories is the categories of podcasts.
func (c *Client) Categories(ctx context.Context) ([]Category, error) {
	var cats []Category
	_, err := c.request(ctx, "/v1/category/list-all", map[string]any{}, &cats)
	return cats, err
}

// CategoryTabs is the lists of a category: 全部, 近期热门, 为你推荐.
func (c *Client) CategoryTabs(ctx context.Context, id string) ([]Category, error) {
	var tabs []Category
	_, err := c.request(ctx, "/v1/category/podcast/list-tabs", map[string]any{"categoryId": id}, &tabs)
	return tabs, err
}

// CategoryPodcasts is a list of a category: its podcasts, each with its
// newest episode, ten at a time.
func (c *Client) CategoryPodcasts(ctx context.Context, id, tab string, after Cursor) ([]Entry, Cursor, error) {
	var items []struct {
		Podcast *Podcast `json:"podcast"`
		Episode *Episode `json:"episode"`
	}
	body := map[string]any{"categoryId": id, "tab": tab, "omitSubscribed": false}
	next, err := c.request(ctx, "/v1/category/podcast/list-by-tab", after.into(body), &items)
	var entries []Entry
	for _, it := range items {
		if it.Podcast != nil {
			entries = append(entries, Entry{Podcast: it.Podcast, Episode: it.Episode})
		}
	}
	return entries, next, err
}
