package qqmusic

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Search finds songs; page starts at 1. It may go straight to a singer
// or an album as well, as the clients show above the songs.
func (c *Client) Search(ctx context.Context, query string, page, num int) ([]Song, int, *Direct, error) {
	var out struct {
		Meta struct {
			Sum flexInt `json:"sum"`
		} `json:"meta"`
		Body struct {
			Song struct {
				List []Song `json:"list"`
			} `json:"song"`
			Zhida json.RawMessage `json:"zhida"`
		} `json:"body"`
	}
	err := c.search(ctx, query, page, num, 0, &out)
	return out.Body.Song.List, int(out.Meta.Sum), parseDirect(out.Body.Zhida), err
}

// parseDirect reads the zhida (直达) of a search: nil if there is none, or
// none this client can open. Being an extra, it never fails the search.
func parseDirect(b json.RawMessage) *Direct {
	var z struct {
		List []struct {
			Type  int    `json:"type"`
			Title string `json:"title"`
			Desc  string `json:"desciption"`
			Pic   string `json:"pic"`
			Info  struct {
				Mid    string  `json:"mid"`
				Songs  flexInt `json:"song_num"`
				Albums flexInt `json:"album_num"`
				MVs    flexInt `json:"mv_num"`
				Extra  string  `json:"extra_desc"`
			} `json:"custom_info"`
		} `json:"list"`
	}
	if len(b) == 0 || json.Unmarshal(b, &z) != nil || len(z.List) == 0 {
		return nil
	}
	it := z.List[0]
	if it.Info.Mid == "" {
		return nil
	}
	// The title is the kind and the name: "歌手: 毛不易", "专辑: 范特西".
	title := strings.ReplaceAll(html.UnescapeString(it.Title), "：", ":")
	label, name, ok := strings.Cut(title, ":")
	if !ok {
		label, name = "", label
	}
	d := &Direct{Mid: it.Info.Mid, Label: strings.TrimSpace(label), Name: strings.TrimSpace(name)}
	switch {
	case it.Type == 2:
		d.Kind = DirectAlbum
		d.Pic = AlbumCover(d.Mid, 300)
		// "周杰伦  10首  2001-09-14"
		d.Desc = strings.Join(strings.Fields(html.UnescapeString(it.Desc)), " · ")
	case it.Info.Songs > 0:
		// Singers, and shows such as 综艺: 明日创作计划, which are
		// singers too.
		d.Kind = DirectSinger
		d.Pic = SingerPhoto(d.Mid, 300)
		d.SongCount, d.AlbumCount, d.MVCount = int(it.Info.Songs), int(it.Info.Albums), int(it.Info.MVs)
		d.Desc = html.UnescapeString(it.Info.Extra)
	default:
		return nil
	}
	if d.Label == "" {
		d.Label = map[DirectKind]string{DirectSinger: "歌手", DirectAlbum: "专辑"}[d.Kind]
	}
	return d
}

// SearchPlaylists finds playlists; page starts at 1.
func (c *Client) SearchPlaylists(ctx context.Context, query string, page, num int) ([]Playlist, int, error) {
	var out struct {
		Meta struct {
			Sum flexInt `json:"sum"`
		} `json:"meta"`
		Body struct {
			Songlist struct {
				List []struct {
					DissID  flexInt `json:"dissid"`
					Name    string  `json:"dissname"`
					Img     string  `json:"imgurl"`
					Intro   string  `json:"introduction"`
					Listen  flexInt `json:"listennum"`
					Count   flexInt `json:"song_count"`
					Creator struct {
						Name string `json:"name"`
					} `json:"creator"`
				} `json:"list"`
			} `json:"songlist"`
		} `json:"body"`
	}
	err := c.search(ctx, query, page, num, 3, &out)
	pls := make([]Playlist, 0, len(out.Body.Songlist.List))
	for _, p := range out.Body.Songlist.List {
		pls = append(pls, Playlist{
			ID: int64(p.DissID), Title: html.UnescapeString(p.Name), Cover: p.Img, Desc: cleanIntro(p.Intro),
			Creator: html.UnescapeString(p.Creator.Name), PlayCount: int64(p.Listen), SongCount: int(p.Count),
		})
	}
	return pls, int(out.Meta.Sum), err
}

// SearchSingers finds singers; page starts at 1.
func (c *Client) SearchSingers(ctx context.Context, query string, page, num int) ([]FoundSinger, int, error) {
	var out struct {
		Meta struct {
			Sum flexInt `json:"sum"`
		} `json:"meta"`
		Body struct {
			Singer struct {
				List []struct {
					ID     flexInt `json:"singerID"`
					Mid    string  `json:"singerMID"`
					Name   string  `json:"singerName"`
					Pic    string  `json:"singerPic"`
					Songs  flexInt `json:"songNum"`
					Albums flexInt `json:"albumNum"`
					MVs    flexInt `json:"mvNum"`
				} `json:"list"`
			} `json:"singer"`
		} `json:"body"`
	}
	err := c.search(ctx, query, page, num, 1, &out)
	sgs := make([]FoundSinger, 0, len(out.Body.Singer.List))
	for _, s := range out.Body.Singer.List {
		photo := s.Pic
		if s.Mid != "" {
			// Larger than the 150px of singerPic.
			photo = SingerPhoto(s.Mid, 300)
		}
		sgs = append(sgs, FoundSinger{
			ID: int64(s.ID), Mid: s.Mid, Name: html.UnescapeString(s.Name), Photo: photo,
			SongCount: int(s.Songs), AlbumCount: int(s.Albums), MVCount: int(s.MVs),
		})
	}
	return sgs, int(out.Meta.Sum), err
}

// search asks the desktop search for one kind of result: 0 songs, 1
// singers, 2 albums, 3 playlists, 4 MVs, 7 songs by their lyrics, 8 users.
func (c *Client) search(ctx context.Context, query string, page, num, kind int, out any) error {
	param := map[string]any{"query": query, "num_per_page": num, "page_num": page, "search_type": kind}
	err := c.Call(ctx, Desktop, "music.search.SearchCgiService", "DoSearchForQQMusicDesktop", param, out)
	// The service turns some requests away now and then (code 2001): the
	// next one goes through.
	for try := 0; try < 2; try++ {
		var ae *APIError
		if !errors.As(err, &ae) || ae.Code != 2001 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(400 * time.Millisecond):
		}
		err = c.Call(ctx, Desktop, "music.search.SearchCgiService", "DoSearchForQQMusicDesktop", param, out)
	}
	return err
}

