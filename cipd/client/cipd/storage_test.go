// Copyright 2015 The LUCI Authors.
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

package cipd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipd/common"
)

func TestUpload(t *testing.T) {
	ctx := makeTestContext()

	ftt.Run("Upload full flow", t, func(c *ftt.Test) {
		storage := mockStorageImpl(c, []expectedHTTPCall{
			{
				Method:  "PUT",
				Path:    "/upl",
				Body:    "01234",
				Headers: http.Header{"Content-Range": []string{"bytes 0-4/13"}},
			},
			{
				Method:  "PUT",
				Path:    "/upl",
				Body:    "56789",
				Headers: http.Header{"Content-Range": []string{"bytes 5-9/13"}},
			},
			// Insert a error.
			{
				Method:  "PUT",
				Path:    "/upl",
				Body:    "abc",
				Headers: http.Header{"Content-Range": []string{"bytes 10-12/13"}},
				Status:  500,
			},
			// Request for uploaded offset #1: failed itself.
			{
				Method:  "PUT",
				Path:    "/upl",
				Body:    "",
				Headers: http.Header{"Content-Range": []string{"bytes */13"}},
				Status:  500,
			},
			// Request for uploaded offset #2: indicates part of data uploaded.
			{
				Method:          "PUT",
				Path:            "/upl",
				Body:            "",
				Headers:         http.Header{"Content-Range": []string{"bytes */13"}},
				Status:          308,
				ResponseHeaders: http.Header{"Range": []string{"bytes=0-7"}},
			},
			// Resume of the upload from returned offset.
			{
				Method:  "PUT",
				Path:    "/upl",
				Body:    "89abc",
				Headers: http.Header{"Content-Range": []string{"bytes 8-12/13"}},
			},
		})
		err := storage.upload(ctx, "http://localhost/upl", bytes.NewReader([]byte("0123456789abc")))
		assert.Loosely(c, err, should.BeNil)
	})
}

func TestDownload(t *testing.T) {
	ctx := makeTestContext()

	ftt.Run("With temp directory", t, func(t *ftt.Test) {
		tempDir := t.TempDir()
		tempFile := filepath.Join(tempDir, "pkg")

		t.Run("Download full flow", func(c *ftt.Test) {
			out, err := os.OpenFile(tempFile, os.O_RDWR|os.O_CREATE, 0666)
			assert.Loosely(c, err, should.BeNil)
			defer out.Close()

			storage := mockStorageImpl(c, []expectedHTTPCall{
				// Simulate a transient error.
				{
					Method: "GET",
					Path:   "/dwn",
					Status: 500,
					Reply:  "error",
				},
				{
					Method: "GET",
					Path:   "/dwn",
					Status: 200,
					Reply:  "file data",
				},
			})
			h := sha256.New()
			err = storage.download(ctx, "http://localhost/dwn", out, h)
			assert.Loosely(c, err, should.BeNil)

			_, _ = out.Seek(0, io.SeekStart)
			fetched, err := io.ReadAll(out)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, string(fetched), should.Equal("file data"))
			assert.Loosely(c, common.HexDigest(h), should.Equal("86f3c70fb6673cf303d2206db5f23c237b665d5df9d3e44efef5114845fc9f59"))
		})
	})
}

////////////////////////////////////////////////////////////////////////////////

func makeTestContext() context.Context {
	ctx, tc := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
	tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
		tc.Add(d)
	})
	return gologger.StdConfig.Use(ctx)
}

func mockStorageImpl(t testing.TB, expectations []expectedHTTPCall) *storageImpl {
	return &storageImpl{
		chunkSize: 5,
		client:    mockClient(t, expectations),
	}
}

type expectedHTTPCall struct {
	Method          string
	Path            string
	Query           url.Values
	Body            string
	Headers         http.Header
	Reply           string
	Status          int
	ResponseHeaders http.Header
}

// mockClient returns http.Client that hits the given mocks.
func mockClient(t testing.TB, expectations []expectedHTTPCall) *http.Client {
	handler := &expectedHTTPCallHandler{t, expectations, 0}
	server := httptest.NewServer(handler)
	t.Cleanup(func() {
		server.Close()
		// All expected calls should be made.
		if handler.index != len(handler.calls) {
			t.Logf("Unfinished calls: %v\n", handler.calls[handler.index:])
		}
		assert.That(t, handler.index, should.Equal(len(handler.calls)))
	})
	return &http.Client{
		Transport: &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				return url.Parse(server.URL)
			},
		},
	}
}

// expectedHTTPCallHandler is http.Handler that serves mocked HTTP calls.
type expectedHTTPCallHandler struct {
	t     testing.TB
	calls []expectedHTTPCall
	index int
}

func (s *expectedHTTPCallHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Unexpected call?
	if s.index == len(s.calls) {
		s.t.Logf("Unexpected call: %v\n", r)
	}
	assert.That(s.t, s.index, should.BeLessThan(len(s.calls)))

	// Fill in defaults.
	exp := s.calls[s.index]
	if exp.Method == "" {
		exp.Method = "GET"
	}
	if exp.Query == nil {
		exp.Query = url.Values{}
	}
	if exp.Headers == nil {
		exp.Headers = http.Header{}
	}

	// Read body and essential headers.
	body, err := io.ReadAll(r.Body)
	assert.NoErr(s.t, err)
	ignorelist := map[string]bool{
		"Accept-Encoding": true,
		"Content-Length":  true,
		"Content-Type":    true,
		"User-Agent":      true,
	}
	headers := http.Header{}
	for k, v := range r.Header {
		_, isExpected := exp.Headers[k]
		if isExpected || !ignorelist[k] {
			headers[k] = v
		}
	}

	// Check that request is what it is expected to be.
	assert.That(s.t, r.Method, should.Equal(exp.Method))
	assert.That(s.t, r.URL.Path, should.Equal(exp.Path))
	assert.That(s.t, r.URL.Query(), should.Match(exp.Query))
	assert.That(s.t, headers, should.Match(exp.Headers))
	assert.That(s.t, string(body), should.Equal(exp.Body))

	// Mocked reply.
	if exp.Status != 0 {
		for k, v := range exp.ResponseHeaders {
			for _, s := range v {
				w.Header().Add(k, s)
			}
		}
		w.WriteHeader(exp.Status)
	}
	if exp.Reply != "" {
		w.Write([]byte(exp.Reply))
	}
	s.index++
}
