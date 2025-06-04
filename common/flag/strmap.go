// Copyright 2016 The LUCI Authors.
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
	"sort"
	"strings"

	"go.chromium.org/luci/common/errors"
)

type strMapFlag map[string]string

// StringMap returns a flag.Getter for parsing map[string]string from a
// a set of colon-separated strings.
// Example:
//
//	-f a:1 -f b:3
//
// The flag.Getter.Set implementation returns an error if the key is already
// in the map.
// Panics if m is nil.
func StringMap(m map[string]string) flag.Getter {
	if m == nil {
		panic("m is nil")
	}
	return strMapFlag(m)
}

func (f strMapFlag) String() string {
	// This encoding is lossy. It is optimized for readability.
	pairs := make([]string, 0, len(f))
	for k, v := range f {
		pairs = append(pairs, fmt.Sprintf("%s:%s", k, v))
	}
	sort.Strings(pairs)
	return strings.Join(pairs, " ")
}

func (f strMapFlag) Set(s string) error {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) == 1 {
		return fmt.Errorf("no colon")
	}
	key := parts[0]
	value := parts[1]
	if _, ok := f[key]; ok {
		return errors.Fmt("key %q is already specified", key)
	}
	f[key] = value
	return nil
}

func (f strMapFlag) Get() any {
	return map[string]string(f)
}
