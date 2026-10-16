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

package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/router"
)

func TestParseDemoHost(t *testing.T) {
	t.Parallel()

	ftt.Run("parseDemoHost", t, func(t *ftt.Test) {
		t.Run("matches CL-only demo subdomain", func(t *ftt.Test) {
			demo, ok := parseDemoHost("c8509444-dot-luci-milo-dev.appspot.com")
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, demo.ChangeID, should.Equal("8509444"))
			assert.Loosely(t, demo.Patchset, should.BeEmpty)
			assert.Loosely(t, demo.VersionID(), should.Equal("c8509444"))
			assert.Loosely(t, demo.Host(), should.Equal("c8509444-dot-luci-milo-dev.appspot.com"))
			assert.Loosely(t, demo.GCSPrefix(), should.Equal("c8509444/latest"))
		})

		t.Run("matches patchset-pinned demo subdomain", func(t *ftt.Test) {
			demo, ok := parseDemoHost("c8509444-p3-dot-luci-milo-dev.appspot.com:443")
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, demo.ChangeID, should.Equal("8509444"))
			assert.Loosely(t, demo.Patchset, should.Equal("3"))
			assert.Loosely(t, demo.VersionID(), should.Equal("c8509444-p3"))
			assert.Loosely(t, demo.Host(), should.Equal("c8509444-p3-dot-luci-milo-dev.appspot.com"))
			assert.Loosely(t, demo.GCSPrefix(), should.Equal("c8509444-p3"))
		})

		t.Run("rejects default, non-demo, or arbitrary-port hosts", func(t *ftt.Test) {
			_, ok := parseDemoHost("luci-milo-dev.appspot.com")
			assert.Loosely(t, ok, should.BeFalse)

			_, ok = parseDemoHost("21426-d228720-dot-luci-milo-dev.appspot.com")
			assert.Loosely(t, ok, should.BeFalse)

			_, ok = parseDemoHost("c8509444-dot-luci-milo-dev.appspot.com:9999")
			assert.Loosely(t, ok, should.BeFalse)
		})
	})
}

func TestGCSStaticDemoServing(t *testing.T) {
	t.Parallel()

	ftt.Run("GCS static demo serving", t, func(t *ftt.Test) {
		gcsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/luci-milo-dev-ui-demos/c8509444-p2/ui/index.html":
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				if r.Method != http.MethodHead {
					_, _ = w.Write([]byte("<!doctype html><title>Demo c8509444-p2</title>"))
				}
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer gcsServer.Close()

		d := &dispatcher{
			demoGCSBucket: "luci-milo-dev-ui-demos",
			gcsBaseURL:    gcsServer.URL,
			httpClient:    gcsServer.Client(),
		}

		t.Run("serves synthesized /ui_version.js when GCS bundle exists", func(t *ftt.Test) {
			req := httptest.NewRequest(http.MethodGet, "https://c8509444-p2-dot-luci-milo-dev.appspot.com/ui_version.js", nil)
			req.Host = "c8509444-p2-dot-luci-milo-dev.appspot.com"
			rec := httptest.NewRecorder()
			d.dispatch(&router.Context{Writer: rec, Request: req})

			assert.Loosely(t, rec.Code, should.Equal(http.StatusOK))
			assert.Loosely(t, rec.Header().Get("Content-Security-Policy"), should.Equal("frame-ancestors 'self'"))
			assert.Loosely(t, rec.Header().Get("X-Frame-Options"), should.Equal("SAMEORIGIN"))
			assert.Loosely(t, rec.Header().Get("X-Content-Type-Options"), should.Equal("nosniff"))
			assert.Loosely(t, rec.Body.String(), should.ContainSubstring("self.UI_VERSION = 'c8509444-p2';"))
		})

		t.Run("serves SPA index.html for deep /ui/* navigation routes including unencoded // in test IDs, encoded test IDs, and spaces", func(t *ftt.Test) {
			spaRoutes := []string{
				"https://c8509444-p2-dot-luci-milo-dev.appspot.com/ui/",
				"https://c8509444-p2-dot-luci-milo-dev.appspot.com/ui/fleet/labs/devices",
				"https://c8509444-p2-dot-luci-milo-dev.appspot.com/ui/p/chromium/builders/ci/Linux%20Tests",
				"https://c8509444-p2-dot-luci-milo-dev.appspot.com/ui/labs/p/chromium/tests/ninja://chrome/test:browser_tests/MyTest.Case/variants",
				"https://c8509444-p2-dot-luci-milo-dev.appspot.com/ui/labs/p/chromium/tests/ninja%3A%2F%2Fchrome%2Ftest%3Abrowser_tests%2FMyTest.Case/variants",
			}
			for _, route := range spaRoutes {
				req := httptest.NewRequest(http.MethodGet, route, nil)
				req.Host = "c8509444-p2-dot-luci-milo-dev.appspot.com"
				rec := httptest.NewRecorder()
				d.dispatch(&router.Context{Writer: rec, Request: req})

				assert.Loosely(t, rec.Code, should.Equal(http.StatusOK))
				assert.Loosely(t, rec.Header().Get("Cache-Control"), should.Equal("no-cache"))
				assert.Loosely(t, rec.Header().Get("Content-Type"), should.Equal("text/html; charset=utf-8"))
				assert.Loosely(t, rec.Body.String(), should.ContainSubstring("Demo c8509444-p2"))
			}
		})

		t.Run("returns 404 directly for /root_sw.js when GCS bundle exists", func(t *ftt.Test) {
			req := httptest.NewRequest(http.MethodGet, "https://c8509444-p2-dot-luci-milo-dev.appspot.com/root_sw.js", nil)
			req.Host = "c8509444-p2-dot-luci-milo-dev.appspot.com"
			rec := httptest.NewRecorder()
			d.dispatch(&router.Context{Writer: rec, Request: req})

			assert.Loosely(t, rec.Code, should.Equal(http.StatusNotFound))
		})
	})
}

