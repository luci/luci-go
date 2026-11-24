// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package actions

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"

	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/cipkg/testutils"

	_ "embed"
)

func TestProcessURL(t *testing.T) {
	ftt.Run("Test action processor for url", t, func(t *ftt.Test) {
		ap := NewActionProcessor()
		pm := testutils.NewMockPackageManage("")

		url := &core.ActionURLFetch{
			Url:           "https://host.not.exist/123",
			HashAlgorithm: core.HashAlgorithm_HASH_SHA256,
			HashValue:     "abcdef",
		}

		pkg, err := ap.Process("", pm, &core.Action{
			Name: "url",
			Spec: &core.Action_Url{Url: url},
		})
		assert.Loosely(t, err, should.BeNil)

		checkReexecArg(t, pkg.Derivation.Args, url)

		t.Run("headers influence derivation output", func(t *ftt.Test) {
			emptyHdrPkg, err := ap.Process("", pm, &core.Action{
				Name: "url",
				Spec: &core.Action_Url{Url: &core.ActionURLFetch{
					Url:           "https://host.not.exist/123",
					HashAlgorithm: core.HashAlgorithm_HASH_SHA256,
					HashValue:     "abcdef",
					Headers:       map[string]string{},
				}},
			})
			assert.NoErr(t, err)
			assert.That(t, emptyHdrPkg.DerivationID, should.Equal(pkg.DerivationID))

			withHdrPkg, err := ap.Process("", pm, &core.Action{
				Name: "url",
				Spec: &core.Action_Url{Url: &core.ActionURLFetch{
					Url:           "https://host.not.exist/123",
					HashAlgorithm: core.HashAlgorithm_HASH_SHA256,
					HashValue:     "abcdef",
					Headers: map[string]string{
						"User-Agent": "curl/8.5.0",
						"Referer":    "https://www.amd.com/",
					},
				}},
			})
			assert.NoErr(t, err)
			assert.That(t, withHdrPkg.DerivationID, should.NotEqual(pkg.DerivationID))

			for range 10 {
				sameHdrPkg, err := ap.Process("", pm, &core.Action{
					Name: "url",
					Spec: &core.Action_Url{Url: &core.ActionURLFetch{
						Url:           "https://host.not.exist/123",
						HashAlgorithm: core.HashAlgorithm_HASH_SHA256,
						HashValue:     "abcdef",
						Headers: map[string]string{
							"Referer":    "https://www.amd.com/",
							"User-Agent": "curl/8.5.0",
						},
					}},
				})
				assert.NoErr(t, err)
				assert.That(t, sameHdrPkg.DerivationID, should.Equal(withHdrPkg.DerivationID))
			}
		})

		t.Run("forwards host env without changing DerivationID", func(t *ftt.Test) {
			t.Setenv("http_proxy", "http://127.0.0.1:8080")
			t.Setenv("HTTPS_PROXY", "http://127.0.0.1:8443")

			proxiedPkg, err := ap.Process("", pm, &core.Action{
				Name: "url",
				Spec: &core.Action_Url{Url: url},
			})
			assert.NoErr(t, err)
			assert.That(t, proxiedPkg.DerivationID, should.Equal(pkg.DerivationID))

			drvEnv := environ.New(proxiedPkg.Derivation.Env)
			assert.That(t, drvEnv.Get("http_proxy"), should.Equal("http://127.0.0.1:8080"))
			assert.That(t, drvEnv.Get("HTTPS_PROXY"), should.Equal("http://127.0.0.1:8443"))
			assert.That(t, drvEnv.Get(envCipkgExec), should.Equal("1"))
		})
	})
}

