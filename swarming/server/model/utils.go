// Copyright 2023 The LUCI Authors.
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

package model

import (
	"go.chromium.org/luci/common/errors"
)

// checkIsHex returns an error if the string doesn't look like a lowercase hex
// string.
func checkIsHex(s string, minLen int) error {
	if len(s) < minLen {
		return errors.New("too small")
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return errors.Reason("bad lowercase hex string %q, wrong char %c", s, c).Err()
		}
	}
	return nil
}
