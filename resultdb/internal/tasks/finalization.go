// Copyright 2020 The LUCI Authors.
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

package tasks

import (
	"context"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/clock"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/span"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// StartInvocationFinalization changes invocation state to FINALIZING
// and enqueues a TryFinalizeInvocation task.
//
// The caller is responsible for ensuring that the invocation is active.
//
// TODO(nodir): this package is not a great place for this function, but there
// is no better package at the moment. Keep it here for now, but consider a
// new package as the code base grows.
func StartInvocationFinalization(ctx context.Context, txn *spanner.ReadWriteTransaction, id invocations.ID) error {
	return txn.BufferWrite([]*spanner.Mutation{
		span.UpdateMap("Invocations", map[string]interface{}{
			"InvocationId": id,
			"State":        pb.Invocation_FINALIZING,
		}),
		Enqueue(TryFinalizeInvocation, "finalize/"+id.RowID(), id, nil, clock.Now(ctx).UTC()),
	})
}
