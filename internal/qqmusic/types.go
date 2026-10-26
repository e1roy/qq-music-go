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
// 500 or 800 pixels, or the first singer's photo if it has no album.
func (s Song) Cover(size int) string {
	if s.Album.Mid != "" {
		return AlbumCover(s.Album.Mid, size)
	}
	if len(s.Singers) > 0 && s.Singers[0].Mid != "" {
		return SingerPhoto(s.Singers[0].Mid, size)
	}
	return ""
}

// Duration formats the length as m:ss.
func (s Song) Duration() string {
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

// User is the profile of a logged-in user.
type User struct {
	Nick   string
	Avatar string
	VIP    bool
}

// Quality is a file format of a song.
type Quality int

const (
	// Standard is MP3 at 128 kbps.
	Standard Quality = iota
	// High is MP3 at 320 kbps.
	High
)

func (q Quality) prefix() (string, string) {
	if q == High {
		return "M800", ".mp3"
	}
	return "M500", ".mp3"
}

func (q Quality) String() string {
	if q == High {
		return "HQ 320k"
	}
	return "标准 128k"
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
