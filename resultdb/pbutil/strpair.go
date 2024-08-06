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
	"strings"

	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/validate"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const maxStringPairKeyLength = 64
const maxStringPairValueLength = 256

const stringPairKeyPattern = `[a-z][a-z0-9_]*(/[a-z][a-z0-9_]*)*`

var stringPairKeyRe = regexpf(`^%s$`, stringPairKeyPattern)
var stringPairRe = regexpf("(?s)^(%s):(.*)$", stringPairKeyPattern)

// StringPair creates a pb.StringPair with the given strings as key/value field values.
func StringPair(k, v string) *pb.StringPair {
	return &pb.StringPair{Key: k, Value: v}
}

// StringPairs creates a slice of pb.StringPair from a list of strings alternating key/value.
//
// Panics if an odd number of tokens is passed.
func StringPairs(pairs ...string) []*pb.StringPair {
	if len(pairs)%2 != 0 {
		panic(fmt.Sprintf("odd number of tokens in %q", pairs))
	}

	strpairs := make([]*pb.StringPair, len(pairs)/2)
	for i := range strpairs {
		strpairs[i] = StringPair(pairs[2*i], pairs[2*i+1])
	}
	return strpairs
}

// StringPairsContain checks if item is present in pairs.
func StringPairsContain(pairs []*pb.StringPair, item *pb.StringPair) bool {
	for _, p := range pairs {
		if p.Key == item.Key && p.Value == item.Value {
			return true
		}
	}
	return false
}

// SortStringPairs sorts in-place the tags slice lexicographically by key, then value.
func SortStringPairs(tags []*pb.StringPair) {
	sort.Slice(tags, func(i, j int) bool {
		if tags[i].Key != tags[j].Key {
			return tags[i].Key < tags[j].Key
		}
		return tags[i].Value < tags[j].Value
	})
}

// ValidateStringPair returns an error if p is invalid.
func ValidateStringPair(p *pb.StringPair) error {
	if err := validate.SpecifiedWithRe(stringPairKeyRe, p.Key); err != nil {
		return errors.Annotate(err, "key").Err()
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
func ValidateStringPairs(pairs []*pb.StringPair) error {
	for _, p := range pairs {
		if err := ValidateStringPair(p); err != nil {
			return errors.Annotate(err, "%q:%q", p.Key, p.Value).Err()
		}
	}
	return nil
}

// StringPairFromString creates a pb.StringPair from the given key:val string.
func StringPairFromString(s string) (*pb.StringPair, error) {
	m := stringPairRe.FindStringSubmatch(s)
	if m == nil {
		return nil, validate.DoesNotMatchReErr(stringPairRe)
	}
	return StringPair(m[1], m[3]), nil
}

// StringPairFromStringUnvalidated is like StringPairFromString, but doesn't
// perform validation.
func StringPairFromStringUnvalidated(s string) *pb.StringPair {
	// TODO: Replace with strings.Cut(s, ":") when we have go 1.18.
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return StringPair(s, "")
	}
	return StringPair(parts[0], parts[1])
}

// StringPairToString converts a StringPair to a key:val string.
func StringPairToString(pair *pb.StringPair) string {
	return fmt.Sprintf("%s:%s", pair.Key, pair.Value)
}

// StringPairsToStrings converts pairs to a slice of "{key}:{value}" strings
// in the same order.
func StringPairsToStrings(pairs ...*pb.StringPair) []string {
	ret := make([]string, len(pairs))
	for i, p := range pairs {
		ret[i] = StringPairToString(p)
	}
	return ret
}

// FromStrpairMap converts a strpair.Map to []*pb.StringPair.
func FromStrpairMap(m strpair.Map) []*pb.StringPair {
	ret := make([]*pb.StringPair, 0, len(m))
	for k, vs := range m {
		for _, v := range vs {
			ret = append(ret, StringPair(k, v))
		}
	}
	SortStringPairs(ret)
	return ret
}
