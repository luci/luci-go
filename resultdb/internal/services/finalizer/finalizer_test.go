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

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestShouldFinalize(t *testing.T) {
	ftt.Run(`ShouldFinalize`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		assertReady := func(invID invocations.ID, expected bool) {
			ready, err := readyToFinalize(ctx, invID)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ready, should.Equal(expected))
		}

		t.Run(`Includes two ACTIVE`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_FINALIZING, nil, "b", "c"),
				insert.InvocationWithInclusions("b", pb.Invocation_ACTIVE, nil),
				insert.InvocationWithInclusions("c", pb.Invocation_ACTIVE, nil),
			)...)

			assertReady("a", false)
		})

		t.Run(`Includes ACTIVE and FINALIZED`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_FINALIZING, nil, "b", "c"),
				insert.InvocationWithInclusions("b", pb.Invocation_ACTIVE, nil),
				insert.InvocationWithInclusions("c", pb.Invocation_FINALIZED, nil),
			)...)

			assertReady("a", false)
		})

		t.Run(`INCLUDES ACTIVE and FINALIZING`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_FINALIZING, nil, "b", "c"),
				insert.InvocationWithInclusions("b", pb.Invocation_ACTIVE, nil),
				insert.InvocationWithInclusions("c", pb.Invocation_FINALIZING, nil),
			)...)

			assertReady("a", false)
		})

		t.Run(`INCLUDES FINALIZING which includes ACTIVE`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_FINALIZING, nil, "b"),
				insert.InvocationWithInclusions("b", pb.Invocation_FINALIZING, nil, "c"),
				insert.InvocationWithInclusions("c", pb.Invocation_ACTIVE, nil),
			)...)

			assertReady("a", false)
		})

		t.Run(`Cycle with one node`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_FINALIZING, nil, "a"),
			)...)

			assertReady("a", true)
		})

		t.Run(`Cycle with two nodes`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_FINALIZING, nil, "b"),
				insert.InvocationWithInclusions("b", pb.Invocation_FINALIZING, nil, "a"),
			)...)

			assertReady("a", true)
		})
	})
}

