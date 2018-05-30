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
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestImpl(t *testing.T) {
	t.Parallel()

	Convey("With mocked service", t, func(c C) {
		ctx, cl := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		cl.SetTimerCallback(func(d time.Duration, t clock.Timer) { cl.Add(d) })

		type call struct {
			Method   string
			Path     string
			Query    url.Values
			Range    string // value of Range request header
			Code     int
			Response interface{}
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
				c.Response = map[string]string{} // empty JSON dict
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

			c.So(r.Method+" "+r.URL.Path, ShouldEqual, next.Method+" "+next.Path)
			c.So(r.URL.Query(), ShouldResemble, next.Query)
			c.So(r.Header.Get("Range"), ShouldEqual, next.Range)

			var response []byte
			var err error

			if next.Response != nil {
				if str, yep := next.Response.(string); yep {
					w.Header().Set("Content-Type", "application/octet-stream")
					response = []byte(str)
				} else {
					w.Header().Set("Content-Type", "application/json")
					response, err = json.Marshal(next.Response)
					c.So(err, ShouldBeNil)
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
			c:                ctx,
			testingTransport: http.DefaultTransport,
			testingBasePath:  srv.URL,
		}

		Convey("Exists - yes", func() {
			expect(call{
				Method: "GET",
				Path:   "/b/bucket/o/a/b/c",
			})
			yes, err := gs.Exists(ctx, "/bucket/a/b/c")
			So(err, ShouldBeNil)
			So(yes, ShouldBeTrue)
		})

		Convey("Exists - no", func() {
			expect(call{
				Method: "GET",
				Path:   "/b/bucket/o/a/b/c",
				Code:   404,
			})
			yes, err := gs.Exists(ctx, "/bucket/a/b/c")
			So(err, ShouldBeNil)
			So(yes, ShouldBeFalse)
		})

		Convey("Exists - error", func() {
			expect(call{
				Method: "GET",
				Path:   "/b/bucket/o/a/b/c",
				Code:   403,
			})
			yes, err := gs.Exists(ctx, "/bucket/a/b/c")
			So(StatusCode(err), ShouldEqual, 403)
			So(yes, ShouldBeFalse)
		})

		Convey("Copy unconditional", func() {
			expect(call{
				Method: "POST",
				Path:   "/b/src_bucket/o/src_obj/copyTo/b/dst_bucket/o/dst_obj",
			})
			So(gs.Copy(ctx, "/dst_bucket/dst_obj", -1, "/src_bucket/src_obj", -1), ShouldBeNil)
		})

		Convey("Copy conditional", func() {
			expect(call{
				Method: "POST",
				Path:   "/b/src_bucket/o/src_obj/copyTo/b/dst_bucket/o/dst_obj",
				Query: url.Values{
					"ifSourceGenerationMatch": {"1"},
					"ifGenerationMatch":       {"2"},
				},
			})
			So(gs.Copy(ctx, "/dst_bucket/dst_obj", 2, "/src_bucket/src_obj", 1), ShouldBeNil)
		})

		Convey("Copy error", func() {
			expect(call{
				Method: "POST",
				Path:   "/b/src_bucket/o/src_obj/copyTo/b/dst_bucket/o/dst_obj",
				Code:   403,
			})
			err := gs.Copy(ctx, "/dst_bucket/dst_obj", -1, "/src_bucket/src_obj", -1)
			So(StatusCode(err), ShouldEqual, 403)
		})

		Convey("Delete present", func() {
			expect(call{
				Method: "DELETE",
				Path:   "/b/bucket/o/a/b/c",
			})
			So(gs.Delete(ctx, "/bucket/a/b/c"), ShouldBeNil)
		})

		Convey("Delete missing", func() {
			expect(call{
				Method: "DELETE",
				Path:   "/b/bucket/o/a/b/c",
				Code:   404,
			})
			So(gs.Delete(ctx, "/bucket/a/b/c"), ShouldBeNil)
		})

		Convey("Delete error", func() {
			expect(call{
				Method: "DELETE",
				Path:   "/b/bucket/o/a/b/c",
				Code:   403,
			})
			So(StatusCode(gs.Delete(ctx, "/bucket/a/b/c")), ShouldEqual, 403)
		})

		Convey("Publish success", func() {
			expect(call{
				Method: "POST",
				Path:   "/b/src_bucket/o/src_obj/copyTo/b/dst_bucket/o/dst_obj",
				Query: url.Values{
					"ifSourceGenerationMatch": {"1"},
				},
			})
			So(gs.Publish(ctx, "/dst_bucket/dst_obj", "/src_bucket/src_obj", 1), ShouldBeNil)
		})

		Convey("Publish bad precondition", func() {
			expect(call{
				Method: "POST",
				Path:   "/b/src_bucket/o/src_obj/copyTo/b/dst_bucket/o/dst_obj",
				Query: url.Values{
					"ifSourceGenerationMatch": {"1"},
				},
				Code: http.StatusPreconditionFailed,
			})
			err := gs.Publish(ctx, "/dst_bucket/dst_obj", "/src_bucket/src_obj", 1)
			So(StatusCode(err), ShouldEqual, http.StatusPreconditionFailed)
			So(err, ShouldErrLike, "unexpected generation number")
		})

		Convey("Publish general error", func() {
			expect(call{
				Method: "POST",
				Path:   "/b/src_bucket/o/src_obj/copyTo/b/dst_bucket/o/dst_obj",
				Query: url.Values{
					"ifSourceGenerationMatch": {"1"},
				},
				Code: 403,
			})
			err := gs.Publish(ctx, "/dst_bucket/dst_obj", "/src_bucket/src_obj", 1)
			So(StatusCode(err), ShouldEqual, 403)
		})

		Convey("Publish missing object", func() {
			expect(call{
				Method: "POST",
				Path:   "/b/src_bucket/o/src_obj/copyTo/b/dst_bucket/o/dst_obj",
				Query: url.Values{
					"ifSourceGenerationMatch": {"1"},
				},
				Code: 404,
			})
			expect(call{
				Method: "GET",
				Path:   "/b/dst_bucket/o/dst_obj",
				Code:   404,
			})
			err := gs.Publish(ctx, "/dst_bucket/dst_obj", "/src_bucket/src_obj", 1)
			So(StatusCode(err), ShouldEqual, 404)
			So(err, ShouldErrLike, "the source object is missing")
		})

		Convey("Publish already published", func() {
			expect(call{
				Method: "POST",
				Path:   "/b/src_bucket/o/src_obj/copyTo/b/dst_bucket/o/dst_obj",
				Query: url.Values{
					"ifSourceGenerationMatch": {"1"},
				},
				Code: 404,
			})
			expect(call{
				Method: "GET",
				Path:   "/b/dst_bucket/o/dst_obj",
			})
			So(gs.Publish(ctx, "/dst_bucket/dst_obj", "/src_bucket/src_obj", 1), ShouldBeNil)
		})

		Convey("Publish error when checking presence", func() {
			expect(call{
				Method: "POST",
				Path:   "/b/src_bucket/o/src_obj/copyTo/b/dst_bucket/o/dst_obj",
				Query: url.Values{
					"ifSourceGenerationMatch": {"1"},
				},
				Code: 404,
			})
			expect(call{
				Method: "GET",
				Path:   "/b/dst_bucket/o/dst_obj",
				Code:   403,
			})
			err := gs.Publish(ctx, "/dst_bucket/dst_obj", "/src_bucket/src_obj", 1)
			So(StatusCode(err), ShouldEqual, 403)
		})

		Convey("StartUpload success", func() {
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
			So(err, ShouldBeNil)
			So(url, ShouldEqual, "http://upload-session.example.com/a/b/c")
		})

		Convey("StartUpload error", func() {
			expect(call{
				Method: "POST",
				Path:   "/upload/storage/v1/b/bucket/o",
				Query: url.Values{
					"name":       {"a/b/c"},
					"uploadType": {"resumable"},
				},
				Code: 403,
			})
			url, err := gs.StartUpload(ctx, "/bucket/a/b/c")
			So(StatusCode(err), ShouldEqual, 403)
			So(url, ShouldEqual, "")
		})

		Convey("CancelUpload success", func() {
			expect(call{
				Method:  "DELETE",
				Path:    "/upload_url",
				NonJSON: true,
				Code:    499,
			})
			So(gs.CancelUpload(ctx, srv.URL+"/upload_url"), ShouldBeNil)
		})

		Convey("CancelUpload error", func() {
			expect(call{
				Method:  "DELETE",
				Path:    "/upload_url",
				NonJSON: true,
				Code:    400,
			})
			So(gs.CancelUpload(ctx, srv.URL+"/upload_url"), ShouldNotBeNil)
		})

		Convey("Reader works", func() {
			expect(call{
				Method: "GET",
				Path:   "/b/bucket/o/a/b/c",
				Response: map[string]string{
					"generation": "123",
					"size":       "1000",
				},
			})
			r, err := gs.Reader(ctx, "/bucket/a/b/c", 0)
			So(err, ShouldBeNil)
			So(r, ShouldNotBeNil)

			So(r.Generation(), ShouldEqual, 123)
			So(r.Size(), ShouldEqual, 1000)

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
			So(err, ShouldBeNil)
			So(n, ShouldEqual, 5)
			So(string(buf), ShouldEqual, "12345")

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
			So(err, ShouldEqual, io.EOF)
			So(n, ShouldEqual, 2)
			So(string(buf), ShouldEqual, "12\x00\x00\x00")

			// Read past the end.
			n, err = r.ReadAt(buf, 1000)
			So(err, ShouldEqual, io.EOF)
			So(n, ShouldEqual, 0)
		})

		Convey("Reader with generation", func() {
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
			So(err, ShouldBeNil)
			So(r, ShouldNotBeNil)
			So(r.Generation(), ShouldEqual, 123)
			So(r.Size(), ShouldEqual, 1000)
		})
	})
}
