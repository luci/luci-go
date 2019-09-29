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

// ParseInvocationName parses an invocation name.
func ParseInvocationName(name string) (id string, err error) {
	err = ParseResourceName(name, "invocations/{invocation_id}", &id)
	return
}

// InvocationName formats an invocation name.
func InvocationName(id string) string {
	return "invocations/" + id
}

// NormalizeInvocation converts inv to the canonical form.
func NormalizeInvocation(inv *pb.Invocation) {
	sortStringPairs(inv.Tags)
}
