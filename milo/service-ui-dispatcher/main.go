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

// Package main contains setup for push-on-green and CL UI demo routing.
// See b/367097786 and b/565833471.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/router"
)

var demoHostRe = regexp.MustCompile(`^c(\d+)(?:-p(\d+))?-dot-luci-milo-dev\.appspot\.com$`)

// demoTarget represents a Gerrit CL demo target parsed from the request Host.
type demoTarget struct {
	ChangeID string
	Patchset string
}

// VersionID returns the canonical UI_VERSION identifier (e.g. "c8509444-p2" or "c8509444").
func (d demoTarget) VersionID() string {
	if d.Patchset != "" {
		return fmt.Sprintf("c%s-p%s", d.ChangeID, d.Patchset)
	}
	return fmt.Sprintf("c%s", d.ChangeID)
}

// Host returns the canonical untainted App Engine demo hostname without port.
func (d demoTarget) Host() string {
	return fmt.Sprintf("%s-dot-luci-milo-dev.appspot.com", d.VersionID())
}

func sanitizeUpstreamHost(host string) (string, bool) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", false
	}
	if h, port, err := net.SplitHostPort(host); err == nil {
		if port != "443" && port != "80" {
			return "", false
		}
		host = h
	} else if strings.Contains(host, ":") {
		return "", false
	}
	return host, true
}

// parseDemoHost extracts the Gerrit change ID and optional patchset from
// c<CL>[-p<PS>]-dot-luci-milo-dev.appspot.com subdomains.
func parseDemoHost(host string) (demoTarget, bool) {
	cleanHost, ok := sanitizeUpstreamHost(host)
	if !ok {
		return demoTarget{}, false
	}
	m := demoHostRe.FindStringSubmatch(strings.ToLower(cleanHost))
	if m == nil {
		return demoTarget{}, false
	}
	return demoTarget{ChangeID: m[1], Patchset: m[2]}, true
}

type dispatcher struct {
	upstreamScheme string
	upstreamHost   string
	httpClient     *http.Client
}

func rejectRedirect(req *http.Request, via []*http.Request) error {
	return http.ErrUseLastResponse
}

func noRedirectHTTPClient(base *http.Client) *http.Client {
	if base != nil && base.CheckRedirect != nil && base.Timeout > 0 {
		return base
	}
	c := &http.Client{Timeout: 30 * time.Second, CheckRedirect: rejectRedirect}
	if base != nil {
		c.Transport = base.Transport
		c.Jar = base.Jar
		if base.Timeout > 0 {
			c.Timeout = base.Timeout
		}
	}
	return c
}

var defaultNoRedirectClient = noRedirectHTTPClient(nil)

func main() {
	d := &dispatcher{
		upstreamScheme: "https",
		httpClient:     defaultNoRedirectClient,
	}
	server.Main(nil, nil, func(srv *server.Server) error {
		srv.Routes.GET("/ui/*path", nil, d.dispatch)
		srv.Routes.GET("/ui_version.js", nil, d.dispatch)
		srv.Routes.GET("/root_sw.js", nil, d.dispatch)
		srv.Routes.GET("/root_sw.js.map", nil, d.dispatch)
		return nil
	})
}

func setSecurityHeaders(h http.Header) {
	h.Set("Content-Security-Policy", "frame-ancestors 'self'")
	h.Set("X-Frame-Options", "SAMEORIGIN")
	h.Set("X-Content-Type-Options", "nosniff")
}

func (d *dispatcher) dispatch(c *router.Context) {
	setSecurityHeaders(c.Writer.Header())
	demo, isDemo := parseDemoHost(c.Request.Host)

	targetURL := *c.Request.URL
	targetURL.Scheme = d.upstreamScheme
	if targetURL.Scheme == "" {
		targetURL.Scheme = "https"
	}
	switch {
	case d.upstreamHost != "":
		targetURL.Host = d.upstreamHost
	case isDemo:
		targetURL.Host = demo.Host()
	default:
		cleanHost, ok := sanitizeUpstreamHost(c.Request.Host)
		if !ok {
			http.Error(c.Writer, "invalid request host", http.StatusBadRequest)
			return
		}
		targetURL.Host = cleanHost
	}

	targetURL.Path = "/new-ui"
	if userInitiatedRollback(c) {
		targetURL.Path = "/old-ui"
	}

	reqCtx := c.Request.Context()
	reqPath := c.Request.URL.Path
	proxy := httputil.NewSingleHostReverseProxy(&targetURL)
	if d.httpClient != nil && d.httpClient.Transport != nil {
		proxy.Transport = d.httpClient.Transport
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		setSecurityHeaders(w.Header())
		http.Error(w, "upstream proxy error", http.StatusBadGateway)
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		setSecurityHeaders(resp.Header)
		if !isDemo {
			return nil
		}
		return d.guardMissingDemoFallback(reqCtx, resp, targetURL.Scheme, targetURL.Host, demo, reqPath)
	}

	proxy.ServeHTTP(c.Writer, c.Request)
}

