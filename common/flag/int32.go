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

// int32Flag is a flag.Getter implementation representing an int32.
type int32Flag int32

// String returns a string representation of the flag value.
func (f int32Flag) String() string {
	return strconv.FormatInt(int64(f), 10)
}

// Set records seeing a flag value.
func (f *int32Flag) Set(s string) error {
	i, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return errors.New("values must be 32-bit integers")
	}
	*f = int32Flag(i)
	return nil
}

// Get retrieves the flag value.
func (f int32Flag) Get() any {
	return int32(f)
}

// Int32 returns a flag.Getter which reads flags into the given int32 pointer.
func Int32(i *int32) flag.Getter {
	return (*int32Flag)(i)
}
