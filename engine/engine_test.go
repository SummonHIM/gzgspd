package engine

import (
	"context"
	"testing"
	"time"

	"github.com/summonhim/gzgspd/config"
)

func TestEngineDuplicateKey(t *testing.T) {
	cfg := &config.Config{Instance: []config.ConfigInstance{
		{Username: "u", Password: "p", KeepAlive: 5, RetryTime: 5},
		{Username: "u", Password: "p2", KeepAlive: 5, RetryTime: 5},
	}}
	if _, err := New(cfg, Options{}); err == nil {
		t.Fatal("expected duplicate key error")
	}
}

func TestEngineRunClosesEvents(t *testing.T) {
	cfg := &config.Config{Instance: []config.ConfigInstance{
		{Username: "u", Password: "p", Interface: "lo", KeepAlive: 1, RetryTime: 1},
	}}
	e, err := New(cfg, Options{})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	// 替换为 fake，避免真实网络
	for _, s := range e.sessions {
		s.newDialer = func(localIP string) (sessionDialer, error) {
			return &fakeDialer{}, nil
		}
		s.prepareInterface = func() error {
			s.loginIf, s.loginIfIP, s.mac = "lo", "127.0.0.1", "00:00:00:00:00:00"
			return nil
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() { _ = e.Run(ctx); close(done) }()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("engine did not stop")
	}

	// 通道应已关闭
	select {
	case _, ok := <-e.Events():
		if ok {
			// 可能仍有缓冲事件，继续读到关闭
			for range e.Events() {
			}
		}
	case <-time.After(time.Second):
		t.Fatal("events channel not closed")
	}
}

func TestEngineInvalidConfig(t *testing.T) {
	if _, err := New(&config.Config{}, Options{}); err == nil {
		t.Fatal("expected validation error for empty instance")
	}
}
