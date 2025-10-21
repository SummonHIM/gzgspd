package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"gzgspd/src"
)

type Flags struct {
	Config string
}

type WorkerInstance struct {
	src.ConfigInstance
	LoginIf      *string
	LoginIfIP    *string
	LoginHost    *string
	Wlanuserip   *string
	Wlanacname   *string
	MAC          *string
	Vlan         *string
	HostName     *string
	Rand         *string
	WlanacIp     *string
	Version      *int
	PortalPageID *int
	TimeStamp    *int64
	UUID         *string
	GroupID      *int
	LogoutUID    *string
}

var Version = "dev"

func doLogin(instance *WorkerInstance) bool {
	// 检查是否需要登录
	slog.Debug(fmt.Sprintf("[%s] Checking portal if login is required.", instance.Username))
	needLogin, needLoginUrl := src.PortalChecker(
		*instance.LoginIf,
		instance.KAliveLink,
	)
	slog.Debug(fmt.Sprintf("[%s] Need login: %t Redirect link: %s", instance.Username, needLogin, needLoginUrl))

	if needLogin && needLoginUrl != "" {
		// 若需要登陆，且获取到重定向登录链接
		slog.Info(fmt.Sprintf("[%s] Login require.", instance.Username))

		nlu, err := url.Parse(needLoginUrl)
		if err != nil {
			slog.Error(fmt.Sprintf("[%s] Failed to parse redirect login link: %v", instance.Username, err))
			return false
		}

		// 提取重定向 URL 的参数
		tWlanuserip := nlu.Query().Get("wlanuserip")
		tWlanacname := nlu.Query().Get("wlanacname")
		tMAC := nlu.Query().Get("mac")
		tVlan := nlu.Query().Get("vlan")
		tHostName := nlu.Query().Get("hostname")
		tRand := nlu.Query().Get("rand")
		instance.LoginHost = &nlu.Host
		instance.Wlanuserip = &tWlanuserip
		instance.Wlanacname = &tWlanacname
		instance.MAC = &tMAC
		instance.Vlan = &tVlan
		instance.HostName = &tHostName
		instance.Rand = &tRand

		// 获取登录基本信息
		portalConfig, err := src.PortalJsonAction(
			*instance.LoginIf,
			*instance.LoginHost,
			instance.UserAgent,
			*instance.Wlanuserip,
			*instance.Wlanacname,
			*instance.MAC,
			*instance.Vlan,
			*instance.HostName,
			*instance.Rand,
		)
		if err != nil {
			slog.Error(fmt.Sprintf("[%s] Failed to fetch Portal Json Action: %v", instance.Username, err))
			return false
		}
		// slog.Debug(fmt.Sprintf("[%s] Portal Config: %v", instance.Username, portalConfig))

		// 提取登录基本信息
		instance.WlanacIp = &portalConfig.ServerForm.Serverip
		instance.Version = &portalConfig.ServerForm.PortalVer
		instance.PortalPageID = &portalConfig.PortalConfig.ID
		instance.TimeStamp = &portalConfig.PortalConfig.Timestamp
		instance.UUID = &portalConfig.PortalConfig.UUID

		// 登录
		loginStat, err := src.QuickAuth(
			*instance.LoginIf,
			*instance.LoginHost,
			instance.UserAgent,
			instance.Username,
			instance.Password,
			*instance.Wlanuserip,
			*instance.Wlanacname,
			*instance.WlanacIp,
			*instance.Vlan,
			*instance.MAC,
			*instance.Version,
			*instance.PortalPageID,
			*instance.TimeStamp,
			*instance.UUID,
			"0",
			*instance.HostName,
			*instance.Rand,
		)
		if loginStat.Code != "0" || err != nil {
			if err != nil {
				slog.Error(fmt.Sprintf("[%s] Login failed: %v", instance.Username, err))
			} else if loginStat.Code != "0" {
				slog.Error(fmt.Sprintf("[%s] Login failed: %s", instance.Username, loginStat.Message))
			} else {
				slog.Error(fmt.Sprintf("[%s] Login failed: Unknown error.", instance.Username))
			}
			return false
		}
		instance.GroupID = &loginStat.GroupID
		instance.LogoutUID = &loginStat.UserID
		slog.Info(fmt.Sprintf("[%s] Login successfully!", instance.Username))
	}

	return true
}

func doLogout(instance *WorkerInstance) {
	slog.Info(fmt.Sprintf("[%s] Logging out...", instance.Username))
	tMac, _ := src.GetIPMAC(*instance.LoginIfIP)
	logoutStat, err := src.QuickAuthDisconn(
		*instance.LoginIf,
		src.PtrOrDefault(instance.LoginHost, "10.20.16.5"),
		instance.UserAgent,
		src.PtrOrDefault(instance.WlanacIp, "10.20.16.2"),
		src.PtrOrDefault(instance.Wlanuserip, *instance.LoginIfIP),
		src.PtrOrDefault(instance.Wlanacname, "NFV-BASE-01"),
		src.PtrOrDefault(instance.Version, 4),
		"0",
		src.PtrOrDefault(instance.LogoutUID, instance.Username+"@SSGSXY"),
		src.PtrOrDefault(instance.MAC, tMac),
		src.PtrOrDefault(instance.GroupID, 19),
		"0",
	)
	if err != nil {
		slog.Error(fmt.Sprintf("[%s] Logout failed: %v", instance.Username, err))
	} else if logoutStat.Code != "0" {
		slog.Error(fmt.Sprintf("[%s] Logout failed: %s", instance.Username, logoutStat.Message))
	} else {
		slog.Info(fmt.Sprintf("[%s] Logged out successfully.", instance.Username))
	}
}

