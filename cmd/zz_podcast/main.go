//go:build probe

// Command zz_podcast is a local, throwaway probe of QQ Music's long audio
// (podcast) APIs. It dumps raw JSON so the shapes can be written down.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"qqmusic/internal/qqmusic"
)

func main() {
	dir := filepath.Join(os.TempDir(), "qqscan", "out")
	os.MkdirAll(dir, 0o755)
	c := qqmusic.New()
	if b, err := os.ReadFile(os.Args[1]); err == nil {
		var cr qqmusic.Credential
		if json.Unmarshal(b, &cr) == nil {
			c.SetCredential(cr)
		}
	}
	ctx := context.Background()

	dump := func(name string, data json.RawMessage, err error) {
		fmt.Printf("%-24s len=%d err=%v\n", name, len(data), err)
		if len(data) > 0 {
			os.WriteFile(filepath.Join(dir, name+".json"), data, 0o644)
		}
	}

	// 1. The podcast area page (播客专区, area 206).
	d, errs, err := c.CallMany(ctx, qqmusic.Web, nil, map[string]qqmusic.Req{
		"PoolApiServer": {"music.area.AreaHome", "getAreaHomePage", map[string]any{"cmd": 0, "encArea": "004Z03m934o6En"}},
	})
	dump("area", d["PoolApiServer"], first(err, errs["PoolApiServer"]))

	// 2. The podcast feed of the discover page.
	d, errs, err = c.CallMany(ctx, qqmusic.Web, nil, map[string]qqmusic.Req{
		"req": {"music.longRadio.recommend", "getRadioList", map[string]any{}},
	})
	dump("longradio", d["req"], first(err, errs["req"]))

	// 3. A podcast album by its mid.
	d, errs, err = c.CallMany(ctx, qqmusic.Web, nil, map[string]qqmusic.Req{
		"req": {"music.musichallAlbum.AlbumInfoServer", "GetAlbumDetail", map[string]any{"albumMid": "001N8TFz49WZol"}},
	})
	dump("album", d["req"], first(err, errs["req"]))

	// 4. Its episodes.
	d, errs, err = c.CallMany(ctx, qqmusic.Web, nil, map[string]qqmusic.Req{
		"req": {"music.musichallAlbum.AlbumSongList", "GetAlbumSongList", map[string]any{"albumMid": "001N8TFz49WZol", "begin": 0, "num": 3, "order": 2}},
	})
	dump("albumsongs", d["req"], first(err, errs["req"]))

	// 5. One episode, by its song id.
	d, errs, err = c.CallMany(ctx, qqmusic.Web, nil, map[string]qqmusic.Req{
		"req": {"music.trackInfo.UniformRuleCtrl", "CgiGetTrackInfo", map[string]any{"ids": []int64{335642933}, "types": []int{0}}},
	})
	dump("track", d["req"], first(err, errs["req"]))

	// 6. Search a podcast by name.
	d, errs, err = c.CallMany(ctx, qqmusic.Web, nil, map[string]qqmusic.Req{
		"req": {"music.search.SearchCgiService", "DoSearchForQQMusicDesktop", map[string]any{"query": "日谈公园", "num_per_page": 5, "page_num": 1, "search_type": 0}},
	})
	dump("search", d["req"], first(err, errs["req"]))

	// 7. Search albums (2) for the same name: podcasts are albums.
	d, errs, err = c.CallMany(ctx, qqmusic.Web, nil, map[string]qqmusic.Req{
		"req": {"music.search.SearchCgiService", "DoSearchForQQMusicDesktop", map[string]any{"query": "日谈公园", "num_per_page": 5, "page_num": 1, "search_type": 2}},
	})
	dump("search_album", d["req"], first(err, errs["req"]))

	// 8. The podcast feed with paging.
	d, errs, err = c.CallMany(ctx, qqmusic.Web, nil, map[string]qqmusic.Req{
		"req": {"music.longRadio.recommend", "getRadioList", map[string]any{"page": 1, "num": 20}},
	})
	dump("longradio_page", d["req"], first(err, errs["req"]))

	// 9. The category behind the feed's "发现更多".
	d, errs, err = c.CallMany(ctx, qqmusic.Web, nil, map[string]qqmusic.Req{
		"req": {"music.area.CategoryArea", "getCategoryAreaInCategoryPlaylist", map[string]any{"categoryId": 42800344}},
	})
	dump("category", d["req"], first(err, errs["req"]))

	// 10. The playback URL of an episode that has a 320k file.
	s := qqmusic.Song{ID: 339871442, Type: 0, Mid: "001z32CP01Zc09", File: qqmusic.File{Size320: 1, MediaMid: "001z32CP01Zc09"}}
	u, got, err := c.SongURL(ctx, s, qqmusic.Best)
	fmt.Printf("%-24s %v %v %v\n", "episode url", got, u, err)

	// 11. The host as a singer.
	d, errs, err = c.CallMany(ctx, qqmusic.Web, nil, map[string]qqmusic.Req{
		"req": {"musichall.singer_info_server", "GetSingerDetail", map[string]any{
			"singer_mids": []string{"000TPT0w1Ocqxl"}, "ex_singer": 1, "wiki_singer": 1, "group_singer": 0, "pic": 1, "photos": 0,
		}},
	})
	dump("singer", d["req"], first(err, errs["req"]))

	// 12. The shows of a host.
	d, errs, err = c.CallMany(ctx, qqmusic.Web, nil, map[string]qqmusic.Req{
		"req": {"music.musichallAlbum.AlbumListServer", "GetAlbumList", map[string]any{
			"singerMid": "000TPT0w1Ocqxl", "order": 1, "begin": 0, "num": 30,
		}},
	})
	dump("singer_albums", d["req"], first(err, errs["req"]))

	// 13. Comments and lyrics of an episode.
	d, errs, err = c.CallMany(ctx, qqmusic.Web, nil, map[string]qqmusic.Req{
		"req": {"music.globalComment.CommentRead", "GetHotCommentList", map[string]any{
			"BizType": 1, "BizId": "339871442", "LastCommentSeqNo": "", "PageNum": 0, "PageSize": 3,
			"HotType": 1, "WithAirborne": 0, "PicEnable": 1,
		}},
	})
	dump("comments", d["req"], first(err, errs["req"]))

	d, errs, err = c.CallMany(ctx, qqmusic.Web, nil, map[string]qqmusic.Req{
		"req": {"music.musichallSong.PlayLyricInfo", "GetPlayLyricInfo", map[string]any{
			"songMID": "001z32CP01Zc09", "songID": 339871442, "trans": 1, "roma": 0, "crypt": 0, "qrc": 0,
		}},
	})
	dump("lyric", d["req"], first(err, errs["req"]))

	// 14. The category channel of the podcast feed, as a string id.
	d, errs, err = c.CallMany(ctx, qqmusic.Web, nil, map[string]qqmusic.Req{
		"req": {"music.area.CategoryArea", "getCategoryAreaInCategoryPlaylist", map[string]any{"categoryId": "42800344"}},
	})
	dump("category_str", d["req"], first(err, errs["req"]))

	// 15. Whether an album resolves by its numeric id alone (a 2100 card
	// only carries that).
	d, errs, err = c.CallMany(ctx, qqmusic.Web, nil, map[string]qqmusic.Req{
		"req": {"music.musichallAlbum.AlbumInfoServer", "GetAlbumDetail", map[string]any{"albumID": 17291185}},
	})
	dump("album_by_id", d["req"], first(err, errs["req"]))

	// 16. A 小宇宙 show: its newest episodes and their audio files.
	d, errs, err = c.CallMany(ctx, qqmusic.Web, nil, map[string]qqmusic.Req{
		"req": {"music.musichallAlbum.AlbumSongList", "GetAlbumSongList", map[string]any{"albumMid": "002JAF3D0nJ33K", "begin": 0, "num": 3, "order": 2}},
	})
	dump("podcastsongs", d["req"], first(err, errs["req"]))

	// 17. Whether the song detail carries any episode text.
	d, errs, err = c.CallMany(ctx, qqmusic.Web, nil, map[string]qqmusic.Req{
		"req": {"music.pf_song_detail_svr", "get_song_detail", map[string]any{
			"song_mid": "001z32CP01Zc09", "song_id": 339871442,
		}},
	})
	dump("song_detail", d["req"], first(err, errs["req"]))

	// 18. The playback URL of an episode that only has a 128k MP3.
	s = qqmusic.Song{ID: 348867947, Type: 0, Mid: "0004Xnzp48B0ox", File: qqmusic.File{Size128: 1, MediaMid: "0004Xnzp48B0ox"}}
	u, got, err = c.SongURL(ctx, s, qqmusic.Best)
	fmt.Printf("%-24s %v %v %v\n", "podcast episode url", got, u, err)
}

func first(a, b error) error {
	if a != nil {
		return a
	}
	return b
}
