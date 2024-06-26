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

package finalizer

import (
	"sort"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestShouldFinalize(t *testing.T) {
	Convey(`ShouldFinalize`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		assertReady := func(invID invocations.ID, expected bool) {
			should, err := readyToFinalize(ctx, invID)
			So(err, ShouldBeNil)
			So(should, ShouldEqual, expected)
		}

		Convey(`Includes two ACTIVE`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_FINALIZING, nil, "b", "c"),
				insert.InvocationWithInclusions("b", pb.Invocation_ACTIVE, nil),
				insert.InvocationWithInclusions("c", pb.Invocation_ACTIVE, nil),
			)...)

			assertReady("a", false)
		})

		Convey(`Includes ACTIVE and FINALIZED`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_FINALIZING, nil, "b", "c"),
				insert.InvocationWithInclusions("b", pb.Invocation_ACTIVE, nil),
				insert.InvocationWithInclusions("c", pb.Invocation_FINALIZED, nil),
			)...)

			assertReady("a", false)
		})

		Convey(`INCLUDES ACTIVE and FINALIZING`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_FINALIZING, nil, "b", "c"),
				insert.InvocationWithInclusions("b", pb.Invocation_ACTIVE, nil),
				insert.InvocationWithInclusions("c", pb.Invocation_FINALIZING, nil),
			)...)

			assertReady("a", false)
		})

		Convey(`INCLUDES FINALIZING which includes ACTIVE`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_FINALIZING, nil, "b"),
				insert.InvocationWithInclusions("b", pb.Invocation_FINALIZING, nil, "c"),
				insert.InvocationWithInclusions("c", pb.Invocation_ACTIVE, nil),
			)...)

			assertReady("a", false)
		})

		Convey(`Cycle with one node`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_FINALIZING, nil, "a"),
			)...)

			assertReady("a", true)
		})

		Convey(`Cycle with two nodes`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_FINALIZING, nil, "b"),
				insert.InvocationWithInclusions("b", pb.Invocation_FINALIZING, nil, "a"),
			)...)

			assertReady("a", true)
		})
	})
}

func TestFinalizeInvocation(t *testing.T) {
	Convey(`FinalizeInvocation`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		ctx, sched := tq.TestingContext(ctx, nil)

		opts := Options{
			ResultDBHostname: "rdb-host",
		}
		Convey(`Changes the state and finalization time`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.InvocationWithInclusions("x", pb.Invocation_FINALIZING, nil),
			)...)

			err := finalizeInvocation(ctx, "x", opts)
			So(err, ShouldBeNil)

			var state pb.Invocation_State
			var finalizeTime time.Time
			testutil.MustReadRow(ctx, "Invocations", invocations.ID("x").Key(), map[string]any{
				"State":        &state,
				"FinalizeTime": &finalizeTime,
			})
			So(state, ShouldEqual, pb.Invocation_FINALIZED)
			So(finalizeTime, ShouldNotResemble, time.Time{})
		})

		Convey(`Enqueues more finalizing tasks`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.InvocationWithInclusions("active", pb.Invocation_ACTIVE, nil, "x"),
				insert.InvocationWithInclusions("finalizing1", pb.Invocation_FINALIZING, nil, "x"),
				insert.InvocationWithInclusions("finalizing2", pb.Invocation_FINALIZING, nil, "x"),
				insert.InvocationWithInclusions("x", pb.Invocation_FINALIZING, nil),
			)...)

			err := finalizeInvocation(ctx, "x", opts)
			So(err, ShouldBeNil)

			// Enqueued TQ tasks.
			invocations := []string{}
			for _, t := range sched.Tasks().Payloads() {
				payload, ok := t.(*taskspb.TryFinalizeInvocation)
				if !ok {
					continue
				}
				invocations = append(invocations, payload.InvocationId)
			}
			sort.Strings(invocations)
			So(invocations, ShouldResemble, []string{"finalizing1", "finalizing2"})
		})

		Convey(`Enqueues a finalization notification and update test metadata tasks`, func() {
			createTime := timestamppb.New(time.Unix(1000, 0))
			testutil.MustApply(ctx,
				insert.Invocation("x", pb.Invocation_FINALIZING, map[string]any{
					"Realm":        "myproject:myrealm",
					"IsExportRoot": true,
					"CreateTime":   createTime,
				}),
			)

			err := finalizeInvocation(ctx, "x", opts)
			So(err, ShouldBeNil)

			So(sched.Tasks().Payloads(), ShouldHaveLength, 4)
			So(sched.Tasks().Payloads()[0], ShouldResembleProto, &taskspb.ExportInvocationToBQ{
				InvocationId: "x",
			})
			So(sched.Tasks().Payloads()[1], ShouldResembleProto, &taskspb.ExportArtifacts{
				InvocationId: "x",
			})
			So(sched.Tasks().Payloads()[2], ShouldResembleProto, &taskspb.UpdateTestMetadata{
				InvocationId: "x",
			})
			// Enqueued pub/sub notification.
			So(sched.Tasks().Payloads()[3], ShouldResembleProto, &taskspb.NotifyInvocationFinalized{
				Message: &pb.InvocationFinalizedNotification{
					Invocation:   "invocations/x",
					Realm:        "myproject:myrealm",
					IsExportRoot: true,
					ResultdbHost: "rdb-host",
					CreateTime:   createTime,
				},
			})
		})

		Convey(`Enqueues more bq_export tasks`, func() {
			bq1, _ := proto.Marshal(&pb.BigQueryExport{
				Dataset:    "dataset",
				Project:    "project",
				Table:      "table",
				ResultType: &pb.BigQueryExport_TestResults_{},
			})
			bq2, _ := proto.Marshal(&pb.BigQueryExport{
				Dataset:    "dataset",
				Project:    "project2",
				Table:      "table1",
				ResultType: &pb.BigQueryExport_TextArtifacts_{},
			})
			testutil.MustApply(ctx,
				insert.Invocation("x", pb.Invocation_FINALIZING, map[string]any{
					"BigQueryExports": [][]byte{bq1, bq2},
				}),
			)

			err := finalizeInvocation(ctx, "x", opts)
			So(err, ShouldBeNil)
			// Enqueued TQ tasks.
			So(sched.Tasks().Payloads()[0], ShouldResembleProto,
				&taskspb.ExportInvocationToBQ{
					InvocationId: "x",
				})
			So(sched.Tasks().Payloads()[1], ShouldResembleProto,
				&taskspb.ExportInvocationArtifactsToBQ{
					InvocationId: "x",
					BqExport: &pb.BigQueryExport{
						Dataset:    "dataset",
						Project:    "project2",
						Table:      "table1",
						ResultType: &pb.BigQueryExport_TextArtifacts_{},
					},
				})
			So(sched.Tasks().Payloads()[2], ShouldResembleProto,
				&taskspb.ExportInvocationTestResultsToBQ{
					InvocationId: "x",
					BqExport: &pb.BigQueryExport{
						Dataset:    "dataset",
						Project:    "project",
						Table:      "table",
						ResultType: &pb.BigQueryExport_TestResults_{},
					},
				})
		})

		Convey(`Enqueue mark submitted tasks`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.InvocationWithInclusions("x", pb.Invocation_FINALIZING, map[string]any{
					"Submitted": true,
				}),
			)...)

			err := finalizeInvocation(ctx, "x", opts)
			So(err, ShouldBeNil)
			// there should be two tasks ahead, test metadata and notify finalized.
			So(sched.Tasks().Payloads()[0], ShouldResembleProto,
				&taskspb.MarkInvocationSubmitted{
					InvocationId: "x",
				})
		})
	})
}
