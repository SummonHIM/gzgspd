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
	IfIP         string
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

func doLogout(instance *WorkerInstance) {
	slog.Info(fmt.Sprintf("[%s] Logging out...", instance.Username))
	logoutStat, err := src.QuickAuthDisconn(
		instance.Interface,
		instance.LoginHost,
		instance.UserAgent,
		instance.WlanacIp,
		instance.Wlanuserip,
		instance.Wlanacname,
		instance.Version,
		"0",
		instance.LogoutUID,
		instance.MAC,
		instance.GroupID,
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

func worker(cfg *src.ConfigInstance, quitSender <-chan struct{}, quitWaiter *sync.WaitGroup) {
	defer quitWaiter.Done()
	slog.Info(fmt.Sprintf("[%s] Starting instance %s", cfg.Username, cfg.Username))
	// 将配置写入当前内存中
	instance := &WorkerInstance{
		ConfigInstance: *cfg,
	}

	// 分析接口的IP
	if cfg.Interface == "" {
		ifname, ip, err := src.GetDefaultIfIP()
		if err != nil {
			slog.Error(fmt.Sprintf("[%s] Failed to get default interface ip: %v", instance.Username, err))
			return
		}
		mac, err := src.GetIPMAC(ip)
		if err != nil {
			slog.Error(fmt.Sprintf("[%s] Failed to get default interface mac: %v", instance.Username, err))
			return
		}
		instance.Interface = ifname
		instance.IfIP = ip
		instance.MAC = mac
	} else if net.ParseIP(cfg.Interface) == nil {
		ip, err := src.GetIfIP(cfg.Interface)
		if err != nil {
			slog.Error(fmt.Sprintf("[%s] Failed to get interface '%s' ip: %v", instance.Username, cfg.Interface, err))
			return
		}
		mac, err := src.GetIfMAC(cfg.Interface)
		if err != nil {
			slog.Error(fmt.Sprintf("[%s] Failed to get interface '%s' mac: %v", instance.Username, cfg.Interface, err))
			return
		}
		instance.IfIP = ip
		instance.MAC = mac
	} else {
		instance.IfIP = cfg.Interface
		mac, err := src.GetIPMAC(cfg.Interface)
		if err != nil {
			slog.Error(fmt.Sprintf("[%s] Failed to get default interface mac: %v", instance.Username, err))
			return
		}
		instance.MAC = mac
	}
	slog.Debug(fmt.Sprintf("[%s] Use interface %s (%s|%s) to send request.", instance.Username, instance.Interface, instance.IfIP, instance.MAC))

	// 为空时提供默认值
	if instance.UserAgent == "" {
		instance.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36"
		slog.Debug(fmt.Sprintf("[%s] Default user agent not set. Return to default '%s'", instance.Username, instance.UserAgent))
	}
	if instance.KAliveLink == "" {
		instance.KAliveLink = "http://1.1.1.1"
		slog.Debug(fmt.Sprintf("[%s] Default keep alive link not set. Return to default '%s'", instance.Username, instance.KAliveLink))
	}
	if instance.LoginHost == "" {
		instance.LoginHost = "10.20.16.5"
	}
	if instance.Wlanacname == "" {
		instance.Wlanacname = "NFV-BASE-01"
	}

	kAliveTicker := time.NewTicker(time.Duration(cfg.KeepAlive) * time.Second)
	defer kAliveTicker.Stop()
	retry := 0

	for {
		// 第一段：非阻塞地优先检查退出
		select {
		case <-quitSender:
			// 先停掉ticker，避免更多tick进来
			kAliveTicker.Stop()
			doLogout(instance)
			return
		default:
		}

		// 第二段：阻塞等待 tick 或 退出
		select {
		case <-quitSender:
			// 登出逻辑
			kAliveTicker.Stop()
			doLogout(instance)
			return
		case <-kAliveTicker.C:
			// 再次非阻塞检查一次退出，确保“同时就绪”时也优先退出
			select {
			case <-quitSender:
				kAliveTicker.Stop()
				doLogout(instance)
				return
			default:
			}

			// 重新检查最新网络接口
			if cfg.Interface == "" {
				ifname, ip, err := src.GetDefaultIfIP()
				if err != nil {
					slog.Error(fmt.Sprintf("[%s] Failed to get default interface ip: %v", instance.Username, err))
					return
				}
				mac, err := src.GetIPMAC(ip)
				if err != nil {
					slog.Error(fmt.Sprintf("[%s] Failed to get default interface mac: %v", instance.Username, err))
					return
				}
				instance.Interface = ifname
				instance.IfIP = ip
				instance.MAC = mac

				slog.Debug(fmt.Sprintf("[%s] Use interface %s (%s|%s) to send request.", instance.Username, instance.Interface, instance.IfIP, instance.MAC))
			}

			// 检查是否需要登录
			slog.Debug(fmt.Sprintf("[%s] Checking portal if login is required.", instance.Username))
			needLogin, needLoginUrl := src.PortalChecker(
				instance.Interface,
				cfg.KAliveLink,
			)
			slog.Debug(fmt.Sprintf("[%s] Need login: %t Redirect link: %s", instance.Username, needLogin, needLoginUrl))

			if needLogin && needLoginUrl != "" {
				// 若需要登陆，且获取到重定向登录链接
				slog.Info(fmt.Sprintf("[%s] Login require.", instance.Username))

				nlu, err := url.Parse(needLoginUrl)
				if err != nil {
					slog.Error(fmt.Sprintf("[%s] Failed to parse redirect login link: %v", instance.Username, err))
					continue
				}

				// 提取重定向 URL 的参数
				instance.LoginHost = nlu.Host
				instance.Wlanuserip = nlu.Query().Get("wlanuserip")
				instance.Wlanacname = nlu.Query().Get("wlanacname")
				instance.MAC = nlu.Query().Get("mac")
				instance.Vlan = nlu.Query().Get("vlan")
				instance.HostName = nlu.Query().Get("hostname")
				instance.Rand = nlu.Query().Get("rand")

				// 获取登录基本信息
				portalConfig, err := src.PortalJsonAction(
					instance.Interface,
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
					slog.Error(fmt.Sprintf("[%s] Failed to fetch Portal Json Action: %v", instance.Username, err))
					continue
				}
				// slog.Debug(fmt.Sprintf("[%s] Portal Config: %v", instance.Username, portalConfig))

				// 提取登录基本信息
				instance.WlanacIp = portalConfig.ServerForm.Serverip
				instance.Version = portalConfig.ServerForm.PortalVer
				instance.PortalPageID = portalConfig.PortalConfig.ID
				instance.TimeStamp = portalConfig.PortalConfig.Timestamp
				instance.UUID = portalConfig.PortalConfig.UUID

				// 登录
				loginStat, err := src.QuickAuth(
					instance.Interface,
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
					if err != nil {
						slog.Error(fmt.Sprintf("[%s] Login failed: %v", instance.Username, err))
					} else if loginStat.Code != "0" {
						slog.Error(fmt.Sprintf("[%s] Login failed: %s", instance.Username, loginStat.Message))
					} else {
						slog.Error(fmt.Sprintf("[%s] Login failed: Unknown error.", instance.Username))
					}

					retry++
					if instance.RetryMax != 0 && retry >= cfg.RetryMax {
						slog.Error(fmt.Sprintf("[%s] reached max retries, stop.", instance.Username))
						return
					}
					timer := time.NewTimer(time.Duration(cfg.RetryTime) * time.Second)
					select {
					case <-quitSender:
						timer.Stop()
						continue // 下一轮触发退出分支
					case <-timer.C:
					}
					continue
				}
				retry = 0
				instance.GroupID = loginStat.GroupID
				instance.LogoutUID = loginStat.UserID
				slog.Info(fmt.Sprintf("[%s] Login successfully!", instance.Username))
			}
			// time.Sleep(time.Duration(cfg.KeepAlive) * time.Second)
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
	slog.Info("Starting GZGS portal daemon...")

	// 监听终止命令
	sigs := make(chan os.Signal, 1)
	quitSender := make(chan struct{})
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	var quitWaiter sync.WaitGroup
	for i := range cfg.Instance {
		quitWaiter.Add(1)
		go worker(&cfg.Instance[i], quitSender, &quitWaiter)
	}

	<-sigs
	slog.Info("Caught termination signal, logging out...")
	close(quitSender)
	quitWaiter.Wait()
	slog.Info("Exiting...")
}
