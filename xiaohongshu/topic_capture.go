package xiaohongshu

import (
	"encoding/base64"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/pkg/errors"
)

// topicSearchPath 创作平台话题搜索接口的路径片段
const topicSearchPath = "/search/topics"

// topicCapture 监听页面自己发出的话题搜索请求（不注入 JS、不自己调接口）。
type topicCapture struct {
	page     *rod.Page
	cancel   func()
	mu       sync.Mutex
	pending  map[proto.NetworkRequestID]capturedSearch // 未完成的请求
	finished []capturedSearch                          // 已完成的请求，按完成顺序
}

type capturedSearch struct {
	id      proto.NetworkRequestID
	keyword string
	known   bool // 是否从请求体读出了关键词
}

// startTopicCapture 开始监听，用完调用 stop。
func startTopicCapture(page *rod.Page) *topicCapture {
	c := &topicCapture{page: page, pending: map[proto.NetworkRequestID]capturedSearch{}}

	// 用未取消的 page 先开启 Network 域并让 rod 记下「已开启」，EachEvent 就不会在结束时关闭它。
	// 否则 stop() 取消 context 后，rod 的 Network.disable 仍会发到 Chrome，但因 ctx 已取消不更新记录，
	// 之后的话题 EachEvent 以为已开启而不再开启，收不到任何网络事件。
	if err := (proto.NetworkEnable{}).Call(page); err != nil {
		slog.Warn("开启网络监听失败", "err", err)
	}

	p, cancel := page.WithCancel()
	c.cancel = cancel

	wait := p.EachEvent(
		func(e *proto.NetworkRequestWillBeSent) {
			if strings.Contains(e.Request.URL, topicSearchPath) {
				kw, known := topicSearchKeyword(requestBody(e.Request))
				c.mu.Lock()
				c.pending[e.RequestID] = capturedSearch{id: e.RequestID, keyword: kw, known: known}
				c.mu.Unlock()
			}
		},
		func(e *proto.NetworkLoadingFinished) {
			c.mu.Lock()
			if s, ok := c.pending[e.RequestID]; ok {
				delete(c.pending, e.RequestID)
				c.finished = append(c.finished, s)
			}
			c.mu.Unlock()
		},
	)
	go wait()
	return c
}

// requestBody 取请求体；新版 Chrome 可能只给 postDataEntries。
func requestBody(r *proto.NetworkRequest) string {
	if r.PostData != "" {
		return r.PostData
	}
	var b strings.Builder
	for _, e := range r.PostDataEntries {
		b.Write(e.Bytes)
	}
	return b.String()
}

func (c *topicCapture) stop() {
	c.cancel()
}

// wait 等待关键词为 keyword 的搜索返回并解析。
// 请求体里读不出关键词时，超时后退而使用最后一个读不出关键词的返回。
func (c *topicCapture) wait(keyword string, timeout time.Duration) ([]TopicInfo, error) {
	deadline := time.Now().Add(timeout)
	for {
		if id, ok := c.latest(keyword, false); ok {
			return c.parse(id)
		}
		if time.Now().After(deadline) {
			// 兜底结果可能不是本次关键词的，安全是因为点击前还会核对联想项文字
			if id, ok := c.latest(keyword, true); ok {
				return c.parse(id)
			}
			return nil, errors.New("等待话题搜索结果超时")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// latest 返回关键词相同的最后一个已完成请求；fallback 时也接受读不出关键词的请求。
func (c *topicCapture) latest(keyword string, fallback bool) (proto.NetworkRequestID, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.finished) - 1; i >= 0; i-- {
		s := c.finished[i]
		if (s.known && NormalizeTopic(s.keyword) == keyword) || (fallback && !s.known) {
			return s.id, true
		}
	}
	return "", false
}

// parse 读取请求的返回内容并解析成话题列表。
func (c *topicCapture) parse(id proto.NetworkRequestID) ([]TopicInfo, error) {
	res, err := proto.NetworkGetResponseBody{RequestID: id}.Call(c.page)
	if err != nil {
		return nil, errors.Wrap(err, "读取话题搜索结果失败")
	}
	body := []byte(res.Body)
	if res.Base64Encoded {
		if body, err = base64.StdEncoding.DecodeString(res.Body); err != nil {
			return nil, errors.Wrap(err, "解码话题搜索结果失败")
		}
	}
	return ParseTopicSearchResponse(body)
}
