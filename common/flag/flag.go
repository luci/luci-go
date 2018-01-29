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

// Package flag provides custom flags.
package flag

import (
	"flag"
	"strconv"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// int32Flag is a flag.Value implementation representing an int32.
type int32Flag int32

// String returns a string representation of the flag value.
func (f int32Flag) String() string {
	return strconv.FormatInt(int64(f), 10)
}

// Set records seeing a flag value.
func (f *int32Flag) Set(s string) error {
	i, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return errors.Reason("values must be 32-bit integers").Err()
	}
	*f = int32Flag(i)
	return nil
}

// Get retrieves the flag value.
func (f *int32Flag) Get() interface{} {
	return int32(*f)
}

// Int32 returns a flag.Value which reads flags into the given int32 pointer.
func Int32(i *int32) flag.Value {
	return (*int32Flag)(i)
}

// int64SliceFlag is a flag.Value implementation representing an []int64.
type int64SliceFlag []int64

// String returns a comma-separated string representation of the flag values.
func (f int64SliceFlag) String() string {
	s := make([]string, len(f))
	for n, i := range f {
		s[n] = strconv.FormatInt(i, 10)
	}
	return strings.Join(s, ", ")
}

// Set records seeing a flag value.
func (f *int64SliceFlag) Set(val string) error {
	i, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return errors.Reason("values must be 64-bit integers").Err()
	}
	*f = append(*f, i)
	return nil
}

// Get retrieves the flag values.
func (f int64SliceFlag) Get() interface{} {
	r := make([]int64, len(f))
	for n, i := range f {
		r[n] = i
	}
	return r
}

// Int64Slice returns a flag.Value which reads flags into the given []int64 pointer.
func Int64Slice(i *[]int64) flag.Value {
	return (*int64SliceFlag)(i)
}
