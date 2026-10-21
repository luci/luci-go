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

// Package main contains setup for push-on-green and opt-in CL UI demos.
// See b/367097786 and b/565833471.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
)

const (
	maxDemoAssetBytes       = 50 << 20 // 50 MiB
	defaultDevDemoGCSBucket = "luci-milo-dev-temp"
	gcsReadOnlyScope        = "https://www.googleapis.com/auth/devstorage.read_only"
)

var (
	demoHostRe      = regexp.MustCompile(`^c(\d+)(?:-p(\d+))?-dot-luci-milo-dev\.appspot\.com$`)
	digitsOnlyRe    = regexp.MustCompile(`^\d+$`)
	gcsBucketNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,61}[a-z0-9]$`)
	uiCommitSHARe   = regexp.MustCompile(`(?m)^\s*self\.UI_COMMIT_SHA\s*=\s*'([0-9a-f]{40})';\s*$`)
)

var immutableAssetContentTypes = map[string]string{
	".js":    "text/javascript; charset=utf-8",
	".mjs":   "text/javascript; charset=utf-8",
	".css":   "text/css; charset=utf-8",
	".map":   "application/json; charset=utf-8",
	".json":  "application/json; charset=utf-8",
	".wasm":  "application/wasm",
	".svg":   "image/svg+xml",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".gif":   "image/gif",
	".webp":  "image/webp",
	".ico":   "image/x-icon",
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".ttf":   "font/ttf",
	".eot":   "application/vnd.ms-fontobject",
	".txt":   "text/plain; charset=utf-8",
	".md":    "text/plain; charset=utf-8",
}

// demoTarget represents a Gerrit CL demo target parsed from the request Host.
type demoTarget struct {
	ChangeID string
	Patchset string
}

// Valid reports whether ChangeID and optional Patchset contain only ASCII digits.
func (d demoTarget) Valid() bool {
	if !digitsOnlyRe.MatchString(d.ChangeID) {
		return false
	}
	return d.Patchset == "" || digitsOnlyRe.MatchString(d.Patchset)
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

// GCSPrefix returns the object prefix inside the demo GCS bucket.
func (d demoTarget) GCSPrefix() string {
	if d.Patchset != "" {
		return fmt.Sprintf("c%s-p%s", d.ChangeID, d.Patchset)
	}
	return fmt.Sprintf("c%s/latest", d.ChangeID)
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
	return demoTarget{
		ChangeID: m[1],
		Patchset: m[2],
	}, true
}

type dispatcher struct {
	demoGCSBucket  string
	gcsBaseURL     string
	upstreamScheme string
	upstreamHost   string
	httpClient     *http.Client
	gcsHTTPClient  *http.Client
}

func renderUIVersionJS(versionID, commitSHA string) string {
	if commitSHA != "" {
		return fmt.Sprintf("\tself.UI_VERSION = '%s';\n\tself.UI_VERSION_TYPE = 'new-ui';\n\tself.UI_COMMIT_SHA = '%s';\n", versionID, commitSHA)
	}
	return fmt.Sprintf("\tself.UI_VERSION = '%s';\n\tself.UI_VERSION_TYPE = 'new-ui';\n", versionID)
}

func extractCommitSHA(body string) string {
	m := uiCommitSHARe.FindStringSubmatch(body)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}

func rejectRedirect(req *http.Request, via []*http.Request) error {
	return http.ErrUseLastResponse
}

func noRedirectHTTPClient(base *http.Client) *http.Client {
	if base != nil && base.CheckRedirect != nil && base.Timeout > 0 {
		return base
	}
	c := &http.Client{
		Timeout:       30 * time.Second,
		CheckRedirect: rejectRedirect,
	}
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

func resolveDemoGCSBucket(explicitBucket, cloudProject string) string {
	b := strings.TrimSpace(explicitBucket)
	proj := strings.TrimSpace(cloudProject)
	if proj == "luci-milo" && b == defaultDevDemoGCSBucket {
		return ""
	}
	if b != "" {
		return b
	}
	if proj == "luci-milo-dev" {
		return defaultDevDemoGCSBucket
	}
	return ""
}

func main() {
	d := &dispatcher{
		gcsBaseURL:     "https://storage.googleapis.com",
		upstreamScheme: "https",
	}
	flag.StringVar(
		&d.demoGCSBucket,
		"demo-gcs-bucket",
		"",
		"Optional GCS bucket containing opt-in Gerrit CL UI demo bundles (defaults to luci-milo-dev-temp on luci-milo-dev).",
	)
	server.Main(nil, nil, func(srv *server.Server) error {
		d.demoGCSBucket = resolveDemoGCSBucket(d.demoGCSBucket, srv.Options.CloudProject)
		if d.demoGCSBucket != "" && d.httpClient == nil {
			if tr, err := auth.GetRPCTransport(srv.Context, auth.AsSelf, auth.WithScopes(gcsReadOnlyScope)); err == nil && tr != nil {
				d.gcsHTTPClient = noRedirectHTTPClient(&http.Client{Transport: tr})
			}
		}
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

// contentTypeForAsset returns the deterministic Content-Type for an allowed
// static demo asset path, whether it should receive a sandboxed non-script CSP,
// and whether the asset path/extension is permitted.
func contentTypeForAsset(assetPath string) (contentType string, sandboxCSP bool, ok bool) {
	if assetPath == "/ui/index.html" {
		return "text/html; charset=utf-8", false, true
	}
	if assetPath == "/ui/ui_version.js" {
		return "text/javascript; charset=utf-8", false, true
	}
	if assetPath == "/ui/manifest.webmanifest" {
		return "application/manifest+json; charset=utf-8", true, true
	}
	if !strings.HasPrefix(assetPath, "/ui/immutable/") || len(assetPath) <= len("/ui/immutable/") {
		return "", false, false
	}
	ext := strings.ToLower(path.Ext(assetPath))
	ct, allowed := immutableAssetContentTypes[ext]
	if !allowed {
		return "", false, false
	}
	return ct, ext != ".js" && ext != ".mjs", true
}

// isStaticAssetRequest reports whether u targets a static asset path whose
// request path is used directly to construct a GCS object path
// (/ui/immutable/* or /ui/manifest.webmanifest). SPA navigation routes (/ui/*)
// always serve the constant /ui/index.html object and never pass u.Path to GCS.
func isStaticAssetRequest(u *url.URL) bool {
	if u == nil {
		return false
	}
	p := u.Path
	escaped := u.EscapedPath()
	return p == "/ui/immutable" || strings.HasPrefix(p, "/ui/immutable/") ||
		escaped == "/ui/immutable" || strings.HasPrefix(escaped, "/ui/immutable/") ||
		p == "/ui/manifest.webmanifest" || escaped == "/ui/manifest.webmanifest"
}

func isSafeStaticAssetURL(u *url.URL) bool {
	if u == nil || u.RawPath != "" || u.EscapedPath() != u.Path {
		return false
	}
	return isSafeAssetPath(u.Path)
}

func isSafeAssetPath(assetPath string) bool {
	if assetPath == "" || !strings.HasPrefix(assetPath, "/") || strings.ContainsAny(assetPath, "%?#\\") {
		return false
	}
	for i := 0; i < len(assetPath); i++ {
		if assetPath[i] <= 0x20 || assetPath[i] >= 0x7f {
			return false
		}
	}
	if path.Clean(assetPath) != assetPath {
		return false
	}
	_, _, ok := contentTypeForAsset(assetPath)
	return ok
}

func (d *dispatcher) buildGCSObjectURL(demo demoTarget, assetPath string) (*url.URL, error) {
	if !demo.Valid() {
		return nil, errors.New("invalid demo target")
	}
	if !gcsBucketNameRe.MatchString(d.demoGCSBucket) {
		return nil, errors.New("invalid demo GCS bucket name")
	}
	if !isSafeAssetPath(assetPath) {
		return nil, errors.New("unsafe demo asset path")
	}
	baseURL := d.gcsBaseURL
	if baseURL == "" {
		baseURL = "https://storage.googleapis.com"
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("unsupported GCS scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return nil, errors.New("missing GCS host")
	}
	prefix := path.Join("/", u.Path, d.demoGCSBucket, demo.GCSPrefix())
	fullPath := path.Join(prefix, assetPath)
	if !strings.HasPrefix(fullPath, prefix+"/") {
		return nil, errors.New("asset path escapes demo GCS prefix")
	}
	u.Path = fullPath
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u, nil
}

func (d *dispatcher) dispatch(c *router.Context) {
	setSecurityHeaders(c.Writer.Header())

	demo, isDemo := parseDemoHost(c.Request.Host)
	if isDemo && d.demoGCSBucket != "" {
		if d.tryServeGCSDemo(c, demo) {
			return
		}
	}

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

func (d *dispatcher) tryServeGCSDemo(c *router.Context, demo demoTarget) bool {
	ctx := c.Request.Context()
	reqPath := c.Request.URL.Path
	isHead := c.Request.Method == http.MethodHead

	if reqPath == "/ui_version.js" {
		body, ok := d.fetchGCSVersionJS(ctx, demo)
		if !ok {
			if !d.gcsBundleExists(ctx, demo) {
				return false
			}
			body = renderUIVersionJS(demo.VersionID(), "")
		}
		bodyBytes := []byte(body)
		h := c.Writer.Header()
		setSecurityHeaders(h)
		h.Set("Content-Type", "text/javascript; charset=utf-8")
		h.Set("Content-Length", strconv.Itoa(len(bodyBytes)))
		h.Set("Cache-Control", "no-cache")
		c.Writer.WriteHeader(http.StatusOK)
		if !isHead {
			_, _ = c.Writer.Write(bodyBytes)
		}
		return true
	}

	if reqPath == "/root_sw.js" || reqPath == "/root_sw.js.map" {
		if !d.gcsBundleExists(ctx, demo) {
			return false
		}
		setSecurityHeaders(c.Writer.Header())
		http.Error(c.Writer, "service worker not found", http.StatusNotFound)
		return true
	}

	if reqPath != "/ui" && !strings.HasPrefix(reqPath, "/ui/") {
		return false
	}

	assetPath := "/ui/index.html"
	cacheControl := "no-cache"
	if isStaticAssetRequest(c.Request.URL) {
		if !isSafeStaticAssetURL(c.Request.URL) {
			setSecurityHeaders(c.Writer.Header())
			http.Error(c.Writer, "invalid demo asset path", http.StatusBadRequest)
			return true
		}
		assetPath = reqPath
		if strings.HasPrefix(assetPath, "/ui/immutable/") {
			cacheControl = "public, max-age=604800, immutable"
		}
	}

	contentType, sandboxCSP, ok := contentTypeForAsset(assetPath)
	if !ok {
		setSecurityHeaders(c.Writer.Header())
		http.Error(c.Writer, "invalid demo asset path", http.StatusBadRequest)
		return true
	}

	objURL, err := d.buildGCSObjectURL(demo, assetPath)
	if err != nil {
		setSecurityHeaders(c.Writer.Header())
		http.Error(c.Writer, "invalid demo asset path", http.StatusBadRequest)
		return true
	}
	method := http.MethodGet
	if isHead {
		method = http.MethodHead
	}
	req, err := http.NewRequestWithContext(ctx, method, objURL.String(), nil)
	if err != nil {
		return false
	}
	req.URL = objURL
	resp, err := d.gcsClient(ctx).Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		setSecurityHeaders(c.Writer.Header())
		http.Error(c.Writer, "unexpected upstream redirect", http.StatusBadGateway)
		return true
	}
	if resp.StatusCode == http.StatusNotFound {
		if assetPath != "/ui/index.html" && d.gcsBundleExists(ctx, demo) {
			setSecurityHeaders(c.Writer.Header())
			http.Error(c.Writer, "demo asset not found", http.StatusNotFound)
			return true
		}
		return false
	}
	if resp.StatusCode != http.StatusOK {
		setSecurityHeaders(c.Writer.Header())
		http.Error(c.Writer, "upstream demo storage error", http.StatusBadGateway)
		return true
	}
	if resp.ContentLength > maxDemoAssetBytes {
		setSecurityHeaders(c.Writer.Header())
		http.Error(c.Writer, "demo asset exceeds size limit", http.StatusBadGateway)
		return true
	}

	var bufferedBody []byte
	if !isHead && resp.ContentLength < 0 {
		bufferedBody, err = io.ReadAll(io.LimitReader(resp.Body, maxDemoAssetBytes+1))
		if err != nil {
			setSecurityHeaders(c.Writer.Header())
			http.Error(c.Writer, "upstream demo storage error", http.StatusBadGateway)
			return true
		}
		if int64(len(bufferedBody)) > maxDemoAssetBytes {
			setSecurityHeaders(c.Writer.Header())
			http.Error(c.Writer, "demo asset exceeds size limit", http.StatusBadGateway)
			return true
		}
	}

	h := c.Writer.Header()
	setSecurityHeaders(h)
	if sandboxCSP {
		h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox; frame-ancestors 'self'")
	}
	h.Set("Content-Type", contentType)
	h.Set("Cache-Control", cacheControl)
	if resp.ContentLength >= 0 {
		h.Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
	} else if !isHead {
		h.Set("Content-Length", strconv.Itoa(len(bufferedBody)))
	}
	if etag := resp.Header.Get("ETag"); etag != "" {
		h.Set("ETag", etag)
	}
	if lastMod := resp.Header.Get("Last-Modified"); lastMod != "" {
		h.Set("Last-Modified", lastMod)
	}
	c.Writer.WriteHeader(http.StatusOK)
	if !isHead {
		if resp.ContentLength < 0 {
			_, _ = c.Writer.Write(bufferedBody)
		} else {
			_, _ = io.Copy(c.Writer, io.LimitReader(resp.Body, maxDemoAssetBytes))
		}
	}
	return true
}

func (d *dispatcher) fetchGCSVersionJS(ctx context.Context, demo demoTarget) (string, bool) {
	verURL, err := d.buildGCSObjectURL(demo, "/ui/ui_version.js")
	if err != nil {
		return "", false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, verURL.String(), nil)
	if err != nil {
		return "", false
	}
	req.URL = verURL
	resp, err := d.gcsClient(ctx).Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength > 64*1024 {
		return "", false
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", false
	}
	bodyStr := string(bodyBytes)
	if !strings.Contains(bodyStr, "self.UI_VERSION =") || isMismatchedUIVersionJS(bodyStr, demo) {
		return "", false
	}
	commitSHA := extractCommitSHA(bodyStr)
	return renderUIVersionJS(demo.VersionID(), commitSHA), true
}

func (d *dispatcher) gcsBundleExists(ctx context.Context, demo demoTarget) bool {
	indexURL, err := d.buildGCSObjectURL(demo, "/ui/index.html")
	if err != nil {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, indexURL.String(), nil)
	if err != nil {
		return false
	}
	req.URL = indexURL
	resp, err := d.gcsClient(ctx).Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (d *dispatcher) gcsClient(ctx context.Context) *http.Client {
	if d.httpClient != nil {
		return noRedirectHTTPClient(d.httpClient)
	}
	if d.gcsHTTPClient != nil {
		return d.gcsHTTPClient
	}
	tr, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(gcsReadOnlyScope))
	if err != nil || tr == nil {
		return defaultNoRedirectClient
	}
	return noRedirectHTTPClient(&http.Client{Transport: tr})
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
	versionURL := &url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   "/new-ui/ui_version.js",
	}
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
