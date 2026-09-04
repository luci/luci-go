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
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestExtractTailLines(t *testing.T) {
	t.Parallel()

	ftt.Run(`ExtractTailLines`, t, func(t *ftt.Test) {
		data := []byte("prefix chunk\nline 1\nline 2\nline 3\n")

		t.Run(`n <= 0`, func(t *ftt.Test) {
			res, ok := ExtractTailLines(data, 0, false)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, res, should.BeNil)
		})

		t.Run(`exact tail lines from chunk`, func(t *ftt.Test) {
			res, ok := ExtractTailLines(data, 2, false)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, string(res), should.Equal("line 2\nline 3\n"))
		})

		t.Run(`without trailing newline`, func(t *ftt.Test) {
			noTrailing := []byte("prefix\nline 1\nline 2")
			res, ok := ExtractTailLines(noTrailing, 1, false)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, string(res), should.Equal("line 2"))
		})

		t.Run(`not enough lines in suffix chunk`, func(t *ftt.Test) {
			res, ok := ExtractTailLines(data, 10, false)
			assert.Loosely(t, ok, should.BeFalse)
			assert.Loosely(t, res, should.BeNil)
		})

		t.Run(`beginning of file reached with fewer lines`, func(t *ftt.Test) {
			res, ok := ExtractTailLines(data, 10, true)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, string(res), should.Equal(string(data)))
		})

		t.Run(`multibyte unicode at start of chunk`, func(t *ftt.Test) {
			raw := append([]byte{0x80, 0xbf}, []byte("line 1\nline 2\n")...)
			res, ok := ExtractTailLines(raw, 1, false)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, string(res), should.Equal("line 2\n"))

			resBeg, okBeg := ExtractTailLines(raw, 5, true)
			assert.Loosely(t, okBeg, should.BeTrue)
			assert.Loosely(t, string(resBeg), should.Equal(string(raw)))
		})
	})
}

func TestFetchTailLines(t *testing.T) {
	t.Parallel()

	ftt.Run(`FetchTailLines`, t, func(t *ftt.Test) {
		ctx := context.Background()
		fullContent := "line 1: hello\nline 2: world\nline 3: luci\nline 4: resultdb\n"

		t.Run(`server returns 200 OK`, func(t *ftt.Test) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(fullContent))
			}))
			defer server.Close()

			var buf bytes.Buffer
			err := FetchTailLines(ctx, server.Client(), server.URL, 2, int64(len(fullContent)), &buf)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal("line 3: luci\nline 4: resultdb\n"))
		})

		t.Run(`suffix range with 206 Partial Content and totalSizeBytes == 0`, func(t *ftt.Test) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				rangeHdr := r.Header.Get("Range")
				// Suffix request
				if rangeHdr != "" {
					w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", len(fullContent)-1, len(fullContent)))
					w.WriteHeader(http.StatusPartialContent)
					w.Write([]byte(fullContent))
					return
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(fullContent))
			}))
			defer server.Close()

			var buf bytes.Buffer
			// totalSizeBytes is 0 initially (default from CLI)
			err := FetchTailLines(ctx, server.Client(), server.URL, 2, 0, &buf)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal("line 3: luci\nline 4: resultdb\n"))
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
			// Initial chunk size 25 bytes is enough to contain the last 2 lines ("line 4\nline 5\n" is 14 bytes)
			err := FetchTailLinesWithInitialChunkSize(ctx, server.Client(), server.URL, 2, 0, 25, &buf)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal("line 4\nline 5\n"))
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
			// Initial chunk size 25 bytes covers "line 3: cccc\nline 4: dddd\n" (26 bytes).
			// Requesting 3 lines (39 bytes) requires doubling chunk size to 50 bytes on second request.
			err := FetchTailLinesWithInitialChunkSize(ctx, server.Client(), server.URL, 3, 0, 25, &buf)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal("line 2: bbbb\nline 3: cccc\nline 4: dddd\n"))
			assert.Loosely(t, requestCount, should.Equal(2))
		})

		t.Run(`chunk boundary in middle of line`, func(t *ftt.Test) {
			content := "line 1\nline 2: a very long line that spans across the boundary\nline 3\n"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				serveHTTPRange(w, r, []byte(content))
			}))
			defer server.Close()

			var buf bytes.Buffer
			// Initial chunk size 20 bytes splits "very long line"
			err := FetchTailLinesWithInitialChunkSize(ctx, server.Client(), server.URL, 2, 0, 20, &buf)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal("line 2: a very long line that spans across the boundary\nline 3\n"))
		})

		t.Run(`chunk boundary immediately before newline`, func(t *ftt.Test) {
			// Total length: "line 1\nline 2\nline 3\n" = 21 bytes
			// Last 8 bytes is "\nline 3\n" where index 0 of the chunk is '\n' after line 2.
			content := "line 1\nline 2\nline 3\n"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				serveHTTPRange(w, r, []byte(content))
			}))
			defer server.Close()

			var buf bytes.Buffer
			// Initial chunk size 8 bytes starts with '\n'
			err := FetchTailLinesWithInitialChunkSize(ctx, server.Client(), server.URL, 2, 0, 8, &buf)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal("line 2\nline 3\n"))
		})

		t.Run(`chunk boundary immediately after newline`, func(t *ftt.Test) {
			// Total length: "line 1\nline 2\nline 3\n" = 21 bytes
			// Last 7 bytes is "line 3\n" where index 0 is 'l', right after '\n'.
			content := "line 1\nline 2\nline 3\n"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				serveHTTPRange(w, r, []byte(content))
			}))
			defer server.Close()

			var buf bytes.Buffer
			// Request 1 line with initial chunk 7 bytes (which lacks preceding '\n')
			// Needs to expand to confirm line start.
			err := FetchTailLinesWithInitialChunkSize(ctx, server.Client(), server.URL, 1, 0, 7, &buf)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal("line 3\n"))
		})

		t.Run(`chunk boundary in middle of unicode multibyte character`, func(t *ftt.Test) {
			// 🎉 is 4 bytes (0xF0, 0x9F, 0x8E, 0x89), ✓ is 3 bytes (0xE2, 0x9C, 0x93)
			content := "line 1: start\nline 2: hello 🎉 world\nline 3: status ✓ passed\n"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				serveHTTPRange(w, r, []byte(content))
			}))
			defer server.Close()

			var buf bytes.Buffer
			// Request 2 lines with chunk size that splits 🎉
			err := FetchTailLinesWithInitialChunkSize(ctx, server.Client(), server.URL, 2, 0, 30, &buf)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal("line 2: hello 🎉 world\nline 3: status ✓ passed\n"))
		})

		t.Run(`empty file returns empty without error`, func(t *ftt.Test) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				serveHTTPRange(w, r, []byte(""))
			}))
			defer server.Close()

			var buf bytes.Buffer
			err := FetchTailLinesWithInitialChunkSize(ctx, server.Client(), server.URL, 10, 0, 64*1024, &buf)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf.Len(), should.Equal(0))
		})
	})

	ftt.Run(`TailCmd flags`, t, func(t *ftt.Test) {
		cmd := TailCmd(base.NewAuthFlags(), ParentTypeWorkUnit)
		run, ok := cmd.CommandRun().(*artifactTailRun)
		assert.Loosely(t, ok, should.BeTrue)
		assert.Loosely(t, run.Flags.Lookup("o"), should.NotBeNil)
		assert.Loosely(t, run.Flags.Lookup("output"), should.NotBeNil)
	})
}
