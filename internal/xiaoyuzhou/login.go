package xiaoyuzhou

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

// LoginPage is 小宇宙's own login page, by phone number: its SMS code needs
// a captcha only a browser solves. Once logged in, the cookies of
// xiaoyuzhoufm.com named TokenCookie and RefreshCookie hold the tokens.
const (
	LoginPage     = accountsBase + "/login?redirectURL=https%3A%2F%2Fwww.xiaoyuzhoufm.com%2F"
	TokenCookie   = "x-jike-access-token"
	RefreshCookie = "x-jike-refresh-token"
)

// QR is a login QR code, to scan with the 小宇宙 app; the login comes as
// cookies of the jar its polls share, as on the login page.
type QR struct {
	URL   string // what the code holds
	Image []byte // a PNG of it
	id    string
	http  *http.Client
}

// QRState is where a QR login is.
type QRState int

const (
	QRWaiting QRState = iota // not scanned yet
	QRScanned                // scanned, waiting for the phone to confirm
	QRDone                   // logged in
	QRExpired                // the code expired: get a new one
)

func (s QRState) String() string {
	return [...]string{"等待扫码", "已扫码", "已登录", "二维码已失效"}[s]
}

// postAccounts posts body to the web API as the login page does.
func postAccounts(ctx context.Context, hc *http.Client, path string, body any) (*http.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webAPIBase+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", accountsBase)
	req.Header.Set("Referer", accountsBase+"/")
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("x-jike-allow-app-token-in-cookie", "true")
	// The id of the login page's app, as its scripts send it.
	req.Header.Set("x-midway-app-id", "v6worU4NnWyL")
	return hc.Do(req)
}

// NewQR gets a login QR code.
func (c *Client) NewQR(ctx context.Context) (*QR, error) {
	jar, _ := cookiejar.New(nil)
	hc := &http.Client{Timeout: c.HTTP.Timeout, Transport: c.HTTP.Transport, Jar: jar}
	// clientId is the service's name for its web login.
	resp, err := postAccounts(ctx, hc, "/v1/auth/qrcode/create", map[string]string{"clientId": "xyz-web"})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var v struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	json.NewDecoder(resp.Body).Decode(&v)
	if resp.StatusCode != http.StatusOK || v.ID == "" || v.URL == "" {
		return nil, fmt.Errorf("xiaoyuzhou: 获取二维码失败（HTTP %d）", resp.StatusCode)
	}
	code, err := qr.Encode(v.URL, qr.M)
	if err != nil {
		return nil, err
	}
	code.Scale = 6
	return &QR{URL: v.URL, Image: code.PNG(), id: v.ID, http: hc}, nil
}

// PollQR asks once where a QR login is. At QRDone the client has the
// login.
func (c *Client) PollQR(ctx context.Context, q *QR) (QRState, error) {
	resp, err := postAccounts(ctx, q.http, "/v1/auth/qrcode/login", map[string]string{"id": q.id})
	if err != nil {
		return QRWaiting, err
	}
	defer resp.Body.Close()
	var v struct {
		Status string `json:"status"`
		Code   int    `json:"code"`
	}
	json.NewDecoder(resp.Body).Decode(&v)
	switch {
	case resp.StatusCode == http.StatusUnauthorized && v.Code == 21:
		return QRExpired, nil
	case resp.StatusCode != http.StatusOK:
		return QRWaiting, fmt.Errorf("xiaoyuzhou: 查询扫码状态失败（HTTP %d，code %d）", resp.StatusCode, v.Code)
	case v.Status == "SCANNED":
		return QRScanned, nil
	// USED is a code confirmed, which the poll that saw it so used to log
	// in: the login page takes both for done.
	case v.Status != "CONFIRMED" && v.Status != "USED":
		return QRWaiting, nil
	}
	// The tokens come as headers, or else as cookies.
	access, refresh := resp.Header.Get(TokenCookie), resp.Header.Get(RefreshCookie)
	if access == "" {
		u, _ := url.Parse(webAPIBase + "/")
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
		return QRDone, fmt.Errorf("xiaoyuzhou: 已确认登录（%s），但没有拿到登录凭证，请刷新二维码重试", v.Status)
	}
	c.SetTokens(access, refresh)
	return QRDone, nil
}
