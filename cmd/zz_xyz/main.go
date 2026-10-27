//go:build probe

// Command zz_xyz is a local, throwaway probe of 小宇宙's user APIs: the
// profile, counts and podcasts of a host, which the app's host page reads,
// and the writes that follow a host and subscribe to a podcast. It reads
// the login the app saved, prints what the service answered, and writes a
// renewed login back, so the app's stays good.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"qqmusic/internal/xiaoyuzhou"
)

// saved is what the app keeps in xiaoyuzhou.json.
type saved struct {
	Credential xiaoyuzhou.Credential `json:"credential"`
	User       xiaoyuzhou.User       `json:"user"`
}

func main() {
	path := flag.String("cred", defaultPath(), "the xiaoyuzhou.json to read the login from")
	uid := flag.String("uid", "", "the host to look at; by default a host of the first subscription")
	follow := flag.Bool("follow", false, "follow and unfollow the host, to check the write")
	sub := flag.String("subscribe", "", "subscribe and unsubscribe this pid, to check the write")
	flag.Parse()

	var sv saved
	b, err := os.ReadFile(*path)
	check("read "+*path, err)
	check("parse "+*path, json.Unmarshal(b, &sv))
	c := xiaoyuzhou.New(sv.Credential)
	c.OnRefresh = func(cr xiaoyuzhou.Credential) {
		sv.Credential = cr
		save(*path, sv)
	}
	if !c.LoggedIn() {
		log.Fatal("no login in ", *path)
	}
	ctx := context.Background()

	me, err := c.Profile(ctx)
	check("Profile", err)
	fmt.Printf("me: uid=%s nickname=%q\n", me.UID, me.Nickname)

	if *uid == "" {
		*uid = hostOf(ctx, c, me.UID)
	}
	if *uid == "" {
		log.Fatal("no host to look at: pass -uid")
	}
	u, err := c.UserProfile(ctx, *uid)
	check("UserProfile", err)
	fmt.Printf("user: uid=%s nickname=%q bio=%q ip=%q relation=%q\n", u.UID, u.Nickname, u.Bio, u.IPLoc, u.Relation)

	stats, err := c.UserStats(ctx, *uid)
	check("UserStats", err)
	fmt.Printf("stats: following=%d followers=%d subscriptions=%d played=%ds\n",
		stats.FollowingCount, stats.FollowerCount, stats.SubscriptionCount, stats.TotalPlayedSeconds)

	shows, err := c.UserPodcasts(ctx, *uid)
	check("UserPodcasts", err)
	fmt.Printf("podcasts: %d\n", len(shows))
	for i, p := range shows {
		if i == 3 {
			break
		}
		fmt.Printf("  %q pid=%s episodes=%d subscribed=%v\n", p.Title, p.Pid, p.EpisodeCount, p.Subscribed())
	}

	if *follow {
		if u.UID == me.UID {
			log.Fatal("that host is the account itself")
		}
		was := u.Following()
		check("SetFollowing", c.SetFollowing(ctx, *uid, !was))
		got, err := c.UserProfile(ctx, *uid)
		// Put the account back where it was whatever the read said.
		if backErr := c.SetFollowing(ctx, *uid, was); backErr != nil {
			check("SetFollowing back", backErr)
		}
		check("UserProfile", err)
		fmt.Printf("followed=%v (was %v), relation=%q\n", got.Following(), was, got.Relation)
		if got.Following() == was {
			log.Fatal("SetFollowing: the relation did not change")
		}
	}

	if *sub != "" {
		p, err := c.Podcast(ctx, *sub)
		check("Podcast", err)
		was := p.Subscribed()
		fmt.Printf("podcast: %q subscribed=%v\n", p.Title, was)
		check("SetSubscription", c.SetSubscription(ctx, *sub, !was))
		p, err = c.Podcast(ctx, *sub)
		if backErr := c.SetSubscription(ctx, *sub, was); backErr != nil {
			check("SetSubscription back", backErr)
		}
		check("Podcast", err)
		fmt.Printf("subscribed=%v (was %v), status=%q\n", p.Subscribed(), was, p.SubscriptionStatus)
		if p.Subscribed() == was {
			log.Fatal("SetSubscription: the subscription did not change")
		}
	}
}

// hostOf is a host of the first podcast the account subscribes to, other
// than the account itself, or "" if there is none.
func hostOf(ctx context.Context, c *xiaoyuzhou.Client, me string) string {
	pods, _, err := c.Subscriptions(ctx, nil)
	if err != nil {
		log.Println("Subscriptions:", err)
		return ""
	}
	for _, p := range pods {
		for _, u := range p.Podcasters {
			if u.UID != "" && u.UID != me {
				return u.UID
			}
		}
	}
	return ""
}

func defaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "xiaoyuzhou.json"
	}
	return filepath.Join(dir, "QQMusicGo", "xiaoyuzhou.json")
}

func save(path string, sv saved) {
	b, err := json.MarshalIndent(sv, "", "  ")
	if err == nil {
		err = os.WriteFile(path, b, 0o600)
	}
	if err != nil {
		log.Println("save:", err)
	}
}

func check(what string, err error) {
	if err != nil {
		log.Fatalf("%s: %v", what, err)
	}
}