func TestFinalizeInvocation(t *testing.T) {
	ftt.Run(`FinalizeInvocation`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx, sched := tq.TestingContext(ctx, nil)

		opts := Options{
			ResultDBHostname: "rdb-host",
		}
		t.Run(`Changes the state and finalization time`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.InvocationWithInclusions("x", pb.Invocation_FINALIZING, nil),
			)...)

			err := finalizeInvocation(ctx, "x", opts)
			assert.Loosely(t, err, should.BeNil)

			var state pb.Invocation_State
			var finalizeTime time.Time
			testutil.MustReadRow(ctx, t, "Invocations", invocations.ID("x").Key(), map[string]any{
				"State":        &state,
				"FinalizeTime": &finalizeTime,
			})
			assert.Loosely(t, state, should.Equal(pb.Invocation_FINALIZED))
			assert.Loosely(t, finalizeTime, should.NotResemble(time.Time{}))
		})

		t.Run(`Enqueues more finalizing tasks`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.InvocationWithInclusions("active", pb.Invocation_ACTIVE, nil, "x"),
				insert.InvocationWithInclusions("finalizing1", pb.Invocation_FINALIZING, nil, "x"),
				insert.InvocationWithInclusions("finalizing2", pb.Invocation_FINALIZING, nil, "x"),
				insert.InvocationWithInclusions("x", pb.Invocation_FINALIZING, nil),
			)...)

			err := finalizeInvocation(ctx, "x", opts)
			assert.Loosely(t, err, should.BeNil)

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
			assert.Loosely(t, invocations, should.Match([]string{"finalizing1", "finalizing2"}))
		})

		t.Run(`Enqueues a finalization notification and update test metadata tasks`, func(t *ftt.Test) {
			createTime := timestamppb.New(time.Unix(1000, 0))
			testutil.MustApply(ctx, t,
				insert.Invocation("x", pb.Invocation_FINALIZING, map[string]any{
					"Realm":        "myproject:myrealm",
					"IsExportRoot": true,
					"CreateTime":   createTime,
				}),
			)

			err := finalizeInvocation(ctx, "x", opts)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, sched.Tasks().Payloads(), should.HaveLength(4))
			assert.Loosely(t, sched.Tasks().Payloads()[0], should.Match(&taskspb.ExportInvocationToBQ{
				InvocationId: "x",
			}))
			assert.Loosely(t, sched.Tasks().Payloads()[1], should.Match(&taskspb.ExportArtifacts{
				InvocationId: "x",
			}))
			assert.Loosely(t, sched.Tasks().Payloads()[2], should.Match(&taskspb.UpdateTestMetadata{
				InvocationId: "x",
			}))
			// Enqueued pub/sub notification.
			assert.Loosely(t, sched.Tasks().Payloads()[3], should.Match(&taskspb.NotifyInvocationFinalized{
				Message: &pb.InvocationFinalizedNotification{
					Invocation:   "invocations/x",
					Realm:        "myproject:myrealm",
					IsExportRoot: true,
					ResultdbHost: "rdb-host",
					CreateTime:   createTime,
				},
			}))
		})

		t.Run(`Enqueues more bq_export tasks`, func(t *ftt.Test) {
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
			testutil.MustApply(ctx, t,
				insert.Invocation("x", pb.Invocation_FINALIZING, map[string]any{
					"BigQueryExports": [][]byte{bq1, bq2},
				}),
			)

			err := finalizeInvocation(ctx, "x", opts)
			assert.Loosely(t, err, should.BeNil)
			// Enqueued TQ tasks.
			assert.Loosely(t, sched.Tasks().Payloads()[0], should.Match(
				&taskspb.ExportInvocationToBQ{
					InvocationId: "x",
				}))
			assert.Loosely(t, sched.Tasks().Payloads()[1], should.Match(
				&taskspb.ExportInvocationArtifactsToBQ{
					InvocationId: "x",
					BqExport: &pb.BigQueryExport{
						Dataset:    "dataset",
						Project:    "project2",
						Table:      "table1",
						ResultType: &pb.BigQueryExport_TextArtifacts_{},
					},
				}))
			assert.Loosely(t, sched.Tasks().Payloads()[2], should.Match(
				&taskspb.ExportInvocationTestResultsToBQ{
					InvocationId: "x",
					BqExport: &pb.BigQueryExport{
						Dataset:    "dataset",
						Project:    "project",
						Table:      "table",
						ResultType: &pb.BigQueryExport_TestResults_{},
					},
				}))
		})

		t.Run(`Enqueue mark submitted tasks`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.InvocationWithInclusions("x", pb.Invocation_FINALIZING, map[string]any{
					"Submitted": true,
				}),
			)...)

			err := finalizeInvocation(ctx, "x", opts)
			assert.Loosely(t, err, should.BeNil)
			// there should be two tasks ahead, test metadata and notify finalized.
			assert.Loosely(t, sched.Tasks().Payloads()[0], should.Match(
				&taskspb.MarkInvocationSubmitted{
					InvocationId: "x",
				}))
		})

		t.Run("For a root invocation", func(t *ftt.Test) {
			rootInvID := rootinvocations.ID("root-inv")
			realm := "myproject:myrealm"
			createTime := time.Unix(1000, 0)

			// Create a root invocation and its legacy shadow.
			// The root invocation needs to be in FINALIZING state.
			// The root invocation root work unit will also be in FINALIZING state.
			row := rootinvocations.NewBuilder(rootInvID).
				WithRealm(realm).
				WithCreateTime(createTime).
				WithState(pb.RootInvocation_FINALIZING).Build()
			testutil.MustApply(ctx, t, insert.RootInvocationWithRootWorkUnit(row)...)

			err := finalizeInvocation(ctx, rootInvID.LegacyInvocationID(), opts)
			assert.Loosely(t, err, should.BeNil)

			// Check RootInvocations table.
			var state pb.RootInvocation_State
			var finalizeTime spanner.NullTime
			testutil.MustReadRow(ctx, t, "RootInvocations", rootInvID.Key(), map[string]any{
				"State":        &state,
				"FinalizeTime": &finalizeTime,
			})
			assert.Loosely(t, state, should.Equal(pb.RootInvocation_FINALIZED))
			assert.Loosely(t, finalizeTime.Valid, should.BeTrue)

			// Check Invocations table.
			var legacyState pb.Invocation_State
			var legacyFinalizeTime spanner.NullTime
			testutil.MustReadRow(ctx, t, "Invocations", rootInvID.LegacyInvocationID().Key(), map[string]any{
				"State":        &legacyState,
				"FinalizeTime": &legacyFinalizeTime,
			})
			assert.Loosely(t, legacyState, should.Equal(pb.Invocation_FINALIZED))
			assert.Loosely(t, legacyFinalizeTime, should.Equal(finalizeTime))

			assert.Loosely(t, sched.Tasks().Payloads(), should.HaveLength(1))
			// Enqueued pub/sub notification.
			assert.Loosely(t, sched.Tasks().Payloads()[0], should.Match(&taskspb.NotifyRootInvocationFinalized{
				Message: &pb.RootInvocationFinalizedNotification{
					RootInvocation: &pb.RootInvocationInfo{
						Name:       rootInvID.Name(),
						Realm:      realm,
						CreateTime: timestamppb.New(createTime),
					},
					ResultdbHost: "rdb-host",
				},
			}))
		})

		t.Run("For a work unit", func(t *ftt.Test) {
			var ms []*spanner.Mutation

			// Create a root invocation for the work unit to be created in.
			rootInvID := rootinvocations.ID("root-inv-for-wu")
			rootRow := rootinvocations.NewBuilder(rootInvID).WithState(pb.RootInvocation_FINALIZING).Build()
			ms = append(ms, insert.RootInvocationWithRootWorkUnit(rootRow)...)

			wuID := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "my-work-unit"}

			// Create a work unit.
			// The work unit needs to be in FINALIZING state.
			wuRow := workunits.NewBuilder(rootInvID, "my-work-unit").WithState(pb.WorkUnit_FINALIZING).Build()
			ms = append(ms, insert.WorkUnit(wuRow)...)
			testutil.MustApply(ctx, t, ms...)

			err := finalizeInvocation(ctx, wuID.LegacyInvocationID(), opts)
			assert.Loosely(t, err, should.BeNil)

			// Check WorkUnits table.
			var state pb.WorkUnit_State
			var finalizeTime spanner.NullTime
			testutil.MustReadRow(ctx, t, "WorkUnits", wuID.Key(), map[string]any{
				"State":        &state,
				"FinalizeTime": &finalizeTime,
			})
			assert.Loosely(t, state, should.Equal(pb.WorkUnit_FINALIZED))
			assert.Loosely(t, finalizeTime.Valid, should.BeTrue)

			// Check Invocations table.
			var legacyState pb.Invocation_State
			var legacyFinalizeTime spanner.NullTime
			testutil.MustReadRow(ctx, t, "Invocations", wuID.LegacyInvocationID().Key(), map[string]any{
				"State":        &legacyState,
				"FinalizeTime": &legacyFinalizeTime,
			})
			assert.Loosely(t, legacyState, should.Equal(pb.Invocation_FINALIZED))
			assert.Loosely(t, legacyFinalizeTime, should.Match(finalizeTime))

			assert.Loosely(t, sched.Tasks().Payloads(), should.HaveLength(1))
			assert.Loosely(t, sched.Tasks().Payloads()[0], should.Match(
				&taskspb.TryFinalizeInvocation{
					InvocationId: string(workunits.ID{"root-inv-for-wu", "root"}.LegacyInvocationID()),
				},
			))
		})
	})
}
