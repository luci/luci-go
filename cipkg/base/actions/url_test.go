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
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/cipkg/internal/testutils"

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
	})
}

func TestReexecURL(t *testing.T) {
	ftt.Run("Test re-execute action processor for url", t, func(t *ftt.Test) {
		ap := NewActionProcessor()
		pm := testutils.NewMockPackageManage("")
		ctx := context.Background()
		out := t.TempDir()

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "something")
		}))
		defer s.Close()

		pkg, err := ap.Process("", pm, &core.Action{
			Name: "url",
			Spec: &core.Action_Url{Url: &core.ActionURLFetch{
				Url:           s.URL,
				HashAlgorithm: core.HashAlgorithm_HASH_SHA256,
				HashValue:     "3fc9b689459d738f8c88a3a48aa9e33542016b7a4052e001aaa536fca74813cb",
			}},
		})
		assert.Loosely(t, err, should.BeNil)

		runWithDrv(t, ctx, pkg.Derivation, out)

		{
			f, err := os.Open(filepath.Join(out, "file"))
			assert.Loosely(t, err, should.BeNil)
			defer f.Close()
			b, err := io.ReadAll(f)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(b), should.Equal("something"))
		}
	})
}