// Suggest returns completions of a partial search query.
func (c *Client) Suggest(ctx context.Context, key string) ([]string, error) {
	b, err := c.get(ctx, "https://c.y.qq.com/splcloud/fcgi-bin/smartbox_new.fcg?format=json&inCharset=utf-8&outCharset=utf-8&key="+url.QueryEscape(key), "")
	if err != nil {
		return nil, err
	}
	var out struct {
		Data struct {
			Song struct {
				Itemlist []struct{ Name, Singer string } `json:"itemlist"`
			} `json:"song"`
			Singer struct {
				Itemlist []struct{ Name string } `json:"itemlist"`
			} `json:"singer"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	var s []string
	for _, it := range out.Data.Singer.Itemlist {
		s = append(s, it.Name)
	}
	for _, it := range out.Data.Song.Itemlist {
		s = append(s, it.Name+" "+it.Singer)
	}
	return s, nil
}

// HotKeys returns what people search for now.
func (c *Client) HotKeys(ctx context.Context) ([]string, error) {
	var out struct {
		VecHotkey []struct {
			Query string `json:"query"`
		} `json:"vec_hotkey"`
	}
	err := c.Call(ctx, Web, "tencent_musicsoso_hotkey.HotkeyService", "GetHotkeyForQQMusicMobile",
		map[string]any{"searchid": "1", "remoteplace": "txt.yqq.top"}, &out)
	keys := make([]string, 0, len(out.VecHotkey))
	for _, k := range out.VecHotkey {
		keys = append(keys, k.Query)
	}
	return keys, err
}

type rawToplist struct {
	TopID       flexInt `json:"topId"`
	Title       string  `json:"title"`
	Intro       string  `json:"intro"`
	Period      string  `json:"period"`
	ListenNum   flexInt `json:"listenNum"`
	FrontPicURL string  `json:"frontPicUrl"`
	HeadPicURL  string  `json:"headPicUrl"`
	Song        []struct {
		Title      string `json:"title"`
		SingerName string `json:"singerName"`
	} `json:"song"`
}

func (r rawToplist) toplist() Toplist {
	t := Toplist{
		TopID: int(r.TopID), Title: r.Title, Period: r.Period, ListenNum: int64(r.ListenNum),
		Intro: cleanIntro(r.Intro), Cover: r.FrontPicURL,
	}
	if t.Cover == "" {
		t.Cover = r.HeadPicURL
	}
	for _, s := range r.Song {
		t.Preview = append(t.Preview, s.Title+" - "+s.SingerName)
	}
	return t
}

func cleanIntro(s string) string {
	s = strings.NewReplacer("<br>", "\n", "<br/>", "\n", "<br />", "\n").Replace(s)
	return strings.TrimSpace(html.UnescapeString(s))
}

// Toplists returns all charts by group.
func (c *Client) Toplists(ctx context.Context) ([]ToplistGroup, error) {
	var out struct {
		Group []struct {
			GroupName string       `json:"groupName"`
			Toplist   []rawToplist `json:"toplist"`
		} `json:"group"`
	}
	if err := c.Call(ctx, Web, "musicToplist.ToplistInfoServer", "GetAll", nil, &out); err != nil {
		return nil, err
	}
	groups := make([]ToplistGroup, 0, len(out.Group))
	for _, g := range out.Group {
		tg := ToplistGroup{Name: g.GroupName}
		for _, t := range g.Toplist {
			tg.Lists = append(tg.Lists, t.toplist())
		}
		groups = append(groups, tg)
	}
	return groups, nil
}

// ToplistDetail returns a chart and its songs.
func (c *Client) ToplistDetail(ctx context.Context, topID, offset, num int) (Toplist, []Song, error) {
	var out struct {
		Data         rawToplist `json:"data"`
		SongInfoList []Song     `json:"songInfoList"`
	}
	err := c.Call(ctx, Web, "musicToplist.ToplistInfoServer", "GetDetail", map[string]any{
		"topId": topID, "offset": offset, "num": num, "period": "",
	}, &out)
	return out.Data.toplist(), out.SongInfoList, err
}

// RecommendPlaylists returns size playlists picked for the user, from the
// from-th, as the 推荐 tab of the client's playlist square. Without a login
// they are picked for anyone. more is false at the last of those picked:
// what follows is only the latest made.
func (c *Client) RecommendPlaylists(ctx context.Context, from, size int) (pls []Playlist, more bool, err error) {
	var out struct {
		HasMore bool `json:"HasMore"`
		List    []struct {
			Playlist struct {
				Basic struct {
					Tid     flexInt `json:"tid"`
					Title   string  `json:"title"`
					Desc    string  `json:"desc"`
					PlayCnt flexInt `json:"play_cnt"`
					SongCnt flexInt `json:"song_cnt"`
					Cover   struct {
						Default string `json:"default_url"`
						Medium  string `json:"medium_url"`
					} `json:"cover"`
					Creator struct {
						Nick string `json:"nick"`
					} `json:"creator"`
				} `json:"basic"`
			} `json:"Playlist"`
		} `json:"List"`
	}
	if err := c.Call(ctx, Web, "music.playlist.PlaylistSquare", "GetRecommendFeed", map[string]any{"From": from, "Size": size}, &out); err != nil {
		return nil, false, err
	}
	pls = make([]Playlist, 0, len(out.List))
	for _, it := range out.List {
		b := it.Playlist.Basic
		cover := b.Cover.Default
		if cover == "" {
			cover = b.Cover.Medium
		}
		pls = append(pls, Playlist{ID: int64(b.Tid), Title: b.Title, Desc: b.Desc, Cover: cover, Creator: b.Creator.Nick, PlayCount: int64(b.PlayCnt), SongCount: int(b.SongCnt)})
	}
	return pls, out.HasMore, nil
}

// PlaylistCategory is a tag of the playlist square.
type PlaylistCategory struct {
	ID   int
	Name string
}

// PlaylistCategories are the tags PlaylistsByCategory takes.
var PlaylistCategories = []PlaylistCategory{
	{10000000, "全部"}, {6, "流行"}, {165, "国语"}, {167, "英语"}, {8, "说唱"}, {15, "电子"},
	{11, "民谣"}, {64, "古风"}, {19, "摇滚"}, {39, "ACG"}, {52, "伤感"}, {58, "治愈"},
	{122, "学习"}, {78, "运动"}, {59, "夜晚"}, {141, "经典"},
}

// PlaylistsByCategory returns the playlists of a tag of the square, the
// most played first; page starts at 1.
func (c *Client) PlaylistsByCategory(ctx context.Context, id, page, size int) ([]Playlist, int, error) {
	var out struct {
		Total     flexInt `json:"total"`
		VPlaylist []struct {
			Tid         flexInt `json:"tid"`
			Title       string  `json:"title"`
			Desc        string  `json:"desc"`
			AccessNum   flexInt `json:"access_num"`
			CoverMedium string  `json:"cover_url_medium"`
			CoverBig    string  `json:"cover_url_big"`
			CreatorInfo struct {
				Nick string `json:"nick"`
			} `json:"creator_info"`
		} `json:"v_playlist"`
	}
	err := c.Call(ctx, Web, "playlist.PlayListPlazaServer", "get_playlist_by_tag", map[string]any{
		"id": id, "sin": (page - 1) * size, "size": size, "order": 5, "cur_page": page,
	}, &out)
	pls := make([]Playlist, 0, len(out.VPlaylist))
	for _, p := range out.VPlaylist {
		cover := p.CoverMedium
		if cover == "" {
			cover = p.CoverBig
		}
		pls = append(pls, Playlist{ID: int64(p.Tid), Title: p.Title, Desc: p.Desc, Cover: cover, Creator: p.CreatorInfo.Nick, PlayCount: int64(p.AccessNum)})
	}
	return pls, int(out.Total), err
}

// PlaylistDetail returns a playlist and songs of it from begin. A
// playlist of the logged-in user's own, as 我喜欢 (dirid 201), takes its
// dirid and an id of 0.
func (c *Client) PlaylistDetail(ctx context.Context, id int64, dirid, begin, num int) (Playlist, []Song, error) {
	param := map[string]any{
		"disstid": id, "dirid": dirid, "tag": 1, "song_begin": begin, "song_num": num,
		"userinfo": 1, "orderlist": 1, "onlysonglist": 0,
	}
	if id == 0 {
		cr := c.Credential()
		if cr.EncryptUin != "" {
			param["enc_host_uin"] = cr.EncryptUin
		}
	}
	var out struct {
		Dirinfo struct {
			ID        flexInt `json:"id"`
			Dirid     flexInt `json:"dirid"`
			Title     string  `json:"title"`
			Picurl    string  `json:"picurl"`
			Desc      string  `json:"desc"`
			HostNick  string  `json:"host_nick"`
			Listennum flexInt `json:"listennum"`
			Songnum   flexInt `json:"songnum"`
		} `json:"dirinfo"`
		Songlist     []Song  `json:"songlist"`
		TotalSongNum flexInt `json:"total_song_num"`
	}
	err := c.Call(ctx, Web, "music.srfDissInfo.DissInfo", "CgiGetDiss", param, &out)
	d := out.Dirinfo
	p := Playlist{
		ID: int64(d.ID), DirID: int(d.Dirid), Title: d.Title, Cover: d.Picurl, Desc: cleanIntro(d.Desc),
		Creator: d.HostNick, PlayCount: int64(d.Listennum), SongCount: int(out.TotalSongNum),
	}
	if p.SongCount == 0 {
		p.SongCount = int(d.Songnum)
	}
	return p, out.Songlist, err
}

// AllPlaylistSongs returns every song of a playlist, in pages.
func (c *Client) AllPlaylistSongs(ctx context.Context, id int64, dirid int) (Playlist, []Song, error) {
	const page = 500
	p, songs, err := c.PlaylistDetail(ctx, id, dirid, 0, page)
	for err == nil && len(songs) < p.SongCount {
		var more []Song
		_, more, err = c.PlaylistDetail(ctx, id, dirid, len(songs), page)
		if len(more) == 0 {
			break
		}
		songs = append(songs, more...)
	}
	return p, songs, err
}

// SingerDetail returns who a singer is.
func (c *Client) SingerDetail(ctx context.Context, mid string) (SingerInfo, error) {
	var out struct {
		SingerList []struct {
			BasicInfo struct {
				Mid  string `json:"singer_mid"`
				Name string `json:"name"`
			} `json:"basic_info"`
			ExInfo struct {
				Desc        string `json:"desc"`
				ForeignName string `json:"foreign_name"`
			} `json:"ex_info"`
		} `json:"singer_list"`
	}
	err := c.Call(ctx, Web, "musichall.singer_info_server", "GetSingerDetail",
		map[string]any{"singer_mids": []string{mid}, "ex_singer": 1, "wiki_singer": 0, "group_singer": 0, "pic": 0, "photos": 0}, &out)
	if err != nil || len(out.SingerList) == 0 {
		return SingerInfo{Mid: mid}, err
	}
	s := out.SingerList[0]
	return SingerInfo{Mid: s.BasicInfo.Mid, Name: s.BasicInfo.Name, ForeignName: s.ExInfo.ForeignName, Desc: cleanIntro(s.ExInfo.Desc)}, nil
}

// Order is how the songs or albums of a singer are sorted.
type Order int

const (
	// Popular is the most popular first.
	Popular Order = iota
	// Latest is the newest first.
	Latest
)

// param is the order as the services take it.
func (o Order) param() int {
	if o == Latest {
		return 0
	}
	return 1
}

// SingerSongs returns songs of a singer from begin, in order, and how
// many there are. The service gives 30 at a time, whatever num is.
func (c *Client) SingerSongs(ctx context.Context, mid string, order Order, begin, num int) ([]Song, int, error) {
	var out struct {
		TotalNum flexInt `json:"totalNum"`
		SongList []struct {
			SongInfo Song `json:"songInfo"`
		} `json:"songList"`
	}
	err := c.Call(ctx, Web, "musichall.song_list_server", "GetSingerSongList",
		map[string]any{"singerMid": mid, "order": order.param(), "begin": begin, "number": num}, &out)
	songs := make([]Song, 0, len(out.SongList))
	for _, s := range out.SongList {
		songs = append(songs, s.SongInfo)
	}
	return songs, int(out.TotalNum), err
}

// SingerAlbums returns albums of a singer from begin, in order, and how
// many there are.
func (c *Client) SingerAlbums(ctx context.Context, mid string, order Order, begin, num int) ([]AlbumInfo, int, error) {
	var out struct {
		Total     flexInt `json:"total"`
		AlbumList []struct {
			ID          flexInt `json:"albumID"`
			Mid         string  `json:"albumMid"`
			Name        string  `json:"albumName"`
			Type        string  `json:"albumType"`
			PublishDate string  `json:"publishDate"`
			SingerName  string  `json:"singerName"`
		} `json:"albumList"`
	}
	err := c.Call(ctx, Web, "music.musichallAlbum.AlbumListServer", "GetAlbumList",
		map[string]any{"singerMid": mid, "order": order.param(), "begin": begin, "num": num}, &out)
	albums := make([]AlbumInfo, 0, len(out.AlbumList))
	for _, al := range out.AlbumList {
		albums = append(albums, AlbumInfo{ID: int64(al.ID), Mid: al.Mid, Name: al.Name, Type: al.Type, PublishDate: al.PublishDate, SingerName: al.SingerName})
	}
	return albums, int(out.Total), err
}

// rawAlbum is the data of GetAlbumDetail.
type rawAlbum struct {
	BasicInfo struct {
		ID          flexInt `json:"albumID"`
		Mid         string  `json:"albumMid"`
		Name        string  `json:"albumName"`
		Type        string  `json:"albumType"`
		PublishDate string  `json:"publishDate"`
		Desc        string  `json:"desc"`
	} `json:"basicInfo"`
	Company struct {
		Name string `json:"name"`
	} `json:"company"`
	Singer struct {
		SingerList []Singer `json:"singerList"`
	} `json:"singer"`
}

func (r rawAlbum) album() AlbumInfo {
	b := r.BasicInfo
	al := AlbumInfo{ID: int64(b.ID), Mid: b.Mid, Name: b.Name, Type: b.Type, PublishDate: b.PublishDate,
		Singers: r.Singer.SingerList, Company: r.Company.Name, Desc: cleanIntro(b.Desc)}
	names := make([]string, len(al.Singers))
	for i, sg := range al.Singers {
		names[i] = sg.Name
	}
	al.SingerName = strings.Join(names, "/")
	return al
}

// albumParam asks for an album by its mid, or by its id when the mid is
// not known, as of the podcasts some cards show.
func albumParam(mid string, id int64) map[string]any {
	if mid == "" {
		return map[string]any{"albumID": id}
	}
	return map[string]any{"albumMid": mid}
}

// Album returns what an album is, by its mid or else its id.
func (c *Client) Album(ctx context.Context, mid string, id int64) (AlbumInfo, error) {
	var out rawAlbum
	if err := c.Call(ctx, Web, "music.musichallAlbum.AlbumInfoServer", "GetAlbumDetail", albumParam(mid, id), &out); err != nil {
		return AlbumInfo{ID: id, Mid: mid}, err
	}
	return out.album(), nil
}

// AlbumSongs returns songs of an album from begin, and how many it has:
// a podcast's newest first.
func (c *Client) AlbumSongs(ctx context.Context, mid string, begin, num int) ([]Episode, int, error) {
	var out struct {
		TotalNum flexInt `json:"totalNum"`
		SongList []struct {
			SongInfo struct {
				Song
				Index flexInt `json:"index_album"`
			} `json:"songInfo"`
			ListenCount flexInt `json:"listenCount"`
			UploadTime  string  `json:"uploadTime"`
		} `json:"songList"`
	}
	err := c.Call(ctx, Web, "music.musichallAlbum.AlbumSongList", "GetAlbumSongList",
		map[string]any{"albumMid": mid, "begin": begin, "num": num, "order": 2}, &out)
	eps := make([]Episode, 0, len(out.SongList))
	for _, s := range out.SongList {
		eps = append(eps, Episode{Song: s.SongInfo.Song, Index: int(s.SongInfo.Index), ListenCount: int64(s.ListenCount), UploadTime: s.UploadTime})
	}
	return eps, int(out.TotalNum), err
}

// PodcastPage is how many episodes of a podcast AlbumDetail and a page of
// them get: a podcast may have hundreds.
const PodcastPage = 30

// AlbumDetail returns what an album is, by its mid or else its id, with its
// songs and how many it has: all of them, or a podcast's newest
// PodcastPage, the next of which AlbumSongs gets.
func (c *Client) AlbumDetail(ctx context.Context, mid string, id int64) (AlbumInfo, []Episode, int, error) {
	al, err := c.Album(ctx, mid, id)
	if err != nil {
		return al, nil, 0, err
	}
	if al.IsPodcast() {
		eps, total, err := c.AlbumSongs(ctx, al.Mid, 0, PodcastPage)
		return al, eps, total, err
	}
	const page = 100
	var eps []Episode
	for {
		more, total, err := c.AlbumSongs(ctx, al.Mid, len(eps), page)
		eps = append(eps, more...)
		if err != nil || len(more) == 0 || len(eps) >= total {
			return al, eps, len(eps), err
		}
	}
}

// NewSongs returns the latest songs.
func (c *Client) NewSongs(ctx context.Context) ([]Song, error) {
	var out struct {
		Songlist []Song `json:"songlist"`
	}
	err := c.Call(ctx, Web, "newsong.NewSongServer", "get_new_song_info", map[string]any{"type": 5}, &out)
	return out.Songlist, err
}

// Lyric returns the LRC lyrics of a song and their translation, if any.
func (c *Client) Lyric(ctx context.Context, s Song) (lyric, trans string, err error) {
	var out struct {
		Lyric string `json:"lyric"`
		Trans string `json:"trans"`
	}
	err = c.Call(ctx, Web, "music.musichallSong.PlayLyricInfo", "GetPlayLyricInfo", map[string]any{
		"songMID": s.Mid, "songID": int64(s.ID), "trans": 1, "roma": 0, "crypt": 0, "qrc": 0,
	}, &out)
	if err != nil {
		return "", "", err
	}
	dec := func(s string) string {
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return ""
		}
		return html.UnescapeString(string(b))
	}
	return dec(out.Lyric), dec(out.Trans), nil
}

// CommentOrder is which of a song's comments come first.
type CommentOrder int

const (
	// HotComments are the most liked first (热门).
	HotComments CommentOrder = iota
	// LatestComments are the newest first (最新).
	LatestComments
)

// Comments returns a page of a song's comments, page from 0; after is the
// SeqNo of the last comment of the pages before. It returns as well
// whether there are more, and how many comments the song has. They need
// no login.
func (c *Client) Comments(ctx context.Context, songID int64, order CommentOrder, page, num int, after string) ([]Comment, bool, int, error) {
	method := "GetHotCommentList"
	if order == LatestComments {
		method = "GetNewCommentList"
	}
	// CommentList2 of the hot ones holds what official accounts pinned,
	// adverts mostly: it is left out.
	var out struct {
		Total       flexInt `json:"TotalCmNum"`
		CommentList struct {
			HasMore  flexInt `json:"HasMore"`
			Comments []struct {
				ID       string  `json:"CmId"`
				SeqNo    string  `json:"SeqNo"`
				Nick     string  `json:"Nick"`
				Avatar   string  `json:"Avatar"`
				Content  string  `json:"Content"`
				Location string  `json:"Location"`
				PubTime  flexInt `json:"PubTime"`
				Likes    flexInt `json:"PraiseNum"`
				Replies  flexInt `json:"ReplyCnt"`
				Sub      []struct {
					Nick    string `json:"Nick"`
					Content string `json:"Content"`
				} `json:"SubComments"`
			} `json:"Comments"`
		} `json:"CommentList"`
	}
	err := c.Call(ctx, Web, "music.globalComment.CommentRead", method, map[string]any{
		"BizType": 1, "BizId": strconv.FormatInt(songID, 10), "LastCommentSeqNo": after,
		"PageNum": page, "PageSize": num, "HotType": 1, "WithAirborne": 0, "PicEnable": 1,
	}, &out)
	if err != nil {
		return nil, false, 0, err
	}
	list := make([]Comment, 0, len(out.CommentList.Comments))
	for _, r := range out.CommentList.Comments {
		cm := Comment{
			ID: r.ID, SeqNo: r.SeqNo, Nick: r.Nick, Avatar: r.Avatar, Content: cleanComment(r.Content),
			Location: r.Location, Likes: int64(r.Likes), Replies: int(r.Replies),
		}
		if r.PubTime > 0 {
			cm.Time = time.Unix(int64(r.PubTime), 0)
		}
		if len(r.Sub) > 0 {
			cm.Reply = &CommentReply{Nick: r.Sub[0].Nick, Content: cleanComment(r.Sub[0].Content)}
		}
		list = append(list, cm)
	}
	return list, out.CommentList.HasMore != 0, int(out.Total), nil
}

// emojiCode is an emoji of the old clients, such as [em]e400846[/em],
// which only they can show.
var emojiCode = regexp.MustCompile(`\[em\][^\[]*\[/em\]`)

// cleanComment makes a comment plain text: its new lines come escaped,
// and pictures and voices leave a mark that shows nothing.
func cleanComment(s string) string {
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, "￼", "")
	s = emojiCode.ReplaceAllString(s, "")
	return strings.TrimSpace(html.UnescapeString(s))
}

// ErrNoSource means the song has no file the login may play.
var ErrNoSource = errors.New("qqmusic: 暂无可播放的音源（可能需要 VIP 或版权受限）")

// SongURL returns where the song's file of a quality is, trying lower
// qualities when there is none. The URL lasts about a day.
func (c *Client) SongURL(ctx context.Context, s Song, q Quality) (string, Quality, error) {
	if !c.LoggedIn() {
		return "", q, ErrNotLoggedIn
	}
	media := s.File.MediaMid
	if media == "" {
		media = s.Mid
	}
	tries := s.File.files(q)
	cr := c.Credential()
	names := make([]string, len(tries))
	for i, t := range tries {
		names[i] = t.prefix + media + t.ext
	}
	mids := make([]string, len(tries))
	types := make([]int, len(tries))
	for i := range tries {
		mids[i], types[i] = s.Mid, s.Type
	}
	var out struct {
		Sip        []string `json:"sip"`
		Midurlinfo []struct {
			Purl     string `json:"purl"`
			Filename string `json:"filename"`
			Result   int    `json:"result"`
		} `json:"midurlinfo"`
	}
	err := c.Call(ctx, Web, "music.vkey.GetVkey", "UrlGetVkey", map[string]any{
		"guid": c.guid, "songmid": mids, "songtype": types, "uin": strconv.FormatInt(cr.MusicID, 10),
		"loginflag": 1, "platform": "20", "filename": names, "ctx": 0,
	}, &out)
	if err != nil {
		return "", q, err
	}
	host := "https://isure.stream.qqmusic.qq.com/"
	for _, s := range out.Sip {
		if strings.HasPrefix(s, "https://") {
			host = s
			break
		}
	}
	for i, info := range out.Midurlinfo {
		if info.Purl != "" {
			got := Standard
			if i < len(tries) {
				got = tries[i].q
			}
			if strings.HasPrefix(info.Purl, "http") {
				return info.Purl, got, nil
			}
			return host + info.Purl, got, nil
		}
	}
	return "", q, ErrNoSource
}

// Profile returns the logged-in user's nick and avatar.
func (c *Client) Profile(ctx context.Context) (User, error) {
	cr := c.Credential()
	if cr.MusicKey == "" {
		return User{}, ErrNotLoggedIn
	}
	b, err := c.getWithLogin(ctx, "https://c6.y.qq.com/rsc/fcgi-bin/fcg_get_profile_homepage.fcg?"+url.Values{
		"format": {"json"}, "inCharset": {"utf-8"}, "outCharset": {"utf-8"}, "cid": {"205360838"},
		"loginUin": {strconv.FormatInt(cr.MusicID, 10)}, "hostUin": {"0"}, "userid": {strconv.FormatInt(cr.MusicID, 10)},
		"reqfrom": {"1"}, "g_tk": {strconv.Itoa(hash33(cr.MusicKey, 5381))},
	}.Encode())
	if err != nil {
		return User{}, err
	}
	var out struct {
		Code int `json:"code"`
		Data struct {
			Creator struct {
				Nick    string `json:"nick"`
				Headpic string `json:"headpic"`
				EncUin  string `json:"encrypt_uin"`
			} `json:"creator"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return User{}, err
	}
	if out.Code != 0 {
		return User{}, ErrNotLoggedIn
	}
	if out.Data.Creator.EncUin != "" && cr.EncryptUin == "" {
		c.mu.Lock()
		if c.cred.MusicKey == cr.MusicKey {
			c.cred.EncryptUin = out.Data.Creator.EncUin
		}
		c.mu.Unlock()
	}
	u := User{Nick: out.Data.Creator.Nick, Avatar: out.Data.Creator.Headpic}
	// Without the membership, the user is taken as no member.
	var vip struct {
		Identity struct {
			VIP        int    `json:"vip"`        // 绿钻
			Eight      int    `json:"eight"`      // 豪华绿钻
			HugeVip    int    `json:"HugeVip"`    // 超级会员
			Overdate   string `json:"overdate"`   // the end of 绿钻
			EightEnd   string `json:"eightEnd"`   // the end of 豪华绿钻
			HugeVipEnd string `json:"HugeVipEnd"` // the end of 超级会员
		} `json:"identity"`
		Send string `json:"send"`
	}
	if c.Call(ctx, Web, "VipLogin.VipLoginInter", "vip_login_base", map[string]any{}, &vip) == nil {
		id := vip.Identity
		u.VIP = id.VIP == 1 || id.Eight == 1 || id.HugeVip == 1
		// The highest membership names the user; the ends of the lower
		// ones stand in when it has none.
		switch {
		case id.HugeVip == 1:
			u.VIPLevel, u.VIPEnd = SuperVIP, vipEnd(id.HugeVipEnd)
		case id.Eight == 1:
			u.VIPLevel, u.VIPEnd = LuxuryVIP, vipEnd(id.EightEnd)
		case id.VIP == 1:
			u.VIPLevel, u.VIPEnd = GreenVIP, vipEnd(id.Overdate)
		}
		if u.VIPEnd.IsZero() {
			u.VIPEnd = vipEnd(id.Overdate)
		}
		if u.VIPEnd.IsZero() {
			u.VIPEnd = vipEnd(vip.Send)
		}
	}
	return u, nil
}

// vipTime is the zone the VIP service writes its dates in.
var vipTime = time.FixedZone("CST", 8*60*60)

// vipEnd reads the end of a membership as the VIP service gives it: a date
// in Beijing time, with or without its time of day, or a Unix time of
// seconds. It is the zero time for one it cannot read.
func vipEnd(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return time.Time{}
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n <= 0 {
			return time.Time{}
		}
		return time.Unix(n, 0).In(vipTime)
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, vipTime); err == nil {
			return t
		}
	}
	return time.Time{}
}

