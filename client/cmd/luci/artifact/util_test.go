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
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestFormatArtifactNames(t *testing.T) {
	t.Parallel()

	ftt.Run(`FormatTestResultArtifactName`, t, func(t *ftt.Test) {
		name := FormatTestResultArtifactName("build-123", "test:foo#bar", "0", "stdout")
		assert.Loosely(t, name, should.Equal("invocations/build-123/tests/test:foo%23bar/results/0/artifacts/stdout"))
	})

	ftt.Run(`FormatWorkUnitArtifactName`, t, func(t *ftt.Test) {
		name := FormatWorkUnitArtifactName("ants-123", "wu-456", "stderr")
		assert.Loosely(t, name, should.Equal("rootInvocations/ants-123/workUnits/wu-456/artifacts/stderr"))
	})
}

func TestFetchHTTPByteRangeEmptyFile(t *testing.T) {
	t.Parallel()

	ftt.Run(`FetchHTTPByteRange on empty file`, t, func(t *ftt.Test) {
		ctx := context.Background()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			serveHTTPRange(w, r, []byte(""))
		}))
		defer server.Close()

		var buf bytes.Buffer
		byteRange := &ByteRange{Start: -1, End: 65536}
		code, size, err := FetchHTTPByteRange(ctx, server.Client(), server.URL, byteRange, &buf)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, code, should.Equal(http.StatusRequestedRangeNotSatisfiable))
		assert.Loosely(t, size, should.Equal(0))
		assert.Loosely(t, buf.Len(), should.Equal(0))
	})
}

// serveHTTPRange is a mock HTTP handler that emulates standard HTTP Range requests.
func serveHTTPRange(w http.ResponseWriter, r *http.Request, content []byte) {
	rangeHdr := r.Header.Get("Range")
	total := int64(len(content))
	if rangeHdr == "" {
		w.WriteHeader(http.StatusOK)
		w.Write(content)
		return
	}

	br, err := ParseByteRange(strings.TrimPrefix(rangeHdr, "bytes="))
	if err != nil || br == nil {
		w.WriteHeader(http.StatusOK)
		w.Write(content)
		return
	}

	var start, end int64
	if br.Start < 0 && br.End >= 0 {
		// Suffix range: bytes=-N
		if br.End >= total {
			start = 0
			end = total - 1
		} else {
			start = total - br.End
			end = total - 1
		}
	} else if br.Start >= 0 && br.End < 0 {
		// bytes=S-
		start = br.Start
		end = total - 1
	} else {
		// bytes=S-E
		start = br.Start
		end = br.End
		if end >= total {
			end = total - 1
		}
	}

	if start < 0 {
		start = 0
	}
	if start > end || start >= total {
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}

	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, total))
	w.WriteHeader(http.StatusPartialContent)
	w.Write(content[start : end+1])
}
