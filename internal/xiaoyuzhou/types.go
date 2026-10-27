package xiaoyuzhou

import "time"

// Picture is an image of the CDN, at the sizes it serves.
type Picture struct {
	PicURL       string `json:"picUrl"`
	ThumbnailURL string `json:"thumbnailUrl"` // 150 px
	SmallPicURL  string `json:"smallPicUrl"`  // 300 px
	MiddlePicURL string `json:"middlePicUrl"` // 800 px
}

// URL is the picture at side pixels or larger, as far as the CDN has one,
// else the original.
func (p Picture) URL(side int) string {
	u := p.MiddlePicURL
	switch {
	case side <= 150:
		u = p.ThumbnailURL
	case side <= 300:
		u = p.SmallPicURL
	}
	if u == "" {
		return p.PicURL
	}
	return u
}

// User is an account: the one logged in, a host, or a listener.
type User struct {
	UID      string `json:"uid"`
	Nickname string `json:"nickname"`
	Avatar   struct {
		Picture Picture `json:"picture"`
	} `json:"avatar"`
}

// Podcast is a show (节目).
type Podcast struct {
	Pid               string  `json:"pid"`
	Title             string  `json:"title"`
	Author            string  `json:"author"`
	Brief             string  `json:"brief"`
	Description       string  `json:"description"`
	SubscriptionCount int64   `json:"subscriptionCount"`
	EpisodeCount      int     `json:"episodeCount"`
	Image             Picture `json:"image"`
	Podcasters        []User  `json:"podcasters"`
}

// Episode is an episode (单集) of a podcast.
type Episode struct {
	Eid            string    `json:"eid"`
	Pid            string    `json:"pid"`
	Title          string    `json:"title"`
	Description    string    `json:"description"`
	Duration       int       `json:"duration"` // seconds
	PubDate        time.Time `json:"pubDate"`
	PlayCount      int64     `json:"playCount"`
	CommentCount   int64     `json:"commentCount"`
	IsPrivateMedia bool      `json:"isPrivateMedia"`
	PayType        string    `json:"payType"` // FREE, or how it is sold
	Image          Picture   `json:"image"`
	Media          struct {
		Size     int64  `json:"size"`
		MimeType string `json:"mimeType"`
		Source   struct {
			URL string `json:"url"`
		} `json:"source"`
	} `json:"media"`
	Enclosure struct {
		URL string `json:"url"`
	} `json:"enclosure"`
	// Podcast is set in the lists that mix podcasts, not in one's own.
	Podcast *Podcast `json:"podcast"`
}

// Paid reports whether the episode is sold.
func (e Episode) Paid() bool { return e.PayType != "" && e.PayType != "FREE" }

// AudioURL is where the episode's sound is; "" for one bought, whose URL
// PrivateMediaURL gives.
func (e Episode) AudioURL() string {
	switch {
	case e.IsPrivateMedia:
		return ""
	case e.Media.Source.URL != "":
		return e.Media.Source.URL
	}
	return e.Enclosure.URL
}

// ChartKind is one of the charts of the day's episodes.
type ChartKind string

const (
	ChartHot      ChartKind = "HOT_EPISODES_IN_24_HOURS" // 最热榜
	ChartRising   ChartKind = "SKYROCKET_EPISODES"       // 锋芒榜
	ChartNewStars ChartKind = "NEW_STAR_EPISODES"        // 新星榜
)

// Chart is a chart's episodes, the first first, and how it is made.
type Chart struct {
	Title    string
	About    string
	Episodes []Episode
}

// Pick is an episode the editors picked, with what a listener said of it.
type Pick struct {
	Episode Episode `json:"episode"`
	Comment struct {
		Text   string `json:"text"`
		Author User   `json:"author"`
	} `json:"comment"`
}

// DayPicks is the picks of a day.
type DayPicks struct {
	Date  string `json:"date"` // 2026-10-27
	Picks []Pick `json:"picks"`
}

// Entry is a podcast or an episode a page of the app lists, with a line
// of why: what it is about, why it is recommended, or what a listener,
// By, said of it. An entry of a category has both: a podcast and its
// newest episode.
type Entry struct {
	Podcast *Podcast
	Episode *Episode
	Note    string
	By      string
}

// Section is a section of the app's front page.
type Section struct {
	Title   string
	Entries []Entry
}

// Category is a category of podcasts (分类), or one of its lists.
type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
