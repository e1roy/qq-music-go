package qqmusic

import (
	"fmt"
	"strings"
	"time"
)

// Singer is an artist of a song.
type Singer struct {
	ID   flexInt `json:"id"`
	Mid  string  `json:"mid"`
	Name string  `json:"name"`
}

// Album is the album of a song.
type Album struct {
	ID   flexInt `json:"id"`
	Mid  string  `json:"mid"`
	Name string  `json:"name"`
}

// File describes the audio files of a song.
type File struct {
	MediaMid string  `json:"media_mid"`
	Size128  flexInt `json:"size_128mp3"`
	Size320  flexInt `json:"size_320mp3"`
	SizeFlac flexInt `json:"size_flac"`
	SizeAAC  flexInt `json:"size_192aac"`
}

// Pay says what playing a song takes.
type Pay struct {
	PayPlay  int `json:"pay_play"`
	PayMonth int `json:"pay_month"`
}

// Song is a track, in the format every song list of the APIs shares.
type Song struct {
	ID       flexInt  `json:"id"`
	Type     int      `json:"type"`
	Mid      string   `json:"mid"`
	Name     string   `json:"name"`
	Title    string   `json:"title"`
	Subtitle string   `json:"subtitle"`
	Singers  []Singer `json:"singer"`
	Album    Album    `json:"album"`
	Interval int      `json:"interval"` // seconds
	File     File     `json:"file"`
	Pay      Pay      `json:"pay"`

	// A song from elsewhere, such as an episode of 小宇宙, plays in the
	// queue as QQ Music's do: Source names where it is from, URL is its
	// sound, empty for one to ask its source for, and Pic its cover.
	Source string `json:"source,omitempty"`
	URL    string `json:"url,omitempty"`
	Pic    string `json:"pic,omitempty"`
}

// Artists joins the names of the song's singers.
func (s Song) Artists() string {
	names := make([]string, len(s.Singers))
	for i, sg := range s.Singers {
		names[i] = sg.Name
	}
	return strings.Join(names, " / ")
}

// DisplayName is the name with its version, such as "(伴奏)", and its
// subtitle, as the clients show it.
func (s Song) DisplayName() string {
	name := s.Name
	if s.Title != s.Name && strings.HasPrefix(s.Title, s.Name) {
		name = s.Title
	}
	if s.Subtitle != "" {
		return name + " (" + s.Subtitle + ")"
	}
	return name
}

// VIP reports whether the song plays only for members.
func (s Song) VIP() bool { return s.Pay.PayPlay == 1 }

// Cover returns the URL of the album's cover at a size of 90, 150, 300,
// 500 or 800 pixels, or the first singer's photo if it has no album. A
// song from elsewhere has the picture it came with.
func (s Song) Cover(size int) string {
	if s.Pic != "" {
		return s.Pic
	}
	if s.Album.Mid != "" {
		return AlbumCover(s.Album.Mid, size)
	}
	if len(s.Singers) > 0 && s.Singers[0].Mid != "" {
		return SingerPhoto(s.Singers[0].Mid, size)
	}
	return ""
}

// Duration formats the length as m:ss, or h:mm:ss for an episode of an
// hour or more.
func (s Song) Duration() string {
	if s.Interval >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s.Interval/3600, s.Interval/60%60, s.Interval%60)
	}
	return fmt.Sprintf("%d:%02d", s.Interval/60, s.Interval%60)
}

// AlbumCover is the URL of an album's cover.
func AlbumCover(mid string, size int) string {
	return fmt.Sprintf("https://y.qq.com/music/photo_new/T002R%dx%dM000%s.jpg", size, size, mid)
}

// SingerPhoto is the URL of a singer's photo.
func SingerPhoto(mid string, size int) string {
	return fmt.Sprintf("https://y.qq.com/music/photo_new/T001R%dx%dM000%s.jpg", size, size, mid)
}

// Playlist is a playlist (歌单) as lists show it.
type Playlist struct {
	ID        int64
	DirID     int
	Title     string
	Cover     string
	Creator   string
	Desc      string
	PlayCount int64
	SongCount int
}

// FeedShelf is a section of the front page made for the user, such as
// 「e1roy」，这是你的音乐盲盒: songs or playlists, under a title the server
// words.
type FeedShelf struct {
	Title     string
	Songs     []Song
	Playlists []Playlist
}

// Toplist is a chart (排行榜).
type Toplist struct {
	TopID     int
	Title     string
	Intro     string
	Period    string
	Cover     string
	ListenNum int64
	Preview   []string // "song - singer" of its first songs
}

// ToplistGroup is a group of charts, such as 巅峰榜.
type ToplistGroup struct {
	Name  string
	Lists []Toplist
}

// SingerInfo is a singer's page: who they are and how many songs they have.
type SingerInfo struct {
	Mid         string
	Name        string
	ForeignName string
	Desc        string
	SongCount   int
}

// FoundSinger is a singer a search found.
type FoundSinger struct {
	ID         int64
	Mid        string
	Name       string
	Photo      string
	SongCount  int
	AlbumCount int
	MVCount    int
}

// DirectKind is what a search goes straight to.
type DirectKind int

const (
	// DirectSinger is a singer's page.
	DirectSinger DirectKind = iota
	// DirectAlbum is an album.
	DirectAlbum
)

