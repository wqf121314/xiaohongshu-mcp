package main

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xpzouying/xiaohongshu-mcp/xiaohongshu"
)

// TestLoginSessions 固定「同一时刻只保留一个待扫码会话」这条约束。
//
// 这是浏览器不再堆积的依据：每个待扫码会话都占着一个浏览器活到超时为止，
// 只要新会话没能关掉旧的，进程就会累积。
func TestLoginSessions(t *testing.T) {
	t.Run("开新会话会关掉上一个", func(t *testing.T) {
		var l loginSessions
		closed := 0

		l.start(nil, func() { closed++ }, time.Minute)
		assert.Equal(t, 0, closed, "第一个会话不该被关")

		l.start(nil, func() {}, time.Minute)
		assert.Equal(t, 1, closed, "开第二个时应关掉第一个")
	})

	t.Run("第一个会话无需关闭任何东西", func(t *testing.T) {
		var l loginSessions
		assert.NotPanics(t, func() { l.start(nil, func() {}, time.Minute) })
	})

	t.Run("会话结束后不会再被关第二次", func(t *testing.T) {
		var l loginSessions
		closed := 0

		seq := l.start(nil, func() { closed++ }, time.Minute)
		l.finish(seq)

		l.start(nil, func() {}, time.Minute)
		assert.Equal(t, 0, closed, "已结束的会话不该再被关闭")
	})

	t.Run("旧会话的收尾不会顶掉新会话", func(t *testing.T) {
		var l loginSessions
		newClosed := 0

		oldSeq := l.start(nil, func() {}, time.Minute)
		l.start(nil, func() { newClosed++ }, time.Minute) // 新会话上位

		// 旧会话此时才走完收尾，它必须认出自己已不是当前会话
		l.finish(oldSeq)

		// 再开一个：如果上一步误清了登记，新会话就永远关不掉了
		l.start(nil, func() {}, time.Minute)
		assert.Equal(t, 1, newClosed, "新会话仍应被后来者关闭")
	})

	t.Run("并发开会话时每个序号唯一", func(t *testing.T) {
		var l loginSessions
		const n = 50

		var mu sync.Mutex
		seen := make(map[uint64]bool, n)

		var wg sync.WaitGroup
		for range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				seq := l.start(nil, func() {}, time.Minute)
				mu.Lock()
				seen[seq] = true
				mu.Unlock()
			}()
		}
		wg.Wait()

		assert.Len(t, seen, n, "序号必须唯一，否则 finish 会误清别人的登记")
	})

	t.Run("到时间自动关掉会话", func(t *testing.T) {
		var l loginSessions
		closed := make(chan struct{})

		l.start(nil, func() { close(closed) }, 20*time.Millisecond)

		select {
		case <-closed:
		case <-time.After(time.Second):
			t.Fatal("超时后应关掉会话")
		}
	})
}

// TestLoginSessionState 后台看到的页面状态要能被取码、查状态读到。
func TestLoginSessionState(t *testing.T) {
	t.Run("没有会话时读不到", func(t *testing.T) {
		var l loginSessions
		_, ok := l.current()
		assert.False(t, ok)
	})

	t.Run("读到当前会话最近一次的状态", func(t *testing.T) {
		var l loginSessions
		seq := l.start(nil, func() {}, time.Minute)
		st := xiaohongshu.LoginPageState{Stage: xiaohongshu.StageRescan, QrSrc: "data:x"}

		l.setState(seq, st)

		cur, ok := l.current()
		require.True(t, ok)
		assert.Equal(t, seq, cur.seq)
		assert.Equal(t, st, cur.state)
	})

	t.Run("旧会话的状态不会写到新会话上", func(t *testing.T) {
		var l loginSessions
		oldSeq := l.start(nil, func() {}, time.Minute)
		l.start(nil, func() {}, time.Minute)

		l.setState(oldSeq, xiaohongshu.LoginPageState{Stage: xiaohongshu.StageVerification})

		cur, _ := l.current()
		assert.Empty(t, cur.state.Stage)
	})

	t.Run("会话结束后读不到", func(t *testing.T) {
		var l loginSessions
		seq := l.start(nil, func() {}, time.Minute)
		l.finish(seq)

		_, ok := l.current()
		assert.False(t, ok)
	})
}

// TestLoginSessionExtend 每发出一张码都重新给足扫码时间，但整个会话有上限。
func TestLoginSessionExtend(t *testing.T) {
	t.Run("重新计时为一张码的等待时间", func(t *testing.T) {
		var l loginSessions
		seq := l.start(nil, func() {}, time.Minute)

		left := l.extend(seq)

		assert.Equal(t, loginQrWait, left)
	})

	t.Run("不超过会话总时长", func(t *testing.T) {
		var l loginSessions
		seq := l.start(nil, func() {}, time.Minute)
		l.cur.started = time.Now().Add(-loginSessionMax + time.Minute)

		left := l.extend(seq)

		assert.InDelta(t, time.Minute.Seconds(), left.Seconds(), 1)
	})

	t.Run("延长后不会按原来的时间关掉", func(t *testing.T) {
		var l loginSessions
		closed := make(chan struct{})
		seq := l.start(nil, func() { close(closed) }, 20*time.Millisecond)

		l.extend(seq)

		select {
		case <-closed:
			t.Fatal("延长后不该在原时间关掉")
		case <-time.After(100 * time.Millisecond):
		}
	})
}
