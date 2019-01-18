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

package logs

import (
	"fmt"
	"html/template"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	ct "go.chromium.org/luci/logdog/appengine/coordinator/coordinatorTest"

	. "github.com/smartystreets/goconvey/convey"
)

func TestHTTP(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration`, t, func() {
		c, _ := ct.Install(true)
		c, tc := testclock.UseTime(c, testclock.TestRecentTimeUTC)
		tls := ct.MakeStream(c, "proj-foo", "testing/+/foo/bar")
		So(tls.Put(c), ShouldBeNil)
		resp := httptest.NewRecorder()
		var options userOptions

		fakeData := func(data []logResp) logData {
			ch := make(chan logResp)
			result := logData{
				ch:         ch,
				options:    options,
				logStreams: []*coordinator.LogStream{tls.Stream},
			}
			go func() {
				defer close(ch)
				for _, item := range data {
					ch <- item
				}
			}()
			return result
		}

		Convey(`Match Streams`, func() {
			streams := []*coordinator.LogStream{
				{Name: "foo/bar/baz"},
				{Name: "foo/bar/stdout"},
				{Name: "foo/baz"},
				{Name: "stdout"},
				{Name: "stderr"},
			}
			Convey(`Single wildcard`, func() {
				streams = matchStreams(streams, "*")
				So(streams, ShouldResemble, []*coordinator.LogStream{
					{Name: "stdout"},
					{Name: "stderr"},
				})
			})
			Convey(`tail`, func() {
				streams = matchStreams(streams, "**/baz")
				So(streams, ShouldResemble, []*coordinator.LogStream{
					{Name: "foo/bar/baz"},
					{Name: "foo/baz"},
				})
			})
			Convey(`foo/baz`, func() {
				streams = matchStreams(streams, "foo/*")
				So(streams, ShouldResemble, []*coordinator.LogStream{
					{Name: "foo/baz"},
				})
			})
			Convey(`double wildcard`, func() {
				streams = matchStreams(streams, "foo/**")
				So(streams, ShouldResemble, []*coordinator.LogStream{
					{Name: "foo/bar/baz"},
					{Name: "foo/bar/stdout"},
					{Name: "foo/baz"},
				})
			})
			Convey(`everything`, func() {
				streams = matchStreams(streams, "**")
				So(streams, ShouldResemble, []*coordinator.LogStream{
					{Name: "foo/bar/baz"},
					{Name: "foo/bar/stdout"},
					{Name: "foo/baz"},
					{Name: "stdout"},
					{Name: "stderr"},
				})
			})
		})

		Convey(`Do nothing`, func() {
			data := fakeData([]logResp{})
			So(serve(c, data, resp), ShouldBeNil)
		})

		Convey(`Single Log raw`, func() {
			options.format = "raw"
			data := fakeData([]logResp{
				{stream: tls.Stream, log: tls.LogEntry(c, 0)},
			})
			So(serve(c, data, resp), ShouldBeNil)
			So(resp.Body.String(), ShouldResemble, "log entry #0\n")
		})

		Convey(`Single Log full HTML`, func() {
			options.format = "full"
			l1 := tls.LogEntry(c, 0)
			tc.Add(time.Minute)
			l2 := tls.LogEntry(c, 1)
			data := fakeData([]logResp{
				{stream: tls.Stream, log: l1},
				{stream: tls.Stream, log: l2},
			})
			So(serve(c, data, resp), ShouldBeNil)
			body := resp.Body.String()
			body = strings.Replace(body, "\n", "", -1)
			body = strings.Replace(body, "\t", "", -1)
			So(body, ShouldContainSubstring, `<div class="line" id="L0_0">`)
			So(body, ShouldContainSubstring, `data-timestamp="1454472306000"`)
			So(body, ShouldContainSubstring, `log entry #1`)
		})

		Convey(`Single error, full HTML`, func() {
			options.format = "full"
			msg := "error encountered <script>alert()</script>"
			data := fakeData([]logResp{
				{err: errors.New(msg)},
			})
			So(serve(c, data, resp).Error(), ShouldResemble, msg)
			body := resp.Body.String()
			body = strings.Replace(body, "\n", "", -1)
			body = strings.Replace(body, "\t", "", -1)
			// Note: HTML escapes don't show up in the GoConvey web interface.
			So(body, ShouldEqual, fmt.Sprintf(`<div class="error line">LOGDOG ERROR: %s</div>`, template.HTMLEscapeString(msg)))
		})
	})
}
