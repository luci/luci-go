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
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/cipkg/internal/testutils"

	. "github.com/smartystreets/goconvey/convey"
)

func TestProcessURL(t *testing.T) {
	Convey("Test action processor for url", t, func() {
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
		So(err, ShouldBeNil)

		checkReexecArg(pkg.Derivation.Args, url)
	})
}

func TestExecuteURL(t *testing.T) {
	Convey("Test execute action url", t, func() {
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

		Convey("Test download file", func() {
			a := &core.ActionURLFetch{
				Url: s.URL,
			}

			err := ActionURLFetchExecutor(ctx, a, out)
			So(err, ShouldBeNil)

			{
				f, err := os.Open(filepath.Join(out, "file"))
				So(err, ShouldBeNil)
				defer f.Close()
				b, err := io.ReadAll(f)
				So(err, ShouldBeNil)
				So(string(b), ShouldEqual, "something")
			}
		})

		Convey("Test download file failed", func() {
			a := &core.ActionURLFetch{
				Url: s.URL + "?status=404",
			}

			err := ActionURLFetchExecutor(ctx, a, out)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "404")
		})

		Convey("Test download file with hash verify", func() {
			a := &core.ActionURLFetch{
				Url:           s.URL,
				HashAlgorithm: core.HashAlgorithm_HASH_SHA256,
				HashValue:     "3fc9b689459d738f8c88a3a48aa9e33542016b7a4052e001aaa536fca74813cb",
			}

			err := ActionURLFetchExecutor(ctx, a, out)
			So(err, ShouldBeNil)

			{
				f, err := os.Open(filepath.Join(out, "file"))
				So(err, ShouldBeNil)
				defer f.Close()
				b, err := io.ReadAll(f)
				So(err, ShouldBeNil)
				So(string(b), ShouldEqual, "something")
			}
		})

		Convey("Test download file with hash verify failed", func() {
			a := &core.ActionURLFetch{
				Url:           s.URL,
				HashAlgorithm: core.HashAlgorithm_HASH_SHA256,
				HashValue:     "abcdef",
			}

			err := ActionURLFetchExecutor(ctx, a, out)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "hash mismatch")
		})
	})
}

func TestReexecURL(t *testing.T) {
	Convey("Test re-execute action processor for url", t, func() {
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
		So(err, ShouldBeNil)

		runWithDrv(ctx, pkg.Derivation, out)

		{
			f, err := os.Open(filepath.Join(out, "file"))
			So(err, ShouldBeNil)
			defer f.Close()
			b, err := io.ReadAll(f)
			So(err, ShouldBeNil)
			So(string(b), ShouldEqual, "something")
		}
	})
}
