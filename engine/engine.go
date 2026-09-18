// Package engine 提供可嵌入的 portal 登录引擎，无全局状态、context 驱动、事件输出。
package engine

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/summonhim/gzgspd/config"
)

// Options 配置 Engine 行为。
type Options struct {
	Logger *slog.Logger
	// BufferedEvents 指定事件通道缓冲，未设置时按会话数推算。
	BufferedEvents int
}

// Engine 管理一组 Session。
type Engine struct {
	cfg      *config.Config
	logger   *slog.Logger
	sessions []*Session
	events   chan Event
	keyIndex map[string]*Session

	mu     sync.Mutex
	cancel context.CancelFunc
}

// New 依据配置构造 Engine。
func New(cfg *config.Config, opts Options) (*Engine, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	buf := opts.BufferedEvents
	if buf <= 0 {
		buf = 2*len(cfg.Instance) + 16
	}

	e := &Engine{
		cfg:      cfg,
		logger:   logger,
		events:   make(chan Event, buf),
		keyIndex: make(map[string]*Session),
	}

	for _, inst := range cfg.Instance {
		s := newSession(sessionConfig{
			inst:   inst,
			key:    inst.Username + "@" + inst.KeyIfName(),
			logger: logger,
			pause:  cfg.PauseDuration(),
			events: e.events,
		})
		if _, dup := e.keyIndex[s.Key()]; dup {
			return nil, fmt.Errorf("duplicate session key %q (same username and interface)", s.Key())
		}
		e.keyIndex[s.Key()] = s
		e.sessions = append(e.sessions, s)
	}

	return e, nil
}

// Sessions 返回全部会话。
func (e *Engine) Sessions() []*Session { return e.sessions }

// Session 按键查找会话。
func (e *Engine) Session(key string) (*Session, bool) {
	s, ok := e.keyIndex[key]
	return s, ok
}

// Events 返回事件通道，Engine.Run 返回后通道关闭。
func (e *Engine) Events() <-chan Event { return e.events }

// Run 启动全部会话，阻塞至 ctx 取消并完成全部登出。
func (e *Engine) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	e.mu.Lock()
	e.cancel = cancel
	e.mu.Unlock()
	defer cancel()

	var wg sync.WaitGroup
	for _, s := range e.sessions {
		wg.Add(1)
		go func(s *Session) {
			defer wg.Done()
			if err := s.Start(ctx); err != nil && err != context.Canceled {
				e.logger.Error("session ended with error", "key", s.Key(), "error", err)
			}
		}(s)
	}
	wg.Wait()
	close(e.events)
	e.logger.Info("all sessions stopped")
	return nil
}

// Stop 取消运行中的 Engine。
func (e *Engine) Stop() {
	e.mu.Lock()
	cancel := e.cancel
	e.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}
