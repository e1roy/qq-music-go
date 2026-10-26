package qqmusic

import "testing"

// TestParseDirect reads zhida as the search sends it.
func TestParseDirect(t *testing.T) {
	singer := `{"list":[{"type":1,"title":"歌手: 毛不易","desciption":"歌曲:536 专辑:101 视频:1703","pic":"http://y.gtimg.cn/x.jpg",
		"custom_info":{"mid":"001BHDR33FZVZ0","song_num":"536","album_num":"101","mv_num":"1703","extra_desc":"916.3万人关注"}}]}`
	d := parseDirect([]byte(singer))
	if d == nil || d.Kind != DirectSinger || d.Label != "歌手" || d.Name != "毛不易" || d.Mid != "001BHDR33FZVZ0" ||
		d.SongCount != 536 || d.AlbumCount != 101 || d.MVCount != 1703 || d.Desc != "916.3万人关注" {
		t.Errorf("singer: %+v", d)
	}

	album := `{"list":[{"type":2,"title":"专辑: 范特西","desciption":"周杰伦  10首  2001-09-14",
		"custom_info":{"mid":"000I5jJB3blWeN","track_num":"10"}}]}`
	d = parseDirect([]byte(album))
	if d == nil || d.Kind != DirectAlbum || d.Name != "范特西" || d.Desc != "周杰伦 · 10首 · 2001-09-14" || d.Pic != AlbumCover("000I5jJB3blWeN", 300) {
		t.Errorf("album: %+v", d)
	}

	show := `{"list":[{"type":4,"title":"综艺：明日创作计划","custom_info":{"mid":"001fXbpZ0Nl6l8","song_num":"204","album_num":"10","mv_num":"0"}}]}`
	d = parseDirect([]byte(show))
	if d == nil || d.Kind != DirectSinger || d.Label != "综艺" || d.Name != "明日创作计划" {
		t.Errorf("show: %+v", d)
	}

	for _, s := range []string{``, `{"list":[]}`, `{"list":[{"type":9,"title":"x","custom_info":{"mid":"y"}}]}`, `{"list":[{"type":1,"custom_info":{"mid":3}}]}`} {
		if d := parseDirect([]byte(s)); d != nil {
			t.Errorf("%s: %+v", s, d)
		}
	}
}

// TestCleanComment makes comments plain text as they come.
func TestCleanComment(t *testing.T) {
	for in, want := range map[string]string{
		`我没有忘记你\n如果幸福的话`:         "我没有忘记你\n如果幸福的话",
		"还有双截棍\uFFFC":            "还有双截棍",
		" 好听[em]e400846[/em] ":   "好听",
		"&quot;晴天&quot; &amp; 雨": `"晴天" & 雨`,
	} {
		if got := cleanComment(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}
