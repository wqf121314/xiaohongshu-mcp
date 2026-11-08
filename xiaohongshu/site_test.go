package xiaohongshu

import (
	"testing"

	"github.com/go-rod/rod/lib/proto"
	"github.com/stretchr/testify/require"
)

func TestSiteFromCookies(t *testing.T) {
	ck := func(domain, name string) *proto.NetworkCookie {
		return &proto.NetworkCookie{Domain: domain, Name: name}
	}
	cases := []struct {
		name string
		cks  []*proto.NetworkCookie
		want site
	}{
		{"没有 cookie", nil, siteXiaohongshu},
		{"只有小红书会话", []*proto.NetworkCookie{ck(".xiaohongshu.com", "web_session")}, siteXiaohongshu},
		{"rednote 会话", []*proto.NetworkCookie{ck(".rednote.com", "web_session")}, siteRednote},
		{"两边都有会话时用 rednote", []*proto.NetworkCookie{
			ck(".xiaohongshu.com", "web_session"), ck(".rednote.com", "web_session"),
		}, siteRednote},
		{"rednote 子域名的会话", []*proto.NetworkCookie{ck("www.rednote.com", "web_session")}, siteRednote},
		{"rednote 的非会话 cookie 不算", []*proto.NetworkCookie{ck(".rednote.com", "a1")}, siteXiaohongshu},
		{"仿冒域名不算", []*proto.NetworkCookie{ck("rednote.com.example.com", "web_session")}, siteXiaohongshu},
		{"相似域名不算", []*proto.NetworkCookie{ck(".notrednote.com", "web_session")}, siteXiaohongshu},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, siteFromCookies(c.cks))
		})
	}
}

func TestIsRednoteURL(t *testing.T) {
	require.True(t, isRednoteURL("https://www.rednote.com/"))
	require.True(t, isRednoteURL("https://www.rednote.com/login?redirectPath=x"))
	require.False(t, isRednoteURL("https://www.xiaohongshu.com/explore"))
	require.False(t, isRednoteURL("https://rednote.com.example.com/"))
	require.False(t, isRednoteURL("://invalid"))
}

func TestMakeSearchURLUsesSite(t *testing.T) {
	require.Equal(t,
		"https://www.rednote.com/search_result?keyword=%E5%A5%A5%E5%85%8B%E5%85%B0%E9%97%B2%E7%BD%AE&source=web_explore_feed",
		makeSearchURL(siteRednote.www, "奥克兰闲置"))
	require.Equal(t,
		"https://www.xiaohongshu.com/search_result?keyword=test&source=web_explore_feed",
		makeSearchURL(siteXiaohongshu.www, "test"))
}
