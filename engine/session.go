package engine

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/summonhim/gzgspd/config"
	"github.com/summonhim/gzgspd/nnet"
	"github.com/summonhim/gzgspd/portal"
)

const (
	defaultUserAgent   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36"
	defaultKAliveLink  = "http://3.3.3.3"
	defaultLoginScheme = "https"
	defaultLoginHost   = "10.20.16.5"
	defaultWlanacIP    = "10.20.16.2"
	defaultWlanacname  = "NFV-BASE-01"
	defaultUserSuffix  = "@SSGSXY"
)

// sessionDialer 是一轮登录所需的网络动作。生产实现包装 portal.Client；
// 测试可注入 fake。这是为可测性与 GUI 可插拔性引入的唯一抽象。
type sessionDialer interface {
	PortalChecker(ctx context.Context, kAliveLink string) (bool, string, error)
	PortalJsonAction(ctx context.Context, r portal.ActionRequest) (*portal.ActionResponse, error)
	QuickAuth(ctx context.Context, r portal.QuickAuthRequest) (*portal.QuickAuthResponse, error)
	QuickAuthDisconn(ctx context.Context, r portal.DisconnRequest) (*portal.QuickAuthResponse, error)
	Close()
}

// Session 是单个账号的登录状态机。
type Session struct {
	cfg    config.ConfigInstance
	keyStr string
	logger *slog.Logger
	pause  time.Duration
	events chan<- Event

	// 可注入依赖（测试用）
	newDialer        func(localIP string) (sessionDialer, error)
	prepareInterface func() error

	mu     sync.RWMutex
	state  State
	paused bool
	dialer sessionDialer
	click  chan struct{} // Pause/Resume/唤醒 信号

	// 运行时状态
	loginIf    string
	loginIfIP  string
	scheme     string
	host       string
	wlanuserip string
	wlanacname string
	mac        string
	vlan       string
	hostname   string
	rand       string
	wlanacIP   string
	version    int
	portalPage int
	timestamp  int64
	uuid       string
	groupID    int
	logoutUID  string
}

type sessionConfig struct {
	inst      config.ConfigInstance
	key       string
	logger    *slog.Logger
	pause     time.Duration
	events    chan<- Event
	newDialer func(localIP string) (sessionDialer, error)
}

func newSession(sc sessionConfig) *Session {
	logger := sc.logger
	if logger == nil {
		logger = slog.Default()
	}
	s := &Session{
		cfg:       sc.inst,
		keyStr:    sc.key,
		logger:    logger.With("key", sc.key),
		pause:     sc.pause,
		events:    sc.events,
		click:     make(chan struct{}, 8),
		state:     StateStarting,
		newDialer: sc.newDialer,
	}
	s.prepareInterface = s.prepareInterfaceImpl
	return s
}

// Key 返回唯一标识，形如 username@interface。
func (s *Session) Key() string { return s.keyStr }

// State 返回当前状态（并发安全）。
func (s *Session) State() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

func (s *Session) setState(st State, msg string, err error) {
	s.mu.Lock()
	s.state = st
	s.mu.Unlock()
	s.logger.Info("state changed", "state", st.String(), "message", msg)
	if s.events != nil {
		select {
		case s.events <- Event{Key: s.Key(), Time: time.Now(), State: st, Message: msg, Err: err}:
		default:
		}
	}
}

func (s *Session) isPaused() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.paused
}

