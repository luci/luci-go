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
	Title   string
	Href    string
	Package bool
	Last    bool
}

// breadcrumbs builds a data for the breadcrumps navigation element.
//
// It contains a prefix path, where each element is clickable.
func breadcrumbs(path string, ver string, pkg bool) []breadcrumb {
	out := []breadcrumb{
		{Title: "[root]", Href: listingPageURL("", "")},
	}
	if path != "" {
		chunks := strings.Split(path, "/")
		for i, ch := range chunks {
			out = append(out, breadcrumb{
				Title: ch,
				Href:  listingPageURL(strings.Join(chunks[:i+1], "/"), ""),
			})
		}
	}
	out[len(out)-1].Package = pkg
	if ver != "" {
		out = append(out, breadcrumb{
			Title: ver,
			Href:  instancePageURL(path, ver),
		})
	}
	out[len(out)-1].Last = true
	return out
}
