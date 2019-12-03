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
	"encoding/hex"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/proto"
	tspb "github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/grpc/grpcutil"

	internalpb "go.chromium.org/luci/resultdb/internal/proto"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMutateInvocation(t *testing.T) {
	Convey("MayMutateInvocation", t, func() {
		ctx := testutil.SpannerTestContext(t)
		ct := testclock.TestRecentTimeUTC

		mayMutate := func() error {
			return mutateInvocation(ctx, "inv", func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
				return nil
			})
		}

		Convey("no token", func() {
			err := mayMutate()
			So(err, ShouldErrLike, `missing "update-token" metadata value`)
			So(grpcutil.Code(err), ShouldEqual, codes.Unauthenticated)
		})

		Convey("with token", func() {
			const token = "update token"
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(updateTokenMetadataKey, token))

			Convey(`no invocation`, func() {
				err := mayMutate()
				So(err, ShouldErrLike, `"invocations/inv" not found`)
				So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
			})

			Convey(`with finalized invocation`, func() {
				testutil.MustApply(ctx, testutil.InsertInvocation("inv", pb.Invocation_COMPLETED, token, ct))
				err := mayMutate()
				So(err, ShouldErrLike, `"invocations/inv" is not active`)
				So(grpcutil.Code(err), ShouldEqual, codes.FailedPrecondition)
			})

			Convey(`with active invocation and different token`, func() {
				testutil.MustApply(ctx, testutil.InsertInvocation("inv", pb.Invocation_ACTIVE, "different token", ct))
				err := mayMutate()
				So(err, ShouldErrLike, `invalid update token`)
				So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)
			})

			Convey(`with exceeded deadline`, func() {
				testutil.MustApply(ctx, testutil.InsertInvocation("inv", pb.Invocation_ACTIVE, token, ct))

				// Mock now to be after deadline.
				clock.Get(ctx).(testclock.TestClock).Add(2 * time.Hour)

				err := mayMutate()
				So(err, ShouldErrLike, `"invocations/inv" is not active`)
				So(grpcutil.Code(err), ShouldEqual, codes.FailedPrecondition)

				// Confirm the invocation has been updated.
				var state pb.Invocation_State
				var ft time.Time
				inv := span.InvocationID("inv")
				testutil.MustReadRow(ctx, "Invocations", inv.Key(), map[string]interface{}{
					"State":        &state,
					"FinalizeTime": &ft,
				})
				So(state, ShouldEqual, pb.Invocation_INTERRUPTED)
				So(ft, ShouldEqual, ct.Add(time.Hour))
			})

			Convey(`with active invocation and same token`, func() {
				testutil.MustApply(ctx, testutil.InsertInvocation("inv", pb.Invocation_ACTIVE, token, ct))

				err := mayMutate()
				So(err, ShouldBeNil)
			})
		})
	})
}

func TestReadInvocation(t *testing.T) {
	Convey(`ReadInvocationFull`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		ct := testclock.TestRecentTimeUTC

		readInv := func() *pb.Invocation {
			txn := span.Client(ctx).ReadOnlyTransaction()
			defer txn.Close()

			inv, err := span.ReadInvocationFull(ctx, txn, "inv")
			So(err, ShouldBeNil)
			return inv
		}

		Convey(`completed`, func() {
			testutil.MustApply(ctx, testutil.InsertInvocation("inv", pb.Invocation_COMPLETED, "", ct))

			inv := readInv()
			expected := &pb.Invocation{
				Name:         "invocations/inv",
				State:        pb.Invocation_COMPLETED,
				CreateTime:   pbutil.MustTimestampProto(ct),
				Deadline:     pbutil.MustTimestampProto(ct.Add(time.Hour)),
				FinalizeTime: pbutil.MustTimestampProto(ct.Add(time.Hour)),
			}
			So(inv, ShouldResembleProto, expected)

			Convey(`with included invocations`, func() {
				testutil.MustApply(ctx,
					testutil.InsertInvocation("included0", pb.Invocation_COMPLETED, "", ct),
					testutil.InsertInvocation("included1", pb.Invocation_COMPLETED, "", ct),
					testutil.InsertInclusion("inv", "included0"),
					testutil.InsertInclusion("inv", "included1"),
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
		ctx := testutil.SpannerTestContext(t)

		test := func(invID span.InvocationID, inv *pb.Invocation, bq_export *pb.BigQueryExport, expProcessAfter time.Time) {
			payload, _ := proto.Marshal(&internalpb.InvocationTask{
				BigqueryExport: bq_export,
			})
			payload_hash := hex.EncodeToString(payload[:8])
			key := spanner.Key{invID.RowID(), payload_hash}
			var processAfter *tspb.Timestamp
			err := span.ReadRow(ctx, span.Client(ctx).Single(), "InvocationTasks", key, map[string]interface{}{
				"ProcessAfter": &processAfter,
			})
			So(err, ShouldBeNil)
			So(processAfter, ShouldResemble, pbutil.MustTimestampProto(expProcessAfter))
		}

		Convey(`insertBQExportingTasks when create an active invocation`, func() {
			invID := span.InvocationID("invID_active")
			inv := &pb.Invocation{
				Name:  "invocations/invID_active",
				State: pb.Invocation_ACTIVE,
			}

			bq_export_1 := &pb.BigQueryExport{
				Project:     "project",
				Dataset:     "dataset",
				Table:       "table",
				TestResults: &pb.BigQueryExport_TestResults{},
			}

			bq_export_2 := &pb.BigQueryExport{
				Project:     "project",
				Dataset:     "dataset",
				Table:       "table2",
				TestResults: &pb.BigQueryExport_TestResults{},
			}

			bq_exports := []*pb.BigQueryExport{
				bq_export_1,
				bq_export_2,
			}
			muts, err := insertBQExportingTasks(invID, inv, bq_exports, now)
			So(err, ShouldBeNil)

			testutil.MustApply(ctx, muts...)

			test(invID, inv, bq_export_1, now.Add(eventualInvocationTaskProcessAfter))
			test(invID, inv, bq_export_2, now.Add(eventualInvocationTaskProcessAfter))
		})

		Convey(`insertBQExportingTasks when create a finalized invocation`, func() {
			invID := span.InvocationID("invID_completed")
			inv := &pb.Invocation{
				Name:  "invocations/invID_completed",
				State: pb.Invocation_COMPLETED,
			}

			bq_export := &pb.BigQueryExport{
				Project:     "project",
				Dataset:     "dataset",
				Table:       "table",
				TestResults: &pb.BigQueryExport_TestResults{},
			}

			bq_exports := []*pb.BigQueryExport{
				bq_export,
			}
			muts, err := insertBQExportingTasks(invID, inv, bq_exports, now)
			So(err, ShouldBeNil)

			testutil.MustApply(ctx, muts...)

			test(invID, inv, bq_export, now)
		})
	})
}
