// Package xiaoyuzhou talks to 小宇宙 (Xiaoyuzhou FM) directly: the API its
// iOS app uses, logged in by a QR code or the tokens of its login page,
// and the podcast pages of its web site, which need no login.
package xiaoyuzhou

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
	apiBase      = "https://api.xiaoyuzhoufm.com"
	webAPIBase   = "https://web-api.xiaoyuzhoufm.com"
	siteBase     = "https://www.xiaoyuzhoufm.com"
	accountsBase = "https://accounts.xiaoyuzhoufm.com"
	browserUA    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

	// pageSize is how many items a page of a list asks for, a string as
	// the app sends it.
	pageSize = "20"
)

// iPhone is the headers of the app the API takes the client for: it
// answers what its app asks. Should it turn this version away, a newer
// one goes here.
var iPhone = [][2]string{
	{"User-Agent", "Xiaoyuzhou/2.57.1 (build:1576; iOS 17.4.1)"},
	{"App-Version", "2.57.1"},
	{"App-BuildNo", "1576"},
	{"BundleID", "app.podcast.cosmos"},
	{"OS", "ios"},
	{"OS-Version", "17.4.1"},
	{"Model", "iPhone14,2"},
	{"Manufacturer", "Apple"},
	{"Market", "AppStore"},
	{"app-permissions", "4"},
	{"WifiConnected", "true"},
	{"Accept-Language", "zh-Hans-CN;q=1.0"},
	{"Timezone", "Asia/Shanghai"},
}

// ErrNotLoggedIn is returned by calls that need a login, with none or one
// whose tokens could not be renewed.
var ErrNotLoggedIn = errors.New("xiaoyuzhou: 未登录或登录已过期")

// APIError is a request the service turned away.
type APIError struct {
	Status  int
	Code    int
	Message string // the service's own, in Chinese
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return "小宇宙：" + e.Message
	}
	return fmt.Sprintf("小宇宙：HTTP %d（code %d）", e.Status, e.Code)
}

// Credential is a login: its tokens, and the device the client says it
// is, made once and kept across logins.
type Credential struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	DeviceID     string `json:"device_id"`
}

// Client calls 小宇宙. Its methods are safe for concurrent use.
type Client struct {
	HTTP *http.Client
	// OnRefresh is called with the login once its tokens were renewed, on
	// the goroutine of the request, for it to be saved.
	OnRefresh func(Credential)

	mu   sync.Mutex
	cred Credential
}

// New returns a client of the login cr, which may be the zero value; a
// device is made for one without.
func New(cr Credential) *Client {
	if cr.DeviceID == "" {
		// An iOS identifierForVendor: a version 4 UUID, upper case.
		var b [16]byte
		rand.Read(b[:])
		b[6] = b[6]&0x0f | 0x40
		b[8] = b[8]&0x3f | 0x80
		cr.DeviceID = strings.ToUpper(fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]))
	}
	return &Client{HTTP: &http.Client{Timeout: 20 * time.Second}, cred: cr}
}

// Credential returns the login in use.
func (c *Client) Credential() Credential {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cred
}

// LoggedIn reports whether the client has a login.
func (c *Client) LoggedIn() bool { return c.Credential().AccessToken != "" }

// SetTokens logs in with tokens got elsewhere, as by LoginPage.
func (c *Client) SetTokens(access, refresh string) {
	c.mu.Lock()
	c.cred.AccessToken, c.cred.RefreshToken = access, refresh
	c.mu.Unlock()
}

// Logout forgets the tokens and keeps the device.
func (c *Client) Logout() { c.SetTokens("", "") }

// Cursor is where a list goes on: the service's own key, given back as it
// came for the next page; nil once the list ends.
type Cursor json.RawMessage

func cursor(raw json.RawMessage) Cursor {
	switch string(raw) {
	case "", "null", "{}":
		return nil
	}
	return Cursor(raw)
}