func (c *Client) getWithLogin(ctx context.Context, u string) ([]byte, error) {
	cr := c.Credential()
	return c.getCookie(ctx, u, fmt.Sprintf("uin=%d; qqmusic_uin=%d; qm_keyst=%s; qqmusic_key=%s", cr.MusicID, cr.MusicID, cr.MusicKey, cr.MusicKey))
}

// CreatedPlaylists returns the playlists the logged-in user made, 我喜欢
// first.
func (c *Client) CreatedPlaylists(ctx context.Context) ([]Playlist, error) {
	cr := c.Credential()
	if cr.MusicKey == "" {
		return nil, ErrNotLoggedIn
	}
	var out struct {
		VPlaylist []struct {
			Tid      flexInt `json:"tid"`
			DirID    flexInt `json:"dirId"`
			DirName  string  `json:"dirName"`
			SongNum  flexInt `json:"songNum"`
			PicURL   string  `json:"picUrl"`
			BigPic   string  `json:"bigpicUrl"`
			Nick     string  `json:"nick"`
			DirShow  flexInt `json:"dirShow"`
			ListenNm flexInt `json:"listenNum"`
		} `json:"v_playlist"`
	}
	if err := c.Call(ctx, Web, "music.musicasset.PlaylistBaseRead", "GetPlaylistByUin", map[string]any{"uin": strconv.FormatInt(cr.MusicID, 10)}, &out); err != nil {
		return nil, err
	}
	pls := make([]Playlist, 0, len(out.VPlaylist))
	for _, p := range out.VPlaylist {
		cover := p.PicURL
		if cover == "" {
			cover = p.BigPic
		}
		pls = append(pls, Playlist{ID: int64(p.Tid), DirID: int(p.DirID), Title: p.DirName, SongCount: int(p.SongNum), Cover: cover, Creator: p.Nick, PlayCount: int64(p.ListenNm)})
	}
	return pls, nil
}

