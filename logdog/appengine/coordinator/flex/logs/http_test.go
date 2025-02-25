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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/logdog/api/logpb"
	ct "go.chromium.org/luci/logdog/appengine/coordinator/coordinatorTest"
)

func TestHTTP(t *testing.T) {
	t.Parallel()

	ftt.Run(`With a testing configuration`, t, func(t *ftt.Test) {
		c, env := ct.Install()
		c, tc := testclock.UseTime(c, testclock.TestRecentTimeUTC)

		const project = "proj-foo"
		const realm = "some-realm"

		env.AddProject(c, project)
		env.ActAsReader(project, realm)

		tls := ct.MakeStream(c, project, realm, "testing/+/foo/bar")
		assert.Loosely(t, tls.Put(c), should.BeNil)

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

		t.Run(`Do nothing`, func(t *ftt.Test) {
			data := fakeData([]logResp{})
			assert.Loosely(t, serve(c, data, resp), should.BeNil)
		})

		t.Run(`Single Log raw`, func(t *ftt.Test) {
			options.format = "raw"
			data := fakeData([]logResp{
				{desc: tls.Desc, log: tls.LogEntry(c, 0)},
			})
			assert.Loosely(t, serve(c, data, resp), should.BeNil)
			assert.Loosely(t, resp.Body.String(), should.Match("log entry #0\n"))
			// Note: It's not helpful to assert HTTP header values here,
			// because this tests serve(), which doesn't set HTTP headers.
		})

		t.Run(`Single Log full HTML`, func(t *ftt.Test) {
			options.format = "full"
			l1 := tls.LogEntry(c, 0)
			tc.Add(time.Minute)
			l2 := tls.LogEntry(c, 1)
			data := fakeData([]logResp{
				{desc: tls.Desc, log: l1},
				{desc: tls.Desc, log: l2},
			})
			assert.Loosely(t, serve(c, data, resp), should.BeNil)
			body := resp.Body.String()
			body = strings.Replace(body, "\n", "", -1)
			body = strings.Replace(body, "\t", "", -1)
			assert.Loosely(t, body, should.ContainSubstring(`<div class="line" id="L0_0">`))
			assert.Loosely(t, body, should.ContainSubstring(`data-timestamp="1454472306000"`))
			assert.Loosely(t, body, should.ContainSubstring(`log entry #1`))
		})

		t.Run(`Single error, full HTML`, func(t *ftt.Test) {
			options.format = "full"
			msg := "error encountered <script>alert()</script>"
			data := fakeData([]logResp{
				{err: errors.New(msg)},
			})
			assert.Loosely(t, serve(c, data, resp).Error(), should.Match(msg))
			body := resp.Body.String()
			body = strings.Replace(body, "\n", "", -1)
			body = strings.Replace(body, "\t", "", -1)
			// Note: HTML escapes don't show up in the GoConvey web interface.
			assert.Loosely(t, body, should.Equal(fmt.Sprintf(`<div class="error line">LOGDOG ERROR: %s</div>`, template.HTMLEscapeString(msg))))
		})
	})

}