// Direct is what a search goes straight to (直达), shown above the songs
// it found.
type Direct struct {
	Kind  DirectKind
	Mid   string // of the singer or the album
	Label string // 歌手, 专辑, 综艺…
	Name  string
	Pic   string
	// A singer's.
	SongCount  int
	AlbumCount int
	MVCount    int
	// A line more: how many follow a singer, or an album's singer, songs
	// and date.
	Desc string
}

// AlbumInfo is an album as lists show it, or with its details.
type AlbumInfo struct {
	ID          int64
	Mid         string
	Name        string
	Type        string // 录音室专辑, EP, Single…
	PublishDate string
	SingerName  string // the names of its singers, "毛不易/李琦"
	// With the details only.
	Singers []Singer
	Company string
	Desc    string
}

// IsPodcast reports whether the album is long audio: a podcast (节目) of
// 小宇宙 or QQ Music's own, or an audiobook. Its songs are episodes.
func (a AlbumInfo) IsPodcast() bool { return a.Type == "人声音频" }

// Episode is a song as an album's list gives it, with what only the list
// has: an episode, for a podcast.
type Episode struct {
	Song
	Index       int    // its number in the album, from 1
	ListenCount int64  // how many times it was played
	UploadTime  string // "2022-01-21"
}

// EpisodeSongs returns the songs of episodes, to play.
func EpisodeSongs(eps []Episode) []Song {
	songs := make([]Song, len(eps))
	for i, e := range eps {
		songs[i] = e.Song
	}
	return songs
}

// Show is a podcast (节目) as cards show it. Some cards know only its id:
// its mid comes with the album.
type Show struct {
	ID        int64  `json:"id"`
	Mid       string `json:"mid"`
	Title     string `json:"title"`
	Host      string `json:"host"`
	Cover     string `json:"cover"`
	Episodes  int    `json:"episodes"` // 0 if not known
	Intro     string `json:"intro"`
	PlayCount int64  `json:"play_count"`
}

// PodcastArea is the page of QQ Music's podcasts (播客专区): shelves of
// shows, of hosts, or of episodes.
type PodcastArea struct {
	Title   string
	Desc    string
	Shelves []PodcastShelf
}

// PodcastShelf is a section of PodcastArea. Each has one kind of card:
// shows, hosts or episodes.
type PodcastShelf struct {
	Title string
	// Hosts says its shows are people (明星播客), each card their photo and
	// name, opening their show, of which only the id is known.
	Hosts    bool
	Shows    []Show
	Episodes []Song
}

// User is the profile of a logged-in user.
type User struct {
	Nick   string
	Avatar string
	VIP    bool
	// VIPEnd is when the membership ends: the zero time for a user who is
	// no member, or one whose end the service left out.
	VIPEnd time.Time
}

// Quality is a file format of a song. A higher one sounds better, so a
// result below the quality asked for means it fell back.
type Quality int

const (
	// AAC is an m4a file, only played when a song has no MP3: it is not
	// one to choose.
	AAC Quality = iota - 1
	// Standard is MP3 at 128 kbps.
	Standard
	// High is MP3 at 320 kbps.
	High
	// Lossless is FLAC.
	Lossless
	// Best is the best file a song has: a choice, not a format.
	Best
)

func (q Quality) String() string {
	switch q {
	case AAC:
		return "AAC"
	case High:
		return "HQ 320k"
	case Lossless:
		return "无损 FLAC"
	case Best:
		return "最高音质"
	}
	return "标准 128k"
}

// Best returns the best quality of the files, Standard if none is known.
func (f File) Best() Quality {
	switch {
	case f.SizeFlac > 0:
		return Lossless
	case f.Size320 > 0:
		return High
	}
	return Standard
}

// Size returns the bytes of the file of q, 0 if there is none; for Best,
// of the best one.
func (f File) Size(q Quality) int64 {
	switch q {
	case Best:
		return f.Size(f.Best())
	case Lossless:
		return int64(f.SizeFlac)
	case High:
		return int64(f.Size320)
	case Standard:
		return int64(f.Size128)
	case AAC:
		return int64(f.SizeAAC)
	}
	return 0
}

// songFile is a file SongURL may ask for.
type songFile struct {
	q           Quality
	prefix, ext string
}

// files returns the files of s to try for q, the best first.
func (f File) files(q Quality) []songFile {
	var fs []songFile
	if q >= Lossless && f.SizeFlac > 0 {
		fs = append(fs, songFile{Lossless, "F000", ".flac"})
	}
	if q >= High && f.Size320 > 0 {
		fs = append(fs, songFile{High, "M800", ".mp3"})
	}
	fs = append(fs, songFile{Standard, "M500", ".mp3"})
	if f.SizeAAC > 0 {
		fs = append(fs, songFile{AAC, "C600", ".m4a"})
	}
	return append(fs, songFile{AAC, "C400", ".m4a"})
}

// Comment is a comment on a song.
type Comment struct {
	ID       string
	SeqNo    string // where the page after it starts
	Nick     string
	Avatar   string
	Content  string
	Time     time.Time // zero if not known
	Location string    // where it was written from, such as 广东
	Likes    int64
	Replies  int
	Reply    *CommentReply // the first reply, shown under it
}

// CommentReply is a reply to a comment.
type CommentReply struct {
	Nick    string
	Content string
}
