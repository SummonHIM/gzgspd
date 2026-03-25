//go:build gui

package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/gen2brain/beeep"
	"github.com/getlantern/systray"
)

type Flags struct {
	Config string
	Daemon bool
}

var Version = "dev"

//go:embed assets/logo.ico
var Icon []byte

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

// 打开日志文件
func openLogFile(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", path)
	case "darwin":
		cmd = exec.Command("open", path)
	case "linux":
		cmd = exec.Command("xdg-open", path)
	default:
		return
	}
	cmd.Start()
}

func runAsTray(flags *Flags) {
	// 读取配置文件
	cfg, err := LoadConfig(flags.Config)
	if err != nil {
		_ = beeep.Notify(
			"GZGS Portal",
			fmt.Sprintf("Failed to load configuration file: %v", err),
			Icon,
		)
		os.Exit(1)
	}

	// 日志输出到文件
	if cfg.LogPath == "" {
		cfg.LogPath = "daemon.log"
	}
	logFile, err := os.OpenFile(cfg.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		_ = beeep.Notify(
			"GZGS Portal",
			fmt.Sprintf("Failed to open log file: %v\n", err),
			Icon,
		)
		os.Exit(1)
	}
	defer logFile.Close()
	slog.SetDefault(slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{
		Level: slog.Level(cfg.LogLevel),
	})))

	_ = beeep.Notify(
		"GZGS Portal",
		fmt.Sprintf("Starting GZGS portal in tray mode (%s)...", Version),
		Icon,
	)
	slog.Info(fmt.Sprintf("Starting GZGS portal in tray mode (%s)...", Version))

	// 启动Worker
	WorkerStatus = make(map[string]WorkerState)
	quitSender := make(chan struct{})
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

	// 启动 Tray
	systray.Run(func() {
		systray.SetIcon(Icon) // 可以替换为自己的图标
		systray.SetTitle("GZGS Portal")
		systray.SetTooltip("Guangzhou College of Technology and Business portal login daemon")

		// 创建每个 Worker 的状态菜单项
		menuItems := make(map[string]*systray.MenuItem)
		for _, inst := range cfg.Instance {
			key := inst.Username + "@" + GetKeyIfName(inst)
			menuItems[key] = systray.AddMenuItem(fmt.Sprintf("%s: initializing", key), "Worker status")
		}

		systray.AddSeparator()

		// 打开日志文件菜单
		mOpenLog := systray.AddMenuItem("Log file", "Open log file.")

		// 退出菜单
		mQuit := systray.AddMenuItem("Exit", "Exit this program.")

		// 定时刷新 Worker 状态
		go func() {
			for {
				time.Sleep(2 * time.Second)
				WorkerStatusLock.Lock()
				for key, status := range WorkerStatus {
					menuItems[key].SetTitle(fmt.Sprintf("%s: %s", key, status))
				}
				WorkerStatusLock.Unlock()
			}
		}()

		// 处理打开日志文件
		go func() {
			for range mOpenLog.ClickedCh {
				openLogFile(cfg.LogPath)
			}
		}()

		// 处理退出
		go func() {
			<-mQuit.ClickedCh
			_ = beeep.Notify(
				"GZGS Portal",
				"Logging out all accounts...",
				Icon,
			)
			slog.Info("Tray exit clicked. Logging out all accounts...")
			close(quitSender)
			wg.Wait()
			systray.Quit()
		}()

	}, func() {
		// 清理回调
		_ = beeep.Notify(
			"GZGS Portal",
			"Goodbye.",
			Icon,
		)
		slog.Info("Tray exited.")
	})
}

func main() {
	// 解析参数
	flags := &Flags{}
	flag.StringVar(&flags.Config, "config", "config.json", "Specify the configuration file path.")
	flag.BoolVar(&flags.Daemon, "daemon", false, "Run as daemon.")
	flag.Parse()

	if flags.Daemon {
		runAsDaemon(flags)
	} else {
		runAsTray(flags)
	}
}
