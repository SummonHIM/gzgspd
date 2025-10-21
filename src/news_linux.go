//go:build linux

package src

import (
	"errors"
	"net"
	"net/http"
	"syscall"
	"time"
)

// NewHttpClientWithIface 根据网卡名称或IP地址绑定本地HTTP客户端
func NewHttpClientWithIface(ifaceNameOrIP string, timeout time.Duration) (*http.Client, error) {
	if ifaceNameOrIP == "" {
		return nil, errors.New("ifaceNameOrIP is empty")
	}

	var ifaceName string

	// 判断是否为 IP
	if ip := net.ParseIP(ifaceNameOrIP); ip != nil {
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
					ifaceName = iface.Name
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
		// 否则直接用作网卡名
		if _, err := net.InterfaceByName(ifaceNameOrIP); err != nil {
			return nil, err
		}
		ifaceName = ifaceNameOrIP
	}

	// 创建 Dialer 并绑定到接口
	dialer := &net.Dialer{
		Timeout: timeout,
		Control: func(network, address string, c syscall.RawConn) error {
			var innerErr error
			err := c.Control(func(fd uintptr) {
				const SO_BINDTODEVICE = 25 // Linux 常量
				innerErr = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, SO_BINDTODEVICE, ifaceName)
			})
			if err != nil {
				return err
			}
			return innerErr
		},
	}

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
