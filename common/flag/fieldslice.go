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

	"google.golang.org/api/googleapi"
)

// fieldSliceFlag is a flag.Getter implementation representing a []googleapi.Field.
type fieldSliceFlag []googleapi.Field

// String returns a comma-separated string representation of the flag values.
func (f fieldSliceFlag) String() string {
	r := make([]string, len(f))
	for i, s := range f {
		r[i] = string(s)
	}
	return strings.Join(r, ", ")
}

// Set records seeing a flag value.
func (f *fieldSliceFlag) Set(val string) error {
	*f = append(*f, googleapi.Field(val))
	return nil
}

// Get retrieves the flag value.
func (f fieldSliceFlag) Get() interface{} {
	return []googleapi.Field(f)
}

// FieldSlice returns a flag.Getter which reads flags into the given []googleapi.Field pointer.
func FieldSlice(f *[]googleapi.Field) flag.Getter {
	return (*fieldSliceFlag)(f)
}
