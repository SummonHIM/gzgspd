package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/url"
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

type WorkerInstance struct {
	ConfigInstance
	LoginIf      string
	LoginIfIP    string
	LoginHost    string
	Wlanuserip   string
	Wlanacname   string
	MAC          string
	Vlan         string
	HostName     string
	Rand         string
	WlanacIp     string
	Version      int
	PortalPageID int
	TimeStamp    int64
	UUID         string
	GroupID      int
	LogoutUID    string
}

type WorkerState string

const (
	StateStarting    WorkerState = "Starting"
	StateNotLoggedIn WorkerState = "Not logged in"
	StateLoggingIn   WorkerState = "Logging in"
	StateLoggedIn    WorkerState = "Logged in"
	StatePaused      WorkerState = "Paused"
	StateLoggingOut  WorkerState = "Logging out"
	StateStopped     WorkerState = "Stopped"
)

var WorkerStatus map[string]WorkerState
var WorkerStatusLock sync.Mutex

var Version = "dev"

//go:embed assets/logo.ico
var Icon []byte

func (w *WorkerInstance) GetStringFallback(val string, defaultVal string) string {
	if val != "" {
		return val
	}
	return defaultVal
}

func doLogin(instance *WorkerInstance, statusKey string, flags *Flags) bool {
	// 检查是否需要登录
	slog.Debug(fmt.Sprintf("[%s] Checking portal if login is required.", statusKey))
	needLogin, needLoginUrl := TelecomPortalChecker(
		instance.LoginIfIP,
		instance.KAliveLink,
	)
	slog.Debug(fmt.Sprintf("[%s] Need login: %t Redirect link: %s", statusKey, needLogin, needLoginUrl))

	if needLogin && needLoginUrl != "" {
		WorkerStatusLock.Lock()
		WorkerStatus[statusKey] = StateLoggingIn
		WorkerStatusLock.Unlock()

		// 若需要登陆，且获取到重定向登录链接
		if !flags.Daemon {
			_ = beeep.Notify(
				"GZGS Portal",
				fmt.Sprintf("[%s] Login require.", statusKey),
				Icon,
			)
		}

		slog.Info(fmt.Sprintf("[%s] Login require.", statusKey))

		nlu, err := url.Parse(needLoginUrl)
		if err != nil {
			WorkerStatusLock.Lock()
			WorkerStatus[statusKey] = StateNotLoggedIn
			WorkerStatusLock.Unlock()
			slog.Error(fmt.Sprintf("[%s] Failed to parse redirect login link: %v", statusKey, err))
			return false
		}

		// 提取重定向 URL 的参数
		tWlanuserip := nlu.Query().Get("wlanuserip")
		tWlanacname := nlu.Query().Get("wlanacname")
		tMAC := nlu.Query().Get("mac")
		tVlan := nlu.Query().Get("vlan")
		tHostName := nlu.Query().Get("hostname")
		tRand := nlu.Query().Get("rand")
		instance.LoginHost = nlu.Host
		instance.Wlanuserip = tWlanuserip
		instance.Wlanacname = tWlanacname
		instance.MAC = tMAC
		instance.Vlan = tVlan
		instance.HostName = tHostName
		instance.Rand = tRand

		// 获取登录基本信息
		portalConfig, err := TelecomPortalJsonAction(
			instance.LoginIfIP,
			instance.LoginHost,
			instance.UserAgent,
			instance.Wlanuserip,
			instance.Wlanacname,
			instance.MAC,
			instance.Vlan,
			instance.HostName,
			instance.Rand,
		)
		if err != nil {
			WorkerStatusLock.Lock()
			WorkerStatus[statusKey] = StateNotLoggedIn
			WorkerStatusLock.Unlock()
			slog.Error(fmt.Sprintf("[%s] Failed to fetch Portal Json Action: %v", statusKey, err))
			return false
		}
		// slog.Debug(fmt.Sprintf("[%s] Portal Config: %v", instance.Username, portalConfig))

		// 提取登录基本信息
		instance.WlanacIp = portalConfig.ServerForm.Serverip
		instance.Version = portalConfig.ServerForm.PortalVer
		instance.PortalPageID = portalConfig.PortalConfig.ID
		instance.TimeStamp = portalConfig.PortalConfig.Timestamp
		instance.UUID = portalConfig.PortalConfig.UUID

		// 登录
		loginStat, err := TelecomQuickAuth(
			instance.LoginIfIP,
			instance.LoginHost,
			instance.UserAgent,
			instance.Username,
			instance.Password,
			instance.Wlanuserip,
			instance.Wlanacname,
			instance.WlanacIp,
			instance.Vlan,
			instance.MAC,
			instance.Version,
			instance.PortalPageID,
			instance.TimeStamp,
			instance.UUID,
			"0",
			instance.HostName,
			instance.Rand,
		)
		if loginStat.Code != "0" || err != nil {
			WorkerStatusLock.Lock()
			WorkerStatus[statusKey] = StateNotLoggedIn
			WorkerStatusLock.Unlock()

			if !flags.Daemon {
				_ = beeep.Notify(
					"GZGS Portal",
					fmt.Sprintf("[%s] Login failed!", statusKey),
					Icon,
				)
			}

			if err != nil {
				slog.Error(fmt.Sprintf("[%s] Login failed: %v", statusKey, err))
			} else if loginStat.Code != "0" {
				slog.Error(fmt.Sprintf("[%s] Login failed: %s", statusKey, loginStat.Message))
			} else {
				slog.Error(fmt.Sprintf("[%s] Login failed: Unknown error.", statusKey))
			}
			return false
		}
		instance.GroupID = loginStat.GroupID
		instance.LogoutUID = loginStat.UserID

		WorkerStatusLock.Lock()
		WorkerStatus[statusKey] = StateLoggedIn
		WorkerStatusLock.Unlock()

		if !flags.Daemon {
			_ = beeep.Notify(
				"GZGS Portal",
				fmt.Sprintf("[%s] Login successfully!", statusKey),
				Icon,
			)
		}

		slog.Info(fmt.Sprintf("[%s] Login successfully!", statusKey))
	}

	return true
}

