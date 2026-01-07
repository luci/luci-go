// Copyright 2026 The LUCI Authors.
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

package testverdictsv2

import "go.chromium.org/luci/common/errors"

const (
	// The maximum result limit for QueryTestVerdicts.
	resultLimitMax = 100

	// The default result limit for QueryTestVerdicts.
	resultLimitDefault = 10
)

// DefaultResultLimit returns the default limit value, or the given resultLimit (if it is non-zero).
func DefaultResultLimit(resultLimit int32) int {
	if resultLimit == 0 {
		return resultLimitDefault
	}
	return int(resultLimit)
}

// ValidateResultLimit verifies a specified result limit (as specified in an RPC) is valid.
func ValidateResultLimit(resultLimit int32) error {
	if resultLimit <= 0 {
		return errors.New("must be positive")
	}
	if resultLimit > resultLimitMax {
		return errors.Fmt("%v is too large, the maximum allowed value is %v", resultLimit, resultLimitMax)
	}
	return nil
}
