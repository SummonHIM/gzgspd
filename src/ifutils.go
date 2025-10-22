package src

import (
	"bytes"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strconv"
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

	case "windows":
		// `route print -4` 输出多条 0.0.0.0 路由，我们取 metric 最小的那条
		cmd := exec.Command("cmd", "/C", "route", "print", "-4")
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			return "", "", err
		}

		lines := strings.Split(out.String(), "\n")
		bestMetric := 1 << 30
		var bestIfaceIP string

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "0.0.0.0") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 5 {
				continue
			}
			// Windows 默认路由格式: 网络目标 子网掩码 网关 接口 metric
			ifaceIP := fields[3]
			metric, _ := strconv.Atoi(fields[4])

			if metric < bestMetric {
				bestMetric = metric
				bestIfaceIP = ifaceIP
			}
		}

		if bestIfaceIP == "" {
			return "", "", fmt.Errorf("cannot find default route")
		}

		ip := net.ParseIP(bestIfaceIP)
		if ip == nil {
			return "", "", fmt.Errorf("invalid interface IP: %s", bestIfaceIP)
		}

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

		return "", "", fmt.Errorf("cannot find interface for IP %s", bestIfaceIP)

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
