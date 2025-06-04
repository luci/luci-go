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

// boolSliceFlag is a flag.Getter implementation representing an []bool.
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
		return errors.New("values must be booleans")
	}
	*f = append(*f, b)
	return nil
}

// Get retrieves the flag value.
func (f boolSliceFlag) Get() any {
	return []bool(f)
}

// BoolSlice returns a flag.Getter which reads flags into the given []bool pointer.
func BoolSlice(b *[]bool) flag.Getter {
	return (*boolSliceFlag)(b)
}
