//go:build integration

package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	xhserrors "github.com/xpzouying/xiaohongshu-mcp/errors"
	"github.com/xpzouying/xiaohongshu-mcp/xiaohongshu"
)

// loginFixturePage 用本机浏览器打开一个固定内容的登录页。
func loginFixturePage(t *testing.T, body string) *rod.Page {
	t.Helper()
	bin := os.Getenv("XHS_TEST_BROWSER")
	if bin == "" {
		var ok bool
		if bin, ok = launcher.LookPath(); !ok {
			t.Skip("未找到本地浏览器，可用 XHS_TEST_BROWSER 指定路径")
		}
	}
	l := launcher.New().Bin(bin).Headless(true)
	controlURL, err := l.Launch()
	require.NoError(t, err)
	b := rod.New().ControlURL(controlURL).NoDefaultDevice()
	require.NoError(t, b.Connect())
	t.Cleanup(func() {
		_ = b.Close()
		l.Cleanup()
	})

	page := b.MustPage()
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

// 扫码后要求再扫一次：再取码时给出验证码（过期就先刷新），而不是另开会话。
func TestGetLoginQrcodeReusesLiveSession(t *testing.T) {
	page := loginFixturePage(t, `<div class="login-container"><img class="qrcode-img" src="data:image/png;base64,TE9HSU4="></div>
<div class="fe-captcha-app"><div class="qrcode-container">
<img class="qrcode-img" src="data:image/png;base64,T0xE">
<div class="qr-scan cursor-pointer" style="width:100px;height:100px"
 onclick="setTimeout(()=>{document.querySelector('.fe-captcha-app .qrcode-img').src='data:image/png;base64,TkVX';this.remove()},300)"></div>
</div></div>`)
	s := NewXiaohongshuService()
	s.logins.start(xiaohongshu.NewLogin(page), func() {}, time.Minute)

	got, err := s.GetLoginQrcode(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "rescan", got.Stage)
	assert.Equal(t, "data:image/png;base64,TkVX", got.Img)
	assert.Equal(t, loginQrWait.String(), got.Timeout)
}

// 出现滑块等验证时报登录风控，不给码。
func TestGetLoginQrcodeStopsOnVerification(t *testing.T) {
	page := loginFixturePage(t, `<div class="login-container"><img class="qrcode-img" src="data:image/png;base64,TE9HSU4="></div>
<div class="fe-captcha-app"><div class="red-captcha-slider"></div></div>`)
	s := NewXiaohongshuService()
	seq := s.logins.start(xiaohongshu.NewLogin(page), func() {}, time.Minute)

	t.Run("后台还没确认时只当作页面在跳转", func(t *testing.T) {
		_, err := s.GetLoginQrcode(context.Background())

		require.Error(t, err)
		assert.NotErrorIs(t, err, xhserrors.ErrLoginRiskControl)
	})

	t.Run("后台确认后报登录风控", func(t *testing.T) {
		s.logins.setState(seq, xiaohongshu.LoginPageState{Stage: xiaohongshu.StageVerification})

		_, err := s.GetLoginQrcode(context.Background())

		assert.ErrorIs(t, err, xhserrors.ErrLoginRiskControl)
	})
}