// FavPlaylists returns the playlists the logged-in user collected.
func (c *Client) FavPlaylists(ctx context.Context) ([]Playlist, error) {
	cr := c.Credential()
	if cr.MusicKey == "" {
		return nil, ErrNotLoggedIn
	}
	var out struct {
		VList []struct {
			Tid      flexInt `json:"tid"`
			Name     string  `json:"name"`
			Logo     string  `json:"logo"`
			Songnum  flexInt `json:"songnum"`
			Nickname string  `json:"nickname"`
		} `json:"v_list"`
	}
	if err := c.Call(ctx, Web, "music.musicasset.PlaylistFavRead", "CgiGetPlaylistFavInfo", map[string]any{"uin": cr.EncryptUin, "offset": 0, "size": 200}, &out); err != nil {
		return nil, err
	}
	pls := make([]Playlist, 0, len(out.VList))
	for _, p := range out.VList {
		pls = append(pls, Playlist{ID: int64(p.Tid), Title: p.Name, Cover: p.Logo, SongCount: int(p.Songnum), Creator: p.Nickname})
	}
	return pls, nil
}

// LikeSong adds songs to 我喜欢, or takes them out of it.
func (c *Client) LikeSong(ctx context.Context, s Song, like bool) error {
	method := "AddSonglist"
	if !like {
		method = "DelSonglist"
	}
	var out struct {
		RetCode int `json:"retCode"`
	}
	// 我喜欢 (dirid 201) turns writes from the web away (code 80105): the
	// app's comm goes through.
	err := c.callComm(ctx, Web, appComm, "music.musicasset.PlaylistDetailWrite", method, map[string]any{
		"dirId":      201,
		"v_songInfo": []map[string]any{{"songId": int64(s.ID), "songType": s.Type}},
	}, &out)
	if err == nil && out.RetCode != 0 {
		err = fmt.Errorf("qqmusic: %s retCode %d", method, out.RetCode)
	}
	return err
}

