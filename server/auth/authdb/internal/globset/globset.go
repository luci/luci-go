// Copyright 2019 The LUCI Authors.
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

// Package globset preprocesses []identity.Glob for faster querying.
package globset

import (
	"regexp"
	"sort"
	"strings"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"
)

// GlobSet encodes same set of identities as some []identity.Glob.
//
// For each possible identity type it has a single regexp against an identity
// value which tests whether it is in the set or not.
//
// Construct it via Builder.
type GlobSet map[identity.Kind]*regexp.Regexp

// Has checks whether 'id' is in the set.
//
// Malformed identities are considered to be not in the set. 'nil' GlobSet is
// considered empty.
func (gs GlobSet) Has(id identity.Identity) bool {
	if gs == nil {
		return false
	}
	sepIdx := strings.IndexRune(string(id), ':')
	if sepIdx == -1 {
		return false
	}
	kind, name := identity.Kind(id[:sepIdx]), string(id[sepIdx+1:])
	if strings.ContainsRune(name, '\n') {
		// Our regexps are not in multi-line mode, reject '\n' explicitly. Note that
		// there should not be '\n' there in the first place, this is just an extra
		// precaution.
		return false
	}
	if re := gs[kind]; re != nil {
		return re.MatchString(name)
	}
	return false
}

// Builder builds GlobSet from individual globs.
//
// Keeps the cache of compiled regexps internally.
type Builder struct {
	kinds   map[identity.Kind]stringset.Set // identity kind => set of matching regexps
	reCache map[string]*regexp.Regexp       // cache of compiled regexps
}

// NewBuilder constructs new Builder instance.
func NewBuilder() *Builder {
	return &Builder{
		kinds:   map[identity.Kind]stringset.Set{},
		reCache: map[string]*regexp.Regexp{},
	}
}

// Reset prepares the builder for building a new GlobSet.
//
// Keeps the cache of compiled regexps.
func (b *Builder) Reset() {
	b.kinds = map[identity.Kind]stringset.Set{}
}

// Add adds a glob to the set being constructed.
//
// Returns an error if the glob is malformed.
func (b *Builder) Add(g identity.Glob) error {
	kind, regexp, err := g.Preprocess()
	if err != nil {
		return err
	}
	ss, ok := b.kinds[kind]
	if !ok {
		ss = stringset.New(1)
		b.kinds[kind] = ss
	}
	ss.Add(regexp)
	return nil
}

// Build returns the fully constructed set or nil if it is empty.
//
// Returns an error if some glob can't be compiled into a regexp.
func (b *Builder) Build() (GlobSet, error) {
	if len(b.kinds) == 0 {
		return nil, nil
	}
	gs := make(GlobSet, len(b.kinds))
	for kind, ss := range b.kinds {
		var err error
		if gs[kind], err = b.toRegexp(ss); err != nil {
			return nil, err
		}
	}
	return gs, nil
}

// toRegexp returns compiled "|".join(sorted(elems)).
func (b *Builder) toRegexp(elems stringset.Set) (*regexp.Regexp, error) {
	// Note that s[i] has form '^...$' (this is also checked by tests).
	s := elems.ToSlice()
	sort.Strings(s)

	uberRe := ""
	switch len(s) {
	case 0:
		panic("impossible")
	case 1:
		uberRe = s[0]
	default:
		sb := strings.Builder{}
		sb.WriteString("^(")
		for i, r := range s {
			if i != 0 {
				sb.WriteRune('|')
			}
			sb.WriteRune('(')
			sb.WriteString(r[1 : len(r)-1])
			sb.WriteRune(')')
		}
		sb.WriteString(")$")
		uberRe = sb.String()
	}

	if re := b.reCache[uberRe]; re != nil {
		return re, nil
	}

	re, err := regexp.Compile(uberRe)
	if err != nil {
		return nil, err
	}

	b.reCache[uberRe] = re
	return re, nil
}
