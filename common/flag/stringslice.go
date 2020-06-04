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
	"strings"
)

// stringSliceFlag is a flag.Getter implementation representing a []string.
type stringSliceFlag []string

// String returns a comma-separated string representation of the flag values.
func (f stringSliceFlag) String() string {
	return strings.Join(f, ", ")
}

// Set records seeing a flag value.
func (f *stringSliceFlag) Set(val string) error {
	*f = append(*f, val)
	return nil
}

// Get retrieves the flag value.
func (f stringSliceFlag) Get() interface{} {
	return []string(f)
}

// StringSlice returns a flag.Getter which reads flags into the given []string pointer.
func StringSlice(s *[]string) flag.Getter {
	return (*stringSliceFlag)(s)
}
