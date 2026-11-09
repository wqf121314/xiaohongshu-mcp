package xiaohongshu

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/pkg/errors"
	"github.com/xpzouying/xiaohongshu-mcp/humanize"
)

// qrcodeTimeout 等登录二维码出现的上限
const qrcodeTimeout = 20 * time.Second

type LoginAction struct {
	page *rod.Page
}

func NewLogin(page *rod.Page) *LoginAction {
	return &LoginAction{page: page}
}

func (a *LoginAction) CheckLoginStatus(ctx context.Context) (bool, error) {
	// 加超时保护：只是查登录态的快速检查，不应无限挂（登录扫码的等待在 Login/WatchLogin 里）
	pp := a.page.Context(ctx).Timeout(30 * time.Second)
	pp.MustNavigate("https://www.xiaohongshu.com/explore").MustWaitLoad()

	time.Sleep(1 * time.Second)

	exists, _, err := pp.Has(loggedInSelector)
	if err != nil {
		return false, errors.Wrap(err, "check login status failed")
	}

	if !exists {
		return false, errors.Wrap(err, "login status element not found")
	}

	return true, nil
}

// CurrentUser 当前登录用户的基础信息。
type CurrentUser struct {
	Nickname string `json:"nickname"`
	UserID   string `json:"userId"`
}

// CurrentUser 从当前页面的 __INITIAL_STATE__ 读取登录用户信息。
// 需在 CheckLoginStatus 之后调用：复用已加载的 explore 页，不做额外导航。
func (a *LoginAction) CurrentUser(ctx context.Context) (*CurrentUser, error) {
	pp := a.page.Context(ctx).Timeout(10 * time.Second)

	res, err := pp.Eval(`() => {
		const u = window.__INITIAL_STATE__ && window.__INITIAL_STATE__.user;
		const info = u && u.userInfo && u.userInfo.value !== undefined ? u.userInfo.value : (u && u.userInfo);
		if (!info || info.guest) return "";
		return JSON.stringify({nickname: info.nickname, userId: info.userId || info.user_id});
	}`)
	if err != nil {
		return nil, errors.Wrap(err, "read current user state failed")
	}

	raw := res.Value.String()
	if raw == "" {
		return nil, errors.New("current user not found in page state")
	}

	var user CurrentUser
	if err := json.Unmarshal([]byte(raw), &user); err != nil {
		return nil, errors.Wrap(err, "unmarshal current user failed")
	}

	return &user, nil
}

func (a *LoginAction) Login(ctx context.Context) error {
	pp := a.page.Context(ctx)

	// 导航到小红书首页，这会触发二维码弹窗
	pp.MustNavigate("https://www.xiaohongshu.com/explore").MustWaitLoad()

	time.Sleep(2 * time.Second)

	if exists, _, _ := pp.Has(loggedInSelector); exists {
		return nil
	}

	pp.MustElement(loggedInSelector)

	return nil
}

func (a *LoginAction) FetchQrcodeImage(ctx context.Context) (string, bool, error) {
	pp := a.page.Context(ctx)

	// 导航到小红书首页，这会触发二维码弹窗
	pp.MustNavigate("https://www.xiaohongshu.com/explore").MustWaitLoad()

	time.Sleep(2 * time.Second)

	if exists, _, _ := pp.Has(loggedInSelector); exists {
		return "", true, nil
	}

	// 海外会被跳到 rednote：首页不弹登录框，要去登录页取码
	if info, err := pp.Info(); err == nil && isRednoteURL(info.URL) {
		pp.MustNavigate(siteRednote.www + "/login").MustWaitLoad()
	}

	// 等二维码要有上限：页面结构变了或被拦截时不能一直挂着
	el, err := pp.Timeout(qrcodeTimeout).Element(loginQrSelector)
	if err != nil {
		return "", false, errors.Wrap(err, "页面上没有找到登录二维码")
	}
	src, err := el.CancelTimeout().Attribute("src")
	if err != nil {
		return "", false, errors.Wrap(err, "get qrcode src failed")
	}
	if src == nil || len(*src) == 0 {
		return "", false, errors.New("qrcode src is empty")
	}

	return *src, false, nil
}

// LoginStage 等扫码期间登录页所处的阶段。
type LoginStage string

const (
	StageWaitScan     LoginStage = "wait_scan"    // 等扫登录二维码
	StageScanned      LoginStage = "scanned"      // 已扫码，等手机上确认
	StageQrExpired    LoginStage = "qr_expired"   // 登录二维码过期
	StageRescan       LoginStage = "rescan"       // 安全验证：要用已登录该账号的 App 再扫一张码
	StageVerification LoginStage = "verification" // 安全验证：滑块、短信等，后台浏览器无法完成
	StageLoggedIn     LoginStage = "logged_in"
	StageUnknown      LoginStage = "unknown" // 页面上什么都没认出来，可能正在跳转
)

// 登录页上用来判断状态的元素。国内站和 rednote 的登录框、安全验证弹窗结构相同。
const (
	loggedInSelector = ".main-container .user .link-wrapper .channel"

	loginQrSelector      = ".login-container .qrcode-img"
	loginScannedSelector = ".login-container .status-btn"          // 「重新扫码」，已扫码待确认时才有
	loginExpiredSelector = ".login-container .status-desc.refresh" // 「刷新」，二维码过期时才有

	// 安全验证（redcaptcha）都挂在 .fe-captcha-app 下；有二维码的是「扫码验证身份」
	captchaSelector        = ".fe-captcha-app"
	captchaQrSelector      = ".fe-captcha-app .qrcode-img"
	captchaScannedSelector = ".fe-captcha-app .qr-scan:not(.cursor-pointer)" // 已扫码 / 验证成功
	captchaExpiredSelector = ".fe-captcha-app .qr-scan.cursor-pointer"       // 过期或绘制失败，点击刷新
)

