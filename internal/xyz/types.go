package xyz

import "time"

// Image is a picture of the CDN, at the sizes it serves.
type Image struct {
	PicURL       string `json:"picUrl"`
	ThumbnailURL string `json:"thumbnailUrl"` // 150 px
	SmallPicURL  string `json:"smallPicUrl"`  // 300 px
	MiddlePicURL string `json:"middlePicUrl"` // 800 px
}

// URL is the picture at side pixels or more, as far as it goes.
func (im Image) URL(side int) string {
	var u string
	switch {
	case side <= 150:
		u = im.ThumbnailURL
	case side <= 300:
		u = im.SmallPicURL
	default:
		u = im.MiddlePicURL
	}
	if u == "" {
		u = im.PicURL
	}
	return u
}

// User is an account: the one logged in, or a host of a podcast.
type User struct {
	UID      string `json:"uid"`
	Nickname string `json:"nickname"`
	Avatar   struct {
		Picture Image `json:"picture"`
	} `json:"avatar"`
}

// Podcast is a show (节目).
type Podcast struct {
	Pid               string `json:"pid"`
	Title             string `json:"title"`
	Author            string `json:"author"`
	Brief             string `json:"brief"`
	Description       string `json:"description"`
	SubscriptionCount int64  `json:"subscriptionCount"`
	EpisodeCount      int    `json:"episodeCount"`
	Image             Image  `json:"image"`
	Podcasters        []User `json:"podcasters"`
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
	PayType        string    `json:"payType"` // FREE, or what it costs
	Image          Image     `json:"image"`
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
	// Its podcast, in the lists that mix podcasts; not in a podcast's own.
	Podcast *Podcast `json:"podcast"`
}

// Audio is the URL of the episode's sound, empty for one bought, which
// PrivateMedia gives.
func (e Episode) Audio() string {
	if e.IsPrivateMedia {
		return ""
	}
	if e.Media.Source.URL != "" {
		return e.Media.Source.URL
	}
	return e.Enclosure.URL
}

// TopCategory is a chart of episodes.
type TopCategory string

const (
	TopHot      TopCategory = "HOT_EPISODES_IN_24_HOURS" // 最热榜
	TopRising   TopCategory = "SKYROCKET_EPISODES"       // 锋芒榜
	TopNewStars TopCategory = "NEW_STAR_EPISODES"        // 新星榜
)

// TopList is a chart: its episodes, the first first.
type TopList struct {
	Title       string
	Information string // how it is made
	Episodes    []Episode
}

// Pick is an episode 小宇宙's editors picked, with what a listener said of
// it.
type Pick struct {
	Episode Episode `json:"episode"`
	Comment struct {
		Text   string `json:"text"`
		Author User   `json:"author"`
	} `json:"comment"`
}

// PickDay is the picks of a day.
type PickDay struct {
	Date  string `json:"date"` // 2026-10-27
	Picks []Pick `json:"picks"`
}

// SearchKind is what a search looks for.
type SearchKind string

const (
	SearchPodcasts SearchKind = "PODCAST"
	SearchEpisodes SearchKind = "EPISODE"
)
