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

package main

import (
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/ptypes"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateUpdateInvocationRequest(t *testing.T) {
	t.Parallel()
	now := testclock.TestRecentTimeUTC
	Convey(`TestValidateUpdateInvocationRequest`, t, func() {
		Convey(`empty`, func() {
			err := validateUpdateInvocationRequest(&pb.UpdateInvocationRequest{}, now)
			So(err, ShouldErrLike, `invocation: name: unspecified`)
		})

		Convey(`invalid id`, func() {
			err := validateUpdateInvocationRequest(&pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{Name: "1"},
			}, now)
			So(err, ShouldErrLike, `invocation: name: does not match`)
		})

		Convey(`empty update mask`, func() {
			err := validateUpdateInvocationRequest(&pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{Name: "invocations/inv"},
			}, now)
			So(err, ShouldErrLike, `update_mask: paths is empty`)
		})

		Convey(`unsupported update mask`, func() {
			err := validateUpdateInvocationRequest(&pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{Name: "invocations/inv"},
				UpdateMask: &field_mask.FieldMask{
					Paths: []string{"name"},
				},
			}, now)
			So(err, ShouldErrLike, `update_mask: unsupported path "name"`)
		})

		Convey(`invalid deadline`, func() {
			deadline, err := ptypes.TimestampProto(now.Add(-time.Hour))
			So(err, ShouldBeNil)
			err = validateUpdateInvocationRequest(&pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{
					Name:     "invocations/inv",
					Deadline: deadline,
				},
				UpdateMask: &field_mask.FieldMask{
					Paths: []string{"deadline"},
				},
			}, now)
			So(err, ShouldErrLike, `invocation: deadline: must be at least 10 seconds in the future`)
		})

		Convey(`valid`, func() {
			deadline, err := ptypes.TimestampProto(now.Add(time.Hour))
			So(err, ShouldBeNil)

			err = validateUpdateInvocationRequest(&pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{
					Name:     "invocations/inv",
					Deadline: deadline,
				},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"deadline"}},
			}, now)
			So(err, ShouldBeNil)
		})
	})
}

func TestUpdateInvocation(t *testing.T) {
	Convey(`TestUpdateInvocation`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		recorder := NewRecorderServer()

		const token = "update token"
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(updateTokenMetadataKey, token))

		validDeadline, _ := ptypes.TimestampProto(clock.Now(ctx).Add(day))
		updateMask := &field_mask.FieldMask{
			Paths: []string{"deadline"},
		}

		Convey(`invalid request`, func() {
			req := &pb.UpdateInvocationRequest{}
			_, err := recorder.UpdateInvocation(ctx, req)
			So(err, ShouldErrLike, `bad request: invocation: name: unspecified`)
			So(grpcutil.Code(err), ShouldEqual, codes.InvalidArgument)
		})

		Convey(`no invocation`, func() {
			req := &pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{
					Name:     "invocations/inv",
					Deadline: validDeadline,
				},
				UpdateMask: updateMask,
			}
			_, err := recorder.UpdateInvocation(ctx, req)
			So(err, ShouldErrLike, `"invocations/inv" not found`)
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
		})

		// Insert the invocation.
		testutil.MustApply(ctx, testutil.InsertInvocation("inv", pb.Invocation_ACTIVE, token, testclock.TestRecentTimeUTC))

		Convey("e2e", func() {
			expected := &pb.Invocation{
				Name:     "invocations/inv",
				Deadline: validDeadline,
			}
			req := &pb.UpdateInvocationRequest{
				Invocation: expected,
				UpdateMask: updateMask,
			}
			inv, err := recorder.UpdateInvocation(ctx, req)
			So(err, ShouldBeNil)
			So(inv.Name, ShouldEqual, expected.Name)
			So(inv.State, ShouldEqual, pb.Invocation_ACTIVE)
			So(inv.Deadline, ShouldResembleProto, expected.Deadline)

			// Read from the database.
			actual := &pb.Invocation{
				Name: expected.Name,
			}
			testutil.MustReadRow(ctx, "Invocations", spanner.Key{"inv"}, map[string]interface{}{
				"Deadline": &actual.Deadline,
			})
			So(actual, ShouldResembleProto, expected)
		})
	})
}