// GuessYouLike returns songs of the personal radio (猜你喜欢).
func (c *Client) GuessYouLike(ctx context.Context) ([]Song, error) {
	var out struct {
		Tracks []Song `json:"tracks"`
	}
	err := c.Call(ctx, Web, "music.radioProxy.MbTrackRadioSvr", "get_radio_track", map[string]any{
		"id": 99, "num": 10, "from": 0, "scene": 0, "song_ids": []int{},
	}, &out)
	return out.Tracks, err
}

// RadarSongs returns songs found from those the user liked (私人雷达),
// page from 1; each page has new ones, until more is false.
func (c *Client) RadarSongs(ctx context.Context, page int) (songs []Song, more bool, err error) {
	var out struct {
		HasMore  bool `json:"HasMore"`
		VecSongs []struct {
			Track Song `json:"Track"`
		} `json:"VecSongs"`
	}
	err = c.Call(ctx, Web, "music.recommend.TrackRelationServer", "GetRadarSong", map[string]any{
		"Page": page, "ReqType": 0, "FavSongs": []any{}, "EntranceSongs": []any{},
	}, &out)
	for _, it := range out.VecSongs {
		songs = append(songs, it.Track)
	}
	return songs, out.HasMore, err
}

// DailySongs returns 每日30首, the playlist made for the user each day,
// and its songs. Its id is a card of the front page's feed.
func (c *Client) DailySongs(ctx context.Context) (Playlist, []Song, error) {
	var out struct {
		Shelves []struct {
			Niches []struct {
				Cards []struct {
					ID    flexInt `json:"id"`
					Title string  `json:"title"`
					Cover string  `json:"cover"`
				} `json:"v_card"`
			} `json:"v_niche"`
		} `json:"v_shelf"`
	}
	err := c.Call(ctx, Web, "music.recommend.RecommendFeed", "get_recommend_feed", map[string]any{
		"direction": 0, "page": 1, "v_cache": []any{}, "v_uniq": []any{}, "s_num": 0,
	}, &out)
	if err != nil {
		return Playlist{}, nil, err
	}
	for _, s := range out.Shelves {
		for _, n := range s.Niches {
			for _, card := range n.Cards {
				if card.Title != "每日30首" || card.ID == 0 {
					continue
				}
				p, songs, err := c.PlaylistDetail(ctx, int64(card.ID), 0, 0, 30)
				p.ID = int64(card.ID)
				if card.Cover != "" {
					p.Cover = card.Cover
				}
				return p, songs, err
			}
		}
	}
	return Playlist{}, nil, errors.New("qqmusic: 没有找到每日30首")
}

