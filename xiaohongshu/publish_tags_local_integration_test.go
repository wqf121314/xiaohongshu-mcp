//go:build integration

package xiaohongshu

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// 本地页面模拟编辑器点击后追加话题，复现「候选行原位改名」点错话题的时序。
func TestVerifyInsertedTopicLocal(t *testing.T) {
	b := newSearchFixtureBrowser(t)
	for _, tc := range []struct {
		name    string
		insert  string // 点击后编辑器追加的话题名，空表示不追加
		wantErr string
	}{
		{"插入同名话题", "散步", ""},
		{"候选原位改名后点错", "散步日记", "插入的话题与请求不符"},
		{"没有插入话题", "", "未确认编辑器插入话题"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page := b.MustPage("about:blank").Timeout(5 * time.Second)
			defer page.Close()
			page.MustSetDocumentContent(`<div id="editor" contenteditable="true"><a data-topic='{"name":"旧话题"}'>#旧话题</a></div>`)
			page.MustEval(`name => {
				if (!name) return;
				setTimeout(() => {
					const a = document.createElement('a');
					a.dataset.topic = JSON.stringify({name});
					a.textContent = '#' + name;
					document.getElementById('editor').append(a);
				}, 120);
			}`, tc.insert)

			err := verifyInsertedTopic(page.MustElement("#editor"), 1, "散步")
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantErr)
			}
		})
	}
}