func TestMissingDemoFallbackGuard(t *testing.T) {
	t.Parallel()

	ftt.Run("guardMissingDemoFallback", t, func(t *ftt.Test) {
		demo := demoTarget{ChangeID: "8509444"}
		d := &dispatcher{}

		t.Run("returns explicit 404 when /ui_version.js soft-routes to default main version or prefix-colliding CL", func(t *ftt.Test) {
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("\tself.UI_VERSION = '21426-d228720';\n\tself.UI_VERSION_TYPE = 'new-ui';\n")),
			}
			err := d.guardMissingDemoFallback(context.Background(), resp, "https", "", demo, "/ui_version.js")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.StatusCode, should.Equal(http.StatusNotFound))

			bodyBytes, err := io.ReadAll(resp.Body)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(bodyBytes), should.ContainSubstring("Demo version c8509444 is not deployed on luci-milo-dev."))
			assert.Loosely(t, string(bodyBytes), should.ContainSubstring("Deploy-Ui-Demo: true"))

			// Verify prefix collision (c850 vs c8509444) and patchset mismatch (c8509444-p3 vs c8509444-p2) are detected.
			assert.Loosely(t, isMismatchedUIVersionJS("\tself.UI_VERSION = 'c8509444';\n", demoTarget{ChangeID: "850"}), should.BeTrue)
			assert.Loosely(t, isMismatchedUIVersionJS("\tself.UI_VERSION = 'c8509444-p2';\n", demoTarget{ChangeID: "8509444", Patchset: "3"}), should.BeTrue)
		})

		t.Run("passes through matching c<CL> version response", func(t *ftt.Test) {
			originalBody := "\tself.UI_VERSION = 'c8509444-1a2b3c';\n\tself.UI_VERSION_TYPE = 'new-ui';\n"
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(originalBody)),
			}
			err := d.guardMissingDemoFallback(context.Background(), resp, "https", "", demo, "/ui_version.js")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.StatusCode, should.Equal(http.StatusOK))

			bodyBytes, err := io.ReadAll(resp.Body)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(bodyBytes), should.Equal(originalBody))
		})

		t.Run("returns explicit 404 when X-AppEngine-Default-Version-Fallback header is present", func(t *ftt.Test) {
			header := make(http.Header)
			header.Set("X-AppEngine-Default-Version-Fallback", "1")
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     header,
				Body:       io.NopCloser(strings.NewReader("<!doctype html>")),
			}
			err := d.guardMissingDemoFallback(context.Background(), resp, "https", "", demo, "/ui/fleet/labs/devices")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.StatusCode, should.Equal(http.StatusNotFound))
		})

		t.Run("returns explicit 404 on /ui/* when upstream /new-ui/ui_version.js probe shows fallback to main", func(t *ftt.Test) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/new-ui/ui_version.js" {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("\tself.UI_VERSION = '21426-d228720';\n\tself.UI_VERSION_TYPE = 'new-ui';\n"))
					return
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("<!doctype html><title>Default Main</title>"))
			}))
			defer upstream.Close()

			u, err := url.Parse(upstream.URL)
			assert.Loosely(t, err, should.BeNil)

			proxyDispatcher := &dispatcher{
				upstreamScheme: u.Scheme,
				upstreamHost:   u.Host,
				httpClient:     upstream.Client(),
			}
			req := httptest.NewRequest(http.MethodGet, "https://c8509444-dot-luci-milo-dev.appspot.com/ui/fleet/labs/devices", nil)
			req.Host = "c8509444-dot-luci-milo-dev.appspot.com"
			rec := httptest.NewRecorder()

			proxyDispatcher.dispatch(&router.Context{Writer: rec, Request: req})
			assert.Loosely(t, rec.Code, should.Equal(http.StatusNotFound))
			assert.Loosely(t, rec.Body.String(), should.ContainSubstring("Demo version c8509444 is not deployed on luci-milo-dev."))
		})

		t.Run("end-to-end reverse proxy preserves /old-ui rollback, security headers, and // in Chromium test IDs", func(t *ftt.Test) {
			var upstreamPath string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upstreamPath = r.URL.Path
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
			}))
			defer upstream.Close()

			u, err := url.Parse(upstream.URL)
			assert.Loosely(t, err, should.BeNil)

			proxyDispatcher := &dispatcher{
				upstreamScheme: u.Scheme,
				upstreamHost:   u.Host,
			}
			req := httptest.NewRequest(http.MethodGet, "http://"+u.Host+"/ui/labs/p/chromium/tests/ninja://chrome/test:browser_tests/MyTest.Case/variants", nil)
			req.Host = "luci-milo-dev.appspot.com"
			req.AddCookie(&http.Cookie{Name: "USER_INITIATED_ROLLBACK", Value: "true"})
			rec := httptest.NewRecorder()

			proxyDispatcher.dispatch(&router.Context{Writer: rec, Request: req})
			assert.Loosely(t, rec.Code, should.Equal(http.StatusOK))
			assert.Loosely(t, upstreamPath, should.Equal("/old-ui/ui/labs/p/chromium/tests/ninja://chrome/test:browser_tests/MyTest.Case/variants"))
			assert.Loosely(t, rec.Header().Get("Content-Security-Policy"), should.Equal("frame-ancestors 'self'"))
		})
	})
}
