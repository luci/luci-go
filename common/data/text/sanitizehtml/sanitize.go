// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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

// attrValueSanitizer sanitizes an attribute value.
type attrValueSanitizer func(string) string

func alwaysSafe(s string) string {
	return s
}

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

type attrMap map[string]attrValueSanitizer

var (
	anchorAttrs = attrMap{
		"alt":  alwaysSafe,
		"href": sanitizeURL,
	}
	trAttrs = attrMap{
		"rowspan": alwaysSafe,
		"colspan": alwaysSafe,
	}
	tdAttrs = attrMap{
		"rowspan": alwaysSafe,
		"colspan": alwaysSafe,
	}
)

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

// printAttrs sanitizes and prints a whitelist of attributes in el
func (s *sanitizer) printAttrs(el *html.Node, whitelist attrMap) {
	for _, a := range el.Attr {
		key := strings.ToLower(a.Key)
		if sanitizer, ok := whitelist[key]; a.Namespace == "" && ok {
			s.p(" ")
			s.p(key)
			s.p("=\"")
			s.p(html.EscapeString(sanitizer(a.Val)))
			s.p("\"")
		}
	}
}

// printElem prints the safe element with a whitelist of attributes.
// If allowedAttrs is nil, all attributes are omitted.
//
// Do not call for unsafe elements.
func (s *sanitizer) printElem(safeElement *html.Node, allowedAttrs attrMap) {
	tag := safeElement.DataAtom.String()
	s.p("<")
	s.p(tag)
	if allowedAttrs == nil {
		// ignore attributes
	} else {
		s.printAttrs(safeElement, allowedAttrs)
	}
	s.p(">")

	s.visitChildren(safeElement)

	s.p("</")
	s.p(tag)
	s.p(">")
}

func (s *sanitizer) visit(n *html.Node) {
	switch n.Type {
	case html.TextNode:
		// print it escaped.
		s.p(html.EscapeString(n.Data))

	case html.ElementNode:
		// This switch statement defines what HTML elements we allow.
		switch n.DataAtom {
		case atom.Br:
			// br is allowed and it should not be closed
			s.p("<br>")

		case atom.Script, atom.Style:
			// ignore entirely
			// do not visit children so we don't print inner text

		case atom.A:
			s.p(`<a rel="noopener" target="_blank"`)
			s.printAttrs(n, anchorAttrs)
			s.p(">")
			s.visitChildren(n)
			s.p("</a>")

		case atom.P, atom.Ol, atom.Ul, atom.Li, atom.Table, atom.Strong, atom.Em:
			// print without attributes
			s.printElem(n, nil)

		case atom.Tr:
			s.printElem(n, trAttrs)

		case atom.Td:
			s.printElem(n, tdAttrs)

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
//  - p, br
//  - strong, em
//  - a
//    - if href attribute is not a valid absolute HTTP(s) link, it is replaced
//      with an innocuous one.
//    - alt attribute is allowed
//  - ul, ol, li
//  - table
//  - tr, td. Attributes rowspan/colspan are allowed.
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
