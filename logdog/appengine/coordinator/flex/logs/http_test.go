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
	"bytes"
	"fmt"
	"html/template"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/logdog/api/logpb"
	ct "go.chromium.org/luci/logdog/appengine/coordinator/coordinatorTest"

	. "github.com/smartystreets/goconvey/convey"
)

func TestHTTP(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration`, t, func() {
		c, env := ct.Install(true)
		c, tc := testclock.UseTime(c, testclock.TestRecentTimeUTC)

		const project = "proj-foo"
		const realm = "some-realm"

		env.AddProject(c, project)
		env.ActAsReader(project, realm)

		tls := ct.MakeStream(c, project, realm, "testing/+/foo/bar")
		So(tls.Put(c), ShouldBeNil)

		resp := httptest.NewRecorder()
		var options userOptions

		fakeData := func(data []logResp) logData {
			ch := make(chan logResp)
			result := logData{
				ch:      ch,
				options: options,
				logDesc: tls.Desc,
			}
			go func() {
				defer close(ch)
				for _, item := range data {
					ch <- item
				}
			}()
			return result
		}

		Convey(`Do nothing`, func() {
			data := fakeData([]logResp{})
			So(serve(c, data, resp), ShouldBeNil)
		})

		Convey(`Single Log raw`, func() {
			options.format = "raw"
			data := fakeData([]logResp{
				{desc: tls.Desc, log: tls.LogEntry(c, 0)},
			})
			So(serve(c, data, resp), ShouldBeNil)
			So(resp.Body.String(), ShouldResemble, "log entry #0\n")
			// Note: It's not helpful to assert HTTP header values here,
			// because this tests serve(), which doesn't set HTTP headers.
		})

		Convey(`Single Log full HTML`, func() {
			options.format = "full"
			l1 := tls.LogEntry(c, 0)
			tc.Add(time.Minute)
			l2 := tls.LogEntry(c, 1)
			data := fakeData([]logResp{
				{desc: tls.Desc, log: l1},
				{desc: tls.Desc, log: l2},
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

func TestHelperFunctions(t *testing.T) {
	t.Parallel()

	Convey(`linkify changes URLs to HTML links`, t, func() {
		Convey(`with nothing that looks like a URL`, func() {
			So(linkify(""), ShouldEqual, "")
			So(linkify(" foo "), ShouldEqual, " foo ")
		})

		Convey(`does normal HTML escaping`, func() {
			So(linkify("<foo>"), ShouldEqual, "&lt;foo&gt;")
		})

		Convey(`with invalid URLs`, func() {
			So(linkify("example.com"), ShouldEqual, "example.com")
			So(linkify("http: //example.com"), ShouldEqual, "http: //example.com")
			So(linkify("ftp://example.com/foo"), ShouldEqual, "ftp://example.com/foo")
			So(linkify("xhttp://example.com/"), ShouldEqual, "xhttp://example.com/")
			So(linkify("http://ex~ample.com"), ShouldEqual, "http://ex~ample.com")
		})

		Convey(`with single simple URLs`, func() {
			So(linkify("http://example.com"), ShouldEqual,
				`<a href="http://example.com">http://example.com</a>`)
			So(linkify("https://example2.com"), ShouldEqual,
				`<a href="https://example2.com">https://example2.com</a>`)
			So(linkify("https://example.com/$foo"), ShouldEqual,
				`<a href="https://example.com/$foo">https://example.com/$foo</a>`)
			So(linkify("https://example.com/5%20%22/#x x"), ShouldEqual,
				`<a href="https://example.com/5%20%22/#x">https://example.com/5%20%22/#x</a> x`)
			So(linkify("https://example.com.:443"), ShouldEqual,
				`<a href="https://example.com.:443">https://example.com.:443</a>`)
		})

		Convey(`with single URLs that have " and & in the path part`, func() {
			So(linkify("https://example.com/\"/x"), ShouldEqual,
				`<a href="https://example.com/%22/x">https://example.com/&#34;/x</a>`)
			So(linkify("https://example.com/&/x"), ShouldEqual,
				`<a href="https://example.com/&amp;/x">https://example.com/&amp;/x</a>`)
		})

		Convey(`with single URLs that have " and <> immediately after the domain`, func() {
			So(linkify(`https://example.com"lol</a>CRAFTED_HTML`), ShouldEqual,
				`<a href="https://example.com">https://example.com</a>&#34;lol&lt;/a&gt;CRAFTED_HTML`)
		})

		Convey(`with single URLs that have " and <> in the path part`, func() {
			So(linkify(`https://example.com/"lol</a>CRAFTED_HTML`), ShouldEqual,

				`<a href="https://example.com/%22lol%3c/a%3eCRAFTED_HTML">https://example.com/&#34;lol&lt;/a&gt;CRAFTED_HTML</a>`)
		})

		Convey(`with multiple URLs`, func() {
			So(linkify("x http://x.com z http://y.com y"), ShouldEqual,
				`x <a href="http://x.com">http://x.com</a> z <a href="http://y.com">http://y.com</a> y`)
			So(linkify("http://x.com http://y.com"), ShouldEqual,
				`<a href="http://x.com">http://x.com</a> <a href="http://y.com">http://y.com</a>`)
		})

		Convey(`lineTemplate uses linkify`, func() {
			lt := logLineStruct{Text: "See https://crbug.com/1167332."}
			w := bytes.NewBuffer([]byte{})
			So(lineTemplate.Execute(w, lt), ShouldBeNil)
			So(w.String(), ShouldContainSubstring,
				`<span class="text">See <a href="https://crbug.com/1167332">https://crbug.com/1167332</a>.</span>`)
		})

	})

	Convey(`contentTypeHeader adjusts based on format and data content type`, t, func() {
		// Using incomplete logData as the headers only depend on metadata, not
		// actual log responses. This is a small unit test only testing one
		// small behavior which is not covered by the larger serve() tests
		// above.

		Convey(`Raw text with default text encoding`, func() {
			So(contentTypeHeader(logData{
				options: userOptions{format: "raw"},
				logDesc: &logpb.LogStreamDescriptor{ContentType: "text/plain; charset=utf-8"},
			}), ShouldEqual, "text/plain; charset=utf-8")
		})

		Convey(`Raw text with alternate text encoding`, func() {
			So(contentTypeHeader(logData{
				options: userOptions{format: "raw"},
				logDesc: &logpb.LogStreamDescriptor{ContentType: "text/plain; charset=iso-unicorns"},
			}), ShouldEqual, "text/plain; charset=iso-unicorns")
		})

		Convey(`HTML with default text encoding`, func() {
			So(contentTypeHeader(logData{
				options: userOptions{format: "full"},
				logDesc: &logpb.LogStreamDescriptor{ContentType: "text/plain; charset=utf-8"},
			}), ShouldEqual, "text/html; charset=utf-8")
		})

		Convey(`HTML with alternate text encoding`, func() {
			So(contentTypeHeader(logData{
				options: userOptions{format: "full"},
				logDesc: &logpb.LogStreamDescriptor{ContentType: "text/plain; charset=iso-unicorns"},
			}), ShouldEqual, "text/html; charset=iso-unicorns")
		})
	})
}
