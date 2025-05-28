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
	"sync/atomic"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/unixsock"
)

// ProxyTransport is the round tripper that can send requests through the proxy
// plus some associated state.
//
// It must be closed when no longer needed.
type ProxyTransport struct {
	RoundTripper http.RoundTripper // sends requests through the proxy
	Network      string            // the resolved proxy address family e.g. "unix"
	Address      string            // the resolved proxy address e.g. the absolute path to the socket

	closed atomic.Bool
	done   func() error
}

// Close releases resources used by the proxy transport.
func (p *ProxyTransport) Close() error {
	if p.closed.Swap(true) {
		return errors.Fmt("already closed")
	}
	if p.done != nil {
		return p.done()
	}
	return nil
}

// NewProxyTransport creates a round tripper that always connects to the given
// proxy address.
//
// Also returns the resolved address of the proxy for e.g. logging it.
func NewProxyTransport(proxyURL string) (*ProxyTransport, error) {
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return nil, errors.Fmt("malformed URL: %w", err)
	}
	if parsed.Scheme != "unix" {
		return nil, errors.Fmt("only unix:// scheme is supported")
	}
	if parsed.Path == "" || parsed.Host != "" {
		return nil, errors.Fmt("unexpected path format in the URL")
	}

	proxyTransport := &ProxyTransport{
		Network: "unix",
		Address: filepath.Clean(parsed.Path),
	}
	if !filepath.IsAbs(proxyTransport.Address) {
		// This should not really be possible.
		return nil, errors.Fmt("not an absolute path")
	}

	// Make sure we reference the socket object using a short path, since "unix"
	// address family has a limit on the path length.
	shortPath, cleanupPath, err := unixsock.Shorten(proxyTransport.Address)
	if err != nil {
		return nil, errors.Fmt("failed to prepare Unix socket path for dialing: %w", err)
	}
	proxyTransport.done = cleanupPath

	dialer := &net.Dialer{Timeout: 15 * time.Second}

	proxyTransport.RoundTripper = &http.Transport{
		DialContext: func(ctx context.Context, network string, addr string) (net.Conn, error) {
			if proxyTransport.closed.Load() {
				return nil, errors.Fmt("using closed ProxyTransport")
			}
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
			return dialer.DialContext(ctx, proxyTransport.Network, shortPath)
		},
		DialTLSContext: func(ctx context.Context, network string, addr string) (net.Conn, error) {
			return nil, errors.Fmt("TLS to the CIPD proxy is not supported")
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
	}

	return proxyTransport, nil
}
