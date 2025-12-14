//go:build linux || darwin

package main

import (
	"fmt"
	"net"
)

// GetDefaultIfIP 获取跃点值最小的本机 IP 地址
// 返回 网口名称，IP地址，Mac地址
func GetDefaultIfIP() (string, string, string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", "", "", err
	}

	for _, iface := range ifaces {
		// 过滤掉 loopback 和未启用的接口
		if (iface.Flags&net.FlagUp == 0) || (iface.Flags&net.FlagLoopback != 0) {
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

			ip := ipNet.IP.To4()
			if ip == nil {
				continue // 忽略 IPv6
			}

			// 排除 link-local 地址 169.254.x.x
			if ip.IsLinkLocalUnicast() {
				continue
			}

			return iface.Name, ip.String(), iface.HardwareAddr.String(), nil
		}
	}

	return "", "", "", fmt.Errorf("no suitable interface found")
}
