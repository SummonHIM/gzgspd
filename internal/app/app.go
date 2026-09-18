// Package app 是进程外壳：解析参数、加载配置、驱动 Engine、处理信号与退出码。
package app

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/summonhim/gzgspd/config"
	"github.com/summonhim/gzgspd/engine"
	"github.com/summonhim/gzgspd/internal/logx"
	"github.com/summonhim/gzgspd/internal/version"
)

// Main 是进程入口，返回退出码。
func Main() int {
	fs := flag.NewFlagSet("gzgspd", flag.ContinueOnError)
	defaultConfig := "config.json"
	if v := firstEnv("GZGSPD_CONFIG_FILE", "GSGZPD_CONFIG_FILE"); v != "" {
		defaultConfig = v
	}
	configFile := fs.String("config", defaultConfig, "Specify the configuration file path.")
	showVersion := fs.Bool("version", false, "Display current version of gzgspd.")
	testConfig := fs.Bool("test", false, "Test configuration and exit.")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return 2
	}

	if *showVersion {
		fmt.Printf("gzgspd %s %s %s with %s %s\n",
			version.Value, runtime.GOOS, runtime.GOARCH, runtime.Version(), version.BuildTime)
		return 0
	}

	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		fmt.Printf("Failed to load configuration file: %v\n", err)
		return 10
	}

	if *testConfig {
		fmt.Println("The configuration file test passed.")
		return 0
	}

	logger, closeLog := logx.New(cfg.LogLevel, cfg.LogPath)
	defer closeLog()

	eng, err := engine.New(cfg, engine.Options{Logger: logger})
	if err != nil {
		fmt.Printf("Failed to initialize engine: %v\n", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("starting gzgspd", "version", version.Value, "sessions", len(eng.Sessions()))
	if err := eng.Run(ctx); err != nil {
		fmt.Printf("%v\n", err)
		return 1
	}
	return 0
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v, ok := os.LookupEnv(k); ok && v != "" {
			return v
		}
	}
	return ""
}
