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
	"crypto/sha1"
	"encoding/hex"
	"io/ioutil"
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

	. "github.com/smartystreets/goconvey/convey"
)

func TestUpload(t *testing.T) {
	ctx := makeTestContext()

	Convey("Upload full flow", t, func(c C) {
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
		So(err, ShouldBeNil)
	})
}

func TestDownload(t *testing.T) {
	ctx := makeTestContext()

	Convey("With temp directory", t, func() {
		tempDir, err := ioutil.TempDir("", "cipd_test")
		So(err, ShouldBeNil)
		defer os.RemoveAll(tempDir)
		tempFile := filepath.Join(tempDir, "pkg")

		Convey("Download full flow", func(c C) {
			out, err := os.OpenFile(tempFile, os.O_RDWR|os.O_CREATE, 0666)
			So(err, ShouldBeNil)
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
			h := sha1.New()
			err = storage.download(ctx, "http://localhost/dwn", out, h)
			So(err, ShouldBeNil)

			out.Seek(0, os.SEEK_SET)
			fetched, err := ioutil.ReadAll(out)
			So(err, ShouldBeNil)
			So(string(fetched), ShouldEqual, "file data")
			So(hex.EncodeToString(h.Sum(nil)), ShouldEqual, "cfb9e9ea5ee050291bc74c7e51fbe578a9f3bd4d")
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

func mockStorageImpl(c C, expectations []expectedHTTPCall) *storageImpl {
	return &storageImpl{
		chunkSize: 5,
		client:    mockClient(c, expectations),
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
func mockClient(c C, expectations []expectedHTTPCall) *http.Client {
	handler := &expectedHTTPCallHandler{c, expectations, 0}
	server := httptest.NewServer(handler)
	Reset(func() {
		server.Close()
		// All expected calls should be made.
		if handler.index != len(handler.calls) {
			c.Printf("Unfinished calls: %v\n", handler.calls[handler.index:])
		}
		c.So(handler.index, ShouldEqual, len(handler.calls))
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
	c     C
	calls []expectedHTTPCall
	index int
}

func (s *expectedHTTPCallHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Unexpected call?
	if s.index == len(s.calls) {
		s.c.Printf("Unexpected call: %v\n", r)
	}
	s.c.So(s.index, ShouldBeLessThan, len(s.calls))

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
	body, err := ioutil.ReadAll(r.Body)
	s.c.So(err, ShouldBeNil)
	blacklist := map[string]bool{
		"Accept-Encoding": true,
		"Content-Length":  true,
		"Content-Type":    true,
		"User-Agent":      true,
	}
	headers := http.Header{}
	for k, v := range r.Header {
		_, isExpected := exp.Headers[k]
		if isExpected || !blacklist[k] {
			headers[k] = v
		}
	}

	// Check that request is what it is expected to be.
	s.c.So(r.Method, ShouldEqual, exp.Method)
	s.c.So(r.URL.Path, ShouldEqual, exp.Path)
	s.c.So(r.URL.Query(), ShouldResemble, exp.Query)
	s.c.So(headers, ShouldResemble, exp.Headers)
	s.c.So(string(body), ShouldEqual, exp.Body)

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
