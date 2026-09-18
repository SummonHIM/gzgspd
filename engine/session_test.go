package engine

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/summonhim/gzgspd/config"
	"github.com/summonhim/gzgspd/portal"
)

type fakeDialer struct {
	needLogin bool
	loginURL  string
	checkErr  error
	authCode  string
	authErr   error

	authCalls   atomic.Int64
	disconnCall atomic.Int64
}

func (f *fakeDialer) PortalChecker(ctx context.Context, kAliveLink string) (bool, string, error) {
	if f.checkErr != nil {
		return false, "", f.checkErr
	}
	return f.needLogin, f.loginURL, nil
}

func (f *fakeDialer) PortalJsonAction(ctx context.Context, r portal.ActionRequest) (*portal.ActionResponse, error) {
	var resp portal.ActionResponse
	resp.ServerForm.Serverip = "10.0.0.9"
	resp.ServerForm.PortalVer = 4
	resp.PortalConfig.ID = 1
	return &resp, nil
}

func (f *fakeDialer) QuickAuth(ctx context.Context, r portal.QuickAuthRequest) (*portal.QuickAuthResponse, error) {
	f.authCalls.Add(1)
	if f.authErr != nil {
		return nil, f.authErr
	}
	code := f.authCode
	if code == "" {
		code = "0"
	}
	return &portal.QuickAuthResponse{Code: code, Message: "msg", GroupID: 19, UserID: "u1"}, nil
}

func (f *fakeDialer) QuickAuthDisconn(ctx context.Context, r portal.DisconnRequest) (*portal.QuickAuthResponse, error) {
	f.disconnCall.Add(1)
	return &portal.QuickAuthResponse{Code: "0"}, nil
}

func (f *fakeDialer) Close() {}

func newTestSession(t *testing.T, inst config.ConfigInstance, pause time.Duration, d sessionDialer) *Session {
	t.Helper()
	events := make(chan Event, 64)
	s := newSession(sessionConfig{
		inst:   inst,
		key:    inst.Username + "@Test",
		pause:  pause,
		events: events,
		newDialer: func(localIP string) (sessionDialer, error) {
			return d, nil
		},
	})
	// 跳过真实网卡探测，并给出稳定的出口地址供日志与登出使用
	s.prepareInterface = func() error {
		s.loginIf, s.loginIfIP, s.mac = "lo", "127.0.0.1", "00:00:00:00:00:00"
		return nil
	}
	return s
}

func TestSessionLoginSuccess(t *testing.T) {
	d := &fakeDialer{needLogin: true, loginURL: "http://p/portal.do?wlanuserip=1&wlanacname=ac&mac=aa&vlan=1&hostname=h&rand=r"}
	inst := config.ConfigInstance{Username: "u", Password: "p", Interface: "lo", KeepAlive: 5, RetryTime: 5}
	s := newTestSession(t, inst, time.Minute, d)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	_ = s.Start(ctx)

	if d.authCalls.Load() == 0 {
		t.Fatal("expected at least one auth attempt")
	}
	if d.disconnCall.Load() == 0 {
		t.Fatal("expected logout on shutdown")
	}
	if s.State() != StateStopped {
		t.Fatalf("expected Stopped, got %v", s.State())
	}
}

