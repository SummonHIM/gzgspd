//go:build windows

package src

import (
	"errors"
	"net"
	"net/http"
	"time"
)

// NewHttpClientWithIface 根据网卡名称或IP地址绑定本地HTTP客户端
func NewHttpClientWithIface(ifaceNameOrIP string, timeout time.Duration) (*http.Client, error) {
	if ifaceNameOrIP == "" {
		return nil, errors.New("ifaceNameOrIP is empty")
	}

	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
	}

	var bindIP net.IP

	// --- 判断是否是 IP ---
	if ip := net.ParseIP(ifaceNameOrIP); ip != nil {
		// 是IP，则查找该IP对应的接口
		ifaces, err := net.Interfaces()
		if err != nil {
			return nil, err
		}
		found := false
		for _, iface := range ifaces {
			addrs, _ := iface.Addrs()
			for _, addr := range addrs {
				var addrIP net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					addrIP = v.IP
				case *net.IPAddr:
					addrIP = v.IP
				}
				if addrIP != nil && addrIP.Equal(ip) {
					bindIP = ip
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return nil, errors.New("no interface found for IP " + ip.String())
		}
	} else {
		// 不是IP，则按网卡名称查找
		iface, err := net.InterfaceByName(ifaceNameOrIP)
		if err != nil {
			return nil, err
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}
		for _, addr := range addrs {
			switch v := addr.(type) {
			case *net.IPNet:
				if v.IP.To4() != nil && !v.IP.IsLoopback() {
					bindIP = v.IP
					break
				}
			case *net.IPAddr:
				if v.IP.To4() != nil && !v.IP.IsLoopback() {
					bindIP = v.IP
					break
				}
			}
		}
		if bindIP == nil {
			return nil, errors.New("no valid IPv4 found on interface " + ifaceNameOrIP)
		}
	}

	// --- 设置本地绑定地址 ---
	dialer.LocalAddr = &net.TCPAddr{IP: bindIP}

	// --- 创建HTTP客户端 ---
	transport := &http.Transport{
		DialContext: dialer.DialContext,
	}

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: transport,
		Timeout:   timeout,
	}

	return client, nil
}