func TestHelperFunctions(t *testing.T) {
	t.Parallel()

	ftt.Run(`linkify changes URLs to HTML links`, t, func(t *ftt.Test) {
		t.Run(`with nothing that looks like a URL`, func(t *ftt.Test) {
			assert.Loosely(t, linkify(""), should.Equal(template.HTML("")))
			assert.Loosely(t, linkify(" foo "), should.Equal(template.HTML(" foo ")))
		})

		t.Run(`does normal HTML escaping`, func(t *ftt.Test) {
			assert.Loosely(t, linkify("<foo>"), should.Equal(template.HTML("&lt;foo&gt;")))
		})

		t.Run(`with invalid URLs`, func(t *ftt.Test) {
			assert.Loosely(t, linkify("example.com"), should.Equal(template.HTML("example.com")))
			assert.Loosely(t, linkify("http: //example.com"), should.Equal(template.HTML("http: //example.com")))
			assert.Loosely(t, linkify("ftp://example.com/foo"), should.Equal(template.HTML("ftp://example.com/foo")))
			assert.Loosely(t, linkify("xhttp://example.com/"), should.Equal(template.HTML("xhttp://example.com/")))
			assert.Loosely(t, linkify("http://ex~ample.com"), should.Equal(template.HTML("http://ex~ample.com")))
		})

		t.Run(`with single simple URLs`, func(t *ftt.Test) {
			assert.Loosely(t, linkify("http://example.com"), should.Equal(
				template.HTML(`<a href="http://example.com">http://example.com</a>`)))
			assert.Loosely(t, linkify("https://example2.com"), should.Equal(
				template.HTML(`<a href="https://example2.com">https://example2.com</a>`)))
			assert.Loosely(t, linkify("https://example.com/$foo"), should.Equal(
				template.HTML(`<a href="https://example.com/$foo">https://example.com/$foo</a>`)))
			assert.Loosely(t, linkify("https://example.com/5%20%22/#x x"), should.Equal(
				template.HTML(`<a href="https://example.com/5%20%22/#x">https://example.com/5%20%22/#x</a> x`)))
			assert.Loosely(t, linkify("https://example.com.:443"), should.Equal(
				template.HTML(`<a href="https://example.com.:443">https://example.com.:443</a>`)))
		})

		t.Run(`with single URLs that have " and & in the path part`, func(t *ftt.Test) {
			assert.Loosely(t, linkify("https://example.com/\"/x"), should.Equal(
				template.HTML(`<a href="https://example.com/%22/x">https://example.com/&#34;/x</a>`)))
			assert.Loosely(t, linkify("https://example.com/&/x"), should.Equal(
				template.HTML(`<a href="https://example.com/&amp;/x">https://example.com/&amp;/x</a>`)))
		})

		t.Run(`with single URLs that have " and <> immediately after the domain`, func(t *ftt.Test) {
			assert.Loosely(t, linkify(`https://example.com"lol</a>CRAFTED_HTML`), should.Equal(
				template.HTML(`<a href="https://example.com">https://example.com</a>&#34;lol&lt;/a&gt;CRAFTED_HTML`)))
		})

		t.Run(`with single URLs that have " and <> in the path part`, func(t *ftt.Test) {
			assert.Loosely(t, linkify(`https://example.com/"lol</a>CRAFTED_HTML`), should.Equal(
				template.HTML(`<a href="https://example.com/%22lol%3c/a%3eCRAFTED_HTML">https://example.com/&#34;lol&lt;/a&gt;CRAFTED_HTML</a>`)))
		})

		t.Run(`with multiple URLs`, func(t *ftt.Test) {
			assert.Loosely(t, linkify("x http://x.com z http://y.com y"), should.Equal(
				template.HTML(`x <a href="http://x.com">http://x.com</a> z <a href="http://y.com">http://y.com</a> y`)))
			assert.Loosely(t, linkify("http://x.com http://y.com"), should.Equal(
				template.HTML(`<a href="http://x.com">http://x.com</a> <a href="http://y.com">http://y.com</a>`)))
		})

		t.Run(`lineTemplate uses linkify`, func(t *ftt.Test) {
			lt := logLineStruct{Text: "See https://crbug.com/1167332."}
			w := bytes.NewBuffer([]byte{})
			assert.Loosely(t, lineTemplate.Execute(w, lt), should.BeNil)
			assert.Loosely(t, w.String(), should.ContainSubstring(
				`<span class="text">See <a href="https://crbug.com/1167332">https://crbug.com/1167332</a>.</span>`))
		})

	})

	ftt.Run(`contentTypeHeader adjusts based on format and data content type`, t, func(t *ftt.Test) {
		// Using incomplete logData as the headers only depend on metadata, not
		// actual log responses. This is a small unit test only testing one
		// small behavior which is not covered by the larger serve() tests
		// above.

		t.Run(`Raw text with default text encoding`, func(t *ftt.Test) {
			assert.Loosely(t, contentTypeHeader(logData{
				options: userOptions{format: "raw"},
				logDesc: &logpb.LogStreamDescriptor{ContentType: "text/plain; charset=utf-8"},
			}), should.Equal("text/plain; charset=utf-8"))
		})

		t.Run(`Raw text with alternate text encoding`, func(t *ftt.Test) {
			assert.Loosely(t, contentTypeHeader(logData{
				options: userOptions{format: "raw"},
				logDesc: &logpb.LogStreamDescriptor{ContentType: "text/plain; charset=iso-unicorns"},
			}), should.Equal("text/plain; charset=iso-unicorns"))
		})

		t.Run(`HTML with default text encoding`, func(t *ftt.Test) {
			assert.Loosely(t, contentTypeHeader(logData{
				options: userOptions{format: "full"},
				logDesc: &logpb.LogStreamDescriptor{ContentType: "text/plain; charset=utf-8"},
			}), should.Equal("text/html; charset=utf-8"))
		})

		t.Run(`HTML with alternate text encoding`, func(t *ftt.Test) {
			assert.Loosely(t, contentTypeHeader(logData{
				options: userOptions{format: "full"},
				logDesc: &logpb.LogStreamDescriptor{ContentType: "text/plain; charset=iso-unicorns"},
			}), should.Equal("text/html; charset=iso-unicorns"))
		})
	})
}
