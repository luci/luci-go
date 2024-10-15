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
	"net/http"
	"strings"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
)

type fakeRoundTripper struct {
	tc       testclock.TestClock
	duration time.Duration

	resp http.Response
	err  error
}

func (t *fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	t.tc.Add(t.duration)
	io.Copy(io.Discard, req.Body)
	return &t.resp, t.err
}

func TestHTTPRoundTripper(t *testing.T) {
	t.Parallel()

	ftt.Run("With a fake round tripper and dummy state", t, func(t *ftt.Test) {
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestTimeUTC)
		ctx, _ = tsmon.WithDummyInMemory(ctx)

		rt := &fakeRoundTripper{tc: tc}
		c := http.Client{Transport: InstrumentTransport(ctx, rt, "foo")}

		t.Run("successful request", func(t *ftt.Test) {
			rt.duration = time.Millisecond * 42
			rt.resp.StatusCode = 200
			rt.resp.Body = io.NopCloser(strings.NewReader("12345678"))
			rt.resp.ContentLength = 8

			req, _ := http.NewRequest("GET", "https://www.example.com", strings.NewReader("12345"))
			req = req.WithContext(ctx)

			resp, err := c.Do(req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.StatusCode, should.Equal(200))

			v := requestBytesMetric.Get(ctx, "www.example.com", "foo")
			assert.Loosely(t, v.Count(), should.Equal(1))
			assert.That(t, v.Sum(), should.Equal(5.0))
			v = responseBytesMetric.Get(ctx, "www.example.com", "foo")
			assert.Loosely(t, v.Count(), should.Equal(1))
			assert.That(t, v.Sum(), should.Equal(8.0))
			v = requestDurationsMetric.Get(ctx, "www.example.com", "foo")
			assert.Loosely(t, v.Count(), should.Equal(1))
			assert.That(t, v.Sum(), should.Equal(42.0))
			iv := responseStatusMetric.Get(ctx, 200, "www.example.com", "foo")
			assert.Loosely(t, iv, should.Equal(1))
		})

		t.Run("error with no response", func(t *ftt.Test) {
			rt.duration = time.Millisecond * 42
			rt.err = errors.New("oops")

			req, _ := http.NewRequest("GET", "https://www.example.com", strings.NewReader("12345"))
			req = req.WithContext(ctx)

			resp, err := c.Do(req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, resp, should.BeNil)

			v := requestBytesMetric.Get(ctx, "www.example.com", "foo")
			assert.Loosely(t, v.Count(), should.Equal(1))
			assert.That(t, v.Sum(), should.Equal(5.0))
			v = responseBytesMetric.Get(ctx, "www.example.com", "foo")
			assert.Loosely(t, v.Count(), should.Equal(1))
			assert.That(t, v.Sum(), should.Equal(0.0))
			v = requestDurationsMetric.Get(ctx, "www.example.com", "foo")
			assert.Loosely(t, v.Count(), should.Equal(1))
			assert.That(t, v.Sum(), should.Equal(42.0))
			iv := responseStatusMetric.Get(ctx, 0, "www.example.com", "foo")
			assert.Loosely(t, iv, should.Equal(1))
		})
	})
}
