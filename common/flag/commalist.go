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
	"strings"

	"go.chromium.org/luci/common/errors"
)

// CommaList is an implementation of flag.Value for parsing a comma
// separated flag argument into a string slice.
type CommaList struct {
	s *[]string
}

// NewCommaList creates a CommaList value.
func NewCommaList(s *[]string) CommaList {
	return CommaList{s: s}
}

// String implements the flag.Value interface.
func (f CommaList) String() string {
	if f.s == nil {
		return ""
	}
	return strings.Join(*f.s, ",")
}

// Set implements the flag.Value interface.
func (f CommaList) Set(s string) error {
	if f.s == nil {
		return errors.Reason("CommaList pointer is nil").Err()
	}
	*f.s = splitCommaList(s)
	return nil
}

// splitCommaList splits a comma separated string into a slice of
// strings.  If the string is empty, return an empty slice.
func splitCommaList(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, ",")
}
