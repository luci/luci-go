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

package pbutil

import (
	"fmt"
	"sort"

	"go.chromium.org/luci/common/errors"

	typepb "go.chromium.org/luci/resultdb/proto/type"
)

const maxStringPairKeyLength = 64
const maxStringPairValueLength = 256

const stringPairKeyPattern = `[a-z][a-z0-9_]*(/[a-z][a-z0-9_]*)*`

var stringPairKeyRe = regexpf(`^%s$`, stringPairKeyPattern)
var stringPairRe = regexpf("^(%s):(.*)$", stringPairKeyPattern)

// StringPair creates a typepb.StringPair with the given strings as key/value field values.
func StringPair(k, v string) *typepb.StringPair {
	return &typepb.StringPair{Key: k, Value: v}
}

// StringPairs creates a slice of typepb.StringPair from a list of strings alternating key/value.
//
// Panics if an odd number of tokens is passed.
func StringPairs(pairs ...string) []*typepb.StringPair {
	if len(pairs)%2 != 0 {
		panic(fmt.Sprintf("odd number of tokens in %q", pairs))
	}

	strpairs := make([]*typepb.StringPair, len(pairs)/2)
	for i := range strpairs {
		strpairs[i] = StringPair(pairs[2*i], pairs[2*i+1])
	}
	return strpairs
}

// StringPairsContain checks if item is present in pairs.
func StringPairsContain(pairs []*typepb.StringPair, item *typepb.StringPair) bool {
	for _, p := range pairs {
		if p.Key == item.Key && p.Value == item.Value {
			return true
		}
	}
	return false
}

// sortStringPairs sorts in-place the tags slice lexicographically by key, then value.
func sortStringPairs(tags []*typepb.StringPair) {
	sort.Slice(tags, func(i, j int) bool {
		if tags[i].Key != tags[j].Key {
			return tags[i].Key < tags[j].Key
		}
		return tags[i].Value < tags[j].Value
	})
}

// ValidateStringPair returns an error if p is invalid.
func ValidateStringPair(p *typepb.StringPair) error {
	if !stringPairKeyRe.MatchString(p.Key) {
		return errors.Annotate(doesNotMatch(stringPairKeyRe), "key").Err()
	}
	if len(p.Key) > maxStringPairKeyLength {
		return errors.Reason("key length must be less or equal to %d", maxStringPairKeyLength).Err()
	}
	if len(p.Value) > maxStringPairValueLength {
		return errors.Reason("value length must be less or equal to %d", maxStringPairValueLength).Err()
	}
	return nil
}

// ValidateStringPairs returns an error if any of the pairs is invalid.
func ValidateStringPairs(pairs []*typepb.StringPair) error {
	for _, p := range pairs {
		if err := ValidateStringPair(p); err != nil {
			return errors.Annotate(err, "%q:%q", p.Key, p.Value).Err()
		}
	}
	return nil
}

// StringPairFromString creates a typepb.StringPair from the given key:val string.
func StringPairFromString(s string) (*typepb.StringPair, error) {
	m := stringPairRe.FindStringSubmatch(s)
	if m == nil {
		return nil, doesNotMatch(stringPairRe)
	}
	return StringPair(m[1], m[3]), nil
}

// StringPairToString converts a StringPair to a key:val string.
func StringPairToString(pair *typepb.StringPair) string {
	return fmt.Sprintf("%s:%s", pair.Key, pair.Value)
}

// StringPairsToStrings converts pairs to a slice of "{key}:{value}" strings
// in the same order.
func StringPairsToStrings(pairs ...*typepb.StringPair) []string {
	ret := make([]string, len(pairs))
	for i, p := range pairs {
		ret[i] = StringPairToString(p)
	}
	return ret
}
