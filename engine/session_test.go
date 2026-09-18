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

func TestSessionCheckerErrorDoesNotLogin(t *testing.T) {
	d := &fakeDialer{checkErr: errors.New("network down")}
	inst := config.ConfigInstance{Username: "u", Password: "p", Interface: "lo", KeepAlive: 1, RetryTime: 1, RetryMax: 1}
	s := newTestSession(t, inst, 50*time.Millisecond, d)

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = s.Start(ctx) }()
	time.Sleep(300 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)

	if d.authCalls.Load() != 0 {
		t.Fatalf("expected no auth attempts when checker fails, got %d", d.authCalls.Load())
	}
}
