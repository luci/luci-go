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

package util

import (
	"fmt"

	resultspb "go.chromium.org/luci/results/proto/v1"
)

// StringPair creates a resultspb.StringPair with the given strings as key/value field values.
func StringPair(k, v string) *resultspb.StringPair {
	return &resultspb.StringPair{Key: k, Value: v}
}

// StringPairs creates a slice of resultspb.StringPair from a list of strings alternating key/value.
//
// Panics if an odd number of tokens is passed.
func StringPairs(pairs ...string) []*resultspb.StringPair {
	if len(pairs)%2 != 0 {
		panic(fmt.Sprintf("odd number of tokens in %q", pairs))
	}

	strpairs := make([]*resultspb.StringPair, len(pairs)/2)
	for i := range strpairs {
		strpairs[i] = StringPair(pairs[2*i], pairs[2*i+1])
	}
	return strpairs
}
