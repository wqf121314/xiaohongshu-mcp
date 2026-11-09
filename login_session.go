package main

import (
	"sync"
	"time"

	"github.com/xpzouying/xiaohongshu-mcp/xiaohongshu"
)

const (
	// loginQrWait 每发出一张码，给扫码留的时间
	loginQrWait = 4 * time.Minute
	// loginSessionMax 一个登录会话最长活多久（扫码后要求再扫一次时会续时间）
	loginSessionMax = 10 * time.Minute
)

// loginSessions 管理「已发出二维码、还在等扫码」的登录会话。
//
// 取一次二维码就要留一个浏览器活着等扫码，否则检测不到登录、也存不了 cookie。
// 但没有任何东西拦着重复调用，于是每调一次就多一个浏览器活到超时为止。
// 这里的约束是：同一时刻只保留一个待扫码会话，开新的就把旧的关掉。
// 会话还记着后台看到的页面状态，取码、查登录状态时直接用它。
type loginSessions struct {
	mu  sync.Mutex
	seq uint64
	cur *loginSession
}

// loginSession 当前待扫码会话的登记，字段只在 loginSessions.mu 下读写。
type loginSession struct {
	seq     uint64
	action  *xiaohongshu.LoginAction
	cancel  func()
	timer   *time.Timer
	started time.Time
	state   xiaohongshu.LoginPageState
}

// start 结束上一个待扫码会话（如果有），登记新的，timeout 后自动 cancel。
// 返回本次会话的序号，用于 finish 等判断自己是不是仍然是当前会话。
func (l *loginSessions) start(action *xiaohongshu.LoginAction, cancel func(), timeout time.Duration) uint64 {
	l.mu.Lock()
	prev := l.cur
	l.seq++
	l.cur = &loginSession{
		seq:     l.seq,
		action:  action,
		cancel:  cancel,
		timer:   time.AfterFunc(timeout, cancel),
		started: time.Now(),
	}
	seq := l.seq
	l.mu.Unlock()

	// 放到锁外调用：取消动作会触发对方 goroutine 的收尾，避免相互等待
	if prev != nil {
		prev.timer.Stop()
		prev.cancel()
	}
	return seq
}

// finish 会话自己结束时清理登记。仅当它仍是当前会话才清，
// 否则会把后来者的登记抹掉，导致后来者永远不会被 start 关闭。
func (l *loginSessions) finish(seq uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.cur != nil && l.cur.seq == seq {
		l.cur.timer.Stop()
		l.cur = nil
	}
}

// setState 记下后台最新看到的页面状态，不是当前会话就忽略。
func (l *loginSessions) setState(seq uint64, st xiaohongshu.LoginPageState) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.cur != nil && l.cur.seq == seq {
		l.cur.state = st
	}
}

// current 返回当前会话的快照，没有会话时 ok 为 false。
func (l *loginSessions) current() (loginSession, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.cur == nil {
		return loginSession{}, false
	}
	return *l.cur, true
}

// extend 又发出一张码时重新计时，但不超过会话总时长。返回剩余时间。
func (l *loginSessions) extend(seq uint64) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.cur == nil || l.cur.seq != seq {
		return 0
	}
	left := min(loginQrWait, time.Until(l.cur.started.Add(loginSessionMax)))
	l.cur.timer.Reset(left)
	return left
}
