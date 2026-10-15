package xiaohongshu

import (
	"context"
	"log/slog"
	"runtime"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/xpzouying/xiaohongshu-mcp/humanize"
)

const (
	// urlOfPublishImage 图文发布页，「文字配图」入口在这里
	urlOfPublishImage = `https://creator.xiaohongshu.com/publish/publish?from=menu&target=image`

	// textImageEntrySelector 「文字配图」入口的候选元素，按文字精确匹配
	textImageEntrySelector = `button, span, div`
	textImageEntryText     = `^\s*文字配图\s*$`
)

// SearchTopicsAction 在发布页输入 #关键词，截取页面自己发出的话题搜索结果。
type SearchTopicsAction struct {
	page *rod.Page
}

func NewSearchTopicsAction(page *rod.Page) *SearchTopicsAction {
	return &SearchTopicsAction{page: page}
}

// Search 搜索话题。不上传、不保存、不发布，输入的内容取完结果后清空。
func (a *SearchTopicsAction) Search(ctx context.Context, keyword string) ([]TopicInfo, error) {
	page := a.page.Context(ctx).Timeout(90 * time.Second)

	if err := ensureCreatorSession(page); err != nil {
		return nil, err
	}
	if err := page.Navigate(urlOfPublishImage); err != nil {
		return nil, errors.Wrap(err, "打开发布页失败")
	}
	if err := page.WaitLoad(); err != nil {
		logrus.Warnf("等待页面加载出现问题: %v，继续尝试", err)
	}
	if err := page.WaitDOMStable(time.Second, 0.1); err != nil {
		logrus.Warnf("等待 DOM 稳定出现问题: %v，继续尝试", err)
	}
	humanize.Delay(ctx, humanize.AfterNavigate)

	if err := clickTextImageEntry(page); err != nil {
		return nil, err
	}
	closeFeatureGuide(page)

	editor, err := getContentElement(page, contentElemTimeout)
	if err != nil {
		return nil, err
	}

	topics, searchErr := typeAndCapture(ctx, editor, keyword)

	// 不论搜索成败都清空正文；用独立超时，调用方取消也要清掉
	clearCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := clearEditor(editor.Context(clearCtx)); err != nil {
		return nil, err
	}
	time.Sleep(time.Second) // 技术等待：等页面自动保存跟上清空后的状态

	if searchErr != nil {
		return nil, searchErr
	}
	return topics, nil
}

// clickTextImageEntry 点「文字配图」进入不需要传图的编辑框。
func clickTextImageEntry(page *rod.Page) error {
	p := page.Timeout(15 * time.Second)
	defer p.CancelTimeout()
	entry, err := p.ElementR(textImageEntrySelector, textImageEntryText)
	if err != nil {
		return errors.Wrap(err, "找不到「文字配图」入口")
	}
	if err := humanize.Click(entry); err != nil {
		return errors.Wrap(err, "点击「文字配图」失败")
	}
	humanize.Delay(page.GetContext(), humanize.AfterNavigate)
	return nil
}

// typeAndCapture 输入 #关键词，等待页面返回该关键词的话题列表。
func typeAndCapture(ctx context.Context, editor *rod.Element, keyword string) ([]TopicInfo, error) {
	capture := startTopicCapture(editor.Page())
	defer capture.stop()

	if err := humanize.Type(ctx, editor, "#"); err != nil {
		return nil, errors.Wrap(err, "输入#失败")
	}
	time.Sleep(200 * time.Millisecond) // 技术等待：等联想下拉框弹出
	if err := humanize.Type(ctx, editor, keyword); err != nil {
		return nil, errors.Wrap(err, "输入关键词失败")
	}

	topics, err := capture.wait(keyword, 5*time.Second)
	if err != nil {
		return nil, errors.Wrap(err, "没有取到话题搜索结果")
	}
	return topics, nil
}

// clearEditor 全选后删除正文，确认清空；不成功再试一次。
func clearEditor(editor *rod.Element) error {
	mod := input.ControlLeft
	if runtime.GOOS == "darwin" {
		mod = input.MetaLeft
	}

	for i := 0; i < 2; i++ {
		ka, err := editor.KeyActions()
		if err != nil {
			return errors.Wrap(err, "清空正文失败")
		}
		if err := ka.Press(mod).Type(input.KeyA).Release(mod).Type(input.Backspace).Do(); err != nil {
			return errors.Wrap(err, "清空正文失败")
		}
		time.Sleep(300 * time.Millisecond) // 技术等待：等编辑器更新

		text, err := editor.Text()
		if err == nil && strings.TrimSpace(text) == "" {
			return nil
		}
		slog.Warn("正文未清空，重试", "text", text, "err", err)
	}
	return errors.New("清空正文失败，草稿箱里可能留下一篇草稿，请手动删除")
}