// Start 运行状态机，阻塞至 ctx 取消并完成登出。
func (s *Session) Start(ctx context.Context) error {
	s.setState(StateStarting, "starting", nil)

	if err := s.prepareInterface(); err != nil {
		s.setState(StateStopped, "interface error", err)
		return err
	}

	newDialer := s.newDialer
	if newDialer == nil {
		newDialer = func(localIP string) (sessionDialer, error) {
			return portal.NewClient(localIP, 5*time.Second)
		}
	}

	dialer, err := newDialer(s.loginIfIP)
	if err != nil {
		s.setState(StateStopped, "dialer error", err)
		return err
	}
	defer dialer.Close()

	s.mu.Lock()
	s.dialer = dialer
	s.mu.Unlock()

	// 默认值回填
	if s.cfg.UserAgent == "" {
		s.cfg.UserAgent = defaultUserAgent
	}
	if s.cfg.KAliveLink == "" {
		s.cfg.KAliveLink = defaultKAliveLink
	}

	s.setState(StateNotLoggedIn, "ready", nil)

	retry := 0
	for {
		if ctx.Err() != nil {
			s.setState(StateLoggingOut, "context canceled, logging out", nil)
			s.doLogout()
			s.setState(StateStopped, "stopped", nil)
			return ctx.Err()
		}

		if s.isPaused() {
			// 暂停中：等待唤醒或 ctx 取消，不执行登录
			t := time.NewTimer(200 * time.Millisecond)
			select {
			case <-ctx.Done():
			case <-s.click:
			case <-t.C:
			}
			t.Stop()
			continue
		}

		// 自动更新默认网口
		if s.cfg.Interface == "" {
			s.refreshDefaultInterface()
		}

		if err := s.attemptLogin(ctx); err != nil {
			if ctx.Err() != nil {
				continue
			}
			retry++
			s.setState(StateNotLoggedIn, "login failed", err)
			if s.cfg.RetryMax != 0 && retry >= s.cfg.RetryMax {
				s.setState(StatePaused, "reached max retries", nil)
				if !s.wait(ctx, s.pause) {
					continue
				}
				retry = 0
			} else if !s.wait(ctx, time.Duration(s.cfg.RetryTime)*time.Second) {
				continue
			}
		} else {
			retry = 0
			if !s.wait(ctx, time.Duration(s.cfg.KeepAlive)*time.Second) {
				continue
			}
		}
	}
}

// wait 等待 d，或提前被 ctx 取消/唤醒信号中断。返回 true 表示等待完成。
func (s *Session) wait(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-s.click:
		return false
	case <-t.C:
		return true
	}
}

// Pause 请求暂停登录（用户手动暂停）。
func (s *Session) Pause() {
	s.mu.Lock()
	s.paused = true
	s.mu.Unlock()
	s.setState(StatePaused, "paused by request", nil)
	select {
	case s.click <- struct{}{}:
	default:
	}
}

// Resume 从暂停恢复。
func (s *Session) Resume() {
	s.mu.Lock()
	s.paused = false
	s.mu.Unlock()
	s.setState(StateNotLoggedIn, "resumed", nil)
	select {
	case s.click <- struct{}{}:
	default:
	}
}

func (s *Session) refreshDefaultInterface() {
	nowIf, nowIP, nowMAC, err := nnet.GetDefaultIfIP()
	if err != nil {
		s.logger.Error("failed to get default interface", "error", err)
		return
	}
	s.mu.Lock()
	if s.loginIf != nowIf || s.loginIfIP != nowIP || s.mac != nowMAC {
		s.logger.Info("interface updated", "name", nowIf, "ip", nowIP, "mac", nowMAC)
	}
	s.loginIf, s.loginIfIP, s.mac = nowIf, nowIP, nowMAC
	s.mu.Unlock()
}

func (s *Session) prepareInterfaceImpl() error {
	instIf := s.cfg.Interface
	if instIf == "" {
		ifname, ip, mac, err := nnet.GetDefaultIfIP()
		if err != nil {
			return err
		}
		s.loginIf, s.loginIfIP, s.mac = ifname, ip, mac
	} else if net.ParseIP(instIf) == nil {
		ip, err := nnet.GetIfIP(instIf)
		if err != nil {
			return err
		}
		mac, err := nnet.GetIfMAC(instIf)
		if err != nil {
			return err
		}
		s.loginIf, s.loginIfIP, s.mac = instIf, ip, mac
	} else {
		mac, err := nnet.GetIPMAC(instIf)
		if err != nil {
			return err
		}
		s.loginIf, s.loginIfIP, s.mac = instIf, instIf, mac
	}
	s.logger.Info("using interface", "name", s.loginIf, "ip", s.loginIfIP, "mac", s.mac)
	return nil
}