// into adds the cursor to the body of a list's request.
func (cur Cursor) into(body map[string]any) map[string]any {
	if cur != nil {
		body["loadMoreKey"] = json.RawMessage(cur)
	}
	return body
}

// request calls the API at path, a GET, or with a body a POST of it as
// JSON, and decodes the data of the answer into out; a list's answer also
// gives the cursor of its next page. Tokens that expired are renewed once
// and the request is made again.
func (c *Client) request(ctx context.Context, path string, body, out any) (Cursor, error) {
	cr := c.Credential()
	if cr.AccessToken == "" {
		return nil, ErrNotLoggedIn
	}
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return nil, err
		}
	}
	status, raw, err := c.send(ctx, path, payload, cr)
	if err == nil && status == http.StatusUnauthorized {
		if err = c.renew(ctx, cr); err == nil {
			status, raw, err = c.send(ctx, path, payload, c.Credential())
		}
	}
	switch {
	case err != nil:
		return nil, err
	case status == http.StatusUnauthorized:
		return nil, ErrNotLoggedIn
	case status != http.StatusOK:
		e := &APIError{Status: status}
		var v struct {
			Code  int    `json:"code"`
			Toast string `json:"toast"`
		}
		if json.Unmarshal(raw, &v) == nil {
			e.Code, e.Message = v.Code, v.Toast
		}
		return nil, e
	}
	var r struct {
		Data json.RawMessage `json:"data"`
		Next json.RawMessage `json:"loadMoreKey"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("xiaoyuzhou: %s: %w", path, err)
	}
	if len(r.Data) > 0 {
		if err := json.Unmarshal(r.Data, out); err != nil {
			return nil, fmt.Errorf("xiaoyuzhou: %s: %w", path, err)
		}
	}
	return cursor(r.Next), nil
}

func (c *Client) send(ctx context.Context, path string, payload []byte, cr Credential) (int, []byte, error) {
	method, body := http.MethodGet, io.Reader(nil)
	if payload != nil {
		method, body = http.MethodPost, bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, apiBase+path, body)
	if err != nil {
		return 0, nil, err
	}
	setAppHeaders(req.Header, cr)
	if payload != nil {
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

func setAppHeaders(h http.Header, cr Credential) {
	for _, kv := range iPhone {
		h.Set(kv[0], kv[1])
	}
	h.Set("Local-Time", time.Now().Format(time.RFC3339))
	h.Set("x-jike-device-id", cr.DeviceID)
	if cr.AccessToken != "" {
		h.Set("x-jike-access-token", cr.AccessToken)
	}
}

// renew trades the refresh token of cr for new tokens, unless a request
// that failed with it alongside did already.
func (c *Client) renew(ctx context.Context, cr Credential) error {
	if c.Credential().AccessToken != cr.AccessToken {
		return nil
	}
	if cr.RefreshToken == "" {
		return ErrNotLoggedIn
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+"/app_auth_tokens.refresh", nil)
	if err != nil {
		return err
	}
	setAppHeaders(req.Header, cr)
	req.Header.Set("x-jike-refresh-token", cr.RefreshToken)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var v struct {
		Access  string `json:"x-jike-access-token"`
		Refresh string `json:"x-jike-refresh-token"`
	}
	json.NewDecoder(resp.Body).Decode(&v)
	if resp.StatusCode != http.StatusOK || v.Access == "" {
		return ErrNotLoggedIn
	}
	// The refresh token may be kept for the next time.
	if v.Refresh == "" {
		v.Refresh = cr.RefreshToken
	}
	c.SetTokens(v.Access, v.Refresh)
	if c.OnRefresh != nil {
		c.OnRefresh(c.Credential())
	}
	return nil
}

// webPage fetches a page of the web site as a browser does: without a
// browser's User-Agent it answers 403.
func (c *Client) webPage(ctx context.Context, url string) ([]byte, error) {
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
		return nil, fmt.Errorf("xiaoyuzhou: GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}