func doLogout(instance *WorkerInstance, statusKey string, flags *Flags) {
	WorkerStatusLock.Lock()
	WorkerStatus[statusKey] = StateLoggingOut
	WorkerStatusLock.Unlock()

	if !flags.Daemon {
		_ = beeep.Notify(
			"GZGS Portal",
			fmt.Sprintf("[%s] Logging out...", statusKey),
			Icon,
		)
	}

	slog.Info(fmt.Sprintf("[%s] Logging out...", statusKey))

	tMac, _ := GetIPMAC(instance.LoginIfIP)

	if instance.Version == 0 {
		instance.Version = 4
	}
	if instance.GroupID == 0 {
		instance.GroupID = 19
	}

	logoutStat, err := TelecomQuickAuthDisconn(
		instance.LoginIfIP,
		instance.GetStringFallback(instance.LoginHost, "10.20.16.5"),
		instance.UserAgent,
		instance.GetStringFallback(instance.WlanacIp, "10.20.16.2"),
		instance.GetStringFallback(instance.Wlanuserip, instance.LoginIfIP),
		instance.GetStringFallback(instance.Wlanacname, "NFV-BASE-01"),
		instance.Version,
		"0",
		instance.GetStringFallback(instance.LogoutUID, instance.Username+"@SSGSXY"),
		instance.GetStringFallback(instance.MAC, tMac),
		instance.GroupID,
		"0",
	)

	if err != nil {
		slog.Error(fmt.Sprintf("[%s] Logout failed: %v", statusKey, err))
	} else if logoutStat.Code != "0" {
		slog.Error(fmt.Sprintf("[%s] Logout failed: %s", statusKey, logoutStat.Message))
	} else {
		slog.Info(fmt.Sprintf("[%s] Logged out successfully.", statusKey))
	}
}

