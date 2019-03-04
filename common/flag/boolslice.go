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

package flag

import (
	"flag"
	"strconv"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// boolSliceFlag is a flag.Value implementation representing an []bool.
type boolSliceFlag []bool

// String returns a comma-separated string representation of the flag values.
func (f boolSliceFlag) String() string {
	s := make([]string, len(f))
	for n, i := range f {
		s[n] = strconv.FormatBool(i)
	}
	return strings.Join(s, ", ")
}

// Set records seeing a flag value.
func (f *boolSliceFlag) Set(val string) error {
	b, err := strconv.ParseBool(val)
	if err != nil {
		return errors.Reason("values must be booleans").Err()
	}
	*f = append(*f, b)
	return nil
}

// Get retrieves the flag values.
func (f boolSliceFlag) Get() interface{} {
	r := make([]bool, len(f))
	for n, b := range f {
		r[n] = b
	}
	return r
}

// BoolSlice returns a flag.Value which reads flags into the given []bool pointer.
func BoolSlice(b *[]bool) flag.Value {
	return (*boolSliceFlag)(b)
}
