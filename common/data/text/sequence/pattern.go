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

// Package sequence implements matching for sequences of strings.
//
// The primary way to use this package is by first making a Pattern:
//
//	pat := NewPattern("^", "/(cat|bear)/", "...", "hello")
//
// Then you can use `pat` to match sequences of strings like:
//
//	pat.In("cat", "says", "hello", "friend") // true
//	pat.In("bear", "hello")                  // true
//	pat.In("dog", "hello")                   // false
//	pat.In("extra", "cat", "hello")          // false
//
// See NewPattern for the types of tokens supported.
//
// You can also manually assemble a Pattern from Matchers (including the special
// Ellipsis and Edge Matchers in this package), but it's anticipated that this
// will be overly verbose for tests (where we expect this package will see the
// most use).
package sequence

import (
	"regexp"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// Pattern is a group of Matchers which can be matched against a string
// sequence.
type Pattern []Matcher

// NewPattern returns a Pattern from a series of tokens.
//
// Tokens can be:
//   - "/a regex/" - A regular expression surrounded by slashes.
//   - "..." - An Ellipsis which matches any number of sequence entries.
//   - "^" at index 0 - Zero-width matches at the beginning of the sequence.
//   - "$" at index -1 - Zero-width matches at the end of the sequence.
//   - "=string" - Literally match anything after the "=". Allows escaping
//     special strings, e.g. "=/regex/", "=...", "=^", "=$", "==something".
//   - "any other string" - Literally match without escaping.
func NewPattern(patternTokens ...string) (Pattern, error) {
	if len(patternTokens) == 0 {
		return nil, nil
	}
	ret := make(Pattern, len(patternTokens))

	prevEllipsis := false
	for i, p := range patternTokens {
		if strings.HasPrefix(p, "=") {
			ret[i] = LiteralMatcher(p[1:])
		} else if strings.HasPrefix(p, "/") && strings.HasSuffix(p, "/") {
			pat, err := regexp.Compile(p[1 : len(p)-1])
			if err != nil {
				return nil, errors.Fmt("invalid regexp (i=%d): %w", i, err)
			}
			ret[i] = RegexpMatcher{pat}
		} else if p == "..." {
			ret[i] = Ellipsis
		} else if p == "^" {
			if i != 0 {
				return nil, errors.Fmt("cannot use `^` for Edge except at beginning (i=%d)", i)
			}
			ret[i] = Edge
		} else if p == "$" {
			if i != len(patternTokens)-1 {
				return nil, errors.Fmt("cannot use `$` for Edge except at end (i=%d)", i)
			}
			ret[i] = Edge
		} else {
			ret[i] = LiteralMatcher(p)
		}
		if ret[i] == Ellipsis {
			if prevEllipsis {
				return nil, errors.Fmt("cannot have multiple Ellipsis in a row (i=%d)", i)
			}
			prevEllipsis = true
		} else {
			prevEllipsis = false
		}
	}

	return ret, nil
}