// 解析网络接口设置
func parseInterface(instanceIf string) (string, string, string, error) {
	if instanceIf == "" {
		// 如果为空
		ifname, ip, mac, err := GetDefaultIfIP()
		if err != nil {
			return "", "", "", fmt.Errorf("failed to get default interface ip: %v", err)
		}
		return ifname, ip, mac, nil
	} else if net.ParseIP(instanceIf) == nil {
		// 如果不为 IP 地址
		ip, err := GetIfIP(instanceIf)
		if err != nil {
			return "", "", "", fmt.Errorf("failed to get interface '%s' ip: %v", instanceIf, err)
		}
		mac, err := GetIfMAC(instanceIf)
		if err != nil {
			return "", "", "", fmt.Errorf("failed to get interface '%s' mac: %v", instanceIf, err)
		}
		return instanceIf, ip, mac, nil
	} else {
		// 如果是 IP 地址
		mac, err := GetIPMAC(instanceIf)
		if err != nil {
			return "", "", "", fmt.Errorf("failed to get interface '%s' mac: %v", instanceIf, err)
		}
		return instanceIf, instanceIf, mac, nil
	}
}

// 工作函数
func worker(cfg ConfigInstance, statusKey string, quitSender <-chan struct{}, flags *Flags) {
	slog.Info(fmt.Sprintf("[%s] Starting instance %s", statusKey, statusKey))
	// 将配置写入当前内存中
	instance := &WorkerInstance{
		ConfigInstance: cfg,
	}

	// 分析接口的IP
	tLoginIf, tLoginIfIP, tMac, err := parseInterface(instance.Interface)
	if err != nil {
		slog.Error(fmt.Sprintf("[%s] Error parsing interface: %v", statusKey, err))
		return
	} else {
		instance.LoginIf = tLoginIf
		instance.LoginIfIP = tLoginIfIP
		instance.MAC = tMac
	}
	slog.Info(fmt.Sprintf("[%s] Use interface %s (%s|%s) to send request.", statusKey, instance.LoginIf, instance.LoginIfIP, instance.MAC))

	// 为空时提供默认值
	if instance.UserAgent == "" {
		instance.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36"
		slog.Debug(fmt.Sprintf("[%s] Default user agent not set. Return to default '%s'", statusKey, instance.UserAgent))
	}
	if instance.KAliveLink == "" {
		instance.KAliveLink = "http://3.3.3.3"
		slog.Debug(fmt.Sprintf("[%s] Default keep alive link not set. Return to default '%s'", statusKey, instance.KAliveLink))
	}

	// kAliveTicker := time.NewTicker(time.Duration(cfg.KeepAlive) * time.Second)
	// defer kAliveTicker.Stop()
	retry := 0

loop:
	for {
		quitSignal := false
		select {
		case <-quitSender:
			// 收到退出信号
			quitSignal = true
			doLogout(instance, statusKey, flags)
			break loop
		default:
			// 自动更新默认网口
			if instance.Interface == "" {
				now_if, now_ip, now_mac, err := GetDefaultIfIP()
				if err != nil {
					slog.Error(fmt.Sprintf("[%s] Error parsing interface: failed to get default interface ip: %v", statusKey, err))
				}
				instance.LoginIf = now_if
				instance.LoginIfIP = now_ip
				instance.MAC = now_mac
				slog.Info(fmt.Sprintf("[%s] Interface has upgrade to %s (%s|%s).", statusKey, instance.LoginIf, instance.LoginIfIP, instance.MAC))
			}

			if !quitSignal {
				// 正常执行登录逻辑
				if doLogin(instance, statusKey, flags) {
					retry = 0
				} else {
					retry++
				}

				// 达到最大错误次数，暂停 10 分钟
				if instance.RetryMax != 0 && retry >= cfg.RetryMax {
					WorkerStatusLock.Lock()
					WorkerStatus[statusKey] = StatePaused
					WorkerStatusLock.Unlock()
					slog.Error(fmt.Sprintf("[%s] reached max retries, stop 10 min.", statusKey))
					time.Sleep(time.Duration(10) * time.Minute)
				} else {
					time.Sleep(time.Duration(cfg.KeepAlive) * time.Second)
				}
			} else {
				slog.Debug(fmt.Sprintf("[%s] Quit signal received. Skip login.", statusKey))
			}
		}
	}

	WorkerStatusLock.Lock()
	WorkerStatus[statusKey] = StateStopped
	WorkerStatusLock.Unlock()
}

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
			slog.Info("Tray exit clicked, shutting down...")
			close(quitSender)
			wg.Wait()
			systray.Quit()
		}()

	}, func() {
		// 清理回调
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
