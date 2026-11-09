package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xpzouying/xiaohongshu-mcp/xiaohongshu"
)

// 有扫码会话在等时，查登录状态直接报会话看到的阶段，不另开浏览器。
func TestCheckLoginStatusReportsLiveSession(t *testing.T) {
	s := NewXiaohongshuService()
	seq := s.logins.start(nil, func() {}, time.Minute)
	s.logins.setState(seq, xiaohongshu.LoginPageState{Stage: xiaohongshu.StageRescan, Captcha: true})

	got, err := s.CheckLoginStatus(context.Background())

	require.NoError(t, err)
	assert.Equal(t, &LoginStatusResponse{LoginStage: "rescan"}, got)
}

func TestCheckLoginStatusHints(t *testing.T) {
	cases := []struct {
		stage xiaohongshu.LoginStage
		want  string
	}{
		{xiaohongshu.StageWaitScan, "无需重新获取"},
		{xiaohongshu.StageScanned, "请在手机上确认登录"},
		{xiaohongshu.StageRescan, "需要再扫一次"},
		{xiaohongshu.StageVerification, "登录风控"},
	}
	for _, c := range cases {
		t.Run(string(c.stage), func(t *testing.T) {
			svc := NewXiaohongshuService()
			seq := svc.logins.start(nil, func() {}, time.Minute)
			svc.logins.setState(seq, xiaohongshu.LoginPageState{Stage: c.stage})
			app := &AppServer{xiaohongshuService: svc}

			res := app.handleCheckLoginStatus(context.Background())

			require.False(t, res.IsError)
			assert.Contains(t, res.Content[0].Text, c.want)
		})
	}
}
