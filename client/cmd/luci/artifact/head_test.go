// Copyright 2026 The LUCI Authors.
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

package artifact

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestExtractHeadLines(t *testing.T) {
	t.Parallel()

	ftt.Run(`ExtractHeadLines`, t, func(t *ftt.Test) {
		data := []byte("line 1\nline 2\nline 3\nline 4\nline 5\n")

		t.Run(`n <= 0`, func(t *ftt.Test) {
			res, ok := ExtractHeadLines(data, 0)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, res, should.BeNil)
		})

		t.Run(`exact lines`, func(t *ftt.Test) {
			res, ok := ExtractHeadLines(data, 2)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, string(res), should.Equal("line 1\nline 2\n"))
		})

		t.Run(`more lines requested than present in buffer`, func(t *ftt.Test) {
			res, ok := ExtractHeadLines(data, 10)
			assert.Loosely(t, ok, should.BeFalse)
			assert.Loosely(t, string(res), should.Equal(string(data)))
		})
	})
}

func TestFetchHeadLines(t *testing.T) {
	t.Parallel()

	ftt.Run(`FetchHeadLines`, t, func(t *ftt.Test) {
		ctx := context.Background()
		fullContent := "line 1: hello\nline 2: world\nline 3: luci\nline 4: resultdb\n"

		t.Run(`server returns 200 OK`, func(t *ftt.Test) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(fullContent))
			}))
			defer server.Close()

			var buf bytes.Buffer
			err := FetchHeadLines(ctx, server.Client(), server.URL, 2, &buf)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal("line 1: hello\nline 2: world\n"))
		})

		t.Run(`multi-chunk where only one chunk is needed`, func(t *ftt.Test) {
			content := "line 1\nline 2\nline 3\nline 4\nline 5\n"
			requestCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount++
				serveHTTPRange(w, r, []byte(content))
			}))
			defer server.Close()

			var buf bytes.Buffer
			// Initial chunk size 25 bytes is enough to contain the first 2 lines ("line 1\nline 2\n" is 14 bytes)
			err := FetchHeadLinesWithInitialChunkSize(ctx, server.Client(), server.URL, 2, 25, &buf)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal("line 1\nline 2\n"))
			assert.Loosely(t, requestCount, should.Equal(1))
		})

		t.Run(`multi-chunk where two chunks are needed`, func(t *ftt.Test) {
			content := "line 1: aaaa\nline 2: bbbb\nline 3: cccc\nline 4: dddd\n"
			requestCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount++
				serveHTTPRange(w, r, []byte(content))
			}))
			defer server.Close()

			var buf bytes.Buffer
			// Initial chunk size 20 bytes only covers "line 1: aaaa\n" (13 bytes) + partial line 2.
			// Requesting 3 lines (39 bytes) requires doubling chunk size to 40 bytes on second request.
			err := FetchHeadLinesWithInitialChunkSize(ctx, server.Client(), server.URL, 3, 20, &buf)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal("line 1: aaaa\nline 2: bbbb\nline 3: cccc\n"))
			assert.Loosely(t, requestCount, should.Equal(2))
		})

		t.Run(`chunk boundary in middle of line`, func(t *ftt.Test) {
			content := "line 1: short\nline 2: a very long line that spans across the boundary\nline 3: end\n"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				serveHTTPRange(w, r, []byte(content))
			}))
			defer server.Close()

			var buf bytes.Buffer
			// Initial chunk size 25 bytes splits "very long line"
			err := FetchHeadLinesWithInitialChunkSize(ctx, server.Client(), server.URL, 2, 25, &buf)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal("line 1: short\nline 2: a very long line that spans across the boundary\n"))
		})

		t.Run(`chunk boundary immediately before newline`, func(t *ftt.Test) {
			// Total length: "line 1\nline 2\nline 3\n" = 21 bytes
			// First 6 bytes is "line 1" (byte 6 is '\n', so chunk of size 6 ends right before '\n').
			content := "line 1\nline 2\nline 3\n"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				serveHTTPRange(w, r, []byte(content))
			}))
			defer server.Close()

			var buf bytes.Buffer
			// Initial chunk size 6 ends immediately before '\n' of line 1
			err := FetchHeadLinesWithInitialChunkSize(ctx, server.Client(), server.URL, 1, 6, &buf)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal("line 1\n"))
		})

		t.Run(`chunk boundary immediately after newline`, func(t *ftt.Test) {
			// Total length: "line 1\nline 2\nline 3\n" = 21 bytes
			// First 7 bytes is "line 1\n" where last byte of chunk 1 is '\n'.
			content := "line 1\nline 2\nline 3\n"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				serveHTTPRange(w, r, []byte(content))
			}))
			defer server.Close()

			var buf bytes.Buffer
			// Request 1 line with chunk size 7 (ending on '\n')
			err := FetchHeadLinesWithInitialChunkSize(ctx, server.Client(), server.URL, 1, 7, &buf)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal("line 1\n"))

			// Request 2 lines with chunk size 7 (needs chunk 2 to get line 2)
			buf.Reset()
			err = FetchHeadLinesWithInitialChunkSize(ctx, server.Client(), server.URL, 2, 7, &buf)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal("line 1\nline 2\n"))
		})

		t.Run(`chunk boundary in middle of unicode multibyte character`, func(t *ftt.Test) {
			// 🎉 is 4 bytes (0xF0, 0x9F, 0x8E, 0x89), ✓ is 3 bytes (0xE2, 0x9C, 0x93)
			content := "line 1: start 🎉 emoji\nline 2: status ✓ passed\nline 3: end\n"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				serveHTTPRange(w, r, []byte(content))
			}))
			defer server.Close()

			var buf bytes.Buffer
			// Request 2 lines with initial chunk 17 bytes that splits 🎉
			err := FetchHeadLinesWithInitialChunkSize(ctx, server.Client(), server.URL, 2, 17, &buf)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal("line 1: start 🎉 emoji\nline 2: status ✓ passed\n"))
		})

		t.Run(`fewer lines than requested from small file reaching EOF`, func(t *ftt.Test) {
			smallContent := "only line 1\nonly line 2\n"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				serveHTTPRange(w, r, []byte(smallContent))
			}))
			defer server.Close()

			var buf bytes.Buffer
			// Request 10 lines from 2-line file
			err := FetchHeadLinesWithInitialChunkSize(ctx, server.Client(), server.URL, 10, 10, &buf)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal(smallContent))
		})
	})
}
