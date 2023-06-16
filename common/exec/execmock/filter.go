// Copyright 2023 The LUCI Authors.
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

package execmock

import (
	"go.chromium.org/luci/common/data/text/sequence"
	"go.chromium.org/luci/common/exec/internal/execmockctx"
)

type unsetMatcher struct{}

var _ sequence.Matcher = unsetMatcher{}

func (unsetMatcher) Matches(txt string) bool { panic("do not call unsetMatcher.Matches") }

type filter struct {
	argFilters []sequence.Pattern
	envFilters map[string][]sequence.Matcher
	limit      uint64

	// literalArgMatchers is a count of the number of LiteralMatcher objects in
	// `argFilters`.
	literalArgMatchers int

	// argMatchers is a count of all the Matcher objects in `argFilters`.
	argMatchers int
}

func (f filter) less(other filter) (less, ok bool) {
	switch {
	case f.literalArgMatchers != other.literalArgMatchers:
		// this is inverted; we want more specific (more literal matchers) to come
		// earlier.
		return f.literalArgMatchers > other.literalArgMatchers, true

	case f.argMatchers != other.argMatchers:
		// this is inverted; we want more specific (more literal matchers) to come
		// earlier.
		return f.argMatchers > other.argMatchers, true

	case f.limit != other.limit:
		switch {
		case f.limit == 0:
			return false, true

		case other.limit == 0:
			return true, true
		}

		return f.limit < other.limit, true

	case len(f.envFilters) != len(other.argFilters):
		return len(f.envFilters) < len(other.envFilters), true
	}

	return false, false
}

func (f filter) matches(mc *execmockctx.MockCriteria) (match bool) {
	for _, pat := range f.argFilters {
		if !pat.In(mc.Args...) {
			return false
		}
	}
	for key, matchers := range f.envFilters {
		for _, mch := range matchers {
			if _, mustBeUnset := mch.(unsetMatcher); mustBeUnset {
				if _, ok := mc.Env.Lookup(key); ok {
					return false
				}
			} else {
				val, ok := mc.Env.Lookup(key)
				if !ok {
					return false
				}
				if !mch.Matches(val) {
					return false
				}
			}
		}
	}
	return true
}

func (f filter) withArgs(argPattern []string) filter {
	seq, err := sequence.NewPattern(argPattern...)
	if err != nil {
		panic(err)
	}

	ret := f

	for _, m := range seq {
		if _, ok := m.(sequence.LiteralMatcher); ok {
			ret.literalArgMatchers++
		}
	}
	ret.argMatchers += len(seq)

	ret.argFilters = make([]sequence.Pattern, len(f.argFilters), len(f.argFilters)+1)
	copy(ret.argFilters, f.argFilters)
	ret.argFilters = append(ret.argFilters, seq)

	return ret
}

func (f filter) withEnv(varName, valuePattern string) filter {
	var pat sequence.Matcher
	if valuePattern == "!" {
		pat = unsetMatcher{}
	} else {
		var err error
		pat, err = sequence.ParseRegLiteral(valuePattern)
		if err != nil {
			panic(err)
		}
	}

	ret := f

	ret.envFilters = make(map[string][]sequence.Matcher, len(f.envFilters)+1)
	ret.envFilters[varName] = append(f.envFilters[varName], pat)

	return ret
}
