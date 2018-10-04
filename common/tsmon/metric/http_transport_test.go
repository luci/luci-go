// Copyright 2016 The LUCI Authors.
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

package metric

import (
	"context"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/tsmon"

	. "github.com/smartystreets/goconvey/convey"
)

type fakeRoundTripper struct {
	tc       testclock.TestClock
	duration time.Duration

	resp http.Response
	err  error
}

func (t *fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	t.tc.Add(t.duration)
	io.Copy(ioutil.Discard, req.Body)
	return &t.resp, t.err
}

func TestHTTPRoundTripper(t *testing.T) {
	t.Parallel()

	Convey("With a fake round tripper and dummy state", t, func() {
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestTimeUTC)
		ctx, _ = tsmon.WithDummyInMemory(ctx)

		rt := &fakeRoundTripper{tc: tc}
		c := http.Client{Transport: InstrumentTransport(ctx, rt, "foo")}

		Convey("successful request", func() {
			rt.duration = time.Millisecond * 42
			rt.resp.StatusCode = 200
			rt.resp.ContentLength = 8

			req, _ := http.NewRequest("GET", "https://www.example.com", strings.NewReader("12345"))
			req = req.WithContext(ctx)

			resp, err := c.Do(req)
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, 200)

			v := requestBytesMetric.Get(ctx, "www.example.com", "foo")
			So(v.Count(), ShouldEqual, 1)
			So(v.Sum(), ShouldEqual, 5)
			v = responseBytesMetric.Get(ctx, "www.example.com", "foo")
			So(v.Count(), ShouldEqual, 1)
			So(v.Sum(), ShouldEqual, 8)
			v = requestDurationsMetric.Get(ctx, "www.example.com", "foo")
			So(v.Count(), ShouldEqual, 1)
			So(v.Sum(), ShouldEqual, 42)
			iv := responseStatusMetric.Get(ctx, 200, "www.example.com", "foo")
			So(iv, ShouldEqual, 1)
		})

		Convey("error with no response", func() {
			rt.duration = time.Millisecond * 42
			rt.err = errors.New("oops")

			req, _ := http.NewRequest("GET", "https://www.example.com", strings.NewReader("12345"))
			req = req.WithContext(ctx)

			resp, err := c.Do(req)
			So(err, ShouldNotBeNil)
			So(resp, ShouldBeNil)

			v := requestBytesMetric.Get(ctx, "www.example.com", "foo")
			So(v.Count(), ShouldEqual, 1)
			So(v.Sum(), ShouldEqual, 5)
			v = responseBytesMetric.Get(ctx, "www.example.com", "foo")
			So(v.Count(), ShouldEqual, 1)
			So(v.Sum(), ShouldEqual, 0)
			v = requestDurationsMetric.Get(ctx, "www.example.com", "foo")
			So(v.Count(), ShouldEqual, 1)
			So(v.Sum(), ShouldEqual, 42)
			iv := responseStatusMetric.Get(ctx, 0, "www.example.com", "foo")
			So(iv, ShouldEqual, 1)
		})
	})
}
