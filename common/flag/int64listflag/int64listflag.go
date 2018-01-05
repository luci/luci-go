// Copyright 2017 The LUCI Authors.
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

// Package int64listflag provides a flag.Value implementation which resolves
// multiple args into an []int64.
package int64listflag

import (
	"flag"
	"strconv"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// Flag is a flag.Value implementation representing an ordered slice of int64s.
type Flag []int64

var _ flag.Value = (*Flag)(nil)

// String returns a comma-separated string representation of the flag values.
func (f Flag) String() string {
	s := make([]string, len(f))
	for n, i := range f {
		s[n] = strconv.FormatInt(i, 10)
	}
	return strings.Join(s, ", ")
}

// Set records seeing a flag value.
func (f *Flag) Set(val string) error {
	i, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return errors.Reason("values must be 64-bit integers").Err()
	}
	*f = append(*f, i)
	return nil
}
