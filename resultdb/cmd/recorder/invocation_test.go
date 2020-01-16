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
	"context"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"

	internalpb "go.chromium.org/luci/resultdb/internal/proto"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
	. "go.chromium.org/luci/resultdb/internal/testutil"
)

func TestMutateInvocation(t *testing.T) {
	Convey("MayMutateInvocation", t, func() {
		ctx := SpannerTestContext(t)
		ct := testclock.TestRecentTimeUTC

		mayMutate := func() error {
			return mutateInvocation(ctx, "inv", func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
				return nil
			})
		}

		Convey("no token", func() {
			err := mayMutate()
			So(err, ShouldHaveAppStatus, codes.Unauthenticated, `missing update-token metadata value`)
		})

		Convey("with token", func() {
			const token = "update token"
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(updateTokenMetadataKey, token))

			Convey(`no invocation`, func() {
				err := mayMutate()
				So(err, ShouldHaveAppStatus, codes.NotFound, `invocations/inv not found`)
			})

			Convey(`with finalized invocation`, func() {
				MustApply(ctx, InsertInvocation("inv", pb.Invocation_FINALIZED, token, ct, false, ""))
				err := mayMutate()
				So(err, ShouldHaveAppStatus, codes.FailedPrecondition, `invocations/inv is not active`)
			})

			Convey(`with active invocation and different token`, func() {
				MustApply(ctx, InsertInvocation("inv", pb.Invocation_ACTIVE, "different token", ct, false, ""))
				err := mayMutate()
				So(err, ShouldHaveAppStatus, codes.PermissionDenied, `invalid update token`)
			})

			Convey(`with exceeded deadline`, func() {
				MustApply(ctx, InsertInvocation("inv", pb.Invocation_ACTIVE, token, ct, false, ""))

				// Mock now to be after deadline.
				clock.Get(ctx).(testclock.TestClock).Add(2 * time.Hour)

				err := mayMutate()
				So(err, ShouldHaveAppStatus, codes.FailedPrecondition, `invocations/inv is not active`)

				// Confirm the invocation has been updated.
				var state pb.Invocation_State
				var ft time.Time
				var interrupted bool
				inv := span.InvocationID("inv")
				MustReadRow(ctx, "Invocations", inv.Key(), map[string]interface{}{
					"State":        &state,
					"Interrupted":  &interrupted,
					"FinalizeTime": &ft,
				})
				So(state, ShouldEqual, pb.Invocation_FINALIZED)
				So(interrupted, ShouldEqual, true)
				So(ft, ShouldEqual, ct.Add(time.Hour))
			})

			Convey(`with active invocation and same token`, func() {
				MustApply(ctx, InsertInvocation("inv", pb.Invocation_ACTIVE, token, ct, false, ""))

				err := mayMutate()
				So(err, ShouldBeNil)
			})
		})
	})
}

func TestReadInvocation(t *testing.T) {
	Convey(`ReadInvocationFull`, t, func() {
		ctx := SpannerTestContext(t)
		ct := testclock.TestRecentTimeUTC

		readInv := func() *pb.Invocation {
			txn := span.Client(ctx).ReadOnlyTransaction()
			defer txn.Close()

			inv, err := span.ReadInvocationFull(ctx, txn, "inv")
			So(err, ShouldBeNil)
			return inv
		}

		Convey(`Finalized`, func() {
			MustApply(ctx, InsertInvocation("inv", pb.Invocation_FINALIZED, "", ct, false, ""))

			inv := readInv()
			expected := &pb.Invocation{
				Name:         "invocations/inv",
				State:        pb.Invocation_FINALIZED,
				CreateTime:   pbutil.MustTimestampProto(ct),
				Deadline:     pbutil.MustTimestampProto(ct.Add(time.Hour)),
				FinalizeTime: pbutil.MustTimestampProto(ct.Add(time.Hour)),
			}
			So(inv, ShouldResembleProto, expected)

			Convey(`with included invocations`, func() {
				MustApply(ctx,
					InsertInvocation("included0", pb.Invocation_FINALIZED, "", ct, false, ""),
					InsertInvocation("included1", pb.Invocation_FINALIZED, "", ct, false, ""),
					InsertInclusion("inv", "included0"),
					InsertInclusion("inv", "included1"),
				)

				inv := readInv()
				So(inv.IncludedInvocations, ShouldResemble, []string{"invocations/included0", "invocations/included1"})
			})
		})
	})
}

func TestInsertBQExportingTasks(t *testing.T) {
	Convey(`insertBQExportingTasks`, t, func() {
		now := testclock.TestRecentTimeUTC
		ctx := SpannerTestContext(t)
		invID := span.InvocationID("invID")

		bqExport1 := &pb.BigQueryExport{
			Project:     "project",
			Dataset:     "dataset",
			Table:       "table",
			TestResults: &pb.BigQueryExport_TestResults{},
		}

		bqExport2 := &pb.BigQueryExport{
			Project:     "project",
			Dataset:     "dataset",
			Table:       "table2",
			TestResults: &pb.BigQueryExport_TestResults{},
		}

		muts := insertBQExportingTasks(invID, now, bqExport1, bqExport2)

		MustApply(ctx, muts...)

		test := func(index int, bqExport *pb.BigQueryExport) {
			invTask := &internalpb.InvocationTask{
				BigqueryExport: bqExport,
			}
			key := span.InvocationID("invID").Key(taskID(taskTypeBqExport, index))
			invTaskRtn := &internalpb.InvocationTask{}
			MustReadRow(ctx, "InvocationTasks", key, map[string]interface{}{
				"Payload": invTaskRtn,
			})
			So(invTaskRtn, ShouldResembleProto, invTask)
		}

		test(0, bqExport1)
		test(1, bqExport2)
	})
}
