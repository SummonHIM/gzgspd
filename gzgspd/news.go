//go:build windows || darwin

package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// NewHttpClientBindIP 根据IP地址绑定本地HTTP客户端
func NewHttpClientBindIP(localIP string, timeout time.Duration) (*http.Client, error) {
	ip := net.ParseIP(localIP)
	if ip == nil {
		return nil, fmt.Errorf("invalid local IP: %s", localIP)
	}

	// 解析本地IP
	localAddr := &net.TCPAddr{
		IP: ip,
	}

	// 自定义Transport
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialer := &net.Dialer{
				LocalAddr: localAddr,
				Timeout:   timeout,
			}
			return dialer.DialContext(ctx, network, addr)
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