// appComm makes a request one of the phone app: some APIs answer it only,
// or answer it otherwise.
var appComm = map[string]any{"ct": 11, "cv": 13020508, "tmeAppID": "qqmusic"}

// feedPages is how many pages of the front page's feed RecommendFeed
// takes: the playlists for the user come on the second.
const feedPages = 2

// RecommendFeed returns the sections of songs and of playlists of the
// phone app's front page, made for the user. Asked by the web, the feed
// has the desktop's sections instead.
func (c *Client) RecommendFeed(ctx context.Context) ([]FeedShelf, error) {
	type card struct {
		Type int `json:"type"`
		// A number for songs and playlists, a link for others.
		ID    json.RawMessage `json:"id"`
		Title string          `json:"title"`
		Cover string          `json:"cover"`
		Cnt   flexInt         `json:"cnt"`
	}
	var shelves []FeedShelf
	// The ids of each shelf's songs, which the feed gives no more of.
	ids := map[int][]int64{}
	for pg := 1; pg <= feedPages; pg++ {
		var out struct {
			Shelves []struct {
				Template string `json:"title_template"`
				Content  string `json:"title_content"`
				Niches   []struct {
					Cards []card `json:"v_card"`
				} `json:"v_niche"`
			} `json:"v_shelf"`
		}
		err := c.callComm(ctx, Web, appComm, "music.recommend.RecommendFeed", "get_recommend_feed", map[string]any{
			"direction": min(pg-1, 1), "page": pg, "v_cache": []any{}, "v_uniq": []any{}, "s_num": len(shelves),
		}, &out)
		if err != nil {
			return nil, err
		}
		for _, s := range out.Shelves {
			title := strings.TrimSpace(strings.ReplaceAll(s.Template, "{String}", s.Content))
			if s.Template == "" {
				title = s.Content
			}
			var cards []card
			for _, n := range s.Niches {
				cards = append(cards, n.Cards...)
			}
			// A shelf of songs or of playlists has them only; one that
			// starts with something else (entries, programs, live
			// shows) is not shown.
			if len(cards) == 0 || title == "" || cards[0].Type != 200 && cards[0].Type != 500 {
				continue
			}
			sh := FeedShelf{Title: title}
			for _, cd := range cards {
				id, _ := strconv.ParseInt(strings.Trim(string(cd.ID), `"`), 10, 64)
				switch {
				case cd.Type != cards[0].Type || id == 0:
				case cd.Type == 200:
					ids[len(shelves)] = append(ids[len(shelves)], id)
				default:
					sh.Playlists = append(sh.Playlists, Playlist{ID: id, Title: cd.Title, Cover: cd.Cover, PlayCount: int64(cd.Cnt)})
				}
			}
			shelves = append(shelves, sh)
		}
	}
	reqs := map[string]Req{}
	for i, list := range ids {
		reqs[strconv.Itoa(i)] = Req{"music.trackInfo.UniformRuleCtrl", "CgiGetTrackInfo", map[string]any{
			"ids": list, "types": make([]int, len(list)),
		}}
	}
	if len(reqs) > 0 {
		data, errs, err := c.CallMany(ctx, Web, nil, reqs)
		if err != nil {
			return nil, err
		}
		for k := range reqs {
			var out struct {
				Tracks []Song `json:"tracks"`
			}
			if errs[k] == nil && json.Unmarshal(data[k], &out) == nil {
				i, _ := strconv.Atoi(k)
				shelves[i].Songs = out.Tracks
			}
		}
	}
	// Those whose songs did not come are left out.
	return slices.DeleteFunc(shelves, func(s FeedShelf) bool {
		return len(s.Songs) == 0 && len(s.Playlists) == 0
	}), nil
}

