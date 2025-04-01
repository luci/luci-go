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

package recorder

import (
	"testing"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestValidateFinalizeInvocationRequest(t *testing.T) {
	ftt.Run(`TestValidateFinalizeInvocationRequest`, t, func(t *ftt.Test) {
		t.Run(`Valid`, func(t *ftt.Test) {
			err := validateFinalizeInvocationRequest(&pb.FinalizeInvocationRequest{
				Name: "invocations/a",
			})
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`Invalid name`, func(t *ftt.Test) {
			err := validateFinalizeInvocationRequest(&pb.FinalizeInvocationRequest{
				Name: "x",
			})
			assert.Loosely(t, err, should.ErrLike(`name: does not match`))
		})
	})
}

func TestFinalizeInvocation(t *testing.T) {
	ftt.Run(`TestFinalizeInvocation`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx, sched := tq.TestingContext(ctx, nil)
		recorder := newTestRecorderServer()

		token, err := generateInvocationToken(ctx, "inv")
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

		t.Run(`Idempotent`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))

			inv, err := recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: "invocations/inv"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, inv.State, should.Equal(pb.Invocation_FINALIZING))
			assert.Loosely(t, inv.FinalizeStartTime, should.NotBeNil)
			finalizeStartTime := inv.FinalizeStartTime
			assert.Loosely(t, sched.Tasks(), should.HaveLength(2))

			inv, err = recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: "invocations/inv"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, inv.State, should.Equal(pb.Invocation_FINALIZING))
			// The finalize start time should be the same as after the first call.
			assert.Loosely(t, inv.FinalizeStartTime, should.Match(finalizeStartTime))
			assert.Loosely(t, sched.Tasks(), should.HaveLength(2))
		})

		t.Run(`Success`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))
			inv, err := recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: "invocations/inv"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, inv.State, should.Equal(pb.Invocation_FINALIZING))
			assert.Loosely(t, inv.FinalizeStartTime, should.NotBeNil)
			finalizeTime := inv.FinalizeStartTime

			// Read the invocation from Spanner to confirm it's really FINALIZING.
			inv, err = invocations.Read(span.Single(ctx), "inv", invocations.ExcludeExtendedProperties)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, inv.State, should.Equal(pb.Invocation_FINALIZING))
			assert.Loosely(t, inv.FinalizeStartTime, should.Match(finalizeTime))

			// Enqueued the finalization task.
			assert.Loosely(t, sched.Tasks().Payloads(), should.Match([]protoreflect.ProtoMessage{
				&taskspb.RunExportNotifications{InvocationId: "inv"},
				&taskspb.TryFinalizeInvocation{InvocationId: "inv"},
			}))
		})
	})
}
