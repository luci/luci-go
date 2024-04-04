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

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMarkInvocationSubmitted(t *testing.T) {
	Convey(`E2E`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		ctx, sched := tq.TestingContext(ctx, nil)
		recorder := newTestRecorderServer()

		Convey(`Empty Invocation`, func() {
			_, err := recorder.MarkInvocationSubmitted(ctx, &pb.MarkInvocationSubmittedRequest{
				Invocation: "",
			})
			So(err, ShouldBeRPCInvalidArgument, "unspecified")
		})

		Convey(`Invalid Invocation`, func() {
			_, err := recorder.MarkInvocationSubmitted(ctx, &pb.MarkInvocationSubmittedRequest{
				Invocation: "random/invocation",
			})
			So(err, ShouldBeRPCInvalidArgument, "invocation: does not match")
		})

		Convey(`Invocation Does Not Exist`, func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:@project", Permission: permSetSubmittedInvocation},
				},
			})

			_, err := recorder.MarkInvocationSubmitted(ctx, &pb.MarkInvocationSubmittedRequest{
				Invocation: "invocations/inv",
			})
			So(err, ShouldBeRPCPermissionDenied, "Caller does not have permission to mark invocations/inv submitted")
		})

		Convey(`Insufficient Permissions`, func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:            "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{},
			})

			_, err := recorder.MarkInvocationSubmitted(ctx, &pb.MarkInvocationSubmittedRequest{
				Invocation: "invocations/inv",
			})
			So(err, ShouldBeRPCPermissionDenied, "Caller does not have permission to mark invocations/inv submitted")
		})

		Convey(`Unfinalized Invocation`, func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:@project", Permission: permSetSubmittedInvocation},
				},
			})
			testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_FINALIZING, map[string]any{
				"Realm": "chromium:try",
			}))
			_, err := recorder.MarkInvocationSubmitted(ctx, &pb.MarkInvocationSubmittedRequest{
				Invocation: "invocations/inv",
			})

			So(err, ShouldBeNil)
			// No tasks queued by call if the invocation is yet to be finalized. Should be invoked by
			// finalizer to mark submitted.
			So(sched.Tasks(), ShouldHaveLength, 0)

			submitted, err := invocations.ReadSubmitted(span.Single(ctx), invocations.ID("inv"))
			So(err, ShouldBeNil)
			So(submitted, ShouldBeTrue)
		})

		Convey(`Already submitted`, func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:@project", Permission: permSetSubmittedInvocation},
				},
			})
			testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_FINALIZED, map[string]any{
				"Realm":     "chromium:try",
				"Submitted": true,
			}))
			_, err := recorder.MarkInvocationSubmitted(ctx, &pb.MarkInvocationSubmittedRequest{
				Invocation: "invocations/inv",
			})

			// Workflow should terminate early if the invocation is already submitted.
			So(err, ShouldBeNil)
			So(sched.Tasks(), ShouldHaveLength, 0)
		})

		Convey(`Success`, func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:@project", Permission: permSetSubmittedInvocation},
				},
			})
			testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_FINALIZED, map[string]any{
				"Realm": "chromium:try",
			}))
			_, err := recorder.MarkInvocationSubmitted(ctx, &pb.MarkInvocationSubmittedRequest{
				Invocation: "invocations/inv",
			})

			So(err, ShouldBeNil)
			So(sched.Tasks(), ShouldHaveLength, 1)
			So(sched.Tasks().Payloads(), ShouldResembleProto, []protoreflect.ProtoMessage{
				&taskspb.MarkInvocationSubmitted{InvocationId: "inv"},
			})
		})
	})
}
