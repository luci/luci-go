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

	"go.chromium.org/luci/common/errors"
)

// commaListFlag implements the flag.Getter returned by CommaList.
type commaListFlag []string

// CommaList returns a flag.Getter for parsing a comma
// separated flag argument into a string slice.
func CommaList(s *[]string) flag.Getter {
	return (*commaListFlag)(s)
}

// String implements the flag.Value interface.
func (f commaListFlag) String() string {
	return strings.Join(f, ",")
}

// Set implements the flag.Value interface.
func (f *commaListFlag) Set(s string) error {
	if f == nil {
		return errors.New("commaListFlag pointer is nil")
	}
	*f = splitCommaList(s)
	return nil
}

// Get retrieves the flag value.
func (f commaListFlag) Get() any {
	return []string(f)
}

// splitCommaList splits a comma separated string into a slice of
// strings.  If the string is empty, return an empty slice.
func splitCommaList(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}