// 解析网络接口设置
func parseInterface(instanceIf string) (string, string, string, error) {
	if instanceIf == "" {
		// 如果为空
		ifname, ip, err := src.GetDefaultIfIP()
		if err != nil {
			return "", "", "", fmt.Errorf("failed to get default interface ip: %v", err)
		}
		mac, err := src.GetIPMAC(ip)
		if err != nil {
			return "", "", "", fmt.Errorf("failed to get default interface mac: %v", err)
		}

		return ifname, ip, mac, nil
	} else if net.ParseIP(instanceIf) == nil {
		// 如果不为 IP 地址
		ip, err := src.GetIfIP(instanceIf)
		if err != nil {
			return "", "", "", fmt.Errorf("failed to get interface '%s' ip: %v", instanceIf, err)
		}
		mac, err := src.GetIfMAC(instanceIf)
		if err != nil {
			return "", "", "", fmt.Errorf("failed to get interface '%s' mac: %v", instanceIf, err)
		}
		return instanceIf, ip, mac, nil
	} else {
		// 如果是 IP 地址
		mac, err := src.GetIPMAC(instanceIf)
		if err != nil {
			return "", "", "", fmt.Errorf("failed to get interface '%s' mac: %v", instanceIf, err)
		}
		return instanceIf, instanceIf, mac, nil
	}
}

// 工作函数
func worker(cfg src.ConfigInstance, quitSender <-chan struct{}, quitWaiter *sync.WaitGroup) {
	defer quitWaiter.Done()

	slog.Info(fmt.Sprintf("[%s] Starting instance %s", cfg.Username, cfg.Username))
	// 将配置写入当前内存中
	instance := &WorkerInstance{
		ConfigInstance: cfg,
	}

	// 分析接口的IP
	tLoginIf, tLoginIfIP, tMac, err := parseInterface(instance.Interface)
	if err != nil {
		slog.Error(fmt.Sprintf("[%s] Error parsing interface: %v", instance.Username, err))
		return
	} else {
		instance.LoginIf = &tLoginIf
		instance.LoginIfIP = &tLoginIfIP
		instance.MAC = &tMac
	}
	slog.Info(fmt.Sprintf("[%s] Use interface %s (%s|%s) to send request.", instance.Username, *instance.LoginIf, *instance.LoginIfIP, *instance.MAC))

	// 为空时提供默认值
	if instance.UserAgent == "" {
		instance.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36"
		slog.Debug(fmt.Sprintf("[%s] Default user agent not set. Return to default '%s'", instance.Username, instance.UserAgent))
	}
	if instance.KAliveLink == "" {
		instance.KAliveLink = "http://3.3.3.3"
		slog.Debug(fmt.Sprintf("[%s] Default keep alive link not set. Return to default '%s'", instance.Username, instance.KAliveLink))
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
			doLogout(instance)
			break loop
		default:
			// 自动更新默认网口
			if instance.Interface == "" {
				now_if, now_ip, err := src.GetDefaultIfIP()
				if err != nil {
					slog.Error(fmt.Sprintf("[%s] Error parsing interface: failed to get default interface ip: %v", instance.Username, err))
				}
				if now_if != *instance.LoginIf || now_ip != *instance.LoginIfIP {
					tLoginIf, tLoginIfIP, tMac, err := parseInterface(instance.Interface)
					if err != nil {
						slog.Error(fmt.Sprintf("[%s] Error parsing interface: %v", instance.Username, err))
					} else {
						instance.LoginIf = &tLoginIf
						instance.LoginIfIP = &tLoginIfIP
						instance.MAC = &tMac
						slog.Info(fmt.Sprintf("[%s] Interface has upgrade to %s (%s|%s).", instance.Username, *instance.LoginIf, *instance.LoginIfIP, *instance.MAC))
					}
				}
			}

			if !quitSignal {
				// 正常执行登录逻辑
				if doLogin(instance) {
					retry = 0
				} else {
					retry++
				}

				// 达到最大错误次数，暂停 10 分钟
				if instance.RetryMax != 0 && retry >= cfg.RetryMax {
					slog.Error(fmt.Sprintf("[%s] reached max retries, stop 10 min.", instance.Username))
					time.Sleep(time.Duration(10) * time.Minute)
				} else {
					time.Sleep(time.Duration(cfg.KeepAlive) * time.Second)
				}
			} else {
				slog.Debug(fmt.Sprintf("[%s] Quit signal received. Skip login.", instance.Username))
			}
		}
	}
}

func main() {
	// 解析参数
	flags := &Flags{}
	flag.StringVar(&flags.Config, "config", "config.json", "Specify the configuration file path.")
	flag.Parse()

	// 读取配置文件
	cfg, err := src.LoadConfig(flags.Config)
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
	quitSender := make(chan struct{})
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	var waitGroups sync.WaitGroup

	for i := range cfg.Instance {
		waitGroups.Add(1)
		go worker(cfg.Instance[i], quitSender, &waitGroups)
	}

	<-sigs
	slog.Info("Caught termination signal, logging out...")
	close(quitSender)
	waitGroups.Wait()
	slog.Info("All instances stopped. Exiting...")
}
