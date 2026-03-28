package main

import (
	"fmt"
	"net"
)

// GetIfIP 传入接口名称，返回 IPv4 地址
func GetIfIP(ifName string) (string, error) {
	iface, err := net.InterfaceByName(ifName)
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

	return "", fmt.Errorf("no IPv4 address found for interface %s", ifName)
}

// GetIfMAC 传入网络接口名，返回该接口的 MAC 地址
func GetIfMAC(ifName string) (string, error) {
	iface, err := net.InterfaceByName(ifName)
	if err != nil {
		return "", fmt.Errorf("interface %s not found: %v", ifName, err)
	}

	mac := iface.HardwareAddr.String()
	if mac == "" {
		return "", fmt.Errorf("no MAC address in %s", ifName)
	}

	return mac, nil
}

// GetIPMAC 根据本机的 IP 地址获取对应的网卡 MAC 地址
func GetIPMAC(ip string) (string, error) {
	netIP := net.ParseIP(ip)
	if netIP == nil {
		return "", fmt.Errorf("invalid ip address")
	}

	netIP = netIP.To4()
	if netIP == nil {
		return "", fmt.Errorf("not an ipv4 address")
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			if ipNet.IP.To4() != nil && ipNet.IP.Equal(netIP) {
				if len(iface.HardwareAddr) == 0 {
					return "", fmt.Errorf("mac address not found")
				}
				return iface.HardwareAddr.String(), nil
			}
		}
	}

	return "", fmt.Errorf("no interface found for ip")
}

// GetKeyIfName 从配置中获取接口字符串，为空则为Auto
func GetKeyIfName(instance ConfigInstance) string {
	var keyIfName string
	if keyIfName == "" {
		keyIfName = "Auto"
	} else {
		keyIfName = instance.Interface
	}
	return keyIfName
}
