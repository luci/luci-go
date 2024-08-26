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

package sanitizehtml

import (
	"bytes"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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
		t.Run(c.in, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := Sanitize(buf, strings.NewReader(c.in))
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, buf.String(), should.Equal(c.out))
		})
	}
}
