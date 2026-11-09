//go:build integration

package xiaohongshu

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 登录页片段，结构取自小红书 / rednote 登录框和 redcaptcha 扫码验证弹窗。
const (
	fixtureLoginQr   = `<div class="login-container"><div class="qrcode"><img class="qrcode-img" src="data:image/png;base64,TE9HSU4="></div></div>`
	fixtureCaptchaQr = `<div class="fe-captcha-app"><div class="qrcode-container"><img class="qrcode-img" src="data:image/png;base64,Q0FQVENIQQ=="></div></div>`
)

// openLoginFixture 打开一个只有 body 的登录页，所有请求都返回它。
func openLoginFixture(t *testing.T, b *rod.Browser, body string) *rod.Page {
	t.Helper()
	page := b.MustPage()
	t.Cleanup(func() { _ = page.Close() })

	router := page.HijackRequests()
	router.MustAdd("*", func(h *rod.Hijack) {
		h.Response.SetHeader("Content-Type", "text/html; charset=utf-8")
		h.Response.SetBody(`<!doctype html><meta charset="utf-8"><body>` + body + `</body>`)
	})
	go router.Run()
	t.Cleanup(func() { _ = router.Stop() })

	require.NoError(t, page.Timeout(30*time.Second).Navigate("https://www.rednote.com/login"))
	require.NoError(t, page.Timeout(30*time.Second).WaitLoad())
	return page
}

func TestReadLoginPage(t *testing.T) {
	b := newSearchFixtureBrowser(t)

	cases := []struct {
		name string
		body string
		want LoginPageState
	}{
		{
			name: "等扫登录码",
			body: fixtureLoginQr,
			want: LoginPageState{Stage: StageWaitScan, QrSrc: "data:image/png;base64,TE9HSU4="},
		},
		{
			name: "登录码已扫，等手机确认",
			body: strings.Replace(fixtureLoginQr, `</div></div>`, `<button class="status-btn">重新扫码</button></div></div>`, 1),
			want: LoginPageState{Stage: StageScanned, QrSrc: "data:image/png;base64,TE9HSU4="},
		},
		{
			name: "登录码过期",
			body: strings.Replace(fixtureLoginQr, `</div></div>`, `<button class="status-desc refresh">刷新</button></div></div>`, 1),
			want: LoginPageState{Stage: StageQrExpired, QrSrc: "data:image/png;base64,TE9HSU4=", Expired: true},
		},
		{
			name: "扫码后要求用 App 再扫一次",
			body: fixtureLoginQr + fixtureCaptchaQr,
			want: LoginPageState{Stage: StageRescan, QrSrc: "data:image/png;base64,Q0FQVENIQQ==", Captcha: true},
		},
		{
			name: "验证码已扫，等手机确认",
			body: fixtureLoginQr + strings.Replace(fixtureCaptchaQr, `</div></div>`, `<div class="qr-scan"></div></div></div>`, 1),
			want: LoginPageState{Stage: StageScanned, QrSrc: "data:image/png;base64,Q0FQVENIQQ==", Captcha: true},
		},
		{
			name: "验证码过期",
			body: fixtureLoginQr + strings.Replace(fixtureCaptchaQr, `</div></div>`, `<div class="qr-scan cursor-pointer"></div></div></div>`, 1),
			want: LoginPageState{Stage: StageRescan, QrSrc: "data:image/png;base64,Q0FQVENIQQ==", Captcha: true, Expired: true},
		},
		{
			name: "滑块等其他验证",
			body: fixtureLoginQr + `<div class="fe-captcha-app"><div class="red-captcha-slider"></div></div>`,
			want: LoginPageState{Stage: StageVerification},
		},
		{
			name: "已登录",
			body: `<div class="main-container"><div class="user"><div class="link-wrapper"><span class="channel">我</span></div></div></div>`,
			want: LoginPageState{Stage: StageLoggedIn},
		},
		{
			name: "什么都认不出",
			body: `<div>加载中</div>`,
			want: LoginPageState{Stage: StageUnknown},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			page := openLoginFixture(t, b, c.body)

			got := NewLogin(page).ReadLoginPage(context.Background())

			assert.Equal(t, c.want, got)
		})
	}
}

