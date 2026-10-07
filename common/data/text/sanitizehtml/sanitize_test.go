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

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

var sanitizeCases = []struct {
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
		`<a rel="noopener" target="_blank" alt="x" href="https://ci.chromium.org">link</a>`,
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

func TestSanitize(t *testing.T) {
	t.Parallel()

	for _, c := range sanitizeCases {
		t.Run(c.in, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := Sanitize(buf, strings.NewReader(c.in))
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, buf.String(), should.Equal(c.out))
		})
	}
}

// FuzzSanitize checks that, whatever the input, an HTML parser reading the
// sanitized output finds only the elements and attributes that Sanitize
// allows, and only http(s) links.
func FuzzSanitize(f *testing.F) {
	for _, c := range sanitizeCases {
		f.Add(c.in)
	}
	f.Fuzz(func(t *testing.T, in string) {
		buf := &bytes.Buffer{}
		if err := Sanitize(buf, strings.NewReader(in)); err != nil {
			return
		}
		out := buf.String()
		// Parse the output as the contents of a <div>, which is how pages
		// embed it.
		nodes, err := html.ParseFragment(strings.NewReader(out), &html.Node{
			Type:     html.ElementNode,
			Data:     "div",
			DataAtom: atom.Div,
		})
		if err != nil {
			t.Fatalf("Sanitize(%q) = %q, which does not parse: %s", in, out, err)
		}
		for _, n := range nodes {
			assertAllowedNode(t, in, out, n)
		}
	})
}

func assertAllowedNode(t *testing.T, in, out string, n *html.Node) {
	t.Helper()
	switch {
	case n.Type == html.TextNode:
	case n.Type != html.ElementNode:
		t.Fatalf("Sanitize(%q) = %q, which parses to a node of type %d", in, out, n.Type)
	case n.Namespace != "" || !allowedElements[n.DataAtom]:
		t.Fatalf("Sanitize(%q) = %q, which contains element <%s>", in, out, n.Data)
	}
	for _, a := range n.Attr {
		if !allowedAttr(n.DataAtom, a) {
			t.Fatalf("Sanitize(%q) = %q, which contains attribute %s=%q on <%s>", in, out, a.Key, a.Val, n.Data)
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		assertAllowedNode(t, in, out, c)
	}
}

// allowedElements are the elements that Sanitize emits.
var allowedElements = map[atom.Atom]bool{
	atom.A:      true,
	atom.Br:     true,
	atom.Code:   true,
	atom.Em:     true,
	atom.H1:     true,
	atom.H2:     true,
	atom.H3:     true,
	atom.H4:     true,
	atom.H5:     true,
	atom.H6:     true,
	atom.Hr:     true,
	atom.Li:     true,
	atom.Ol:     true,
	atom.P:      true,
	atom.Pre:    true,
	atom.Strong: true,
	atom.Ul:     true,
}

// allowedAttr reports whether Sanitize may emit attribute a on element e.
func allowedAttr(e atom.Atom, a html.Attribute) bool {
	if e != atom.A || a.Namespace != "" {
		return false
	}
	switch a.Key {
	case "rel":
		return a.Val == "noopener"
	case "target":
		return a.Val == "_blank"
	case "alt":
		return true
	case "href":
		return strings.HasPrefix(a.Val, "http://") ||
			strings.HasPrefix(a.Val, "https://") ||
			strings.HasPrefix(a.Val, "about:invalid#sanitized&reason=")
	}
	return false
}
