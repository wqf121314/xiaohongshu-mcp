package xiaohongshu

import (
	"context"
	"errors"
	"time"

	"github.com/go-rod/rod"
	myerrors "github.com/xpzouying/xiaohongshu-mcp/errors"
	"github.com/xpzouying/xiaohongshu-mcp/humanize"
)

type NavigateAction struct {
	page *rod.Page
}

func NewNavigate(page *rod.Page) *NavigateAction {
	return &NavigateAction{page: page}
}

func (n *NavigateAction) ToExplorePage(ctx context.Context) error {
	page := n.page.Context(ctx).Timeout(60 * time.Second) // 加超时保护，避免 MustNavigate/MustWaitStable 无限挂

	page.MustNavigate("https://www.xiaohongshu.com/explore").
		MustWaitLoad().
		MustElement(`div#app`)

	return nil
}

func (n *NavigateAction) ToProfilePage(ctx context.Context) error {
	page := n.page.Context(ctx).Timeout(60 * time.Second) // 加超时保护，避免 MustNavigate/MustWaitStable 无限挂

	// First navigate to explore page
	if err := n.ToExplorePage(ctx); err != nil {
		return err
	}

	page.MustWaitStable()

	// 侧边栏的「我」只在登录后出现；没有就是登录失效，直接报错，不等满超时
	profileLink, err := page.Sleeper(rod.NotFoundSleeper).Element(`div.main-container li.user.side-bar-component a.link-wrapper span.channel`)
	if err != nil {
		if errors.Is(err, &rod.ElementNotFoundError{}) {
			return myerrors.ErrLoginRequired
		}
		return err
	}
	humanize.Delay(ctx, humanize.BeforeClick)
	if err := humanize.Click(profileLink); err != nil {
		return err
	}

	// Wait for navigation to complete
	page.MustWaitLoad()

	return nil
}
