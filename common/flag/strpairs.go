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

package flag

import (
	"flag"
	"fmt"
	"strings"

	"go.chromium.org/luci/common/data/strpair"
)

// strpairsFlag implements the flag.Value returned by StrPairs.
type stringPairsFlag strpair.Map

// StrPairs returns a flag.Value for parsing strpair.Map from a
// a set of colon-separated strings.
// Example: -f a:1 -f a:2 -f b:3
// Panics if m is nil.
func StringPairs(m strpair.Map) flag.Value {
	if m == nil {
		panic("m is nil")
	}
	return stringPairsFlag(m)
}

// String implements the flag.Value interface.
func (f stringPairsFlag) String() string {
	return strings.Join(strpair.Map(f).Format(), ", ")
}

// Set implements the flag.Value interface.
func (f stringPairsFlag) Set(s string) error {
	parts := strings.Split(s, ":")
	if len(parts) == 1 {
		return fmt.Errorf("no colon")
	}
	strpair.Map(f).Add(parts[0], parts[1])
	return nil
}

// Set implements the flag.Getter interface.
func (f stringPairsFlag) Get() interface{} {
	return strpair.Map(f)
}
