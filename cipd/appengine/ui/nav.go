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

package ui

import (
	"strings"
)

type breadcrumb struct {
	Title string
	Href  string
	Last  bool
}

// breadcrumbs builds a data for the breadcrumps navigation element.
//
// It contains a prefix path, where each element is clickable.
func breadcrumbs(pfx string) []breadcrumb {
	out := []breadcrumb{
		{Title: "[root]", Href: prefixPageURL("")},
	}
	if pfx != "" {
		chunks := strings.Split(pfx, "/")
		for i, ch := range chunks {
			out = append(out, breadcrumb{
				Title: ch,
				Href:  prefixPageURL(strings.Join(chunks[:i+1], "/")),
			})
		}
	}
	out[len(out)-1].Last = true
	return out
}

type listingItem struct {
	Title  string
	Href   string
	Back   bool
	Active bool
}

// listing takes prefix "a/b" and children ["a/b/c", "a/b/d"] and returns
// items titled ["c", "d"].
func pathListing(pfx string, children []string, out []listingItem, cb func(ch string) listingItem) []listingItem {
	if pfx != "" {
		pfx += "/"
	}
	for _, ch := range children {
		itm := cb(ch)
		itm.Title = strings.TrimPrefix(ch, pfx)
		out = append(out, itm)
	}
	return out
}

// prefixesListing formats a list of child prefixes of 'pfx'.
//
// The result includes '..' if 'pfx' is not root.
func prefixesListing(pfx string, prefixes []string) []listingItem {
	out := make([]listingItem, 0, len(prefixes)+1)
	if pfx != "" {
		parent := ""
		if idx := strings.LastIndex(pfx, "/"); idx != -1 {
			parent = pfx[:idx]
		}
		out = append(out, listingItem{
			Back: true,
			Href: prefixPageURL(parent),
		})
	}
	return pathListing(pfx, prefixes, out, func(p string) listingItem {
		return listingItem{
			Href: prefixPageURL(p),
		}
	})
}

// packagesListing formats a list of packages under 'pfx'.
//
// One of them can be hilighted as Active.
func packagesListing(pfx string, pkgs []string, active string) []listingItem {
	out := make([]listingItem, 0, len(pkgs))
	return pathListing(pfx, pkgs, out, func(p string) listingItem {
		return listingItem{
			Href:   packagePageURL(p, ""),
			Active: p == active,
		}
	})
}