func TestRefreshQrcode(t *testing.T) {
	b := newSearchFixtureBrowser(t)

	t.Run("点刷新后拿到新的验证码", func(t *testing.T) {
		body := fixtureLoginQr + `<div class="fe-captcha-app"><div class="qrcode-container">
<img class="qrcode-img" src="data:image/png;base64,T0xE">
<div class="qr-scan cursor-pointer" style="width:100px;height:100px"
 onclick="setTimeout(()=>{document.querySelector('.fe-captcha-app .qrcode-img').src='data:image/png;base64,TkVX';this.remove()},300)"></div>
</div></div>`
		page := openLoginFixture(t, b, body)
		login := NewLogin(page)
		old := login.ReadLoginPage(context.Background())
		require.True(t, old.Expired)

		got, err := login.RefreshQrcode(context.Background(), old)

		require.NoError(t, err)
		assert.Equal(t, LoginPageState{Stage: StageRescan, QrSrc: "data:image/png;base64,TkVX", Captcha: true}, got)
	})
}

func TestQrcodeImage(t *testing.T) {
	b := newSearchFixtureBrowser(t)

	t.Run("png 的 src 原样返回", func(t *testing.T) {
		page := openLoginFixture(t, b, fixtureLoginQr+fixtureCaptchaQr)
		login := NewLogin(page)

		img, err := login.QrcodeImage(context.Background(), LoginPageState{Stage: StageRescan, Captcha: true})

		require.NoError(t, err)
		assert.Equal(t, "data:image/png;base64,Q0FQVENIQQ==", img)
	})

	t.Run("src 是网址时对元素截图", func(t *testing.T) {
		body := `<div class="login-container"><img class="qrcode-img" style="width:50px;height:50px;background:#000" src="https://example.invalid/qr.png"></div>`
		page := openLoginFixture(t, b, body)

		img, err := NewLogin(page).QrcodeImage(context.Background(), LoginPageState{Stage: StageWaitScan})

		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(img, pngDataURLPrefix))
		assert.Greater(t, len(img), len(pngDataURLPrefix)+100)
	})
}

func TestWatchLogin(t *testing.T) {
	b := newSearchFixtureBrowser(t)

	t.Run("依次报出再扫一次和登录成功", func(t *testing.T) {
		body := fixtureLoginQr + `<script>
setTimeout(()=>{document.body.insertAdjacentHTML('beforeend', '` + fixtureCaptchaQr + `')}, 800);
setTimeout(()=>{document.body.innerHTML='<div class="main-container"><div class="user"><div class="link-wrapper"><span class="channel">我</span></div></div></div>'}, 1800);
</script>`
		page := openLoginFixture(t, b, body)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var stages []LoginStage
		ok := NewLogin(page).WatchLogin(ctx, func(st LoginPageState) { stages = append(stages, st.Stage) })

		assert.True(t, ok)
		assert.Equal(t, []LoginStage{StageWaitScan, StageRescan, StageLoggedIn}, stages)
	})

	t.Run("验证码加载中的空弹窗不算登录风控", func(t *testing.T) {
		body := fixtureLoginQr + `<script>
setTimeout(()=>{document.body.insertAdjacentHTML('beforeend', '<div class="fe-captcha-app"><div class="loading-container"></div></div>')}, 600);
setTimeout(()=>{document.querySelector('.fe-captcha-app').innerHTML='<div class="qrcode-container"><img class="qrcode-img" src="data:image/png;base64,Q0FQVENIQQ=="></div>'}, 1800);
setTimeout(()=>{document.body.innerHTML='<div class="main-container"><div class="user"><div class="link-wrapper"><span class="channel">我</span></div></div></div>'}, 2800);
</script>`
		page := openLoginFixture(t, b, body)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var stages []LoginStage
		ok := NewLogin(page).WatchLogin(ctx, func(st LoginPageState) { stages = append(stages, st.Stage) })

		assert.True(t, ok)
		assert.NotContains(t, stages, StageVerification)
		assert.Contains(t, stages, StageRescan)
	})

	t.Run("无二维码的验证弹窗持续出现才报登录风控", func(t *testing.T) {
		body := fixtureLoginQr + `<script>
setTimeout(()=>{document.body.insertAdjacentHTML('beforeend', '<div class="fe-captcha-app"><div class="red-captcha-slider"></div></div>')}, 600);
</script>`
		page := openLoginFixture(t, b, body)
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()

		var stages []LoginStage
		NewLogin(page).WatchLogin(ctx, func(st LoginPageState) { stages = append(stages, st.Stage) })

		assert.Equal(t, []LoginStage{StageWaitScan, StageUnknown, StageVerification}, stages)
	})

	t.Run("ctx 结束返回 false", func(t *testing.T) {
		page := openLoginFixture(t, b, fixtureLoginQr)
		ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
		defer cancel()

		ok := NewLogin(page).WatchLogin(ctx, func(LoginPageState) {})

		assert.False(t, ok)
	})
}