func (d *dispatcher) client() *http.Client {
	if d.httpClient == nil {
		return defaultNoRedirectClient
	}
	return noRedirectHTTPClient(d.httpClient)
}

func isMismatchedUIVersionJS(bodyStr string, demo demoTarget) bool {
	if !strings.Contains(bodyStr, "self.UI_VERSION =") {
		return false
	}
	prefix := "self.UI_VERSION = '" + demo.VersionID()
	return !strings.Contains(bodyStr, prefix+"'") && !strings.Contains(bodyStr, prefix+"-")
}

// guardMissingDemoFallback detects when App Engine soft-routes an unuploaded
// c<CL>-dot-luci-milo-dev.appspot.com subdomain to the default push-on-green
// version and replaces the silent 200 OK fallback with an explicit 404.
func (d *dispatcher) guardMissingDemoFallback(ctx context.Context, resp *http.Response, scheme, host string, demo demoTarget, reqPath string) error {
	softRouted := resp.Header.Get("X-AppEngine-Default-Version-Fallback") != ""
	if !softRouted && reqPath == "/ui_version.js" && resp.Body != nil {
		bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		_ = resp.Body.Close()
		if err != nil {
			return err
		}
		if isMismatchedUIVersionJS(string(bodyBytes), demo) {
			softRouted = true
		} else {
			resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}
	} else if !softRouted && resp.StatusCode == http.StatusOK && (reqPath == "/ui" || strings.HasPrefix(reqPath, "/ui/")) {
		softRouted = d.probeUpstreamSoftRouted(ctx, scheme, host, demo)
	}
	if !softRouted {
		return nil
	}
	if resp.Body != nil {
		_ = resp.Body.Close()
	}
	msg := fmt.Sprintf(
		"Demo version %s is not deployed on luci-milo-dev.\n"+
			"To automatically deploy a demo for this CL, add 'Deploy-Ui-Demo: true' to your Gerrit commit message footers and run CQ+1 (or run 'make deploy-ui-demo' locally).\n",
		demo.VersionID(),
	)
	body := []byte(msg)
	resp.StatusCode = http.StatusNotFound
	resp.Status = "404 Not Found"
	setSecurityHeaders(resp.Header)
	resp.Header.Set("Content-Type", "text/plain; charset=utf-8")
	resp.Header.Set("Cache-Control", "no-cache")
	resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
	resp.ContentLength = int64(len(body))
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return nil
}

func (d *dispatcher) probeUpstreamSoftRouted(ctx context.Context, scheme, host string, demo demoTarget) bool {
	if host == "" {
		return false
	}
	if scheme == "" {
		scheme = "https"
	}
	versionURL := &url.URL{Scheme: scheme, Host: host, Path: "/new-ui/ui_version.js"}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, versionURL.String(), nil)
	if err != nil {
		return false
	}
	req.URL = versionURL
	resp, err := d.client().Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.Header.Get("X-AppEngine-Default-Version-Fallback") != "" || resp.StatusCode == http.StatusNotFound {
		return true
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return false
	}
	return isMismatchedUIVersionJS(string(bodyBytes), demo)
}

func userInitiatedRollback(c *router.Context) bool {
	cookie, err := c.Request.Cookie("USER_INITIATED_ROLLBACK")
	if err != nil {
		if !errors.Is(err, http.ErrNoCookie) {
			logging.WithError(err).Errorf(c.Request.Context(), "failed to read USER_INITIATED_ROLLBACK cookie")
		}
		return false
	}
	return cookie.Value == "true"
}
