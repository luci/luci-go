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

func TestParseByteRange(t *testing.T) {
	t.Parallel()

	ftt.Run(`ParseByteRange`, t, func(t *ftt.Test) {
		t.Run(`empty`, func(t *ftt.Test) {
			br, err := ParseByteRange("")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, br, should.BeNil)
		})

		t.Run(`start and end`, func(t *ftt.Test) {
			br, err := ParseByteRange("0-100")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, br, should.Resemble(&ByteRange{Start: 0, End: 100}))

			br2, err := ParseByteRange("10:50")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, br2, should.Resemble(&ByteRange{Start: 10, End: 50}))
		})

		t.Run(`start to EOF`, func(t *ftt.Test) {
			br, err := ParseByteRange("500-")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, br, should.Resemble(&ByteRange{Start: 500, End: -1}))

			br2, err := ParseByteRange("250")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, br2, should.Resemble(&ByteRange{Start: 250, End: -1}))
		})

		t.Run(`suffix range`, func(t *ftt.Test) {
			br, err := ParseByteRange("-500")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, br, should.Resemble(&ByteRange{Start: -1, End: 500}))
		})

		t.Run(`invalid ranges`, func(t *ftt.Test) {
			_, err := ParseByteRange("abc")
			assert.Loosely(t, err, should.NotBeNil)

			_, err = ParseByteRange("100-50")
			assert.Loosely(t, err, should.NotBeNil)

			_, err = ParseByteRange("-0")
			assert.Loosely(t, err, should.NotBeNil)
		})
	})
}

func TestFetchHTTPByteRange(t *testing.T) {
	t.Parallel()

	ftt.Run(`FetchHTTPByteRange`, t, func(t *ftt.Test) {
		ctx := context.Background()
		fullContent := "line 1: hello\nline 2: world\nline 3: luci\nline 4: resultdb\n"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rangeHdr := r.Header.Get("Range")
			if rangeHdr == "bytes=0-14" {
				w.WriteHeader(http.StatusPartialContent)
				w.Write([]byte(fullContent[:15]))
				return
			}
			if rangeHdr == "bytes=15-" {
				w.WriteHeader(http.StatusPartialContent)
				w.Write([]byte(fullContent[15:]))
				return
			}
			if rangeHdr == "bytes=-10" {
				w.WriteHeader(http.StatusPartialContent)
				w.Write([]byte(fullContent[len(fullContent)-10:]))
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(fullContent))
		}))
		defer server.Close()

		t.Run(`full content`, func(t *ftt.Test) {
			var buf bytes.Buffer
			_, _, err := FetchHTTPByteRange(ctx, server.Client(), server.URL, nil, &buf)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal(fullContent))
		})

		t.Run(`server partial content range start-end`, func(t *ftt.Test) {
			var buf bytes.Buffer
			_, _, err := FetchHTTPByteRange(ctx, server.Client(), server.URL, &ByteRange{Start: 0, End: 14}, &buf)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal(fullContent[:15]))
		})

		t.Run(`server partial content range start to EOF`, func(t *ftt.Test) {
			var buf bytes.Buffer
			_, _, err := FetchHTTPByteRange(ctx, server.Client(), server.URL, &ByteRange{Start: 15, End: -1}, &buf)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal(fullContent[15:]))
		})

		t.Run(`server partial content suffix range`, func(t *ftt.Test) {
			var buf bytes.Buffer
			_, _, err := FetchHTTPByteRange(ctx, server.Client(), server.URL, &ByteRange{Start: -1, End: 10}, &buf)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal(fullContent[len(fullContent)-10:]))
		})
	})
}
