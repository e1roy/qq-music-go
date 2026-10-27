package xyz

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"

	"rsc.io/qr"
)

// The login by QR code is that of accounts.xiaoyuzhoufm.com, which needs
// no captcha, unlike the one by SMS: the code opens a page of the app,
// which asks the user to confirm.
const (
	accountsURL = "https://accounts.xiaoyuzhoufm.com"
	qrClientID  = "xyz-web"
	qrAppID     = "v6worU4NnWyL"
)

// LoginPage is 小宇宙's own login page, by phone number and SMS code behind
// a captcha that only a browser solves; once logged in, its cookies of
// xiaoyuzhoufm.com hold the tokens, named TokenCookie and RefreshCookie.
const (
	LoginPage     = accountsURL + "/login?redirectURL=https%3A%2F%2Fwww.xiaoyuzhoufm.com%2F"
	TokenCookie   = "x-jike-access-token"
	RefreshCookie = "x-jike-refresh-token"
)

// SetTokens logs in with the tokens of a login made elsewhere, as on
// LoginPage.
func (c *Client) SetTokens(access, refresh string) { c.setTokens(access, refresh) }

// QR is a code to scan with the 小宇宙 app. Its polls share a cookie jar,
// as the page's do: the login comes as cookies.
type QR struct {
	ID    string
	URL   string // what the code holds
	Image []byte // a PNG of it
	http  *http.Client
}

// QRState is how far the login of a QR code went.
type QRState int

const (
	QRWaiting QRState = iota
	QRScanned
	QRDone
	QRExpired
)

func (s QRState) String() string {
	return [...]string{"等待扫码", "已扫码", "已登录", "二维码已失效"}[s]
}

// webPost posts body to the web API as the accounts page does.
func webPost(ctx context.Context, hc *http.Client, path string, body any, h map[string]string) (*http.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webAPIURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", accountsURL)
	req.Header.Set("Referer", accountsURL+"/")
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("x-jike-allow-app-token-in-cookie", "true")
	for k, v := range h {
		req.Header.Set(k, v)
	}
	return hc.Do(req)
}

// NewQR gets a QR code to log in with.
func (c *Client) NewQR(ctx context.Context) (*QR, error) {
	jar, _ := cookiejar.New(nil)
	hc := &http.Client{Timeout: c.HTTP.Timeout, Jar: jar}
	resp, err := webPost(ctx, hc, "/v1/auth/qrcode/create", map[string]string{"clientId": qrClientID}, map[string]string{"x-midway-app-id": qrAppID})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var v struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil || resp.StatusCode != http.StatusOK || v.ID == "" || v.URL == "" {
		return nil, fmt.Errorf("xyz: 获取二维码失败（HTTP %d）", resp.StatusCode)
	}
	code, err := qr.Encode(v.URL, qr.M)
	if err != nil {
		return nil, err
	}
	code.Scale = 6
	return &QR{ID: v.ID, URL: v.URL, Image: code.PNG(), http: hc}, nil
}

// PollQR asks how far the login of q went. Once confirmed, the client has
// the login, which the answers set as cookies, or as headers.
func (c *Client) PollQR(ctx context.Context, q *QR) (QRState, error) {
	resp, err := webPost(ctx, q.http, "/v1/auth/qrcode/login", map[string]string{"id": q.ID}, nil)
	if err != nil {
		return QRWaiting, err
	}
	defer resp.Body.Close()
	var v struct {
		Status string `json:"status"`
		Code   int    `json:"code"`
	}
	json.NewDecoder(resp.Body).Decode(&v)
	if resp.StatusCode == http.StatusUnauthorized && v.Code == 21 {
		return QRExpired, nil
	}
	if resp.StatusCode != http.StatusOK {
		return QRWaiting, fmt.Errorf("xyz: 查询扫码状态失败（HTTP %d，code %d）", resp.StatusCode, v.Code)
	}
	// The page takes USED, as CONFIRMED, for logged in: the code was used
	// to log in, by the poll that saw it confirmed.
	switch v.Status {
	case "SCANNED":
		return QRScanned, nil
	case "CONFIRMED", "USED":
	default:
		return QRWaiting, nil
	}
	access, refresh := resp.Header.Get("x-jike-access-token"), resp.Header.Get("x-jike-refresh-token")
	if access == "" {
		u, _ := url.Parse(webAPIURL + "/")
		for _, ck := range q.http.Jar.Cookies(u) {
			switch ck.Name {
			case TokenCookie:
				access = ck.Value
			case RefreshCookie:
				refresh = ck.Value
			}
		}
	}
	if access == "" {
		return QRDone, fmt.Errorf("xyz: 已确认登录（%s），但没有拿到登录凭证，请刷新二维码重试", v.Status)
	}
	c.setTokens(access, refresh)
	return QRDone, nil
}
