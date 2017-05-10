// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package sanitizehtml

import (
	"bytes"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSanitize(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in, out string
	}{
		// Scripts
		{
			`<script src="evil.js"/>`,
			``,
		},

		// Paragraphs
		{
			`<p style="font-size: 100">hi</p>`,
			`<p>hi</p>`,
		},
		{
			`<P>hi</P>`,
			`<p>hi</p>`,
		},
		{
			`a<br>b`,
			`a<br>b`,
		},

		// Lists
		{
			`<ul foo="bar">
				<li x="y">a</li>
				<li>a</li>
			</ul>`,
			`<ul>
				<li>a</li>
				<li>a</li>
			</ul>`,
		},

		// Links
		{
			`<a href="https://ci.chromium.org" alt="x">link</a>`,
			`<a rel="noopener" target="_blank" href="https://ci.chromium.org" alt="x">link</a>`,
		},
		{
			`<a href="javascript:evil.js">link</a>`,
			`<a rel="noopener" target="_blank" href="about:invalid#sanitized&amp;reason=disallowed-scheme">link</a>`,
		},
		{
			`<a href="about:blank">link</a>`,
			`<a rel="noopener" target="_blank" href="about:invalid#sanitized&amp;reason=disallowed-scheme">link</a>`,
		},
		{
			`<a href="%">link</a>`,
			`<a rel="noopener" target="_blank" href="about:invalid#sanitized&amp;reason=malformed-url">link</a>`,
		},
		{
			`<a href="/foo">link</a>`,
			`<a rel="noopener" target="_blank" href="about:invalid#sanitized&amp;reason=disallowed-scheme">link</a>`,
		},
		{
			`<a href="https:///foo">link</a>`,
			`<a rel="noopener" target="_blank" href="about:invalid#sanitized&amp;reason=relative-url">link</a>`,
		},
		{
			`<<a href=abc>`,
			`&lt;<a rel="noopener" target="_blank" href="about:invalid#sanitized&amp;reason=disallowed-scheme"></a>`,
		},

		// Tables
		{
			`<table>
				<tr colspan="2">
					<td rowspan=2>a</td>
				</tr>
				<tr style="">
					<td>b</td>
					<td>c</td>
				</tr>
			</table>`,
			`<table>
				<tr colspan="2">
					<td rowspan="2">a</td>
				</tr>
				<tr>
					<td>b</td>
					<td>c</td>
				</tr>
			</table>`,
		},

		// Other
		{
			`<div><strong>hello</strong></div>`,
			`<strong>hello</strong>`,
		},
		{
			`&lt;`,
			`&lt;`,
		},
		{
			`&foobar;`,
			`&amp;foobar;`,
		},
		{
			`<div><p>foo</p>`,
			`<p>foo</p>`,
		},
		{
			`<p></a alt="blah"></p>`,
			`<p></p>`,
		},
		{
			`<p><a>blah</p></a>`,
			`<p><a rel="noopener" target="_blank">blah</a></p>`,
		},
	}

	for _, c := range cases {
		c := c
		Convey(c.in, t, func() {
			buf := &bytes.Buffer{}
			err := Sanitize(buf, strings.NewReader(c.in))
			So(err, ShouldBeNil)
			So(buf.String(), ShouldEqual, c.out)
		})
	}
}