func TestSessionPauseOnMaxRetryResetsCounter(t *testing.T) {
	d := &fakeDialer{needLogin: true, loginURL: "http://p/portal.do?a=1", authErr: errors.New("boom")}
	inst := config.ConfigInstance{Username: "u", Password: "p", Interface: "lo", KeepAlive: 1, RetryTime: 1, RetryMax: 2}
	s := newTestSession(t, inst, 50*time.Millisecond, d)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = s.Start(ctx) }()

	deadline := time.After(5 * time.Second)
	for {
		if d.authCalls.Load() >= 3 {
			break
		}
		if s.State() == StateStopped {
			t.Fatal("session stopped unexpectedly")
		}
		select {
		case <-deadline:
			t.Fatal("session did not exceed max retries in time")
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSessionContextCancelIsFast(t *testing.T) {
	d := &fakeDialer{needLogin: true, loginURL: "http://p/portal.do?a=1"}
	inst := config.ConfigInstance{Username: "u", Password: "p", Interface: "lo", KeepAlive: 3600, RetryTime: 3600}
	s := newTestSession(t, inst, time.Minute, d)

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = s.Start(ctx) }()
	time.Sleep(200 * time.Millisecond)

	start := time.Now()
	cancel()
	deadline := time.Now().Add(2 * time.Second)
	for s.State() != StateStopped {
		if time.Now().After(deadline) {
			t.Fatal("session did not stop after cancel")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("shutdown too slow")
	}
}

func TestSessionCheckerUnreachableIsNotFailure(t *testing.T) {
	// 3.3.3.3 是诱饵地址：未登录时超时是正常表现，不能当成登录失败。
	// 旧实现（executor/worker.go）把探测错误视作"无需登录"，本轮成功、重试清零。
	// 回归：探测报错不应导致状态进入 Not logged in(login failed) 或 Paused。
	d := &fakeDialer{checkErr: errors.New("dial tcp 3.3.3.3:80: i/o timeout")}
	inst := config.ConfigInstance{Username: "u", Password: "p", Interface: "lo", KeepAlive: 1, RetryTime: 1, RetryMax: 1}
	s := newTestSession(t, inst, 50*time.Millisecond, d)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = s.Start(ctx) }()

	// 若被误判为失败，RetryMax=1 且 pause=50ms 会很快进入 Paused
	time.Sleep(400 * time.Millisecond)

	if st := s.State(); st == StatePaused {
		t.Fatal("unreachable keep-alive link must not be treated as login failure (state became Paused)")
	}
	if d.authCalls.Load() != 0 {
		t.Fatalf("expected no auth attempts when checker is unreachable, got %d", d.authCalls.Load())
	}
}

func TestSessionRealCheckStillFails(t *testing.T) {
	// 真正需要登录时，登录失败仍应触发重试与暂停逻辑。
	d := &fakeDialer{
		needLogin: true,
		loginURL:  "http://p/portal.do?a=1",
		authErr:   errors.New("boom"),
	}
	inst := config.ConfigInstance{Username: "u", Password: "p", Interface: "lo", KeepAlive: 1, RetryTime: 1, RetryMax: 1}
	s := newTestSession(t, inst, 50*time.Millisecond, d)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = s.Start(ctx) }()

	deadline := time.After(3 * time.Second)
	for {
		if s.State() == StatePaused {
			break
		}
		select {
		case <-deadline:
			t.Fatal("expected Paused after real login failures")
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	if d.authCalls.Load() == 0 {
		t.Fatal("expected auth attempts for real login failure")
	}
}

func TestSessionManualPauseStopsAttempts(t *testing.T) {
	d := &fakeDialer{needLogin: false}
	inst := config.ConfigInstance{Username: "u", Password: "p", Interface: "lo", KeepAlive: 1, RetryTime: 1}
	s := newTestSession(t, inst, time.Minute, d)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = s.Start(ctx) }()

	// 先让它正常跑几轮
	time.Sleep(100 * time.Millisecond)
	s.Pause()
	time.Sleep(50 * time.Millisecond)

	if s.State() != StatePaused {
		t.Fatalf("expected Paused after Pause(), got %v", s.State())
	}

	// 暂停后不应再调用登录相关网络动作
	before := d.authCalls.Load() + d.disconnCall.Load()
	time.Sleep(200 * time.Millisecond)
	after := d.authCalls.Load() + d.disconnCall.Load()
	if after != before {
		t.Fatalf("expected no network activity while paused, before=%d after=%d", before, after)
	}

	// Resume 后应恢复为未登录状态并继续
	s.Resume()
	deadline := time.After(2 * time.Second)
	for s.State() != StateNotLoggedIn {
		select {
		case <-deadline:
			t.Fatalf("session did not resume, state=%v", s.State())
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
}