func TestExecuteURL(t *testing.T) {
	ftt.Run("Test execute action url", t, func(t *ftt.Test) {
		ctx := context.Background()
		out := t.TempDir()

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if status := r.URL.Query().Get("status"); status != "" {
				i, err := strconv.ParseInt(status, 0, 0)
				if err != nil {
					panic(err)
				}
				w.WriteHeader(int(i))
			}
			fmt.Fprint(w, "something")
		}))
		defer s.Close()

		t.Run("Test download file", func(t *ftt.Test) {
			a := &core.ActionURLFetch{
				Url: s.URL,
			}

			err := ActionURLFetchExecutor(ctx, a, out)
			assert.Loosely(t, err, should.BeNil)

			{
				f, err := os.Open(filepath.Join(out, "file"))
				assert.Loosely(t, err, should.BeNil)
				defer f.Close()
				b, err := io.ReadAll(f)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, string(b), should.Equal("something"))
			}
		})

		t.Run("Test download file with name and mode", func(t *ftt.Test) {
			a := &core.ActionURLFetch{
				Url:  s.URL,
				Name: "else.txt",
				Mode: 0o644,
			}

			err := ActionURLFetchExecutor(ctx, a, out)
			assert.Loosely(t, err, should.BeNil)

			{
				f, err := os.Open(filepath.Join(out, "else.txt"))
				assert.Loosely(t, err, should.BeNil)
				defer f.Close()
				b, err := io.ReadAll(f)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, string(b), should.Equal("something"))
				if runtime.GOOS != "windows" {
					s, err := f.Stat()
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, s.Mode(), should.Equal(fs.FileMode(0o644)))
				}
			}
		})

		t.Run("Test download file failed", func(t *ftt.Test) {
			a := &core.ActionURLFetch{
				Url: s.URL + "?status=404",
			}

			err := ActionURLFetchExecutor(ctx, a, out)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("404"))
		})

		t.Run("Test download file with hash verify", func(t *ftt.Test) {
			a := &core.ActionURLFetch{
				Url:           s.URL,
				HashAlgorithm: core.HashAlgorithm_HASH_SHA256,
				HashValue:     "3fc9b689459d738f8c88a3a48aa9e33542016b7a4052e001aaa536fca74813cb",
			}

			err := ActionURLFetchExecutor(ctx, a, out)
			assert.Loosely(t, err, should.BeNil)

			{
				f, err := os.Open(filepath.Join(out, "file"))
				assert.Loosely(t, err, should.BeNil)
				defer f.Close()
				b, err := io.ReadAll(f)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, string(b), should.Equal("something"))
			}
		})

		t.Run("Test download file with sha512 verify", func(t *ftt.Test) {
			a := &core.ActionURLFetch{
				Url:           s.URL,
				HashAlgorithm: core.HashAlgorithm_HASH_SHA512,
				HashValue:     "983d43ddff6da90f6a5d3b6172446a1ffe228b803fe64fdd5dcfab5646078a896851fe82f623c9d6e5654b3d2f363a04ec17cfb62b607437a9c7c132d511e522",
			}

			err := ActionURLFetchExecutor(ctx, a, out)
			assert.Loosely(t, err, should.BeNil)

			{
				f, err := os.Open(filepath.Join(out, "file"))
				assert.Loosely(t, err, should.BeNil)
				defer f.Close()
				b, err := io.ReadAll(f)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, string(b), should.Equal("something"))
			}
		})

		t.Run("Test download file with hash verify failed", func(t *ftt.Test) {
			a := &core.ActionURLFetch{
				Url:           s.URL,
				HashAlgorithm: core.HashAlgorithm_HASH_SHA256,
				HashValue:     "abcdef",
			}

			err := ActionURLFetchExecutor(ctx, a, out)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("hash mismatch"))
		})

		t.Run("Test download file with custom headers", func(t *ftt.Test) {
			var gotHeader http.Header
			hdrSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotHeader = r.Header.Clone()
				fmt.Fprint(w, "something")
			}))
			defer hdrSrv.Close()

			a := &core.ActionURLFetch{
				Url: hdrSrv.URL,
				Headers: map[string]string{
					"user-agent": "curl/8.5.0",
					"Referer":    "https://first.example/",
					"referer":    "https://www.amd.com/",
					"Accept":     "application/octet-stream",
				},
			}

			err := ActionURLFetchExecutor(ctx, a, out)
			assert.NoErr(t, err)
			assert.That(t, gotHeader.Values("User-Agent"), should.Match([]string{"curl/8.5.0"}))
			assert.That(t, gotHeader.Values("Referer"), should.Match([]string{"https://www.amd.com/"}))
			assert.That(t, gotHeader.Values("Accept"), should.Match([]string{"application/octet-stream"}))
		})
	})
}

func TestReexecURL(t *testing.T) {
	ftt.Run("Test re-execute action processor for url", t, func(t *ftt.Test) {
		ap := NewActionProcessor()
		pm := testutils.NewMockPackageManage("")
		ctx := context.Background()
		out := t.TempDir()

		t.Run("direct", func(t *ftt.Test) {
			var gotUA string
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotUA = r.Header.Get("User-Agent")
				fmt.Fprint(w, "something")
			}))
			defer s.Close()

			pkg, err := ap.Process("", pm, &core.Action{
				Name: "url",
				Spec: &core.Action_Url{Url: &core.ActionURLFetch{
					Url:           s.URL,
					HashAlgorithm: core.HashAlgorithm_HASH_SHA256,
					HashValue:     "3fc9b689459d738f8c88a3a48aa9e33542016b7a4052e001aaa536fca74813cb",
					Headers: map[string]string{
						"User-Agent": "crowbar-test/1.0",
					},
				}},
			})
			assert.Loosely(t, err, should.BeNil)

			runWithDrv(t, ctx, pkg.Derivation, out)
			assert.That(t, gotUA, should.Equal("crowbar-test/1.0"))

			f, err := os.Open(filepath.Join(out, "file"))
			assert.Loosely(t, err, should.BeNil)
			defer f.Close()
			b, err := io.ReadAll(f)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(b), should.Equal("something"))
		})

		t.Run("via http_proxy", func(t *ftt.Test) {
			var proxyHits atomic.Int32
			proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Host == "proxy-target.invalid" && r.URL.Path == "/file" {
					proxyHits.Add(1)
					fmt.Fprint(w, "something")
					return
				}
				http.Error(w, "unexpected request", http.StatusBadGateway)
			}))
			defer proxySrv.Close()

			clearProxyEnv(t)
			t.Setenv("http_proxy", proxySrv.URL)

			pkg, err := ap.Process("", pm, &core.Action{
				Name: "url",
				Spec: &core.Action_Url{Url: &core.ActionURLFetch{
					Url:           "http://proxy-target.invalid/file",
					HashAlgorithm: core.HashAlgorithm_HASH_SHA256,
					HashValue:     "3fc9b689459d738f8c88a3a48aa9e33542016b7a4052e001aaa536fca74813cb",
				}},
			})
			assert.NoErr(t, err)

			assert.NoErr(t, execDrv(t, ctx, pkg.Derivation, out))
			assert.That(t, proxyHits.Load(), should.Equal(int32(1)))

			b, err := os.ReadFile(filepath.Join(out, "file"))
			assert.NoErr(t, err)
			assert.That(t, string(b), should.Equal("something"))
		})
	})
}
