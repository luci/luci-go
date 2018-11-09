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
	"strconv"

	"go.chromium.org/luci/common/errors"
)

// int64Flag is a flag.Value implementation representing an int64.
type int64Flag int64

// String returns a string representation of the flag value.
func (f int64Flag) String() string {
	return strconv.FormatInt(int64(f), 10)
}

// Set records seeing a flag value.
func (f *int64Flag) Set(s string) error {
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return errors.Reason("values must be 64-bit integers").Err()
	}
	*f = int64Flag(i)
	return nil
}

// Get retrieves the flag value.
func (f *int64Flag) Get() interface{} {
	return int64(*f)
}

// Int64 returns a flag.Value which reads flags into the given int64 pointer.
func Int64(i *int64) flag.Value {
	return (*int64Flag)(i)
}
