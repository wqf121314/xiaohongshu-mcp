//go:build integration

package xiaohongshu

import (
	"context"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/stretchr/testify/require"
	"github.com/xpzouying/xiaohongshu-mcp/errors"
)

// 登录失效时首页侧边栏没有「我」，应马上报未登录，而不是等满 60 秒超时。
func TestToProfilePageNotLoggedInFailsFast(t *testing.T) {
	b := newSearchFixtureBrowser(t)
	page := b.MustPage()
	defer page.Close()

	router := page.HijackRequests()
	router.MustAdd("*", func(h *rod.Hijack) {
		h.Response.SetHeader("Content-Type", "text/html; charset=utf-8")
		h.Response.SetBody(`<!doctype html><meta charset="utf-8">
<div id="app"><div class="main-container"><ul><li class="side-bar-component">发现</li></ul>
<button class="login-btn">登录</button></div></div>`)
	})
	go router.Run()
	defer func() { _ = router.Stop() }()

	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()

	start := time.Now()
	err := NewNavigate(page).ToProfilePage(ctx)

	require.ErrorIs(t, err, errors.ErrLoginRequired)
	require.Less(t, time.Since(start), 20*time.Second)
}
