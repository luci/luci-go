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

package invocations

import (
	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/resultdb/internal/spanutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// MarkFinalizing creates a mutation to mark the given invocation as FINALIZING.
// The caller MUST check the invocation is currently in ACTIVE state, or this
// will incorrectly overwrite the FinalizeStartTime.
func MarkFinalizing(id ID) *spanner.Mutation {
	return spanutil.UpdateMap("Invocations", map[string]any{
		"InvocationId":      id,
		"State":             pb.Invocation_FINALIZING,
		"FinalizeStartTime": spanner.CommitTimestamp,
	})
}

// MarkFinalized creates a mutation to mark the given invocation as FINALIZED.
// The caller MUST check the invocation is currently in FINALIZING state, or this
// will incorrectly overwrite the FinalizeTime.
func MarkFinalized(id ID) *spanner.Mutation {
	return spanutil.UpdateMap("Invocations", map[string]any{
		"InvocationId": id,
		"State":        pb.Invocation_FINALIZED,
		"FinalizeTime": spanner.CommitTimestamp,
	})
}
