//go:build windows

package main

import (
	"fmt"
	"net"

	"golang.org/x/sys/windows"
)

// GetDefaultIfIP 获取跃点值最小的本机 IP 地址
// 返回 网口名称，IP地址，Mac地址
func GetDefaultIfIP() (string, string, string, error) {
	dst := net.ParseIP("1.1.1.1").To4()
	if dst == nil {
		return "", "", "", fmt.Errorf("invalid dst ip")
	}

	var sa windows.SockaddrInet4
	copy(sa.Addr[:], dst)

	var ifIndex uint32
	if err := windows.GetBestInterfaceEx(&sa, &ifIndex); err != nil {
		return "", "", "", err
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return "", "", "", err
	}

	for _, iface := range ifaces {
		if uint32(iface.Index) != ifIndex {
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
				continue
			}

			// 排除 169.254.x.x
			if ip.IsLinkLocalUnicast() {
				continue
			}

			return iface.Name,
				ip.String(),
				iface.HardwareAddr.String(),
				nil
		}
	}

	return "", "", "", fmt.Errorf("primary interface not found")
}
