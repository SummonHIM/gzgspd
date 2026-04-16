package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/summonhim/gzgspd/config"
	"github.com/summonhim/gzgspd/executor"
	"github.com/summonhim/gzgspd/nnet"
)

var (
	Version   string = "dev"
	BuildTime int64  = 0
)

func runAsDaemon(ConfigFile string) error {
	// 读取配置文件
	cfg, err := config.LoadConfig(ConfigFile)
	if err != nil {
		return fmt.Errorf("Failed to load configuration file: %v", err)
	}

	// 初始化日志系统
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.Level(cfg.LogLevel),
	})))
	slog.Info(fmt.Sprintf("Starting GZGS portal daemon (%s)...", Version))

	// 监听终止命令
	executor.WorkerStatus = make(map[string]executor.WorkerState)
	quitSender := make(chan struct{})
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	var wg sync.WaitGroup

	for _, inst := range cfg.Instance {
		key := inst.Username + "@" + nnet.GetKeyIfName(inst)
		executor.WorkerStatus[key] = executor.StateStarting

		wg.Add(1)
		go func(ci config.ConfigInstance) {
			defer wg.Done()
			executor.Worker(ci, key, quitSender)
		}(inst)
	}

	<-sigs
	slog.Info("Caught termination signal, logging out...")
	close(quitSender)
	wg.Wait()
	slog.Info("All instances stopped. Exiting...")
	return nil
}

func testConfig(ConfigFile string) error {
	_, err := config.LoadConfig(ConfigFile)
	if err != nil {
		return fmt.Errorf("%v", err)
	}
	return nil
}

func main() {
	// 解析参数
	flags := &config.Flags{}
	config.ParseFlags(flags)

	if flags.ActionShowVersion {
		formattedBuildTime := time.Unix(BuildTime, 0)
		fmt.Printf(
			"gzgspd %s %s %s with %s %s\n",
			Version,
			runtime.GOOS,
			runtime.GOARCH,
			runtime.Version(),
			formattedBuildTime.Format("2006-01-02 15:04:05"),
		)
		os.Exit(0)
	}

	if flags.ActionTestConfig {
		err := testConfig(flags.ConfigFile)
		if err != nil {
			fmt.Printf("%s", err)
			os.Exit(10)
		} else {
			fmt.Printf("The configuration file test passed.")
			os.Exit(0)
		}
	}

	err := runAsDaemon(flags.ConfigFile)
	if err != nil {
		fmt.Printf("%s", err)
		os.Exit(1)
	} else {
		os.Exit(0)
	}
}
