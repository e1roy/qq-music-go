// Package xyz is a small client of 小宇宙 (Xiaoyuzhou FM): its app's API
// at api.xiaoyuzhoufm.com, which takes the tokens of a login by QR code
// scanned with the 小宇宙 app, and its web pages, which need none.
package xyz

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	apiURL    = "https://api.xiaoyuzhoufm.com"
	webAPIURL = "https://web-api.xiaoyuzhoufm.com"
	siteURL   = "https://www.xiaoyuzhoufm.com"
	browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

	// The iOS app the API is told it is. The open source clients all send
	// this one; should the service turn it away, a newer one goes here.
	appVersion = "2.57.1"
	appBuild   = "1576"
)

// ErrNotLoggedIn is returned by calls that need a login, with none or one
// that expired and could not be refreshed.
var ErrNotLoggedIn = errors.New("xyz: 未登录或登录已过期")

// Credential is a login: the tokens the QR code gave, and the device the
// app says it is, made once and kept.
type Credential struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	DeviceID     string `json:"device_id"`
}

// Client calls 小宇宙. Its methods are safe for concurrent use.
type Client struct {
	HTTP *http.Client
	// Refreshed is called with the login once its tokens were refreshed,
	// on the goroutine of the call, for it to be saved.
	Refreshed func(Credential)

	mu   sync.Mutex
	cred Credential
}

// New returns a client with the login cr, which may be the zero value.
func New(cr Credential) *Client {
	if cr.DeviceID == "" {
		cr.DeviceID = newDeviceID()
	}
	return &Client{HTTP: &http.Client{Timeout: 20 * time.Second}, cred: cr}
}

// newDeviceID is an iOS identifierForVendor: an upper case UUID.
func newDeviceID() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return strings.ToUpper(fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]))
}

// Credential returns the login in use.
func (c *Client) Credential() Credential {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cred
}

// LoggedIn reports whether the client has a login.
func (c *Client) LoggedIn() bool { return c.Credential().AccessToken != "" }

// Logout forgets the tokens; the device stays.
func (c *Client) Logout() {
	c.mu.Lock()
	c.cred.AccessToken, c.cred.RefreshToken = "", ""
	c.mu.Unlock()
}

func (c *Client) setTokens(access, refresh string) {
	c.mu.Lock()
	c.cred.AccessToken, c.cred.RefreshToken = access, refresh
	c.mu.Unlock()
}

// APIError is what the service said of a request it turned away.
type APIError struct {
	Status int
	Code   int
	Toast  string // in Chinese, for the user
}

func (e *APIError) Error() string {
	if e.Toast != "" {
		return "小宇宙：" + e.Toast
	}
	return fmt.Sprintf("小宇宙：HTTP %d（code %d）", e.Status, e.Code)
}

// appHeaders are those of the iOS app, as the open source clients send.
func (c *Client) appHeaders(h http.Header, cr Credential) {
	h.Set("User-Agent", "Xiaoyuzhou/"+appVersion+" (build:"+appBuild+"; iOS 17.4.1)")
	h.Set("App-Version", appVersion)
	h.Set("App-BuildNo", appBuild)
	h.Set("BundleID", "app.podcast.cosmos")
	h.Set("OS", "ios")
	h.Set("OS-Version", "17.4.1")
	h.Set("Model", "iPhone14,2")
	h.Set("Manufacturer", "Apple")
	h.Set("Market", "AppStore")
	h.Set("app-permissions", "4")
	h.Set("WifiConnected", "true")
	h.Set("Accept-Language", "zh-Hans-CN;q=1.0")
	h.Set("Timezone", "Asia/Shanghai")
	h.Set("Local-Time", time.Now().Format(time.RFC3339))
	h.Set("x-jike-device-id", cr.DeviceID)
	if cr.AccessToken != "" {
		h.Set("x-jike-access-token", cr.AccessToken)
	}
}

// envelope is what the API answers: data, an object or a list, and where
// a list goes on.
type envelope struct {
	Data        json.RawMessage `json:"data"`
	LoadMoreKey json.RawMessage `json:"loadMoreKey"`
}

// call sends a request to the API: a GET without body, a POST of body as
// JSON. A login that expired is refreshed once and the request sent again.
func (c *Client) call(ctx context.Context, path string, body any) (envelope, error) {
	var env envelope
	cr := c.Credential()
	if cr.AccessToken == "" {
		return env, ErrNotLoggedIn
	}
	status, raw, err := c.send(ctx, path, body, cr)
	if err == nil && status == http.StatusUnauthorized {
		if err = c.refresh(ctx, cr); err != nil {
			return env, err
		}
		status, raw, err = c.send(ctx, path, body, c.Credential())
		if err == nil && status == http.StatusUnauthorized {
			return env, ErrNotLoggedIn
		}
	}
	if err != nil {
		return env, err
	}
	if status != http.StatusOK {
		return env, apiError(status, raw)
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return env, fmt.Errorf("xyz: %s: %w", path, err)
	}
	return env, nil
}

func (c *Client) send(ctx context.Context, path string, body any, cr Credential) (int, []byte, error) {
	method, rd := http.MethodGet, io.Reader(nil)
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		method, rd = http.MethodPost, bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, apiURL+path, rd)
	if err != nil {
		return 0, nil, err
	}
	c.appHeaders(req.Header, cr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	return resp.StatusCode, raw, err
}

func apiError(status int, raw []byte) error {
	e := &APIError{Status: status}
	var v struct {
		Code  int    `json:"code"`
		Toast string `json:"toast"`
	}
	if json.Unmarshal(raw, &v) == nil {
		e.Code, e.Toast = v.Code, v.Toast
	}
	return e
}

// refresh trades the refresh token of cr for new tokens, unless another
// call did already.
func (c *Client) refresh(ctx context.Context, cr Credential) error {
	c.mu.Lock()
	if c.cred.AccessToken != cr.AccessToken {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()
	if cr.RefreshToken == "" {
		return ErrNotLoggedIn
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/app_auth_tokens.refresh", nil)
	if err != nil {
		return err
	}
	c.appHeaders(req.Header, cr)
	req.Header.Set("x-jike-refresh-token", cr.RefreshToken)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var v struct {
		Success bool   `json:"success"`
		Access  string `json:"x-jike-access-token"`
		Refresh string `json:"x-jike-refresh-token"`
	}
	json.NewDecoder(resp.Body).Decode(&v)
	if resp.StatusCode != http.StatusOK || v.Access == "" {
		return ErrNotLoggedIn
	}
	if v.Refresh == "" {
		v.Refresh = cr.RefreshToken
	}
	c.setTokens(v.Access, v.Refresh)
	if c.Refreshed != nil {
		c.Refreshed(c.Credential())
	}
	return nil
}

// page fetches a web page of the site as a browser: without the UA of
// one, it answers 403.
func (c *Client) page(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", browserUA)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("xyz: GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}
