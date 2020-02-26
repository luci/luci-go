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

package main

import (
	"bytes"
	"flag"
	"fmt"
	"strconv"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// commaCodeListFlag implements the flag.Getter returned by CommaCodeList.
type commaCodeListFlag []int32

// CommaCodeList returns a flag.Getter for parsing a comma separated flag argument
// into a string slice.
func CommaCodeList(nums *[]int32) flag.Getter {
	return (*commaCodeListFlag)(nums)
}

// String implements the flag.Value interface.
func (f commaCodeListFlag) String() string {
	var b bytes.Buffer
	for _, n := range f {
		b.WriteString(fmt.Sprint(n))
	}
	return b.String()
}

// Set implements the flag.Value interface.
func (f *commaCodeListFlag) Set(s string) error {
	if f == nil {
		return errors.Reason("commaCodeListFlag pointer is nil").Err()
	}
	nums, err := splitCommaCodeList(s)
	if err != nil {
		return err
	}
	*f = nums
	return nil
}

// Get retrieves the flag value.
func (f commaCodeListFlag) Get() interface{} {
	return []int32(f)
}

// splitCommaCodeList splits a comma separated string into a slice of
// strings.  If the string is empty, return an empty slice.
func splitCommaCodeList(s string) ([]int32, error) {
	var ret []int32
	for _, n := range strings.Split(s, ",") {
		i, err := strconv.ParseInt(n, 10, 32)
		if err != nil {
			return nil, errors.Reason("values must be 32-bit integers").Err()
		}
		ret = append(ret, int32(i))
	}
	return ret, nil
}
