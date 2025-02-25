// Copyright 2018 The LUCI Authors.
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

package gs

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestUploader(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(c *ftt.Test) {
		ctx, cl := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		cl.SetTimerCallback(func(d time.Duration, t clock.Timer) { cl.Add(d) })

		type call struct {
			ContentRange string // expected Content-Range request header
			Body         []byte // expected request body

			Code  int    // HTTP response code (required)
			Range string // Range response header
			Err   error  // error to return instead of a response
		}

		expected := []call{}
		expect := func(c call) { expected = append(expected, c) }

		requestMock := func(r *http.Request) (*http.Response, error) {
			if len(expected) == 0 {
				t.Fatalf("unexpected call with Content-Range %q", r.Header.Get("Content-Range"))
			}
			next := expected[0]
			expected = expected[1:]

			assert.Loosely(c, r.Method, should.Equal("PUT"))
			assert.Loosely(c, r.URL.Path, should.Equal("/file"))
			assert.Loosely(c, r.Header.Get("Content-Range"), should.Equal(next.ContentRange))

			body, err := io.ReadAll(r.Body)
			assert.Loosely(c, err, should.BeNil)
			if len(body) == 0 {
				body = nil
			}
			assert.Loosely(c, body, should.Match(next.Body))

			if next.Err != nil {
				return nil, next.Err
			}

			resp := httptest.NewRecorder()
			if next.Range != "" {
				resp.Header().Set("Range", next.Range)
			}
			resp.WriteHeader(next.Code)
			return resp.Result(), nil
		}

		upl := Uploader{
			Context:     ctx,
			UploadURL:   "http://example.com/file",
			FileSize:    5,
			requestMock: requestMock,
		}

		c.Run("Happy path", func(c *ftt.Test) {
			expect(call{
				ContentRange: "bytes 0-2/5",
				Body:         []uint8{0, 1, 2},
				Code:         308,
				Range:        "bytes=0-2",
			})
			n, err := upl.Write([]byte{0, 1, 2})
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, n, should.Equal(3))

			expect(call{
				ContentRange: "bytes 3-4/5",
				Body:         []uint8{3, 4},
				Code:         201,
			})
			n, err = upl.Write([]byte{3, 4})
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, n, should.Equal(2))
		})

		c.Run("Restarts successfully", func(c *ftt.Test) {
			expect(call{
				ContentRange: "bytes 0-2/5",
				Body:         []uint8{0, 1, 2},
				Code:         308,
				Range:        "bytes=0-2",
			})
			n, err := upl.Write([]byte{0, 1, 2})
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, n, should.Equal(3))

			// Tries to upload, fails, resumes, fails during the resuming, retries,
			// succeeds.
			expect(call{
				ContentRange: "bytes 3-4/5",
				Body:         []uint8{3, 4},
				Code:         500,
			})
			expect(call{
				ContentRange: "bytes */5",
				Code:         500,
			})
			expect(call{
				ContentRange: "bytes */5",
				Code:         308,
				Range:        "bytes=0-2",
			})
			expect(call{
				ContentRange: "bytes 3-4/5",
				Body:         []uint8{3, 4},
				Code:         201,
			})
			n, err = upl.Write([]byte{3, 4})
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, n, should.Equal(2))
		})

		c.Run("Restarting on first write", func(c *ftt.Test) {
			expect(call{
				ContentRange: "bytes 0-2/5",
				Body:         []uint8{0, 1, 2},
				Code:         500,
			})
			expect(call{
				ContentRange: "bytes */5",
				Code:         308,
				// Note: no Range header here
			})
			expect(call{
				ContentRange: "bytes 0-2/5",
				Body:         []uint8{0, 1, 2},
				Code:         308,
				Range:        "bytes=0-2",
			})
			n, err := upl.Write([]byte{0, 1, 2})
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, n, should.Equal(3))
		})

		c.Run("Restarting with far away offset", func(c *ftt.Test) {
			expect(call{
				ContentRange: "bytes 0-2/5",
				Body:         []uint8{0, 1, 2},
				Code:         308,
				Range:        "bytes=0-2",
			})
			n, err := upl.Write([]byte{0, 1, 2})
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, n, should.Equal(3))

			expect(call{
				ContentRange: "bytes 3-4/5",
				Body:         []uint8{3, 4},
				Code:         500,
			})
			expect(call{
				ContentRange: "bytes */5",
				Code:         308,
				Range:        "bytes=0-1",
			})
			n, err = upl.Write([]byte{3, 4})
			assert.Loosely(c, err, should.Match(&RestartUploadError{Offset: 2}))
			assert.Loosely(c, n, should.BeZero)
		})
	})
}
