//go:build !gui

package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

type Flags struct {
	Config string
}

var Version = "dev"

func runAsDaemon(flags *Flags) {
	// 读取配置文件
	cfg, err := LoadConfig(flags.Config)
	if err != nil {
		fmt.Printf("Failed to load configuration file: %v", err)
		os.Exit(1)
	}

	// 初始化日志系统
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.Level(cfg.LogLevel),
	})))
	slog.Info(fmt.Sprintf("Starting GZGS portal daemon (%s)...", Version))

	// 监听终止命令
	WorkerStatus = make(map[string]WorkerState)
	quitSender := make(chan struct{})
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	var wg sync.WaitGroup

	for _, inst := range cfg.Instance {
		key := inst.Username + "@" + GetKeyIfName(inst)
		WorkerStatus[key] = StateStarting

		wg.Add(1)
		go func(ci ConfigInstance) {
			defer wg.Done()
			worker(ci, key, quitSender, flags)
		}(inst)
	}

	<-sigs
	slog.Info("Caught termination signal, logging out...")
	close(quitSender)
	wg.Wait()
	slog.Info("All instances stopped. Exiting...")
}

func main() {
	// 解析参数
	flags := &Flags{}
	flag.StringVar(&flags.Config, "config", "config.json", "Specify the configuration file path.")
	flag.Parse()

	runAsDaemon(flags)
}
