// Copyright 2026 The LUCI Authors.
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

package data

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

// TypeInfo is a parsed and ready-to-use version of QueryNodesRequest.TypeInfo.
//
// Produced by [ParseTypeInfo].
type TypeInfo struct {
	Wanted        TypeMatcher
	UnknownJSONPB bool
	Known         TypeMatcher
}

// ParseTypeInfo parses the TypeInfo proto message into a usable [TypeInfo].
func ParseTypeInfo(ti *orchestratorpb.QueryNodesRequest_TypeInfo) (*TypeInfo, error) {
	wanted, err := MakeTypeMatcher(ti.GetWanted())
	if err != nil {
		return nil, fmt.Errorf("wanted: %w", err)
	}
	known, err := MakeTypeMatcher(ti.GetKnown())
	if err != nil {
		return nil, fmt.Errorf("known: %w", err)
	}
	return &TypeInfo{
		wanted,
		ti.GetUnknownJsonpb(),
		known,
	}, nil
}

// TypeMatcher is a compiled version of a TurboCI TypeInfo proto.
//
// A zero-initialized TypeMatcher never matches anything.
type TypeMatcher struct {
	// patterns is a normalized list of patterns, meaning that:
	//   * it never contains a prefix and also other patterns matched by that
	//     prefix.
	//   * it is sorted.
	patterns []string
}

// isMatchingPattern returns true if `pattern` is a prefix pattern (i.e. ends
// with *), and it matches `typeURL`.
func isMatchingPattern(pat, typeURL string) bool {
	last := len(pat) - 1
	return pat[last] == '*' && strings.HasPrefix(typeURL, pat[:last])
}

// Match returns `true` if any pattern in this TypeMatcher matches `typeURL`.
func (t TypeMatcher) Match(typeURL string) bool {
	// Empty matcher never matches anything.
	if len(t.patterns) == 0 {
		return false
	}

	idx, ok := slices.BinarySearch(t.patterns, typeURL)
	if ok {
		// Found an exact match, we are done.
		return true
	}

	// We now need to see if the BinarySearch found a suffix pattern which
	// matches `typeURL`, or if this matcher just does not match `typeURL`.

	// Example:
	//   patterns: [a b.*]
	//   typeURL: b.Foo
	//   idx == 2
	//   -> match

	if idx == 0 {
		// This comes before any patterns in the matcher - therefore it cannot
		// match any pattern in this set.
		return false
	}

	// Look up the pattern before `typeURL` - if it ends with *, see if it shares
	// a prefix.
	return isMatchingPattern(t.patterns[idx-1], typeURL)
}

// MatchValue returns `true` if this TypeMatcher matches the type of the
// given Value.
func (t TypeMatcher) MatchValue(v *orchestratorpb.Value) bool {
	return t.Match(v.GetValue().GetTypeUrl())
}

// MatchDatum returns `true` if this TypeMatcher matches the type of the
// given Datum.
func (t TypeMatcher) MatchDatum(d *orchestratorpb.Datum) bool {
	return t.Match(d.GetValue().GetValue().GetTypeUrl())
}

func validateCheckPattern(typeURL string) error {
	if !strings.HasPrefix(typeURL, TypePrefix) {
		return fmt.Errorf("expected prefix %q (got %q)", TypePrefix, typeURL)
	}

	hasSuffixPattern := strings.HasSuffix(typeURL, ".*") || strings.HasSuffix(typeURL, "/*")
	if hasSuffixPattern {
		// Trim off the *; the remaining pattern must not have ANY stars now.
		typeURL = typeURL[:len(typeURL)-1]
	}

	if strings.Contains(typeURL, "*") {
		if hasSuffixPattern {
			return errors.New("multiple *")
		}
		return errors.New("'*' may only be used in a suffix after a '.' or '/'")
	}

	return nil
}

// MakeTypeMatcher accepts a `TypeSet` and returns a TypeMatcher which can be
// used to match Value and Datum objects by their TypeURL.
func MakeTypeMatcher(ts *orchestratorpb.TypeSet) (TypeMatcher, error) {
	if len(ts.GetTypeUrls()) == 0 {
		return TypeMatcher{}, nil
	}

	var errs []error

	// We add patterns in order.
	urls := slices.Clone(ts.GetTypeUrls())
	slices.Sort(urls)

	ret := TypeMatcher{patterns: make([]string, 0, len(urls))}

	// prev tracks the most recently added pattern; since we are following urls
	// in sorted order, we only need to check this most recently inserted rule
	// does not cover our next pattern.
	var prev string

	for _, typeURL := range urls {
		if err := validateCheckPattern(typeURL); err != nil {
			errs = append(errs, fmt.Errorf("type_set: %w", err))
			continue
		}
		if typeURL == prev || (prev != "" && isMatchingPattern(prev, typeURL)) {
			// We have a rule in ret which already matches this pattern; skip it.
			continue
		}
		ret.patterns = append(ret.patterns, typeURL)
		prev = typeURL
	}

	if len(errs) > 0 {
		return TypeMatcher{}, errors.Join(errs...)
	}
	return ret, nil
}