// podcastArea is the encArea of 播客专区, as its H5 page has it: no service
// lists the areas.
const podcastArea = "004Z03m934o6En"

// rawCard is a card of the podcast area, or a podcast PodcastPicks gives.
type rawCard struct {
	Type int `json:"type"`
	// A number for shows and episodes, a name for the others.
	ID       json.RawMessage `json:"id"`
	Subid    string          `json:"subid"`
	Title    string          `json:"title"`
	Subtitle string          `json:"subtitle"`
	Cover    string          `json:"cover"`
	Cnt      flexInt         `json:"cnt"`
	Misc     struct {
		Num        flexInt `json:"num"`
		Recommend  string  `json:"recommend"`
		SchemaType string  `json:"schemaType"`
		SchemaID   string  `json:"schemaNewParams"`
	} `json:"miscellany"`
}

// id is the card's number, 0 if it has a name instead.
func (r rawCard) id() int64 {
	n, _ := strconv.ParseInt(strings.Trim(string(r.ID), `"`), 10, 64)
	return n
}

// rawArea is the data of getAreaHomePage.
type rawArea struct {
	Title string `json:"title"`
	Share struct {
		Desc string `json:"desc"`
	} `json:"share"`
	Shelves []struct {
		Template string `json:"title_template"`
		Content  string `json:"title_content"`
		Niches   []struct {
			Cards []rawCard `json:"v_card"`
		} `json:"v_niche"`
	} `json:"v_shelf"`
}

// area reads the shelves of shows, of hosts and of episodes, by shelf
// the ids of the episodes, whose songs are to be asked for. Banners and
// topics, which link elsewhere, are left out.
func (r rawArea) area() (PodcastArea, map[int][]int64) {
	a := PodcastArea{Title: r.Title, Desc: r.Share.Desc}
	ids := map[int][]int64{}
	for _, s := range r.Shelves {
		title := strings.TrimSpace(strings.ReplaceAll(s.Template, "{String}", s.Content))
		if title == "" {
			continue
		}
		sh := PodcastShelf{Title: title}
		// A shelf may show a show twice: its cards are told apart by it.
		has := func(id int64, mid string) bool {
			return slices.ContainsFunc(sh.Shows, func(s Show) bool { return s.Mid == mid && (mid != "" || s.ID == id) })
		}
		var eps []int64
		for _, n := range s.Niches {
			for _, cd := range n.Cards {
				switch cd.Type {
				case 400:
					if (cd.Subid != "" || cd.id() != 0) && !has(cd.id(), cd.Subid) {
						sh.Shows = append(sh.Shows, Show{ID: cd.id(), Mid: cd.Subid, Title: html.UnescapeString(cd.Title),
							Host: html.UnescapeString(cd.Subtitle), Cover: cd.Cover, Episodes: int(cd.Misc.Num)})
					}
				case 2100:
					// The show a host's card opens is an album's id.
					if id, _ := strconv.ParseInt(cd.Misc.SchemaID, 10, 64); id != 0 && cd.Misc.SchemaType == "2" && !has(id, "") {
						sh.Hosts = true
						sh.Shows = append(sh.Shows, Show{ID: id, Title: html.UnescapeString(cd.Title), Cover: cd.Cover})
					}
				case 200:
					if id := cd.id(); id != 0 {
						eps = append(eps, id)
					}
				}
			}
		}
		switch {
		case len(sh.Shows) > 0:
			a.Shelves = append(a.Shelves, sh)
		case len(eps) > 0:
			ids[len(a.Shelves)] = eps
			a.Shelves = append(a.Shelves, sh)
		}
	}
	return a, ids
}

