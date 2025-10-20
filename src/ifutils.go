package src

import (
	"fmt"
	"net"
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
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", "", err
	}

	for _, iface := range ifaces {
		// 忽略未启用或 loopback 的接口
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			if ip4 := ipNet.IP.To4(); ip4 != nil {
				// 找到第一个正常的 IPv4 地址
				return iface.Name, ip4.String(), nil
			}
		}
	}

	return "", "", fmt.Errorf("no default interface IP found")
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
