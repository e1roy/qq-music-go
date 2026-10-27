// Command probe checks the QQ Music APIs the app uses, against the live
// service. With -login it logs in by QR code first and saves the login to
// -cred; with -cred alone it reuses a saved login.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"qqmusic/internal/qqmusic"
)

func main() {
	login := flag.String("login", "", "log in by QR code first: qq or wx")
	credPath := flag.String("cred", "", "file to load/save the login")
	flag.Parse()
	ctx := context.Background()
	c := qqmusic.New()

	if *credPath != "" {
		if b, err := os.ReadFile(*credPath); err == nil {
			var cr qqmusic.Credential
			if json.Unmarshal(b, &cr) == nil {
				c.SetCredential(cr)
			}
		}
	}
	if *login != "" {
		kind := qqmusic.QRQQ
		if *login == "wx" {
			kind = qqmusic.QRWeChat
		}
		qr, err := c.NewQR(ctx, kind)
		check("NewQR", err)
		p := filepath.Join(os.TempDir(), fmt.Sprintf("qqmusic_qr_%d.png", time.Now().Unix()))
		os.WriteFile(p, qr.Image, 0o644)
		exec.Command("cmd", "/c", "start", "", p).Run()
		fmt.Println("scan", p)
		for deadline := time.Now().Add(3 * time.Minute); time.Now().Before(deadline); {
			st, err := c.PollQR(ctx, qr)
			check("PollQR", err)
			fmt.Println("state:", st)
			if st == qqmusic.QRDone {
				break
			}
			if st == qqmusic.QRExpired || st == qqmusic.QRCancelled {
				os.Exit(1)
			}
			time.Sleep(2 * time.Second)
		}
		if *credPath != "" {
			b, _ := json.MarshalIndent(c.Credential(), "", "  ")
			os.WriteFile(*credPath, b, 0o600)
		}
	}

	songs, total, direct, err := c.Search(ctx, "周杰伦", 1, 5)
	check("Search", err)
	if direct != nil {
		fmt.Printf("Search direct: %s: %s %s songs=%d albums=%d %q\n", direct.Label, direct.Name, direct.Mid, direct.SongCount, direct.AlbumCount, direct.Desc)
	}
	fmt.Printf("Search: %d of %d, first %q by %q vip=%v cover=%s\n", len(songs), total, songs[0].Name, songs[0].Artists(), songs[0].VIP(), songs[0].Cover(300))

	sgs, total, err := c.SearchSingers(ctx, "毛不易", 1, 5)
	check("SearchSingers", err)
	if len(sgs) > 0 {
		fmt.Printf("SearchSingers: %d of %d, first %q %s songs=%d albums=%d\n", len(sgs), total, sgs[0].Name, sgs[0].Mid, sgs[0].SongCount, sgs[0].AlbumCount)
	}

	sug, err := c.Suggest(ctx, "周杰")
	check("Suggest", err)
	fmt.Println("Suggest:", sug)

	hot, err := c.HotKeys(ctx)
	check("HotKeys", err)
	fmt.Println("HotKeys:", len(hot), hot[:min(5, len(hot))])

	groups, err := c.Toplists(ctx)
	check("Toplists", err)
	fmt.Printf("Toplists: %d groups, %q has %d, first %+v\n", len(groups), groups[0].Name, len(groups[0].Lists), groups[0].Lists[0].Title)

	tl, tsongs, err := c.ToplistDetail(ctx, 26, 0, 10)
	check("ToplistDetail", err)
	fmt.Printf("ToplistDetail: %q %d songs, first %q\n", tl.Title, len(tsongs), tsongs[0].Name)

	rec, more, err := c.RecommendPlaylists(ctx, 0, 20)
	check("RecommendPlaylists", err)
	fmt.Printf("RecommendPlaylists: %d more=%v, first %q id %d\n", len(rec), more, rec[0].Title, rec[0].ID)

	cat, ctotal, err := c.PlaylistsByCategory(ctx, 6, 1, 10)
	check("PlaylistsByCategory", err)
	fmt.Printf("PlaylistsByCategory: %d of %d, first %q\n", len(cat), ctotal, cat[0].Title)

	pl, psongs, err := c.PlaylistDetail(ctx, rec[0].ID, 0, 0, 30)
	check("PlaylistDetail", err)
	fmt.Printf("PlaylistDetail: %q by %q, %d/%d songs\n", pl.Title, pl.Creator, len(psongs), pl.SongCount)

	ns, err := c.NewSongs(ctx)
	check("NewSongs", err)
	fmt.Println("NewSongs:", len(ns))

	lrc, trans, err := c.Lyric(ctx, songs[0])
	check("Lyric", err)
	fmt.Printf("Lyric: %d bytes, trans %d bytes, %d lines parsed\n", len(lrc), len(trans), len(qqmusic.ParseLRC(lrc, trans)))

	for _, o := range []qqmusic.CommentOrder{qqmusic.HotComments, qqmusic.LatestComments} {
		cms, more, ctotal, err := c.Comments(ctx, int64(songs[0].ID), o, 0, 10, "")
		check("Comments", err)
		if len(cms) == 0 {
			log.Fatalf("Comments %d: none", o)
		}
		next, _, _, err := c.Comments(ctx, int64(songs[0].ID), o, 1, 10, cms[len(cms)-1].SeqNo)
		check("Comments page 2", err)
		fmt.Printf("Comments %d: %d+%d of %d more=%v, first %q by %q likes=%d at %s\n", o, len(cms), len(next), ctotal, more,
			cms[0].Content, cms[0].Nick, cms[0].Likes, cms[0].Time.Format(time.DateTime))
	}

	area, err := c.PodcastArea(ctx)
	check("PodcastArea", err)
	fmt.Printf("PodcastArea: %q %q, %d shelves\n", area.Title, area.Desc, len(area.Shelves))
	for _, sh := range area.Shelves {
		fmt.Printf("  %q hosts=%v shows=%d episodes=%d\n", sh.Title, sh.Hosts, len(sh.Shows), len(sh.Episodes))
	}
	picks, err := c.PodcastPicks(ctx)
	check("PodcastPicks", err)
	if len(picks) == 0 {
		log.Fatal("PodcastPicks: none")
	}
	fmt.Printf("PodcastPicks: %d, first %q by %q id=%d\n", len(picks), picks[0].Title, picks[0].Host, picks[0].ID)
	// A pick knows only its id: the show and its newest episodes by it.
	show, eps, etotal, err := c.AlbumDetail(ctx, "", picks[0].ID)
	check("AlbumDetail podcast", err)
	fmt.Printf("AlbumDetail podcast: %q mid=%s podcast=%v, %d of %d episodes\n", show.Name, show.Mid, show.IsPodcast(), len(eps), etotal)
	if len(eps) > 0 {
		fmt.Printf("  newest: #%d %q %s plays=%d %s\n", eps[0].Index, eps[0].Name, eps[0].UploadTime, eps[0].ListenCount, eps[0].Duration())
	}
	shows, more, err := c.SearchPodcasts(ctx, "日谈公园", 1, 20)
	check("SearchPodcasts", err)
	fmt.Printf("SearchPodcasts: %d more=%v\n", len(shows), more)
	for _, s := range shows[:min(3, len(shows))] {
		fmt.Printf("  %q by %q, %d episodes\n", s.Title, s.Host, s.Episodes)
	}
	found, more, err := c.SearchEpisodes(ctx, "日谈公园", 1, 30)
	check("SearchEpisodes", err)
	fmt.Printf("SearchEpisodes: %d more=%v\n", len(found), more)

	if !c.LoggedIn() {
		_, _, err := c.SongURL(ctx, songs[0], qqmusic.Standard)
		fmt.Println("SongURL without login:", err)
		return
	}
	if c.Credential().NeedsRefresh() {
		check("Refresh", c.Refresh(ctx))
	}
	u, err := c.Profile(ctx)
	check("Profile", err)
	fmt.Printf("Profile: %+v enc=%s\n", u, c.Credential().EncryptUin)

	mine, err := c.CreatedPlaylists(ctx)
	check("CreatedPlaylists", err)
	fmt.Println("CreatedPlaylists:", len(mine))
	for _, p := range mine[:min(3, len(mine))] {
		fmt.Printf("  %q tid=%d dirid=%d songs=%d\n", p.Title, p.ID, p.DirID, p.SongCount)
	}
	_, liked, err := c.PlaylistDetail(ctx, 0, 201, 0, 10)
	fmt.Println("Liked songs:", len(liked), err)

	fav, err := c.FavPlaylists(ctx)
	fmt.Println("FavPlaylists:", len(fav), err)

	guess, err := c.GuessYouLike(ctx)
	fmt.Println("GuessYouLike:", len(guess), err)

	radar, more, err := c.RadarSongs(ctx, 1)
	fmt.Println("RadarSongs:", len(radar), more, err)

	daily, dsongs, err := c.DailySongs(ctx)
	fmt.Printf("DailySongs: %q id %d, %d songs, %v\n", daily.Title, daily.ID, len(dsongs), err)

	shelves, err := c.RecommendFeed(ctx)
	fmt.Println("RecommendFeed:", len(shelves), err)
	for _, sh := range shelves {
		fmt.Printf("  %q songs=%d playlists=%d\n", sh.Title, len(sh.Songs), len(sh.Playlists))
	}

	// Playback URLs of free and VIP songs.
	free, _, _, _ := c.Search(ctx, "小星星 儿歌", 1, 10)
	for _, s := range append(free[:3], songs[:3]...) {
		for _, q := range []qqmusic.Quality{qqmusic.Standard, qqmusic.High, qqmusic.Lossless} {
			url, got, err := c.SongURL(ctx, s, q)
			if err != nil {
				fmt.Printf("SongURL %q vip=%v %v: %v\n", s.Name, s.VIP(), q, err)
				continue
			}
			resp, err := http.Get(url)
			check("download", err)
			head := make([]byte, 4)
			io.ReadFull(resp.Body, head)
			resp.Body.Close()
			fmt.Printf("SongURL %q vip=%v want %v got %v: HTTP %d %d bytes head %q\n", s.Name, s.VIP(), q, got, resp.StatusCode, resp.ContentLength, head)
		}
	}
	if *credPath != "" {
		b, _ := json.MarshalIndent(c.Credential(), "", "  ")
		os.WriteFile(*credPath, b, 0o600)
	}
}

func check(what string, err error) {
	if err != nil {
		log.Fatalf("%s: %v", what, err)
	}
}
