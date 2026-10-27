package app

import (
	"context"
	"errors"

	"qqmusic/internal/qqmusic"
)

// startSession checks a saved login, refreshing it when it is old.
func (a *app) startSession() {
	if !a.api.LoggedIn() {
		return
	}
	a.loggingIn = true
	a.do(func(ctx context.Context) func() {
		if a.api.Credential().NeedsRefresh() {
			a.api.Refresh(ctx)
		}
		u, err := a.api.Profile(ctx)
		if errors.Is(err, qqmusic.ErrNotLoggedIn) {
			if a.api.Refresh(ctx) == nil {
				u, err = a.api.Profile(ctx)
			}
		}
		return func() {
			a.loggingIn = false
			if errors.Is(err, qqmusic.ErrNotLoggedIn) {
				a.api.Logout()
				a.saveCredential()
				a.closeUserMenu()
				a.toast("登录已过期，请重新登录")
				return
			}
			a.saveCredential()
			a.user = u
			a.loadMine()
		}
	})
}

// loggedIn is called on the main thread once a login succeeded.
func (a *app) loggedIn() {
	a.saveCredential()
	a.login.open = false
	a.login.gen.Add(1)
	a.toast("登录成功")
	a.do(func(ctx context.Context) func() {
		u, err := a.api.Profile(ctx)
		return func() {
			if err == nil {
				a.user = u
			}
			a.saveCredential() // with the encrypted uin Profile learned
			a.loadMine()
			a.loadHome(true)
		}
	})
}

func (a *app) loadMine() {
	a.do(func(ctx context.Context) func() {
		mine, err := a.api.CreatedPlaylists(ctx)
		fav, err2 := a.api.FavPlaylists(ctx)
		_, liked, err3 := a.api.AllPlaylistSongs(ctx, 0, 201)
		return func() {
			// What failed keeps what it showed: the playlists in the
			// sidebar, the hearts of the songs liked.
			if err != nil && len(mine) == 0 {
				a.toast("获取歌单失败：" + errText(err))
			} else {
				a.myLists = mine
			}
			if err2 == nil {
				a.favLists = fav
			}
			if err3 == nil {
				a.setLiked(liked)
			}
		}
	})
}

// setLiked shows as liked the songs of 我喜欢 and no others, but for the
// likes on the way: a list read before they are done does not have them.
func (a *app) setLiked(songs []qqmusic.Song) {
	a.liked = make(map[int64]bool, len(songs))
	for _, s := range songs {
		a.liked[int64(s.ID)] = true
	}
	for id, like := range a.liking {
		a.liked[id] = like
	}
}

// closeUserMenu drops the account's popup, with the login it is about.
func (a *app) closeUserMenu() {
	a.userMenu, a.userPopup = false, presence{}
}

func (a *app) logout() {
	a.api.Logout()
	a.saveCredential()
	a.closeUserMenu()
	a.user = qqmusic.User{}
	a.myLists, a.favLists = nil, nil
	a.liked, a.liking = map[int64]bool{}, map[int64]bool{}
	a.home.guess, a.home.daily, a.home.radar, a.home.feed = nil, nil, nil, nil
	a.queue.SetRadio(false)
	a.saveQueue()
	delete(a.lists, page{kind: pagePlaylist, dirid: 201})
	a.toast("已退出登录")
}

func (a *app) toggleLike(s qqmusic.Song) {
	if !fromQQ(s) {
		a.toast("小宇宙单集不能加入「我喜欢」")
		return
	}
	if !a.api.LoggedIn() {
		a.openLogin()
		return
	}
	id := int64(s.ID)
	like := !a.liked[id]
	a.liked[id], a.liking[id] = like, like
	a.do(func(ctx context.Context) func() {
		err := a.api.LikeSong(ctx, s, like)
		return func() {
			// Logged out meanwhile, the hearts were all forgotten.
			if !a.api.LoggedIn() {
				return
			}
			// Taken back meanwhile, the taking back is still on the way.
			if a.liking[id] == like {
				delete(a.liking, id)
			}
			switch {
			case err != nil:
				a.liked[id] = !like
				a.toast("操作失败：" + errText(err))
			case like:
				a.toast("已添加到我喜欢")
			default:
				a.toast("已取消喜欢")
			}
			// A load of 我喜欢 on the way began before the like was done: it
			// would show the heart as it was. It is dropped, and the list
			// loads again.
			liked := page{kind: pagePlaylist, dirid: 201}
			if l := a.lists[liked]; l != nil {
				l.gen++
				l.loading = false
				a.loadList(liked)
			}
		}
	})
}
