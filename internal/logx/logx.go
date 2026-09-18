// Package logx 依据配置构造 slog.Logger，负责日志落盘。
package logx

import (
	"fmt"
	"io"
	"log/slog"
	"os"
)

// New 返回一个 logger 与一个清理函数。
// path 为空时只写 stdout；否则同时写入文件（追加模式）。
func New(level int, path string) (*slog.Logger, func() error) {
	var w io.Writer = os.Stdout
	closer := func() error { return nil }

	if path != "" {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err == nil {
			w = io.MultiWriter(os.Stdout, f)
			closer = f.Close
		} else {
			fmt.Fprintf(os.Stderr, "failed to open log file %q: %v\n", path, err)
		}
	}

	logger := slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: slog.Level(level),
	}))
	return logger, closer
}
