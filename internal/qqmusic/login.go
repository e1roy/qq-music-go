package qqmusic

import (
	"context"
	crand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Credential is a QQ Music login.
type Credential struct {
	MusicID      int64  `json:"musicid"`
	StrMusicID   string `json:"str_musicid"`
	MusicKey     string `json:"musickey"`
	RefreshKey   string `json:"refresh_key"`
	RefreshToken string `json:"refresh_token"`
	AccessToken  string `json:"access_token"`
	OpenID       string `json:"openid"`
	UnionID      string `json:"unionid"`
	ExpiredAt    int64  `json:"expired_at"`
	CreateTime   int64  `json:"musickeyCreateTime"`
	KeyExpiresIn int64  `json:"keyExpiresIn"`
	EncryptUin   string `json:"encryptUin"`
	LoginType    int    `json:"loginType"` // 1 WeChat, 2 QQ
}

// UnmarshalJSON takes the numbers of the API as numbers or strings.
func (cr *Credential) UnmarshalJSON(b []byte) error {
	var raw struct {
		MusicID      flexInt `json:"musicid"`
		StrMusicID   string  `json:"str_musicid"`
		MusicKey     string  `json:"musickey"`
		RefreshKey   string  `json:"refresh_key"`
		RefreshToken string  `json:"refresh_token"`
		AccessToken  string  `json:"access_token"`
		OpenID       string  `json:"openid"`
		UnionID      string  `json:"unionid"`
		ExpiredAt    flexInt `json:"expired_at"`
		CreateTime   flexInt `json:"musickeyCreateTime"`
		KeyExpiresIn flexInt `json:"keyExpiresIn"`
		EncryptUin   string  `json:"encryptUin"`
		LoginType    flexInt `json:"loginType"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*cr = Credential{
		MusicID: int64(raw.MusicID), StrMusicID: raw.StrMusicID, MusicKey: raw.MusicKey, RefreshKey: raw.RefreshKey,
		RefreshToken: raw.RefreshToken, AccessToken: raw.AccessToken, OpenID: raw.OpenID, UnionID: raw.UnionID,
		ExpiredAt: int64(raw.ExpiredAt), CreateTime: int64(raw.CreateTime), KeyExpiresIn: int64(raw.KeyExpiresIn),
		EncryptUin: raw.EncryptUin, LoginType: int(raw.LoginType),
	}
	if cr.LoginType == 0 && cr.MusicKey != "" {
		if strings.HasPrefix(cr.MusicKey, "W_X") {
			cr.LoginType = 1
		} else {
			cr.LoginType = 2
		}
	}
	return nil
}

// NeedsRefresh reports whether the musickey is about to expire.
func (cr Credential) NeedsRefresh() bool {
	if cr.CreateTime == 0 || cr.KeyExpiresIn == 0 {
		return false
	}
	return time.Now().Unix() > cr.CreateTime+cr.KeyExpiresIn-24*3600
}

// QRKind is the app a login QR code is scanned with.
type QRKind int

const (
	QRQQ QRKind = iota
	QRWeChat
)

// QR is a login QR code to show.
type QR struct {
	Kind  QRKind
	Image []byte // PNG for QQ, JPEG for WeChat
	id    string // qrsig or uuid
	jar   http.CookieJar
}

// QRState is where a QR login is.
type QRState int

const (
	QRWaiting   QRState = iota // not scanned yet
	QRScanned                  // scanned, waiting for the phone to confirm
	QRDone                     // logged in
	QRExpired                  // the code expired: get a new one
	QRCancelled                // the user declined on the phone
)

func (s QRState) String() string {
	return [...]string{"等待扫码", "已扫码，请在手机上确认", "登录成功", "二维码已失效", "已取消登录"}[s]
}

func (c *Client) loginHTTP(jar http.CookieJar) *http.Client {
	return &http.Client{
		Jar:     jar,
		Timeout: 40 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func doGet(ctx context.Context, h *http.Client, u string, q url.Values, referer string) (*http.Response, []byte, error) {
	if q != nil {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	resp, err := h.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return resp, b, err
}

// NewQR gets a login QR code.
func (c *Client) NewQR(ctx context.Context, kind QRKind) (*QR, error) {
	jar, _ := cookiejar.New(nil)
	h := c.loginHTTP(jar)
	if kind == QRWeChat {
		_, page, err := doGet(ctx, h, "https://open.weixin.qq.com/connect/qrconnect", url.Values{
			"appid":         {"wx48db31d50e334801"},
			"redirect_uri":  {"https://y.qq.com/portal/wx_redirect.html?login_type=2&surl=https://y.qq.com/"},
			"response_type": {"code"}, "scope": {"snsapi_login"}, "state": {"STATE"},
			"href": {"https://y.qq.com/mediastyle/music_v17/src/css/popup_wechat.css#wechat_redirect"},
		}, "")
		if err != nil {
			return nil, err
		}
		m := regexp.MustCompile(`uuid=([^"&]+)"`).FindSubmatch(page)
		if m == nil {
			m = regexp.MustCompile(`/connect/qrcode/([^"]+)"`).FindSubmatch(page)
		}
		if m == nil {
			return nil, errors.New("qqmusic: 获取微信二维码失败")
		}
		uuid := string(m[1])
		_, img, err := doGet(ctx, h, "https://open.weixin.qq.com/connect/qrcode/"+uuid, nil, "https://open.weixin.qq.com/connect/qrconnect")
		if err != nil {
			return nil, err
		}
		return &QR{Kind: kind, Image: img, id: uuid, jar: jar}, nil
	}
	resp, img, err := doGet(ctx, h, "https://ssl.ptlogin2.qq.com/ptqrshow", url.Values{
		"appid": {"716027609"}, "e": {"2"}, "l": {"M"}, "s": {"3"}, "d": {"72"}, "v": {"4"},
		"t": {strconv.FormatFloat(rand.Float64(), 'f', 16, 64)}, "daid": {"383"}, "pt_3rd_aid": {"100497308"},
	}, "https://xui.ptlogin2.qq.com/")
	if err != nil {
		return nil, err
	}
	var qrsig string
	for _, ck := range resp.Cookies() {
		if ck.Name == "qrsig" {
			qrsig = ck.Value
		}
	}
	if qrsig == "" {
		return nil, errors.New("qqmusic: 获取 QQ 二维码失败")
	}
	return &QR{Kind: kind, Image: img, id: qrsig, jar: jar}, nil
}

var (
	ptuiRe   = regexp.MustCompile(`ptuiCB\((.*)\)`)
	ptArgRe  = regexp.MustCompile(`'((?:\\.|[^'])*)'`)
	sigxRe   = regexp.MustCompile(`[?&]ptsigx=([^&]+)`)
	uinRe    = regexp.MustCompile(`[?&]uin=([^&]+)`)
	wxCodeRe = regexp.MustCompile(`window\.wx_errcode=(\d+);window\.wx_code='([^']*)'`)
	codeRe   = regexp.MustCompile(`[?&]code=([^&]+)`)
)

// PollQR asks once where a QR login is; WeChat's answer waits up to half
// a minute. At QRDone the client has the login.
func (c *Client) PollQR(ctx context.Context, qr *QR) (QRState, error) {
	h := c.loginHTTP(qr.jar)
	if qr.Kind == QRWeChat {
		_, b, err := doGet(ctx, h, "https://lp.open.weixin.qq.com/connect/l/qrconnect", url.Values{
			"uuid": {qr.id}, "_": {strconv.FormatInt(time.Now().UnixMilli(), 10)},
		}, "https://open.weixin.qq.com/")
		if err != nil {
			var ne interface{ Timeout() bool }
			if errors.As(err, &ne) && ne.Timeout() && ctx.Err() == nil {
				return QRWaiting, nil
			}
			return QRWaiting, err
		}
		m := wxCodeRe.FindSubmatch(b)
		if m == nil {
			return QRWaiting, fmt.Errorf("qqmusic: 微信登录状态无法解析: %.100s", b)
		}
		switch string(m[1]) {
		case "405":
			return QRDone, c.loginWeChat(ctx, string(m[2]))
		case "404":
			return QRScanned, nil
		case "403":
			return QRCancelled, nil
		case "402", "500":
			return QRExpired, nil
		default: // 408: no change within the long poll
			return QRWaiting, nil
		}
	}
	_, b, err := doGet(ctx, h, "https://ssl.ptlogin2.qq.com/ptqrlogin", url.Values{
		"u1": {"https://graph.qq.com/oauth2.0/login_jump"}, "ptqrtoken": {strconv.Itoa(hash33(qr.id, 0))},
		"ptredirect": {"0"}, "h": {"1"}, "t": {"1"}, "g": {"1"}, "from_ui": {"1"}, "ptlang": {"2052"},
		"action": {fmt.Sprintf("0-0-%d", time.Now().UnixMilli())}, "js_ver": {"20102616"}, "js_type": {"1"},
		"pt_uistyle": {"40"}, "aid": {"716027609"}, "daid": {"383"}, "pt_3rd_aid": {"100497308"}, "has_onekey": {"1"},
	}, "https://xui.ptlogin2.qq.com/")
	if err != nil {
		return QRWaiting, err
	}
	m := ptuiRe.FindSubmatch(b)
	if m == nil {
		return QRWaiting, fmt.Errorf("qqmusic: QQ 登录状态无法解析: %.100s", b)
	}
	args := ptArgRe.FindAllSubmatch(m[1], -1)
	if len(args) == 0 {
		return QRWaiting, errors.New("qqmusic: QQ 登录状态无法解析")
	}
	switch string(args[0][1]) {
	case "0":
		if len(args) < 3 {
			return QRWaiting, errors.New("qqmusic: QQ 登录缺少跳转地址")
		}
		return QRDone, c.loginQQSig(ctx, qr.jar, string(args[2][1]))
	case "66":
		return QRWaiting, nil
	case "67":
		return QRScanned, nil
	case "65":
		return QRExpired, nil
	case "68":
		return QRCancelled, nil
	default:
		msg := ""
		if len(args) > 4 {
			msg = string(args[4][1])
		}
		return QRWaiting, fmt.Errorf("qqmusic: QQ 登录失败: %s", msg)
	}
}

// loginQQSig follows the check_sig URL of a QQ login, which sets p_skey.
func (c *Client) loginQQSig(ctx context.Context, jar http.CookieJar, checkSig string) error {
	sigx := sigxRe.FindStringSubmatch(checkSig)
	uin := uinRe.FindStringSubmatch(checkSig)
	if sigx == nil || uin == nil {
		return errors.New("qqmusic: QQ 登录跳转地址无法解析")
	}
	h := c.loginHTTP(jar)
	_, _, err := doGet(ctx, h, "https://ssl.ptlogin2.graph.qq.com/check_sig", url.Values{
		"uin": {uin[1]}, "pttype": {"1"}, "service": {"ptqrlogin"}, "nodirect": {"0"}, "ptsigx": {sigx[1]},
		"s_url": {"https://graph.qq.com/oauth2.0/login_jump"}, "ptlang": {"2052"}, "ptredirect": {"100"},
		"aid": {"716027609"}, "daid": {"383"}, "j_later": {"0"}, "low_login_hour": {"0"}, "regmaster": {"0"},
		"pt_login_type": {"3"}, "pt_aid": {"0"}, "pt_aaid": {"16"}, "pt_light": {"0"}, "pt_3rd_aid": {"100497308"},
	}, "https://xui.ptlogin2.qq.com/")
	if err != nil {
		return err
	}
	return c.LoginQQCookies(ctx, jar)
}

// LoginQQCookies finishes a QQ login whose cookies of graph.qq.com, p_skey
// among them, are in jar: as a QR login leaves them, or a QQ login page
// shown in a webview.
func (c *Client) LoginQQCookies(ctx context.Context, jar http.CookieJar) error {
	graph, _ := url.Parse("https://graph.qq.com/")
	var pskey string
	for _, ck := range jar.Cookies(graph) {
		if ck.Name == "p_skey" {
			pskey = ck.Value
		}
	}
	if pskey == "" {
		return errors.New("qqmusic: 没有拿到 p_skey，QQ 登录未完成")
	}
	form := url.Values{
		"response_type": {"code"}, "client_id": {"100497308"},
		"redirect_uri": {"https://y.qq.com/portal/wx_redirect.html?login_type=1&surl=https://y.qq.com/"},
		"scope":        {"get_user_info,get_app_friends"}, "state": {"state"}, "switch": {""},
		"from_ptlogin": {"1"}, "src": {"1"}, "update_auth": {"1"}, "openapi": {"1010_1030"},
		"g_tk": {strconv.Itoa(hash33(pskey, 5381))}, "auth_time": {strconv.FormatInt(time.Now().UnixMilli(), 10)},
		"ui": {uuid4()},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://graph.qq.com/oauth2.0/authorize", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", "https://xui.ptlogin2.qq.com/")
	resp, err := c.loginHTTP(jar).Do(req)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	m := codeRe.FindStringSubmatch(resp.Header.Get("Location"))
	if m == nil {
		return fmt.Errorf("qqmusic: QQ 授权失败 (HTTP %d)", resp.StatusCode)
	}
	var cr Credential
	if err := c.callComm(ctx, Web, map[string]any{"tmeLoginType": 2}, "QQConnectLogin.LoginServer", "QQLogin", map[string]any{"code": m[1]}, &cr); err != nil {
		return err
	}
	return c.acceptLogin(cr, 2)
}

func (c *Client) loginWeChat(ctx context.Context, code string) error {
	if code == "" {
		return errors.New("qqmusic: 微信登录没有返回 code")
	}
	var cr Credential
	if err := c.callComm(ctx, Web, map[string]any{"tmeLoginType": 1}, "music.login.LoginServer", "Login",
		map[string]any{"code": code, "strAppid": "wx48db31d50e334801"}, &cr); err != nil {
		return err
	}
	return c.acceptLogin(cr, 1)
}

func (c *Client) acceptLogin(cr Credential, loginType int) error {
	if cr.MusicKey == "" || cr.MusicID == 0 {
		return errors.New("qqmusic: 登录接口没有返回 musickey")
	}
	if cr.LoginType == 0 {
		cr.LoginType = loginType
	}
	if cr.CreateTime == 0 {
		cr.CreateTime = time.Now().Unix()
	}
	c.SetCredential(cr)
	return nil
}

// Refresh renews the musickey of the login.
func (c *Client) Refresh(ctx context.Context) error {
	cr := c.Credential()
	if cr.RefreshKey == "" {
		return errors.New("qqmusic: 登录凭证无法刷新")
	}
	param := map[string]any{
		"openid": cr.OpenID, "refresh_token": cr.RefreshToken, "refresh_key": cr.RefreshKey,
		"musickey": cr.MusicKey, "loginMode": 2,
	}
	switch cr.LoginType {
	case 1:
		param["str_musicid"] = cr.StrMusicID
		if cr.StrMusicID == "" {
			param["str_musicid"] = strconv.FormatInt(cr.MusicID, 10)
		}
		param["unionid"] = cr.UnionID
	default:
		param["access_token"] = cr.AccessToken
		param["expired_in"] = cr.ExpiredAt
		param["musicid"] = cr.MusicID
	}
	var fresh Credential
	if err := c.callComm(ctx, Web, map[string]any{"tmeLoginType": cr.LoginType}, "music.login.LoginServer", "Login", param, &fresh); err != nil {
		return err
	}
	if fresh.EncryptUin == "" {
		fresh.EncryptUin = cr.EncryptUin
	}
	return c.acceptLogin(fresh, cr.LoginType)
}

// Logout forgets the login.
func (c *Client) Logout() { c.SetCredential(Credential{}) }

func uuid4() string {
	var b [16]byte
	crand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func (c *Client) getCookie(ctx context.Context, u, cookie string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", "https://y.qq.com/")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Cookie", cookie)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// QQLoginPage is the QQ login page to show in a webview; once it goes to
// LoginDoneURLPrefix, its cookies of graph.qq.com finish the login with
// LoginQQCookies.
const QQLoginPage = "https://xui.ptlogin2.qq.com/cgi-bin/xlogin?appid=716027609&daid=383&style=33&login_text=%E7%99%BB%E5%BD%95&hide_title_bar=1&hide_border=1&target=self&s_url=https%3A%2F%2Fgraph.qq.com%2Foauth2.0%2Flogin_jump&pt_3rd_aid=100497308&pt_feedback_link=https%3A%2F%2Fsupport.qq.com%2Fproducts%2F77942%3FcustomInfo%3D.appid100497308&theme=2&verify_theme="

// LoginDoneURLPrefix is where the QQ login page goes once logged in.
const LoginDoneURLPrefix = "https://graph.qq.com/oauth2.0/login_jump"
