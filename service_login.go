package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-rod/rod"
	"github.com/sirupsen/logrus"
	"github.com/xpzouying/xiaohongshu-mcp/cookies"
	xhserrors "github.com/xpzouying/xiaohongshu-mcp/errors"
	"github.com/xpzouying/xiaohongshu-mcp/xiaohongshu"
)

// CheckLoginStatus 检查登录状态。
//
// 有扫码会话在等时，直接报它看到的登录页阶段：那个浏览器还没存 cookie，
// 另开浏览器只能查出「未登录」，看不出扫码进行到了哪一步。
func (s *XiaohongshuService) CheckLoginStatus(ctx context.Context) (*LoginStatusResponse, error) {
	if sess, ok := s.logins.current(); ok && sess.state.Stage != xiaohongshu.StageLoggedIn {
		return &LoginStatusResponse{LoginStage: string(sess.state.Stage)}, nil
	}

	b := newBrowser()
	defer b.Close()

	page := b.NewPage()
	defer page.Close()

	loginAction := xiaohongshu.NewLogin(page)

	isLoggedIn, err := loginAction.CheckLoginStatus(ctx)
	if err != nil {
		return nil, err
	}

	response := &LoginStatusResponse{
		IsLoggedIn: isLoggedIn,
	}

	// 已登录时从当前页读取真实账号信息；读不到只记 warn，不影响状态返回。
	if isLoggedIn {
		if user, err := loginAction.CurrentUser(ctx); err != nil {
			logrus.Warnf("failed to get current user info: %v", err)
		} else {
			response.Username = user.Nickname
			response.UserID = user.UserID
		}
	}

	return response, nil
}

// GetLoginQrcode 获取登录的扫码二维码。
//
// 还有会话在等扫码时，给出它页面上现在的那张码（登录码，或扫码后要求再扫一次的验证码），
// 不另开浏览器：新开会话会让手机上已经扫过的那张作废。
func (s *XiaohongshuService) GetLoginQrcode(ctx context.Context) (*LoginQrcodeResponse, error) {
	if sess, ok := s.logins.current(); ok {
		return s.qrcodeFromSession(ctx, sess)
	}

	b := newBrowser()
	// 建页和取码都可能 panic。只有后台接手扫码后，才由它关闭浏览器。
	handedOff := false
	defer func() {
		if !handedOff {
			b.Close()
		}
	}()

	page := b.NewPage()
	loginAction := xiaohongshu.NewLogin(page)

	img, loggedIn, err := loginAction.FetchQrcodeImage(ctx)
	if err != nil {
		return nil, err
	}
	if loggedIn {
		return &LoginQrcodeResponse{Timeout: "0s", IsLoggedIn: true}, nil
	}

	first := xiaohongshu.LoginPageState{Stage: xiaohongshu.StageWaitScan, QrSrc: img}
	s.waitScanInBackground(loginAction, page, b.Close, first)
	handedOff = true

	return &LoginQrcodeResponse{
		Timeout: loginQrWait.String(),
		Img:     img,
		Stage:   string(first.Stage),
	}, nil
}

// qrcodeFromSession 从还在等扫码的会话取当前的码；码过期就先点刷新。
func (s *XiaohongshuService) qrcodeFromSession(ctx context.Context, sess loginSession) (*LoginQrcodeResponse, error) {
	st := sess.action.ReadLoginPage(ctx)
	// 验证弹窗刚出现时码可能还在加载，后台确认过是登录风控才算
	if st.Stage == xiaohongshu.StageVerification && sess.state.Stage != xiaohongshu.StageVerification {
		st.Stage = xiaohongshu.StageUnknown
	}
	switch st.Stage {
	case xiaohongshu.StageLoggedIn:
		return &LoginQrcodeResponse{Timeout: "0s", IsLoggedIn: true}, nil
	case xiaohongshu.StageVerification:
		return nil, xhserrors.ErrLoginRiskControl
	case xiaohongshu.StageUnknown:
		return nil, errors.New("登录页正在跳转，请几秒后再试")
	}

	if st.Expired {
		var err error
		if st, err = sess.action.RefreshQrcode(ctx, st); err != nil {
			return nil, err
		}
	}

	img, err := sess.action.QrcodeImage(ctx, st)
	if err != nil {
		return nil, err
	}

	left := s.logins.extend(sess.seq)
	logrus.Infof("登录会话 #%d 仍在等扫码，给出页面上当前的二维码（%s），剩余 %s", sess.seq, st.Stage, left.Round(time.Second))
	return &LoginQrcodeResponse{
		Timeout: left.Round(time.Second).String(),
		Img:     img,
		Stage:   string(st.Stage),
	}, nil
}