// attemptLogin 执行一轮探测+登录。
//
// 返回 nil 表示本轮无需登录或登录成功。
//
// 关于 keep-alive 探测错误：配置里的 keep_alive_link（默认 http://3.3.3.3）是一个
// 诱饵地址，用于触发校园网关的重定向，它在未登录和已登录状态下都可能不可达
// （超时/连接失败）。真正的"需要登录"信号是被重定向到 portal 页面，而不是能否
// 连通诱饵地址。因此探测返回错误时，既不能判定为"在线"，也不能判定为"登录失败"
// ——不能触发重试计数，否则已登录的会话会被反复判失败并最终进入暂停。
// 这里与重构前的 executor/worker.go 行为保持一致：探测错误按"本轮无需登录"处理。
func (s *Session) attemptLogin(ctx context.Context) error {
	dialer := s.dialer

	needLogin, needLoginURL, err := dialer.PortalChecker(ctx, s.cfg.KAliveLink)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		s.logger.Debug("keep-alive probe failed; assuming online", "error", err)
		return nil
	}
	if !needLogin || needLoginURL == "" {
		return nil
	}

	s.setState(StateLoggingIn, "login required", nil)

	nlu, err := url.Parse(needLoginURL)
	if err != nil {
		return fmt.Errorf("parse redirect link: %w", err)
	}
	q := nlu.Query()
	s.scheme = nlu.Scheme
	s.host = nlu.Host
	s.wlanuserip = q.Get("wlanuserip")
	s.wlanacname = q.Get("wlanacname")
	s.mac = firstNonEmpty(q.Get("mac"), s.mac)
	s.vlan = q.Get("vlan")
	s.hostname = q.Get("hostname")
	s.rand = q.Get("rand")

	portalConfig, err := dialer.PortalJsonAction(ctx, portal.ActionRequest{
		Scheme: s.scheme, Host: s.host, UserAgent: s.cfg.UserAgent,
		Wlanuserip: s.wlanuserip, Wlanacname: s.wlanacname,
		MAC: s.mac, VLAN: s.vlan, Hostname: s.hostname, Rand: s.rand,
	})
	if err != nil {
		return fmt.Errorf("portal json action: %w", err)
	}

	s.wlanacIP = portalConfig.ServerForm.Serverip
	s.version = portalConfig.ServerForm.PortalVer
	s.portalPage = portalConfig.PortalConfig.ID
	s.timestamp = portalConfig.PortalConfig.Timestamp
	s.uuid = portalConfig.PortalConfig.UUID

	loginStat, err := dialer.QuickAuth(ctx, portal.QuickAuthRequest{
		Scheme: s.scheme, Host: s.host, UserAgent: s.cfg.UserAgent,
		UserID: s.cfg.Username, Password: s.cfg.Password,
		Wlanuserip: s.wlanuserip, Wlanacname: s.wlanacname, WlanacIP: s.wlanacIP,
		VLAN: s.vlan, MAC: s.mac, Version: s.version, PortalPageID: s.portalPage,
		Timestamp: s.timestamp, UUID: s.uuid, PortalType: "0",
		Hostname: s.hostname, Rand: s.rand,
	})
	if err != nil {
		return fmt.Errorf("quick auth: %w", err)
	}
	if loginStat.Code != "0" {
		return fmt.Errorf("login rejected: %s", loginStat.Message)
	}

	s.groupID = loginStat.GroupID
	s.logoutUID = loginStat.UserID
	s.setState(StateLoggedIn, "login successful", nil)
	return nil
}

func (s *Session) doLogout() {
	dialer := s.dialer
	if dialer == nil {
		return
	}
	if s.version == 0 {
		s.version = 4
	}
	if s.groupID == 0 {
		s.groupID = 19
	}

	stat, err := dialer.QuickAuthDisconn(context.Background(), portal.DisconnRequest{
		Scheme:        firstNonEmpty(s.scheme, defaultLoginScheme),
		Host:          firstNonEmpty(s.host, defaultLoginHost),
		UserAgent:     firstNonEmpty(s.cfg.UserAgent, defaultUserAgent),
		WlanacIP:      firstNonEmpty(s.wlanacIP, defaultWlanacIP),
		Wlanuserip:    firstNonEmpty(s.wlanuserip, s.loginIfIP),
		Wlanacname:    firstNonEmpty(s.wlanacname, defaultWlanacname),
		Version:       s.version,
		PortalType:    "0",
		UserID:        firstNonEmpty(s.logoutUID, s.cfg.Username+defaultUserSuffix),
		MAC:           firstNonEmpty(s.mac, s.macByIP()),
		GroupID:       s.groupID,
		ClearOperator: "0",
	})
	if err != nil {
		s.logger.Error("logout failed", "error", err)
		return
	}
	if stat.Code != "0" {
		s.logger.Error("logout failed", "message", stat.Message)
		return
	}
	s.logger.Info("logged out")
}

func (s *Session) macByIP() string {
	mac, _ := nnet.GetIPMAC(s.loginIfIP)
	return mac
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
