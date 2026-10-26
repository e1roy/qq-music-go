// Package qqmusic is a small client of the QQ Music web APIs: search,
// charts, playlists, lyrics, playback URLs and login by QR code or by the
// cookies of a QQ login page.
package qqmusic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	musicuURL = "https://u.y.qq.com/cgi-bin/musicu.fcg"
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

// ErrNotLoggedIn is returned by calls that need a valid login.
var ErrNotLoggedIn = errors.New("qqmusic: 未登录或登录已过期")

// APIError is a non-zero code of a musicu request.
type APIError struct {
	Module, Method string
	Code           int
}

func (e *APIError) Error() string {
	return fmt.Sprintf("qqmusic: %s.%s code %d", e.Module, e.Method, e.Code)
}

// Platform chooses the comm parameters of a request.
type Platform int

const (
	// Web is y.qq.com: the platform login and playback URLs need.
	Web Platform = iota
	// Desktop is the Windows client: search answers it only.
	Desktop
)

// Client calls the APIs. Its methods are safe for concurrent use.
type Client struct {
	HTTP *http.Client

	mu   sync.RWMutex
	cred Credential
	guid string
}

// New returns a client with no login.
func New() *Client {
	return &Client{
		HTTP: &http.Client{Timeout: 20 * time.Second},
		guid: strconv.FormatInt(rand.Int64N(9e9)+1e9, 10),
	}
}

// Credential returns the login in use.
func (c *Client) Credential() Credential {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cred
}

// SetCredential replaces the login, the zero value to log out.
func (c *Client) SetCredential(cr Credential) {
	c.mu.Lock()
	c.cred = cr
	c.mu.Unlock()
}

// LoggedIn reports whether the client has a login.
func (c *Client) LoggedIn() bool {
	cr := c.Credential()
	return cr.MusicID != 0 && cr.MusicKey != ""
}

// hash33 is the g_tk hash of QQ's web pages.
func hash33(s string, h int) int {
	for _, r := range s {
		h += (h << 5) + int(r)
	}
	return h & 0x7fffffff
}

// Req is one module call in a musicu request.
type Req struct {
	Module string `json:"module"`
	Method string `json:"method"`
	Param  any    `json:"param"`
}

type reply struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
}

func (c *Client) comm(p Platform, cr Credential, extra map[string]any) map[string]any {
	m := map[string]any{}
	switch p {
	case Desktop:
		m["ct"], m["cv"] = 19, 2201
		m["platform"], m["chid"] = "0", "0"
		m["guid"] = c.guid
	default:
		m["ct"], m["cv"] = 24, 4747474
		m["platform"], m["chid"] = "yqq.json", "0"
		m["format"], m["inCharset"], m["outCharset"] = "json", "utf-8", "utf-8"
		m["notice"], m["need_new_code"] = 0, 1
	}
	gtk := 5381
	if cr.MusicKey != "" {
		gtk = hash33(cr.MusicKey, 5381)
		m["uin"] = cr.MusicID
		m["qq"] = strconv.FormatInt(cr.MusicID, 10)
		m["authst"] = cr.MusicKey
		m["tmeLoginType"] = cr.LoginType
	} else {
		m["uin"] = 0
	}
	m["g_tk"], m["g_tk_new_20200303"] = gtk, gtk
	for k, v := range extra {
		m[k] = v
	}
	return m
}

// CallMany sends several module calls in one request and returns each
// call's data by its name. A call whose code is not 0 gets an error.
func (c *Client) CallMany(ctx context.Context, p Platform, extraComm map[string]any, reqs map[string]Req) (map[string]json.RawMessage, map[string]error, error) {
	cr := c.Credential()
	body := map[string]any{"comm": c.comm(p, cr, extraComm)}
	for k, r := range reqs {
		body[k] = r
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, nil, err
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, musicuURL, bytes.NewReader(b))
	if err != nil {
		return nil, nil, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("Referer", "https://y.qq.com/")
	hreq.Header.Set("Origin", "https://y.qq.com")
	hreq.Header.Set("User-Agent", userAgent)
	if cr.MusicKey != "" {
		uin := strconv.FormatInt(cr.MusicID, 10)
		hreq.Header.Set("Cookie", fmt.Sprintf("uin=%s; qqmusic_uin=%s; qm_keyst=%s; qqmusic_key=%s", uin, uin, cr.MusicKey, cr.MusicKey))
	}
	resp, err := c.HTTP.Do(hreq)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("qqmusic: HTTP %s", resp.Status)
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil, nil, fmt.Errorf("qqmusic: %w", err)
	}
	data := map[string]json.RawMessage{}
	errs := map[string]error{}
	for k, r := range reqs {
		var rep reply
		if err := json.Unmarshal(all[k], &rep); err != nil {
			errs[k] = fmt.Errorf("qqmusic: %s.%s: %w", r.Module, r.Method, err)
			continue
		}
		if rep.Code != 0 {
			if rep.Code == 1000 || rep.Code == 104400 || rep.Code == 104401 {
				errs[k] = ErrNotLoggedIn
			} else {
				errs[k] = &APIError{r.Module, r.Method, rep.Code}
			}
			data[k] = rep.Data
			continue
		}
		data[k] = rep.Data
	}
	return data, errs, nil
}

// Call sends one module call and decodes its data into out.
func (c *Client) Call(ctx context.Context, p Platform, module, method string, param, out any) error {
	return c.callComm(ctx, p, nil, module, method, param, out)
}

func (c *Client) callComm(ctx context.Context, p Platform, extra map[string]any, module, method string, param, out any) error {
	if param == nil {
		param = map[string]any{}
	}
	data, errs, err := c.CallMany(ctx, p, extra, map[string]Req{"req": {module, method, param}})
	if err != nil {
		return err
	}
	if err := errs["req"]; err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data["req"], out); err != nil {
		return fmt.Errorf("qqmusic: %s.%s: %w", module, method, err)
	}
	return nil
}

// get fetches a URL with QQ Music's headers.
func (c *Client) get(ctx context.Context, url, referer string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if referer == "" {
		referer = "https://y.qq.com/"
	}
	req.Header.Set("Referer", referer)
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("qqmusic: GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// Fetch downloads a picture or other public file.
func (c *Client) Fetch(ctx context.Context, url string) ([]byte, error) {
	if strings.HasPrefix(url, "http://") {
		url = "https://" + strings.TrimPrefix(url, "http://")
	}
	return c.get(ctx, url, "")
}

// flexInt decodes a JSON number or a string of one.
type flexInt int64

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		fl, err2 := strconv.ParseFloat(s, 64)
		if err2 != nil {
			return err
		}
		n = int64(fl)
	}
	*f = flexInt(n)
	return nil
}
