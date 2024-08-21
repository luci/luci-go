// Copyright 2017 The LUCI Authors.
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
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestImpl(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocked service", t, func(c *ftt.Test) {
		ctx, cl := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		cl.SetTimerCallback(func(d time.Duration, t clock.Timer) { cl.Add(d) })

		type call struct {
			Method   string
			Path     string
			Query    url.Values
			Range    string // value of Range request header
			Code     int
			Response any
			Location string // value of Location response header
			NonJSON  bool   // if true, do not put alt=json in the expected URL
		}

		lock := sync.Mutex{}
		expected := []call{}

		expect := func(c call) {
			lock.Lock()
			defer lock.Unlock()
			if c.Query == nil {
				c.Query = url.Values{}
			}
			if c.Query.Get("alt") == "" && !c.NonJSON {
				c.Query.Set("alt", "json")
			}
			if c.Response == nil {
				c.Response = map[string]string{"size": "123"}
			}
			expected = append(expected, c)
		}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			lock.Lock()
			next := call{Method: "???", Path: "???"}
			if len(expected) > 0 {
				next = expected[0]
				expected = expected[1:]
			}
			lock.Unlock()

			q := r.URL.Query()
			q.Del("prettyPrint") // not really relevant to anything

			assert.Loosely(c, r.Method+" "+r.URL.Path, should.Equal(next.Method+" "+next.Path))
			assert.Loosely(c, q, should.Resemble(next.Query))
			assert.Loosely(c, r.Header.Get("Range"), should.Equal(next.Range))

			var response []byte
			var err error

			if next.Response != nil {
				if str, yep := next.Response.(string); yep {
					w.Header().Set("Content-Type", "application/octet-stream")
					response = []byte(str)
				} else {
					w.Header().Set("Content-Type", "application/json")
					response, err = json.Marshal(next.Response)
					assert.Loosely(c, err, should.BeNil)
				}
			}

			if next.Location != "" {
				w.Header().Set("Location", next.Location)
			}
			if next.Code != 0 {
				w.WriteHeader(next.Code)
			}
			if next.Response != nil {
				w.Write(response)
			}
		}))
		defer srv.Close()

		gs := &impl{
			ctx:              ctx,
			testingTransport: http.DefaultTransport,
			testingBasePath:  srv.URL,
		}

		c.Run("Size - exists", func(c *ftt.Test) {
			expect(call{
				Method: "GET",
				Path:   "/b/bucket/o/a/b/c",
			})
			s, yes, err := gs.Size(ctx, "/bucket/a/b/c")
			assert.Loosely(c, err, should.BeNil)
			assert.That(c, s, should.Equal[uint64](123))
			assert.Loosely(c, yes, should.BeTrue)
		})

		c.Run("Size - missing", func(c *ftt.Test) {
			expect(call{
				Method: "GET",
				Path:   "/b/bucket/o/a/b/c",
				Code:   http.StatusNotFound,
			})
			s, yes, err := gs.Size(ctx, "/bucket/a/b/c")
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, s, should.BeZero)
			assert.Loosely(c, yes, should.BeFalse)
		})

		c.Run("Size - error", func(c *ftt.Test) {
			expect(call{
				Method: "GET",
				Path:   "/b/bucket/o/a/b/c",
				Code:   http.StatusForbidden,
			})
			s, yes, err := gs.Size(ctx, "/bucket/a/b/c")
			assert.Loosely(c, StatusCode(err), should.Equal(http.StatusForbidden))
			assert.Loosely(c, s, should.BeZero)
			assert.Loosely(c, yes, should.BeFalse)
		})

		c.Run("Copy unconditional", func(c *ftt.Test) {
			expect(call{
				Method: "POST",
				Path:   "/b/src_bucket/o/src_obj/copyTo/b/dst_bucket/o/dst_obj",
			})
			assert.Loosely(c, gs.Copy(ctx, "/dst_bucket/dst_obj", -1, "/src_bucket/src_obj", -1), should.BeNil)
		})

		c.Run("Copy conditional", func(c *ftt.Test) {
			expect(call{
				Method: "POST",
				Path:   "/b/src_bucket/o/src_obj/copyTo/b/dst_bucket/o/dst_obj",
				Query: url.Values{
					"ifSourceGenerationMatch": {"1"},
					"ifGenerationMatch":       {"2"},
				},
			})
			assert.Loosely(c, gs.Copy(ctx, "/dst_bucket/dst_obj", 2, "/src_bucket/src_obj", 1), should.BeNil)
		})

		c.Run("Copy error", func(c *ftt.Test) {
			expect(call{
				Method: "POST",
				Path:   "/b/src_bucket/o/src_obj/copyTo/b/dst_bucket/o/dst_obj",
				Code:   http.StatusForbidden,
			})
			err := gs.Copy(ctx, "/dst_bucket/dst_obj", -1, "/src_bucket/src_obj", -1)
			assert.Loosely(c, StatusCode(err), should.Equal(http.StatusForbidden))
		})

		c.Run("Delete present", func(c *ftt.Test) {
			expect(call{
				Method: "DELETE",
				Path:   "/b/bucket/o/a/b/c",
			})
			assert.Loosely(c, gs.Delete(ctx, "/bucket/a/b/c"), should.BeNil)
		})

		c.Run("Delete missing", func(c *ftt.Test) {
			expect(call{
				Method: "DELETE",
				Path:   "/b/bucket/o/a/b/c",
				Code:   http.StatusNotFound,
			})
			assert.Loosely(c, gs.Delete(ctx, "/bucket/a/b/c"), should.BeNil)
		})

		c.Run("Delete error", func(c *ftt.Test) {
			expect(call{
				Method: "DELETE",
				Path:   "/b/bucket/o/a/b/c",
				Code:   http.StatusForbidden,
			})
			assert.Loosely(c, StatusCode(gs.Delete(ctx, "/bucket/a/b/c")), should.Equal(http.StatusForbidden))
		})

		c.Run("Publish success", func(c *ftt.Test) {
			expect(call{
				Method: "POST",
				Path:   "/b/src_bucket/o/src_obj/copyTo/b/dst_bucket/o/dst_obj",
				Query: url.Values{
					"ifGenerationMatch":       {"0"},
					"ifSourceGenerationMatch": {"1"},
				},
			})
			assert.Loosely(c, gs.Publish(ctx, "/dst_bucket/dst_obj", "/src_bucket/src_obj", 1), should.BeNil)
		})

		c.Run("Publish bad precondition on srcGen", func(c *ftt.Test) {
			expect(call{
				Method: "POST",
				Path:   "/b/src_bucket/o/src_obj/copyTo/b/dst_bucket/o/dst_obj",
				Query: url.Values{
					"ifGenerationMatch":       {"0"},
					"ifSourceGenerationMatch": {"1"},
				},
				Code: http.StatusPreconditionFailed,
			})
			expect(call{
				Method: "GET",
				Path:   "/b/dst_bucket/o/dst_obj",
				Code:   http.StatusNotFound,
			})
			err := gs.Publish(ctx, "/dst_bucket/dst_obj", "/src_bucket/src_obj", 1)
			assert.Loosely(c, StatusCode(err), should.Equal(http.StatusPreconditionFailed))
			assert.Loosely(c, err, should.ErrLike("unexpected generation number"))
		})

		c.Run("Publish general error", func(c *ftt.Test) {
			expect(call{
				Method: "POST",
				Path:   "/b/src_bucket/o/src_obj/copyTo/b/dst_bucket/o/dst_obj",
				Query: url.Values{
					"ifGenerationMatch":       {"0"},
					"ifSourceGenerationMatch": {"1"},
				},
				Code: http.StatusForbidden,
			})
			err := gs.Publish(ctx, "/dst_bucket/dst_obj", "/src_bucket/src_obj", 1)
			assert.Loosely(c, StatusCode(err), should.Equal(http.StatusForbidden))
		})

		c.Run("Publish missing source object", func(c *ftt.Test) {
			expect(call{
				Method: "POST",
				Path:   "/b/src_bucket/o/src_obj/copyTo/b/dst_bucket/o/dst_obj",
				Query: url.Values{
					"ifGenerationMatch":       {"0"},
					"ifSourceGenerationMatch": {"1"},
				},
				Code: http.StatusNotFound,
			})
			expect(call{
				Method: "GET",
				Path:   "/b/dst_bucket/o/dst_obj",
				Code:   http.StatusNotFound,
			})
			err := gs.Publish(ctx, "/dst_bucket/dst_obj", "/src_bucket/src_obj", 1)
			assert.Loosely(c, StatusCode(err), should.Equal(http.StatusNotFound))
			assert.Loosely(c, err, should.ErrLike("the source object is missing"))
		})

		c.Run("Publish already published (failed precondition on dstGen)", func(c *ftt.Test) {
			expect(call{
				Method: "POST",
				Path:   "/b/src_bucket/o/src_obj/copyTo/b/dst_bucket/o/dst_obj",
				Query: url.Values{
					"ifGenerationMatch":       {"0"},
					"ifSourceGenerationMatch": {"1"},
				},
				Code: http.StatusPreconditionFailed,
			})
			expect(call{
				Method: "GET",
				Path:   "/b/dst_bucket/o/dst_obj",
			})
			assert.Loosely(c, gs.Publish(ctx, "/dst_bucket/dst_obj", "/src_bucket/src_obj", 1), should.BeNil)
		})

		c.Run("Publish already published, only srcDst precondition", func(c *ftt.Test) {
			expect(call{
				Method: "POST",
				Path:   "/b/src_bucket/o/src_obj/copyTo/b/dst_bucket/o/dst_obj",
				Query: url.Values{
					"ifGenerationMatch": {"0"},
				},
				Code: http.StatusPreconditionFailed,
			})
			assert.Loosely(c, gs.Publish(ctx, "/dst_bucket/dst_obj", "/src_bucket/src_obj", -1), should.BeNil)
		})

		c.Run("Publish error when checking presence", func(c *ftt.Test) {
			expect(call{
				Method: "POST",
				Path:   "/b/src_bucket/o/src_obj/copyTo/b/dst_bucket/o/dst_obj",
				Query: url.Values{
					"ifGenerationMatch":       {"0"},
					"ifSourceGenerationMatch": {"1"},
				},
				Code: http.StatusNotFound,
			})
			expect(call{
				Method: "GET",
				Path:   "/b/dst_bucket/o/dst_obj",
				Code:   http.StatusForbidden,
			})
			err := gs.Publish(ctx, "/dst_bucket/dst_obj", "/src_bucket/src_obj", 1)
			assert.Loosely(c, StatusCode(err), should.Equal(http.StatusForbidden))
		})

		c.Run("StartUpload success", func(c *ftt.Test) {
			expect(call{
				Method: "POST",
				Path:   "/upload/storage/v1/b/bucket/o",
				Query: url.Values{
					"name":       {"a/b/c"},
					"uploadType": {"resumable"},
				},
				Location: "http://upload-session.example.com/a/b/c",
			})
			url, err := gs.StartUpload(ctx, "/bucket/a/b/c")
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, url, should.Equal("http://upload-session.example.com/a/b/c"))
		})

		c.Run("StartUpload error", func(c *ftt.Test) {
			expect(call{
				Method: "POST",
				Path:   "/upload/storage/v1/b/bucket/o",
				Query: url.Values{
					"name":       {"a/b/c"},
					"uploadType": {"resumable"},
				},
				Code: http.StatusForbidden,
			})
			url, err := gs.StartUpload(ctx, "/bucket/a/b/c")
			assert.Loosely(c, StatusCode(err), should.Equal(http.StatusForbidden))
			assert.Loosely(c, url, should.BeEmpty)
		})

		c.Run("CancelUpload success", func(c *ftt.Test) {
			expect(call{
				Method:  "DELETE",
				Path:    "/upload_url",
				NonJSON: true,
				Code:    499,
			})
			assert.Loosely(c, gs.CancelUpload(ctx, srv.URL+"/upload_url"), should.BeNil)
		})

		c.Run("CancelUpload error", func(c *ftt.Test) {
			expect(call{
				Method:  "DELETE",
				Path:    "/upload_url",
				NonJSON: true,
				Code:    400,
			})
			assert.Loosely(c, gs.CancelUpload(ctx, srv.URL+"/upload_url"), should.NotBeNil)
		})

		c.Run("Reader works", func(c *ftt.Test) {
			expect(call{
				Method: "GET",
				Path:   "/b/bucket/o/a/b/c",
				Response: map[string]string{
					"generation": "123",
					"size":       "1000",
				},
			})
			r, err := gs.Reader(ctx, "/bucket/a/b/c", 0)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, r, should.NotBeNil)

			assert.Loosely(c, r.Generation(), should.Equal(123))
			assert.Loosely(c, r.Size(), should.Equal(1000))

			// Read from the middle.
			expect(call{
				Method: "GET",
				Path:   "/b/bucket/o/a/b/c",
				Query: url.Values{
					"alt":        {"media"},
					"generation": {"123"},
				},
				Range:    "bytes=100-104",
				Response: "12345",
			})
			buf := make([]byte, 5)
			n, err := r.ReadAt(buf, 100)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, n, should.Equal(5))
			assert.Loosely(c, string(buf), should.Equal("12345"))

			// Read close to the end.
			expect(call{
				Method: "GET",
				Path:   "/b/bucket/o/a/b/c",
				Query: url.Values{
					"alt":        {"media"},
					"generation": {"123"},
				},
				Range:    "bytes=998-999",
				Response: "12",
			})
			buf = make([]byte, 5)
			n, err = r.ReadAt(buf, 998)
			assert.Loosely(c, err, should.Equal(io.EOF))
			assert.Loosely(c, n, should.Equal(2))
			assert.Loosely(c, string(buf), should.Equal("12\x00\x00\x00"))

			// Read past the end.
			n, err = r.ReadAt(buf, 1000)
			assert.Loosely(c, err, should.Equal(io.EOF))
			assert.Loosely(c, n, should.BeZero)
		})

		c.Run("Reader with generation", func(c *ftt.Test) {
			expect(call{
				Method: "GET",
				Path:   "/b/bucket/o/a/b/c",
				Query: url.Values{
					"generation": {"123"},
				},
				Response: map[string]string{
					"generation": "123",
					"size":       "1000",
				},
			})
			r, err := gs.Reader(ctx, "/bucket/a/b/c", 123)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, r, should.NotBeNil)
			assert.Loosely(c, r.Generation(), should.Equal(123))
			assert.Loosely(c, r.Size(), should.Equal(1000))
		})
	})
}
