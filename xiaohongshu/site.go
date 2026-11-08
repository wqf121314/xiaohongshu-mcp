package xiaohongshu

import (
	"net/url"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// site 账号所在的站点。
//
// 海外 IP 打开小红书首页、笔记、主页会被 302 到 rednote.com，但搜索页和创作平台不跳；
// 在 rednote 登录的会话只在 rednote 域名下有效，所以这几处要按账号所在站点拼地址。
type site struct {
	www     string // 主站
	creator string // 创作平台
}

var (
	siteXiaohongshu = site{www: "https://www.xiaohongshu.com", creator: "https://creator.xiaohongshu.com"}
	siteRednote     = site{www: "https://www.rednote.com", creator: "https://creator.rednote.com"}
)

// siteOf 按浏览器里已加载的 cookie 判断站点，读不到按小红书处理。
func siteOf(page *rod.Page) site {
	cks, err := page.Browser().GetCookies()
	if err != nil {
		return siteXiaohongshu
	}
	return siteFromCookies(cks)
}

// siteFromCookies 有 rednote 的 web_session 就是在 rednote 登录的。
// 只有扫码成功才会保存 cookie，所以存下来的 rednote 会话一定来自登录。
func siteFromCookies(cks []*proto.NetworkCookie) site {
	for _, c := range cks {
		if c.Name == "web_session" && hasDomain(c.Domain, "rednote.com") {
			return siteRednote
		}
	}
	return siteXiaohongshu
}

// isRednoteURL 地址是否在 rednote 域名下。
func isRednoteURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && hasDomain(u.Hostname(), "rednote.com")
}

// isSiteAPIHost 是否小红书或 rednote 的接口子域名。
func isSiteAPIHost(host string) bool {
	return strings.HasSuffix(host, ".xiaohongshu.com") || strings.HasSuffix(host, ".rednote.com")
}

// hasDomain host 是 domain 本身或其子域名；cookie 的 domain 可能带前导点。
func hasDomain(host, domain string) bool {
	host = strings.TrimPrefix(host, ".")
	return host == domain || strings.HasSuffix(host, "."+domain)
}