// LoginPageState 登录页此刻的样子。
type LoginPageState struct {
	Stage   LoginStage
	QrSrc   string // 当前要扫的那张码的 src，用来判断码换没换
	Captcha bool   // 这张码是安全验证的码，不是登录码
	Expired bool   // 这张码已过期，要点刷新才能扫
}

// ReadLoginPage 读取登录页当前状态，只看不点。
func (a *LoginAction) ReadLoginPage(ctx context.Context) LoginPageState {
	pp := a.page.Context(ctx)
	find := func(sel string) *rod.Element {
		ok, el, err := pp.Has(sel)
		if err != nil || !ok {
			return nil
		}
		return el
	}
	srcOf := func(el *rod.Element) string {
		src, err := el.Attribute("src")
		if err != nil || src == nil {
			return ""
		}
		return *src
	}

	if find(loggedInSelector) != nil {
		return LoginPageState{Stage: StageLoggedIn}
	}

	// 安全验证弹窗盖在登录框上面，先看它
	if find(captchaSelector) != nil {
		qr := find(captchaQrSelector)
		if qr == nil {
			return LoginPageState{Stage: StageVerification}
		}
		st := LoginPageState{Stage: StageRescan, QrSrc: srcOf(qr), Captcha: true}
		if find(captchaExpiredSelector) != nil {
			st.Expired = true
		} else if find(captchaScannedSelector) != nil {
			st.Stage = StageScanned
		}
		return st
	}

	qr := find(loginQrSelector)
	if qr == nil {
		return LoginPageState{Stage: StageUnknown}
	}
	st := LoginPageState{Stage: StageWaitScan, QrSrc: srcOf(qr)}
	if find(loginExpiredSelector) != nil {
		st.Stage, st.Expired = StageQrExpired, true
	} else if find(loginScannedSelector) != nil {
		st.Stage = StageScanned
	}
	return st
}

// verifySettle 没有二维码的验证弹窗要持续这么久才算登录风控：
// 扫码验证的码加载出来之前，弹窗里也是空的。
const verifySettle = 3 * time.Second

// WatchLogin 每 500ms 读一次登录页，状态变了就回调。
// 登录成功返回 true；ctx 结束（超时或被新会话取代）返回 false。
func (a *LoginAction) WatchLogin(ctx context.Context, onChange func(LoginPageState)) bool {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var last LoginPageState
	var verifySince time.Time
	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			st := a.ReadLoginPage(ctx)
			// ctx 结束时读到的是空页面，不能当成状态变化
			if ctx.Err() != nil {
				return false
			}
			if st.Stage != StageVerification {
				verifySince = time.Time{}
			} else if verifySince.IsZero() {
				verifySince = time.Now()
			}
			if st.Stage == StageVerification && time.Since(verifySince) < verifySettle {
				st = LoginPageState{Stage: StageUnknown}
			}
			if st != last {
				onChange(st)
				last = st
			}
			if st.Stage == StageLoggedIn {
				return true
			}
		}
	}
}

// qrRefreshWait 点刷新后等新码出现的上限
const qrRefreshWait = 10 * time.Second

// RefreshQrcode 当前这张码过期时点刷新，等到新码出现后返回新状态。
func (a *LoginAction) RefreshQrcode(ctx context.Context, old LoginPageState) (LoginPageState, error) {
	sel := loginExpiredSelector
	if old.Captcha {
		sel = captchaExpiredSelector
	}
	// 点击和等新码共用一个上限
	ctx, cancel := context.WithTimeout(ctx, qrRefreshWait)
	defer cancel()

	el, err := a.page.Context(ctx).Sleeper(rod.NotFoundSleeper).Element(sel)
	if err != nil {
		return old, errors.Wrap(err, "没有找到二维码的刷新按钮")
	}
	if err := humanize.Click(el); err != nil {
		return old, errors.Wrap(err, "点击刷新二维码失败")
	}

	for ctx.Err() == nil {
		time.Sleep(500 * time.Millisecond)
		st := a.ReadLoginPage(ctx)
		if st.QrSrc != "" && st.QrSrc != old.QrSrc && !st.Expired {
			return st, nil
		}
	}
	return old, errors.New("刷新后没有等到新的二维码")
}

// QrcodeImage 取当前要扫的那张码，返回 data:image/png;base64 格式。
// src 本身就是 png 时直接用，否则对元素截图。
func (a *LoginAction) QrcodeImage(ctx context.Context, st LoginPageState) (string, error) {
	sel := loginQrSelector
	if st.Captcha {
		sel = captchaQrSelector
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	el, err := a.page.Context(ctx).Sleeper(rod.NotFoundSleeper).Element(sel)
	if err != nil {
		return "", errors.Wrap(err, "页面上没有找到二维码")
	}

	if src, err := el.Attribute("src"); err == nil && src != nil && strings.HasPrefix(*src, pngDataURLPrefix) {
		return *src, nil
	}
	shot, err := el.Screenshot(proto.PageCaptureScreenshotFormatPng, 0)
	if err != nil {
		return "", errors.Wrap(err, "二维码截图失败")
	}
	return pngDataURLPrefix + base64.StdEncoding.EncodeToString(shot), nil
}

const pngDataURLPrefix = "data:image/png;base64,"
