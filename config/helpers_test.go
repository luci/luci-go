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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestHelpers(t *testing.T) {
	t.Parallel()

	ftt.Run("DownloadConfigFromSignedURL", t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run("nil client", func(t *ftt.Test) {
			content, err := DownloadConfigFromSignedURL(ctx, nil, "")
			assert.Loosely(t, content, should.BeNil)
			assert.Loosely(t, err, should.ErrLike("http client is nil"))
		})

		t.Run("empty signed url", func(t *ftt.Test) {
			content, err := DownloadConfigFromSignedURL(ctx, http.DefaultClient, "")
			assert.Loosely(t, content, should.BeNil)
			assert.Loosely(t, err, should.ErrLike("empty signedURL"))
		})

		t.Run("ok - compressed response", func(t *ftt.Test) {
			fakeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Loosely(t, r.Header.Get("Accept-Encoding"), should.Equal("gzip"))
				buf := &bytes.Buffer{}
				gw := gzip.NewWriter(buf)
				_, err := gw.Write([]byte("config content"))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, gw.Close(), should.BeNil)
				w.Header().Set("Content-Encoding", "gzip")
				_, err = w.Write(buf.Bytes())
				assert.Loosely(t, err, should.BeNil)
			}))
			defer fakeSrv.Close()

			content, err := DownloadConfigFromSignedURL(ctx, http.DefaultClient, fakeSrv.URL)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, content, should.Match([]byte("config content")))
		})

		t.Run("ok - uncompressed response", func(t *ftt.Test) {
			fakeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Loosely(t, r.Header.Get("Accept-Encoding"), should.Equal("gzip"))
				fmt.Fprint(w, "config content")
			}))
			defer fakeSrv.Close()

			content, err := DownloadConfigFromSignedURL(ctx, http.DefaultClient, fakeSrv.URL)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, content, should.Match([]byte("config content")))
		})

		t.Run("http error", func(t *ftt.Test) {
			fakeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, "internal error")
			}))
			defer fakeSrv.Close()

			content, err := DownloadConfigFromSignedURL(ctx, http.DefaultClient, fakeSrv.URL)
			assert.Loosely(t, content, should.BeNil)
			assert.Loosely(t, err, should.ErrLike(`failed to download file, got http response code: 500, body: "internal error"`))
		})
	})
}
