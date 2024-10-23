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

package recorder

import (
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestMarkInvocationSubmitted(t *testing.T) {
	ftt.Run(`E2E`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx, sched := tq.TestingContext(ctx, nil)
		recorder := newTestRecorderServer()

		t.Run(`Empty Invocation`, func(t *ftt.Test) {
			_, err := recorder.MarkInvocationSubmitted(ctx, &pb.MarkInvocationSubmittedRequest{
				Invocation: "",
			})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("unspecified"))
		})

		t.Run(`Invalid Invocation`, func(t *ftt.Test) {
			_, err := recorder.MarkInvocationSubmitted(ctx, &pb.MarkInvocationSubmittedRequest{
				Invocation: "random/invocation",
			})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("invocation: does not match"))
		})

		t.Run(`Invocation Does Not Exist`, func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:@project", Permission: permSetSubmittedInvocation},
				},
			})

			_, err := recorder.MarkInvocationSubmitted(ctx, &pb.MarkInvocationSubmittedRequest{
				Invocation: "invocations/inv",
			})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("Caller does not have permission to mark invocations/inv submitted"))
		})

		t.Run(`Insufficient Permissions`, func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:            "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{},
			})

			_, err := recorder.MarkInvocationSubmitted(ctx, &pb.MarkInvocationSubmittedRequest{
				Invocation: "invocations/inv",
			})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("Caller does not have permission to mark invocations/inv submitted"))
		})

		t.Run(`Unfinalized Invocation`, func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:@project", Permission: permSetSubmittedInvocation},
				},
			})
			testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_FINALIZING, map[string]any{
				"Realm": "chromium:try",
			}))
			_, err := recorder.MarkInvocationSubmitted(ctx, &pb.MarkInvocationSubmittedRequest{
				Invocation: "invocations/inv",
			})

			assert.Loosely(t, err, should.BeNil)
			// No tasks queued by call if the invocation is yet to be finalized. Should be invoked by
			// finalizer to mark submitted.
			assert.Loosely(t, sched.Tasks(), should.HaveLength(0))

			submitted, err := invocations.ReadSubmitted(span.Single(ctx), invocations.ID("inv"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, submitted, should.BeTrue)
		})

		t.Run(`Already submitted`, func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:@project", Permission: permSetSubmittedInvocation},
				},
			})
			testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_FINALIZED, map[string]any{
				"Realm":     "chromium:try",
				"Submitted": true,
			}))
			_, err := recorder.MarkInvocationSubmitted(ctx, &pb.MarkInvocationSubmittedRequest{
				Invocation: "invocations/inv",
			})

			// Workflow should terminate early if the invocation is already submitted.
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, sched.Tasks(), should.HaveLength(0))
		})

		t.Run(`Success`, func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:@project", Permission: permSetSubmittedInvocation},
				},
			})
			testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_FINALIZED, map[string]any{
				"Realm": "chromium:try",
			}))
			_, err := recorder.MarkInvocationSubmitted(ctx, &pb.MarkInvocationSubmittedRequest{
				Invocation: "invocations/inv",
			})

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, sched.Tasks(), should.HaveLength(1))
			assert.Loosely(t, sched.Tasks().Payloads(), should.Resemble([]protoreflect.ProtoMessage{
				&taskspb.MarkInvocationSubmitted{InvocationId: "inv"},
			}))
		})
	})
}
