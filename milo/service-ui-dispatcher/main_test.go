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

		t.Run("resolves default demo GCS bucket only for luci-milo-dev and rejects dev bucket on luci-milo", func(t *ftt.Test) {
			assert.Loosely(t, resolveDemoGCSBucket("", "luci-milo-dev"), should.Equal(defaultDevDemoGCSBucket))
			assert.Loosely(t, resolveDemoGCSBucket("", "luci-milo"), should.BeEmpty)
			assert.Loosely(t, resolveDemoGCSBucket("", ""), should.BeEmpty)
			assert.Loosely(t, resolveDemoGCSBucket(defaultDevDemoGCSBucket, "luci-milo"), should.BeEmpty)
			assert.Loosely(t, resolveDemoGCSBucket("custom-demo-bucket", "luci-milo-dev"), should.Equal("custom-demo-bucket"))
			assert.Loosely(t, resolveDemoGCSBucket("custom-demo-bucket", "luci-milo"), should.Equal("custom-demo-bucket"))
		})
	})
}

func TestGCSStaticDemoServing(t *testing.T) {
	t.Parallel()

	ftt.Run("GCS static demo serving", t, func(t *ftt.Test) {
		var requestedPaths []string
		var redirectFollowed bool
		gcsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestedPaths = append(requestedPaths, r.URL.Path)
			switch r.URL.Path {
			case "/luci-milo-dev-ui-demos/c8509444-p2/ui/index.html":
				// Even if GCS metadata is misconfigured, dispatcher enforces text/html for index.html.
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				if r.Method != http.MethodHead {
					_, _ = w.Write([]byte("<!doctype html><title>Demo c8509444-p2</title>"))
				}
			case "/luci-milo-dev-ui-demos/c8509444/latest/ui/index.html":
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				if r.Method != http.MethodHead {
					_, _ = w.Write([]byte("<!doctype html><title>Demo c8509444</title>"))
				}
			case "/luci-milo-dev-ui-demos/c8509444/latest/ui/ui_version.js":
				w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("\tself.UI_VERSION = 'c8509444';\n\tmaliciousPayload();\n\tself.UI_VERSION_TYPE = 'new-ui';\n\tself.UI_COMMIT_SHA = '6f86e190247751075b5778c4d51d3a7d3eab06f9';\n"))
			case "/luci-milo-dev-ui-demos/c8509444-p2/ui/manifest.webmanifest":
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				if r.Method != http.MethodHead {
					_, _ = w.Write([]byte(`{"name":"LUCI UI"}`))
				}
			case "/luci-milo-dev-ui-demos/c8509444-p2/ui/immutable/index-abc.js":
				// Even if GCS returns text/html on a .js object, dispatcher derives text/javascript from extension.
				body := []byte("console.log('immutable bundle');")
				w.Header().Set("Content-Type", "text/html")
				w.Header().Set("ETag", `"etag-index-abc"`)
				w.Header().Set("Last-Modified", "Mon, 19 Oct 2026 00:00:00 GMT")
				w.Header().Set("Content-Length", "32")
				w.WriteHeader(http.StatusOK)
				if r.Method != http.MethodHead {
					_, _ = w.Write(body)
				}
			case "/luci-milo-dev-ui-demos/c8509444-p2/ui/immutable/oversized-asset.js":
				w.Header().Set("Content-Type", "text/javascript")
				w.Header().Set("Content-Length", "60000000")
				w.WriteHeader(http.StatusOK)
			case "/luci-milo-dev-ui-demos/c8509444-p2/ui/immutable/storage-error.js":
				w.WriteHeader(http.StatusInternalServerError)
			case "/luci-milo-dev-ui-demos/c8509444-p2/ui/immutable/logo-123.svg":
				w.Header().Set("Content-Type", "image/svg+xml")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("<svg></svg>"))
			case "/luci-milo-dev-ui-demos/c8509444-p2/ui/immutable/redirect-asset.js":
				http.Redirect(w, r, "/attacker-public-bucket/evil.html", http.StatusFound)
			case "/attacker-public-bucket/evil.html":
				redirectFollowed = true
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("<script>alert(1)</script>"))
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

		t.Run("serves GCS /ui/ui_version.js with UI_COMMIT_SHA when present in bundle and strips extraneous statements", func(t *ftt.Test) {
			req := httptest.NewRequest(http.MethodGet, "https://c8509444-dot-luci-milo-dev.appspot.com/ui_version.js", nil)
			req.Host = "c8509444-dot-luci-milo-dev.appspot.com"
			rec := httptest.NewRecorder()
			d.dispatch(&router.Context{Writer: rec, Request: req})

			assert.Loosely(t, rec.Code, should.Equal(http.StatusOK))
			assert.Loosely(t, rec.Body.String(), should.ContainSubstring("self.UI_VERSION = 'c8509444';"))
			assert.Loosely(t, rec.Body.String(), should.ContainSubstring("self.UI_COMMIT_SHA = '6f86e190247751075b5778c4d51d3a7d3eab06f9';"))
			assert.Loosely(t, rec.Body.String(), should.NotContainSubstring("maliciousPayload"))
		})

		t.Run("handles HEAD requests on /ui_version.js and static assets without writing body bytes", func(t *ftt.Test) {
			headVerReq := httptest.NewRequest(http.MethodHead, "https://c8509444-p2-dot-luci-milo-dev.appspot.com/ui_version.js", nil)
			headVerReq.Host = "c8509444-p2-dot-luci-milo-dev.appspot.com"
			headVerRec := httptest.NewRecorder()
			d.dispatch(&router.Context{Writer: headVerRec, Request: headVerReq})

			assert.Loosely(t, headVerRec.Code, should.Equal(http.StatusOK))
			assert.Loosely(t, headVerRec.Header().Get("Content-Type"), should.Equal("text/javascript; charset=utf-8"))
			assert.Loosely(t, headVerRec.Header().Get("Content-Length"), should.NotBeEmpty)
			assert.Loosely(t, headVerRec.Body.Len(), should.BeZero)

			headAssetReq := httptest.NewRequest(http.MethodHead, "https://c8509444-p2-dot-luci-milo-dev.appspot.com/ui/immutable/index-abc.js", nil)
			headAssetReq.Host = "c8509444-p2-dot-luci-milo-dev.appspot.com"
			headAssetRec := httptest.NewRecorder()
			d.dispatch(&router.Context{Writer: headAssetRec, Request: headAssetReq})

			assert.Loosely(t, headAssetRec.Code, should.Equal(http.StatusOK))
			assert.Loosely(t, headAssetRec.Header().Get("Content-Type"), should.Equal("text/javascript; charset=utf-8"))
			assert.Loosely(t, headAssetRec.Header().Get("Content-Length"), should.Equal("32"))
			assert.Loosely(t, headAssetRec.Header().Get("ETag"), should.Equal(`"etag-index-abc"`))
			assert.Loosely(t, headAssetRec.Header().Get("Last-Modified"), should.Equal("Mon, 19 Oct 2026 00:00:00 GMT"))
			assert.Loosely(t, headAssetRec.Body.Len(), should.BeZero)
		})

		t.Run("serves /ui/manifest.webmanifest with sandboxed CSP and rejects oversized or 5xx upstream responses", func(t *ftt.Test) {
			manifestReq := httptest.NewRequest(http.MethodGet, "https://c8509444-p2-dot-luci-milo-dev.appspot.com/ui/manifest.webmanifest", nil)
			manifestReq.Host = "c8509444-p2-dot-luci-milo-dev.appspot.com"
			manifestRec := httptest.NewRecorder()
			d.dispatch(&router.Context{Writer: manifestRec, Request: manifestReq})

			assert.Loosely(t, manifestRec.Code, should.Equal(http.StatusOK))
			assert.Loosely(t, manifestRec.Header().Get("Content-Type"), should.Equal("application/manifest+json; charset=utf-8"))
			assert.Loosely(t, manifestRec.Header().Get("Content-Security-Policy"), should.ContainSubstring("sandbox"))

			oversizedReq := httptest.NewRequest(http.MethodGet, "https://c8509444-p2-dot-luci-milo-dev.appspot.com/ui/immutable/oversized-asset.js", nil)
			oversizedReq.Host = "c8509444-p2-dot-luci-milo-dev.appspot.com"
			oversizedRec := httptest.NewRecorder()
			d.dispatch(&router.Context{Writer: oversizedRec, Request: oversizedReq})
			assert.Loosely(t, oversizedRec.Code, should.Equal(http.StatusBadGateway))
			assert.Loosely(t, oversizedRec.Header().Get("X-Content-Type-Options"), should.Equal("nosniff"))

			errReq := httptest.NewRequest(http.MethodGet, "https://c8509444-p2-dot-luci-milo-dev.appspot.com/ui/immutable/storage-error.js", nil)
			errReq.Host = "c8509444-p2-dot-luci-milo-dev.appspot.com"
			errRec := httptest.NewRecorder()
			d.dispatch(&router.Context{Writer: errRec, Request: errReq})
			assert.Loosely(t, errRec.Code, should.Equal(http.StatusBadGateway))
			assert.Loosely(t, errRec.Header().Get("X-Content-Type-Options"), should.Equal("nosniff"))
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

		t.Run("serves immutable assets with deterministic Content-Type and sandboxes SVG assets", func(t *ftt.Test) {
			req := httptest.NewRequest(http.MethodGet, "https://c8509444-p2-dot-luci-milo-dev.appspot.com/ui/immutable/index-abc.js", nil)
			req.Host = "c8509444-p2-dot-luci-milo-dev.appspot.com"
			rec := httptest.NewRecorder()
			d.dispatch(&router.Context{Writer: rec, Request: req})

			assert.Loosely(t, rec.Code, should.Equal(http.StatusOK))
			assert.Loosely(t, rec.Header().Get("Content-Type"), should.Equal("text/javascript; charset=utf-8"))
			assert.Loosely(t, rec.Header().Get("Cache-Control"), should.ContainSubstring("immutable"))
			assert.Loosely(t, rec.Header().Get("X-Content-Type-Options"), should.Equal("nosniff"))
			assert.Loosely(t, rec.Body.String(), should.ContainSubstring("immutable bundle"))

			svgReq := httptest.NewRequest(http.MethodGet, "https://c8509444-p2-dot-luci-milo-dev.appspot.com/ui/immutable/logo-123.svg", nil)
			svgReq.Host = "c8509444-p2-dot-luci-milo-dev.appspot.com"
			svgRec := httptest.NewRecorder()
			d.dispatch(&router.Context{Writer: svgRec, Request: svgReq})

			assert.Loosely(t, svgRec.Code, should.Equal(http.StatusOK))
			assert.Loosely(t, svgRec.Header().Get("Content-Type"), should.Equal("image/svg+xml"))
			assert.Loosely(t, svgRec.Header().Get("Content-Security-Policy"), should.ContainSubstring("sandbox"))
		})

		t.Run("returns 404 directly without falling through to App Engine when an asset or root_sw.js is missing in an existing GCS bundle", func(t *ftt.Test) {
			for _, missingPath := range []string{"/ui/immutable/missing-chunk.js", "/root_sw.js"} {
				req := httptest.NewRequest(http.MethodGet, "https://c8509444-p2-dot-luci-milo-dev.appspot.com"+missingPath, nil)
				req.Host = "c8509444-p2-dot-luci-milo-dev.appspot.com"
				rec := httptest.NewRecorder()
				d.dispatch(&router.Context{Writer: rec, Request: req})

				assert.Loosely(t, rec.Code, should.Equal(http.StatusNotFound))
			}
		})

		t.Run("rejects percent-encoded, dot-segment, and disallowed-extension static asset paths without calling GCS", func(t *ftt.Test) {
			badTargets := []string{
				"https://c8509444-p2-dot-luci-milo-dev.appspot.com/ui/immutable/..%2f..%2f..%2f..%2f..%2fattacker-public-bucket/evil.html",
				"https://c8509444-p2-dot-luci-milo-dev.appspot.com/ui/immutable/%2e%2e/%2e%2e/attacker-public-bucket/evil.html",
				"https://c8509444-p2-dot-luci-milo-dev.appspot.com/ui/immutable/index-abc.js%3falt=media",
				"https://c8509444-p2-dot-luci-milo-dev.appspot.com/ui/immutable/index-abc.js%23fragment",
				"https://c8509444-p2-dot-luci-milo-dev.appspot.com/ui/immutable/sub%5cfile.js",
				"https://c8509444-p2-dot-luci-milo-dev.appspot.com/ui/immutable//index-abc.js",
				"https://c8509444-p2-dot-luci-milo-dev.appspot.com/ui/immutable/evil.html",
				"https://c8509444-p2-dot-luci-milo-dev.appspot.com/ui/immutable/index%20abc.js",
			}
			for _, target := range badTargets {
				requestedPaths = nil
				req := httptest.NewRequest(http.MethodGet, target, nil)
				req.Host = "c8509444-p2-dot-luci-milo-dev.appspot.com"
				rec := httptest.NewRecorder()
				d.dispatch(&router.Context{Writer: rec, Request: req})

				assert.Loosely(t, rec.Code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, requestedPaths, should.BeEmpty)
			}

			// Also verify unencoded ".." in URL.Path when RawPath is empty.
			requestedPaths = nil
			rawTraversalReq := httptest.NewRequest(http.MethodGet, "https://c8509444-p2-dot-luci-milo-dev.appspot.com/ui/immutable/index-abc.js", nil)
			rawTraversalReq.Host = "c8509444-p2-dot-luci-milo-dev.appspot.com"
			rawTraversalReq.URL.Path = "/ui/immutable/../../../../../attacker-public-bucket/evil.html"
			rawTraversalReq.URL.RawPath = ""
			rec := httptest.NewRecorder()
			d.dispatch(&router.Context{Writer: rec, Request: rawTraversalReq})

			assert.Loosely(t, rec.Code, should.Equal(http.StatusBadRequest))
			assert.Loosely(t, requestedPaths, should.BeEmpty)
		})

		t.Run("does not follow upstream GCS 3xx redirects", func(t *ftt.Test) {
			redirectFollowed = false
			req := httptest.NewRequest(http.MethodGet, "https://c8509444-p2-dot-luci-milo-dev.appspot.com/ui/immutable/redirect-asset.js", nil)
			req.Host = "c8509444-p2-dot-luci-milo-dev.appspot.com"
			rec := httptest.NewRecorder()
			d.dispatch(&router.Context{Writer: rec, Request: req})

			assert.Loosely(t, rec.Code, should.Equal(http.StatusBadGateway))
			assert.Loosely(t, redirectFollowed, should.BeFalse)
			assert.Loosely(t, rec.Body.String(), should.NotContainSubstring("<script>"))
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
