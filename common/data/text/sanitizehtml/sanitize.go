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

// Package sanitizehtml implements a sanitizer of a very limited HTML.
// See Sanitize comment.
package sanitizehtml

import (
	"bufio"
	"io"
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func sanitizeURL(s string) string {
	const sanitizedPrefix = "about:invalid#sanitized&reason="
	switch u, err := url.Parse(s); {
	case err != nil:
		return sanitizedPrefix + "malformed-url"

	case u.Scheme != "http" && u.Scheme != "https":
		return sanitizedPrefix + "disallowed-scheme"

	case u.Host == "":
		return sanitizedPrefix + "relative-url"

	default:
		// re-serialize the URL to ensure that what we return is what we think
		// we parsed.
		return u.String()
	}
}

type stringWriter interface {
	WriteString(string) (int, error)
}

type sanitizer struct {
	sw  stringWriter
	err error
}

// p prints the text, unless there was an error before.
func (s *sanitizer) p(safeMarkup string) {
	if s.err == nil {
		_, s.err = s.sw.WriteString(safeMarkup)
	}
}

// printAttr prints a space and then an HTML attribute node.
func (s *sanitizer) printAttr(key, value string) {
	s.p(" ")
	s.p(key)
	s.p("=\"")
	s.p(html.EscapeString(value))
	s.p("\"")
}

func (s *sanitizer) visit(n *html.Node) {
	switch n.Type {
	case html.TextNode:
		// print it escaped.
		s.p(html.EscapeString(n.Data))

	case html.ElementNode:
		// This switch statement defines what HTML elements we allow.
		switch n.DataAtom {
		case atom.Br, atom.Hr:
			// br, hr are allowed and it should not be closed
			tag := n.DataAtom.String()
			s.p("<")
			s.p(tag)
			s.p(">")
		case atom.Script, atom.Style:
			// ignore entirely
			// do not visit children so we don't print inner text

		case atom.A:
			s.p(`<a rel="noopener" target="_blank"`)

			for _, a := range n.Attr {
				if a.Namespace != "" {
					continue
				}
				switch strings.ToLower(a.Key) {
				case "href":
					s.printAttr("href", sanitizeURL(a.Val))

				case "alt":
					s.printAttr("alt", a.Val)
				}
			}

			s.p(">")
			s.visitChildren(n)
			s.p("</a>")
		// TODO: markdown can populate the class attribute
		// if a language is specified in a triple-backtick
		case atom.P, atom.Ol, atom.Ul, atom.Li, atom.Strong,
			atom.Em, atom.Code, atom.Pre, atom.H1, atom.H2,
			atom.H3, atom.H4, atom.H5, atom.H6:
			// print without attributes
			tag := n.DataAtom.String()
			s.p("<")
			s.p(tag)
			s.p(">")

			s.visitChildren(n)

			s.p("</")
			s.p(tag)
			s.p(">")

		default:
			// ignore the element, but visit children.
			s.visitChildren(n)
		}

	default:
		// ignore the node, but visit children.
		s.visitChildren(n)
	}
}

func (s *sanitizer) visitChildren(n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		s.visit(c)
	}
}

// Sanitize strips all HTML nodes except allowed ones.
//
// Unless explicitly specified, attributes are stripped.
// Allowed elements:
//  - p, br, hr
//  - h1, h2, h3, h4, h5, h6
//  - strong, em
//  - a
//    - if href attribute is not a valid absolute HTTP(s) link, it is replaced
//      with an innocuous one.
//    - alt attribute is allowed
//  - ul, ol, li
//  - code, pre
//
// Elements <script> and <style> are ignored entirely.
// For all other HTML nodes, Sanitize ignores the node, but visits its children.
func Sanitize(w io.Writer, r io.Reader) (err error) {
	var root *html.Node
	root, err = html.Parse(r)
	if err != nil {
		return err
	}

	sw, ok := w.(stringWriter)
	if !ok {
		bw := bufio.NewWriter(w)
		defer func() {
			ferr := bw.Flush()
			if err == nil {
				err = ferr
			}
		}()
		sw = bw
	}

	s := sanitizer{sw: sw}
	s.visit(root)
	return s.err
}
