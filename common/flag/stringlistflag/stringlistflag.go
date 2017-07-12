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

// Package stringlistflag provides a flag.Value implementation which resolves
// multiple args into a []string.
package stringlistflag

import (
	"flag"
	"fmt"
	"strings"
)

// Flag is a flag.Value implementation which represents an ordered set of
// strings.
//
// For example, this allows you to construct a flag that would behave like:
//   -myflag Foo
//   -myflag Bar
//   -myflag Bar
//
// And then myflag would be []string{"Foo", "Bar", "Bar"}
type Flag []string

var _ flag.Value = (*Flag)(nil)

func (f Flag) String() string {
	return strings.Join(f, ", ")
}

// Set implements flag.Value's Set function.
func (f *Flag) Set(val string) error {
	if val == "" {
		return fmt.Errorf("must have an argument value")
	}

	*f = append(*f, val)
	return nil
}
