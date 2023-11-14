// Copyright 2023 The LUCI Authors.
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

package config

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/klauspost/compress/gzip"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestHelpers(t *testing.T) {
	t.Parallel()

	Convey("DownloadConfigFromSignedURL", t, func() {
		ctx := context.Background()

		Convey("nil client", func() {
			content, err := DownloadConfigFromSignedURL(ctx, nil, "")
			So(content, ShouldBeNil)
			So(err, ShouldErrLike, "http client is nil")
		})

		Convey("empty signed url", func() {
			content, err := DownloadConfigFromSignedURL(ctx, http.DefaultClient, "")
			So(content, ShouldBeNil)
			So(err, ShouldErrLike, "empty signedURL")
		})

		Convey("ok - compressed response", func(c C) {
			fakeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				c.So(r.Header.Get("Accept-Encoding"), ShouldEqual, "gzip")
				buf := &bytes.Buffer{}
				gw := gzip.NewWriter(buf)
				_, err := gw.Write([]byte("config content"))
				c.So(err, ShouldBeNil)
				c.So(gw.Close(), ShouldBeNil)
				w.Header().Set("Content-Encoding", "gzip")
				_, err = w.Write(buf.Bytes())
				c.So(err, ShouldBeNil)
			}))
			defer fakeSrv.Close()

			content, err := DownloadConfigFromSignedURL(ctx, http.DefaultClient, fakeSrv.URL)
			So(err, ShouldBeNil)
			So(content, ShouldEqual, []byte("config content"))
		})

		Convey("ok - uncompressed response", func(c C) {
			fakeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				c.So(r.Header.Get("Accept-Encoding"), ShouldEqual, "gzip")
				fmt.Fprint(w, "config content")
			}))
			defer fakeSrv.Close()

			content, err := DownloadConfigFromSignedURL(ctx, http.DefaultClient, fakeSrv.URL)
			So(err, ShouldBeNil)
			So(content, ShouldEqual, []byte("config content"))
		})

		Convey("http error", func() {
			fakeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, "internal error")
			}))
			defer fakeSrv.Close()

			content, err := DownloadConfigFromSignedURL(ctx, http.DefaultClient, fakeSrv.URL)
			So(content, ShouldBeNil)
			So(err, ShouldErrLike, `failed to download file, got http response code: 500, body: "internal error"`)
		})
	})
}
