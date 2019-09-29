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
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const invocationIDPattern = `[a-z][a-z0-9_\-]*`

var invocationIDRe = regexpfFull("%s", invocationIDPattern)
var invocationNameRe = regexpfFull("invocations/(%s)", invocationIDPattern)

// ValidateInvocationID returns an error if id is invalid.
func ValidateInvocationID(id string) error {
	if !invocationIDRe.MatchString(id) {
		return doesNotMatch(invocationIDRe)
	}
	return nil
}

// ParseInvocationName extracts invocation id from name.
func ParseInvocationName(name string) (id string, err error) {
	err = parseName(name, invocationNameRe, &id)
	return
}

// InvocationName produces an invocation name from an id.
func InvocationName(id string) string {
	return "invocations/" + id
}

// NormalizeInvocation converts inv to the canonical form.
func NormalizeInvocation(inv *pb.Invocation) {
	sortStringPairs(inv.Tags)
}
