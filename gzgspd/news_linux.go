//go:build linux

package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// NewHttpClientBindIP 根据IP地址绑定本地HTTP客户端
func NewHttpClientBindIP(localIP string, timeout time.Duration) (*http.Client, error) {
	ip := net.ParseIP(localIP)
	if ip == nil {
		return nil, fmt.Errorf("invalid local IP: %s", localIP)
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			var d net.Dialer
			d.Timeout = timeout

			d.Control = func(network, address string, c syscall.RawConn) error {
				var controlErr error
				err := c.Control(func(fd uintptr) {
					// 对 IPv4 TCP socket 设置绑定
					sa := &syscall.SockaddrInet4{}
					copy(sa.Addr[:], ip.To4())
					// 直接用 syscall.Bind 绑定IP
					controlErr = syscall.Bind(int(fd), sa)
				})
				if err != nil {
					return err
				}
				return controlErr
			}

			return d.DialContext(ctx, network, addr)
		},
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