// waitScanInBackground 在后台盯着登录页，扫上了就存 cookie。
//
// 浏览器必须一直活着才检测得到扫码，所以这里不能提前关；但也不能任由它堆积——
// 再开一个会话就会把上一个还在等的关掉，同一时刻只留一个。
// 扫码后小红书可能要求再扫一次或做安全验证，页面的每次变化都记日志并登记，供取码、查状态使用。
func (s *XiaohongshuService) waitScanInBackground(
	loginAction *xiaohongshu.LoginAction, page *rod.Page, closeBrowser func(), first xiaohongshu.LoginPageState,
) {
	ctx, cancel := context.WithCancel(context.Background())
	seq := s.logins.start(loginAction, cancel, loginQrWait)
	s.logins.setState(seq, first)
	logrus.Infof("等待扫码登录，会话 #%d，超时 %s", seq, loginQrWait)

	go func() {
		defer closeBrowser()
		defer cancel()
		defer s.logins.finish(seq)

		last := first
		loggedIn := loginAction.WatchLogin(ctx, func(st xiaohongshu.LoginPageState) {
			s.logins.setState(seq, st)
			if st != last {
				logLoginChange(seq, last, st)
			}
			if st.Stage != last.Stage && (st.Stage == xiaohongshu.StageRescan || st.Stage == xiaohongshu.StageVerification) {
				saveLoginShot(page, seq, st.Stage)
			}
			last = st
		})

		if loggedIn {
			if err := saveCookies(page); err != nil {
				logrus.Errorf("扫码成功但保存 cookies 失败，会话 #%d: %v", seq, err)
				return
			}
			logrus.Infof("扫码登录成功，cookies 已保存，会话 #%d", seq)
			return
		}

		// 没等到登录：要么超时，要么被新取的二维码取代
		logrus.Infof("登录会话 #%d 结束，未登录（超时或已被新的二维码取代），最后状态：%s", seq, last.Stage)
	}()
}

// logLoginChange 记下登录页的变化：阶段、换码、过期。
func logLoginChange(seq uint64, prev, cur xiaohongshu.LoginPageState) {
	note := ""
	switch {
	case cur.Expired:
		note = "，二维码已过期"
	case cur.QrSrc != "" && cur.QrSrc != prev.QrSrc:
		note = "，页面换了新的二维码"
	}
	logrus.Infof("登录会话 #%d：%s%s", seq, cur.Stage, note)
}

// saveLoginShot 出现安全验证时给页面截图，存在 cookies 同目录的 login-debug 下，便于事后排查。
func saveLoginShot(page *rod.Page, seq uint64, stage xiaohongshu.LoginStage) {
	dir := filepath.Join(filepath.Dir(cookies.GetCookiesFilePath()), "login-debug")
	name := filepath.Join(dir, fmt.Sprintf("%s-%d-%s.png", time.Now().Format("20060102-150405"), seq, stage))

	data, err := page.Timeout(10*time.Second).Screenshot(false, nil)
	if err == nil {
		err = os.MkdirAll(dir, 0o755)
	}
	if err == nil {
		err = os.WriteFile(name, data, 0o644)
	}
	if err != nil {
		logrus.Warnf("登录会话 #%d：页面截图失败: %v", seq, err)
		return
	}
	logrus.Infof("登录会话 #%d：页面截图已存 %s", seq, name)
}
