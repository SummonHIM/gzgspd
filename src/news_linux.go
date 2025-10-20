//go:build linux

package src

import (
	"net"
	"net/http"
	"syscall"
	"time"
)

// NewHttpClientWithIface 绑定网卡的 HTTP 客户端
func NewHttpClientWithIface(ifaceName string, timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout: timeout,
		Control: func(network, address string, c syscall.RawConn) error {
			var innerErr error
			err := c.Control(func(fd uintptr) {
				// Linux 下的 SO_BINDTODEVICE 常量
				const SO_BINDTODEVICE = 25
				// 绑定到指定网卡
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
	return client
}
