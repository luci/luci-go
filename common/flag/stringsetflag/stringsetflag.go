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

// Package stringsetflag provides a flag.Value implementation which resolves
// multiple args into a stringset.
package stringsetflag

import (
	"flag"
	"fmt"
	"sort"
	"strings"

	"github.com/luci/luci-go/common/data/stringset"
)

// Flag is a flag.Value implementation which represents an unordered set of
// strings.
//
// For example, this allows you to construct a flag that would behave like:
//   -myflag Foo
//   -myflag Bar
//   -myflag Bar
//
// And then myflag.Data.Has("Bar") would be true.
type Flag struct{ Data stringset.Set }

var _ flag.Value = (*Flag)(nil)

func (f Flag) String() string {
	if f.Data == nil {
		return ""
	}
	slc := f.Data.ToSlice()
	sort.Strings(slc)
	return strings.Join(slc, ",")
}

// Set implements flag.Value's Set function.
func (f *Flag) Set(val string) error {
	if val == "" {
		return fmt.Errorf("must have an argument value")
	}

	if f.Data == nil {
		f.Data = stringset.NewFromSlice(val)
	} else {
		f.Data.Add(val)
	}
	return nil
}
