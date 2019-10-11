// Copyright 2019 The LUCI Authors.
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

package pbutil

import (
	"fmt"
)

var inclusionNameRe = regexpf("^invocations/(%s)/inclusions/(%s)$", invocationIDPattern, invocationIDPattern)

// ParseInclusionName retrieves invocation ids.
func ParseInclusionName(name string) (includingInvocationID, includedInvocationID string, err error) {
	m := inclusionNameRe.FindStringSubmatch(name)
	if m == nil {
		return "", "", doesNotMatch(inclusionNameRe)
	}
	return m[1], m[2], nil
}

// InclusionName formats an inclusion name.
// Does not validate invocation ids, use ValidateInvocationID.
func InclusionName(includingInvocationID, includedInvocationID string) string {
	return fmt.Sprintf("invocations/%s/inclusions/%s", includingInvocationID, includedInvocationID)
}
