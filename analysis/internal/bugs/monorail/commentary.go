// Copyright 2022 The LUCI Authors.
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

package monorail

import "strings"

// Commentary represents part of a bug comment.
type Commentary struct {
	// The comment body. This should be the most important information to surface
	// to the user, and appears first. Do not include leading or trailing new line
	// character.
	Body string
	// Text to appear in the footer of the comment, such as links to more information.
	// This information appears last. Do not include leading or trailing new line
	// character.
	Footer string
}

// MergeCommentary merges one or more commentary items into a bug comment.
// All commentary bodies appear first, followed by all footers.
func MergeCommentary(cs ...Commentary) string {
	var bodies []string
	var footers []string
	for _, c := range cs {
		if c.Body != "" {
			bodies = append(bodies, c.Body)
		}
		if c.Footer != "" {
			footers = append(footers, c.Footer)
		}
	}

	// Footer content is packed together tightly, without blank lines.
	footer := strings.Join(footers, "\n")
	if footer != "" {
		bodies = append(bodies, footer)
	}

	// Bodies (and the final footer) are separated by a blank line.
	return strings.Join(bodies, "\n\n")
}
