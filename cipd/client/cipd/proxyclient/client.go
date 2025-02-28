// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package proxyclient contains some helpers for implementing CIPD proxy client.
package proxyclient

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"time"

	"go.chromium.org/luci/common/errors"
)

// ProxyAddr is the resolved address of the proxy server.
type ProxyAddr struct {
	Network string // e.g. "unix"
	Address string // e.g. the absolute path to the socket
}

// NewProxyTransport creates a round tripper that always connects to the given
// proxy address.
//
// Also returns the resolved address of the proxy for e.g. logging it.
func NewProxyTransport(proxyURL string) (http.RoundTripper, ProxyAddr, error) {
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return nil, ProxyAddr{}, errors.Annotate(err, "malformed URL").Err()
	}
	if parsed.Scheme != "unix" {
		return nil, ProxyAddr{}, errors.Reason("only unix:// scheme is supported").Err()
	}
	if parsed.Path == "" || parsed.Host != "" {
		return nil, ProxyAddr{}, errors.Reason("unexpected path format in the URL").Err()
	}

	proxyAddr := ProxyAddr{
		Network: "unix",
		Address: filepath.Clean(parsed.Path),
	}
	if !filepath.IsAbs(proxyAddr.Address) {
		// This should not really be possible.
		return nil, ProxyAddr{}, errors.Reason("not an absolute path").Err()
	}

	dialer := &net.Dialer{Timeout: 15 * time.Second}

	return &http.Transport{
		DialContext: func(ctx context.Context, network string, addr string) (net.Conn, error) {
			// Network is always "tcp" (it is hardcoded in http.Transport code). it is
			// unlikely to change. We'll ignore it and use "unix" instead.
			_ = network
			// This is the TCP address the client is attempting to connect to (before
			// DNS resolution). Being an HTTP client, it will also put this into
			// the `Host` header, which the proxy will read. So we can totally ignore
			// `addr` and just rely on the `Host` header to identify the target of
			// the call.
			_ = addr
			// Dial the proxy to send it the request.
			return dialer.DialContext(ctx, proxyAddr.Network, proxyAddr.Address)
		},
		DialTLSContext: func(ctx context.Context, network string, addr string) (net.Conn, error) {
			return nil, errors.Reason("TLS to the CIPD proxy is not supported").Err()
		},
		// Disable compression as it just wastes CPU on the localhost.
		DisableCompression: true,
		// Same.
		DisableKeepAlives: true,
		// The proxy server is HTTP/2.
		ForceAttemptHTTP2: true,
		// These are defaults from http.DefaultTransport.
		MaxIdleConns:    100,
		IdleConnTimeout: 90 * time.Second,
	}, proxyAddr, nil
}
