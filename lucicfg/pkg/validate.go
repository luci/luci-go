// Copyright 2025 The LUCI Authors.
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

package pkg

import (
	"strconv"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// ValidateName returns an error if given an invalid package name.
func ValidateName(name string) error {
	if name == "" {
		return errors.New("cannot be empty")
	}
	for _, token := range strings.Split(name, "/") {
		if err := validatePathComponent(token); err != nil {
			return errors.Annotate(err, "%s", token).Err()
		}
	}
	if len(name) > 300 {
		return errors.New("should be no longer than 300 characters")
	}
	return nil
}

// ValidateVersion parses and validates the "<major>.<minor>.<patch>" string.
func ValidateVersion(ver string) (LucicfgVersion, error) {
	var val LucicfgVersion
	chunks := strings.Split(ver, ".")
	if len(chunks) != 3 {
		return val, errors.Reason("expecting <major>.<minor>.<patch>").Err()
	}
	for i, chunk := range chunks {
		num, err := strconv.ParseUint(chunk, 10, 16)
		if err != nil {
			return val, errors.Reason("%q: not a positive number", chunk).Err()
		}
		val[i] = int(num)
	}
	return val, nil
}

func validatePathComponent(p string) error {
	if p == "" {
		return errors.New("empty path component")
	}
	for idx, r := range p {
		switch {
		case r >= 'a' && r <= 'z':
		case (r >= '0' && r <= '9'), r == '-', r == '_':
			if idx == 0 {
				return errors.New("must begin with a letter")
			}
		default:
			return errors.Reason("invalid character at %d (%c)", idx, r).Err()
		}
	}
	return nil
}
