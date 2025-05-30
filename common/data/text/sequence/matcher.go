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

package sequence

import (
	"regexp"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// Matcher is a single element of a Pattern and can match a single element in
// a sequence.
//
// There are also two 'special' Matchers:
//   - Ellipsis - Unconditionally matches zero or more elements in a sequence.
//   - Edge - Matches the beginning or end of a sequence with zero-width.
type Matcher interface {
	Matches(tok string) bool
}

type ellipsis struct{}

func (ellipsis) Matches(tok string) bool { panic("don't call Ellipsis.Matches") }

type edge struct{}

func (edge) Matches(tok string) bool { panic("don't call Edge.Matches") }

// LiteralMatcher matches a sequence element with exactly this content.
type LiteralMatcher string

// Matches implements Matcher.
func (l LiteralMatcher) Matches(tok string) bool {
	return (string)(l) == tok
}

// RegexpMatcher matches a sequence element with this Regexp.
type RegexpMatcher struct{ R *regexp.Regexp }

// Matches implements Matcher.
func (r RegexpMatcher) Matches(tok string) bool {
	return r.R.MatchString(tok)
}

var (
	// Ellipsis is a special Pattern Matcher which matches any number of tokens of
	// arbitrary length.
	Ellipsis = ellipsis{}

	// Edge is a special 0-width Pattern Matcher which only matches at the
	// beginning or the end of a command.
	Edge = edge{}
)

// ParseRegLiteral parses `token` as either a regex or literal matcher.
//
// If `token` starts and ends with `/` it's contents is parsed as
// a RegexpMatcher, otherwise `token` is returned as a LiteralMatcher.
func ParseRegLiteral(token string) (Matcher, error) {
	if strings.HasPrefix(token, "/") && strings.HasSuffix(token, "/") {
		pat, err := regexp.Compile(token[1 : len(token)-1])
		if err != nil {
			return nil, errors.Fmt("invalid regexp: %w", err)
		}
		return RegexpMatcher{pat}, nil
	}
	return LiteralMatcher(token), nil
}
