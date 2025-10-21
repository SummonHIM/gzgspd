package src

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
)

// GetIfIP 根据接口名返回 IPv4 地址
func GetIfIP(ifaceName string) (string, error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return "", err
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		if ip4 := ipNet.IP.To4(); ip4 != nil {
			return ip4.String(), nil
		}
	}

	return "", fmt.Errorf("no IPv4 address found for interface %s", ifaceName)
}

// GetDefaultIfIP 返回默认接口（非 loopback）IP
func GetDefaultIfIP() (string, string, error) {
	switch runtime.GOOS {
	case "linux":
		// 使用 `ip route get 8.8.8.8` 获取默认网关接口和本地 IP
		out, err := exec.Command("ip", "route", "get", "8.8.8.8").Output()
		if err != nil {
			return "", "", err
		}
		parts := strings.Fields(string(out))
		var ifaceName, ipAddr string
		for i, part := range parts {
			if part == "dev" && i+1 < len(parts) {
				ifaceName = parts[i+1]
			} else if part == "src" && i+1 < len(parts) {
				ipAddr = parts[i+1]
			}
		}
		if ifaceName == "" || ipAddr == "" {
			return "", "", fmt.Errorf("cannot find default interface")
		}
		return ifaceName, ipAddr, nil

	case "darwin":
		// macOS 使用 `route -n get default` 获取默认接口
		out, err := exec.Command("route", "-n", "get", "default").Output()
		if err != nil {
			return "", "", err
		}
		var ifaceName, ipAddr string
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "interface:") {
				ifaceName = strings.TrimSpace(strings.TrimPrefix(line, "interface:"))
				iface, err := net.InterfaceByName(ifaceName)
				if err != nil {
					break
				}
				addrs, _ := iface.Addrs()
				for _, addr := range addrs {
					if ipNet, ok := addr.(*net.IPNet); ok {
						if ip := ipNet.IP.To4(); ip != nil {
							ipAddr = ip.String()
							break
						}
					}
				}
				break
			}
		}
		if ifaceName == "" || ipAddr == "" {
			return "", "", fmt.Errorf("cannot find default interface")
		}
		return ifaceName, ipAddr, nil

	case "windows":
		// 使用 CMD route print 获取默认路由
		out, err := exec.Command("cmd", "/C", "route", "print", "-4").Output()
		if err != nil {
			return "", "", err
		}
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "0.0.0.0") {
				fields := strings.Fields(line)
				if len(fields) < 5 {
					continue
				}
				ifaceIP := fields[3] // Interface IP
				ip := net.ParseIP(ifaceIP)
				if ip == nil {
					continue
				}
				// 根据接口 IP 查找网卡名
				ifaces, _ := net.Interfaces()
				for _, iface := range ifaces {
					addrs, _ := iface.Addrs()
					for _, addr := range addrs {
						if ipnet, ok := addr.(*net.IPNet); ok {
							if ipnet.IP.Equal(ip) {
								return iface.Name, ip.String(), nil
							}
						}
					}
				}
			}
		}
		return "", "", fmt.Errorf("cannot find default interface")

	default:
		return "", "", fmt.Errorf("platform %s not supported", runtime.GOOS)
	}
}

// GetInterfaceMAC 传入网络接口名，返回该接口的 MAC 地址
func GetIfMAC(ifaceName string) (string, error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return "", fmt.Errorf("interface %s not found: %v", ifaceName, err)
	}

	mac := iface.HardwareAddr.String()
	if mac == "" {
		return "", fmt.Errorf("no MAC addredd in %s", ifaceName)
	}

	return mac, nil
}

// GetIPMAC 根据本机的 IP 地址获取对应的网卡 MAC 地址
func GetIPMAC(ip string) (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("无法获取网卡列表: %v", err)
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var currentIP net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				currentIP = v.IP
			case *net.IPAddr:
				currentIP = v.IP
			}

			if currentIP == nil {
				continue
			}

			// 只比对 IP 部分，避免掺杂子网掩码
			if strings.Split(currentIP.String(), "/")[0] == ip {
				return iface.HardwareAddr.String(), nil
			}
		}
	}
	return "", fmt.Errorf("未找到 IP %s 对应的 MAC 地址", ip)
}
