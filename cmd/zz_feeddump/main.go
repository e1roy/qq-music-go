package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"qqmusic/internal/qqmusic"
)

func main() {
	c := qqmusic.New()
	b, _ := os.ReadFile(os.Args[1])
	var cr qqmusic.Credential
	json.Unmarshal(b, &cr)
	c.SetCredential(cr)
	ctx := context.Background()
	shelves, err := c.RecommendFeed(ctx)
	fmt.Println("RecommendFeed:", len(shelves), err)
	for _, sh := range shelves {
		fmt.Printf("  %q songs=%d playlists=%d\n", sh.Title, len(sh.Songs), len(sh.Playlists))
		for _, s := range sh.Songs[:min(2, len(sh.Songs))] {
			fmt.Printf("    %s - %s %s\n", s.DisplayName(), s.Artists(), s.Cover(150))
		}
		for _, p := range sh.Playlists[:min(2, len(sh.Playlists))] {
			fmt.Printf("    %d %s %d %s\n", p.ID, p.Title, p.PlayCount, p.Cover)
		}
	}
	p, songs, err := c.PlaylistDetail(ctx, 211112, 0, 0, 3)
	fmt.Println("AI playlist:", p.Title, len(songs), err)
}