// PodcastArea returns QQ Music's podcasts (播客专区), 小宇宙's among them:
// shelves of shows, of hosts and of episodes. It needs no login. Its shows
// are old ones; PodcastPicks has new ones.
func (c *Client) PodcastArea(ctx context.Context) (PodcastArea, error) {
	var out rawArea
	if err := c.Call(ctx, Web, "music.area.AreaHome", "getAreaHomePage", map[string]any{"cmd": 0, "encArea": podcastArea}, &out); err != nil {
		return PodcastArea{}, err
	}
	a, ids := out.area()
	if len(ids) > 0 {
		// The songs of the episodes, all in one request.
		reqs := map[string]Req{}
		for i, list := range ids {
			reqs[strconv.Itoa(i)] = Req{"music.trackInfo.UniformRuleCtrl", "CgiGetTrackInfo", map[string]any{
				"ids": list, "types": make([]int, len(list)),
			}}
		}
		data, errs, err := c.CallMany(ctx, Web, nil, reqs)
		if err != nil {
			return PodcastArea{}, err
		}
		for k := range reqs {
			var out struct {
				Tracks []Song `json:"tracks"`
			}
			if errs[k] == nil && json.Unmarshal(data[k], &out) == nil {
				i, _ := strconv.Atoi(k)
				a.Shelves[i].Episodes = out.Tracks
			}
		}
	}
	// Those whose songs did not come are left out.
	a.Shelves = slices.DeleteFunc(a.Shelves, func(s PodcastShelf) bool {
		return len(s.Shows) == 0 && len(s.Episodes) == 0
	})
	return a, nil
}

// PodcastPicks returns six podcasts, with what each is about, others at
// each call: 换一批 is calling it again. They know only their ids.
func (c *Client) PodcastPicks(ctx context.Context) ([]Show, error) {
	var out struct {
		RadioList []rawCard `json:"radioList"`
	}
	err := c.Call(ctx, Web, "music.longRadio.recommend", "getRadioList", map[string]any{}, &out)
	shows := make([]Show, 0, len(out.RadioList))
	for _, cd := range out.RadioList {
		if id := cd.id(); id != 0 {
			shows = append(shows, Show{ID: id, Title: html.UnescapeString(cd.Title), Host: html.UnescapeString(cd.Subtitle),
				Cover: cd.Cover, Intro: cleanIntro(cd.Misc.Recommend), PlayCount: int64(cd.Cnt)})
		}
	}
	return shows, err
}

// podcasts reports which of the albums are podcasts, asking of them all in
// one request: what a search finds does not say.
func (c *Client) podcasts(ctx context.Context, mids []string) (map[string]bool, error) {
	is := map[string]bool{}
	if len(mids) == 0 {
		return is, nil
	}
	reqs := map[string]Req{}
	for _, mid := range mids {
		reqs[mid] = Req{"music.musichallAlbum.AlbumInfoServer", "GetAlbumDetail", map[string]any{"albumMid": mid}}
	}
	data, errs, err := c.CallMany(ctx, Web, nil, reqs)
	if err != nil {
		return nil, err
	}
	for mid := range reqs {
		var out rawAlbum
		if errs[mid] == nil && json.Unmarshal(data[mid], &out) == nil {
			is[mid] = out.album().IsPodcast()
		}
	}
	return is, nil
}

// SearchPodcasts finds podcasts; page starts at 1. The search finds albums
// of every kind, of which it keeps the podcasts: a page may have fewer
// than num, or none, and more says whether pages follow.
func (c *Client) SearchPodcasts(ctx context.Context, query string, page, num int) (shows []Show, more bool, err error) {
	var out struct {
		Meta struct {
			Sum flexInt `json:"sum"`
		} `json:"meta"`
		Body struct {
			Album struct {
				List []struct {
					ID      flexInt  `json:"albumID"`
					Mid     string   `json:"albumMID"`
					Name    string   `json:"albumName"`
					Singers []Singer `json:"singer_list"`
					Count   flexInt  `json:"song_count"`
				} `json:"list"`
			} `json:"album"`
		} `json:"body"`
	}
	// A podcast's name finds it as the clients do, straight (直达) from a
	// search of songs: the albums found may not have it. Being an extra, it
	// never fails the search.
	if page == 1 {
		var top struct {
			Body struct {
				Zhida json.RawMessage `json:"zhida"`
			} `json:"body"`
		}
		if c.search(ctx, query, 1, 1, 0, &top) == nil {
			shows = directPodcasts(top.Body.Zhida)
		}
	}
	if err := c.search(ctx, query, page, num, 2, &out); err != nil {
		return nil, false, err
	}
	list := out.Body.Album.List
	mids := make([]string, 0, len(list))
	for _, al := range list {
		if al.Mid != "" {
			mids = append(mids, al.Mid)
		}
	}
	is, err := c.podcasts(ctx, mids)
	if err != nil {
		return nil, false, err
	}
	for _, al := range list {
		if is[al.Mid] && !slices.ContainsFunc(shows, func(s Show) bool { return s.Mid == al.Mid }) {
			host := Song{Singers: al.Singers}.Artists()
			shows = append(shows, Show{ID: int64(al.ID), Mid: al.Mid, Title: html.UnescapeString(al.Name), Host: html.UnescapeString(host),
				Cover: AlbumCover(al.Mid, 300), Episodes: int(al.Count)})
		}
	}
	return shows, len(list) > 0 && page*num < int(out.Meta.Sum), nil
}

// directPodcasts reads the podcasts a search goes straight to (直达), such
// as "播客: 来都来了 | 听了再走" beside a singer: long audio (channel 10025).
func directPodcasts(b json.RawMessage) []Show {
	var z struct {
		List []struct {
			Title string `json:"title"`
			Info  struct {
				Mid     string          `json:"mid"`
				Channel json.RawMessage `json:"channel_type"`
				Tracks  json.RawMessage `json:"track_num"`
			} `json:"custom_info"`
		} `json:"list"`
	}
	if len(b) == 0 || json.Unmarshal(b, &z) != nil {
		return nil
	}
	var shows []Show
	for _, it := range z.List {
		if it.Info.Mid == "" || strings.Trim(string(it.Info.Channel), `"`) != "10025" {
			continue
		}
		// The title is the kind and the name.
		title := strings.ReplaceAll(html.UnescapeString(it.Title), "：", ":")
		if _, name, ok := strings.Cut(title, ":"); ok {
			title = name
		}
		n, _ := strconv.Atoi(strings.Trim(string(it.Info.Tracks), `"`))
		shows = append(shows, Show{Mid: it.Info.Mid, Title: strings.TrimSpace(title), Cover: AlbumCover(it.Info.Mid, 300), Episodes: n})
	}
	return shows
}

// SearchEpisodes finds episodes of podcasts; page starts at 1. As
// SearchPodcasts, it keeps of the songs found those of podcasts.
func (c *Client) SearchEpisodes(ctx context.Context, query string, page, num int) (eps []Song, more bool, err error) {
	songs, total, _, err := c.Search(ctx, query, page, num)
	if err != nil {
		return nil, false, err
	}
	var mids []string
	for _, s := range songs {
		if s.Album.Mid != "" && !slices.Contains(mids, s.Album.Mid) {
			mids = append(mids, s.Album.Mid)
		}
	}
	is, err := c.podcasts(ctx, mids)
	if err != nil {
		return nil, false, err
	}
	for _, s := range songs {
		if is[s.Album.Mid] {
			eps = append(eps, s)
		}
	}
	return eps, len(songs) > 0 && page*num < total, nil
}
