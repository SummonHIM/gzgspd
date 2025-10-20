//go:build windows

package src

import (
	"log/slog"
	"net"
	"net/http"
	"time"
)

// NewHttpClientWithIface 绑定网卡的 HTTP 客户端
func NewHttpClientWithIface(ifaceName string, timeout time.Duration) *http.Client {
	if ifaceName == "" {
		slog.Warn("ifaceName is nil")
	}
	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
	}

	if iface, err := net.InterfaceByName(ifaceName); err == nil {
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip.To4() != nil && !ip.IsLoopback() {
				dialer.LocalAddr = &net.TCPAddr{IP: ip}
				break
			}
		}
	}

	transport := &http.Transport{DialContext: dialer.DialContext}
	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: transport,
		Timeout:   timeout,
	}
}
