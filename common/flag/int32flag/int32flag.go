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

// Package int32flag provides a flag.Value implementation which resolves
// an arg into an int32.
package int32flag

import (
	"flag"
	"strconv"

	"go.chromium.org/luci/common/errors"
)

// Flag is a flag.Value implementation representing an int32.
type Flag int32

var _ flag.Value = (*Flag)(nil)

// String returns a string representation of the flag value.
func (f Flag) String() string {
	return strconv.FormatInt(int64(f), 10)
}

// Set records seeing a flag value.
func (f *Flag) Set(s string) error {
	i, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return errors.Reason("values must be 32-bit integers").Err()
	}
	*f = Flag(i)
	return nil
}

// Get retrieves the flag value.
func (f *Flag) Get() interface{} {
	return int32(*f)
}
