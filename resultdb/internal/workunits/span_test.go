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

package workunits

import (
	"context"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestWriteWorkUnit(t *testing.T) {
	ftt.Run("TestWriteWorkUnit", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		now := testclock.TestRecentTimeUTC
		ctx, _ = testclock.UseTime(ctx, now)

		id := ID{
			RootInvocationID: "root-inv-id",
			WorkUnitID:       "work-unit-id",
		}
		parentID := ID{
			RootInvocationID: "root-inv-id",
			WorkUnitID:       "root",
		}
		row := NewBuilder("root-inv-id", "work-unit-id").
			WithFinalizationState(pb.WorkUnit_ACTIVE).
			WithParentWorkUnitID(parentID.WorkUnitID).
			Build()
		parentRow := NewBuilder("root-inv-id", "root").WithFinalizationState(pb.WorkUnit_ACTIVE).Build()

		LegacyCreateOptions := LegacyCreateOptions{
			ExpectedTestResultsExpirationTime: now.Add(2 * 24 * time.Hour),
		}

		// Insert rows in the parent RootInvocationShards table.
		testutil.MustApply(
			ctx, t,
			rootinvocations.InsertForTesting(rootinvocations.NewBuilder("root-inv-id").Build())...,
		)
		commitTime, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			mutations := Create(parentRow.Clone(), LegacyCreateOptions)
			mutations = append(mutations, Create(row.Clone(), LegacyCreateOptions)...)
			span.BufferWrite(ctx, mutations...)
			return nil
		})
		assert.Loosely(t, err, should.BeNil)

		// Validation
		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		// Validate WorkUnits table entry.
		readWorkUnit, err := Read(ctx, id, AllFields)
		assert.Loosely(t, err, should.BeNil)
		row.CreateTime = commitTime.In(time.UTC)
		row.LastUpdated = commitTime.In(time.UTC)
		row.SecondaryIndexShardID = id.shardID(secondaryIndexShardCount)
		assert.Loosely(t, readWorkUnit, should.Match(row))

		// Validate the parent work unit has a child work units entry.
		rootWorkUnit, err := Read(ctx, parentID, AllFields)
		assert.Loosely(t, err, should.BeNil)
		assert.That(t, rootWorkUnit.ChildWorkUnits, should.Match([]ID{id}))

		// Validate Legacy Invocations table entry.
		legacyInvID := invocations.ID("workunit:root-inv-id:work-unit-id")
		readLegacyInv, err := invocations.Read(ctx, legacyInvID, invocations.AllFields)
		assert.Loosely(t, err, should.BeNil)

		expectedLegacyInv := &pb.Invocation{
			Name:                   legacyInvID.Name(),
			State:                  pb.Invocation_State(row.FinalizationState),
			Realm:                  row.Realm,
			Deadline:               pbutil.MustTimestampProto(row.Deadline),
			CreateTime:             timestamppb.New(commitTime),
			CreatedBy:              row.CreatedBy,
			ModuleId:               row.ModuleID,
			Tags:                   row.Tags,
			Properties:             row.Properties,
			SourceSpec:             &pb.SourceSpec{Inherit: true},
			IsSourceSpecFinal:      true,
			Instructions:           row.Instructions,
			ExtendedProperties:     row.ExtendedProperties,
			IsExportRoot:           false,
			TestResultVariantUnion: row.ModuleID.ModuleVariant,
		}
		expectedLegacyInv.Instructions.Instructions[0].Name = "invocations/workunit:root-inv-id:work-unit-id/instructions/step"

		assert.That(t, readLegacyInv, should.Match(expectedLegacyInv))
		var legacyCreateRequestID spanner.NullString
		var invocationType int64
		var expectedTestResultsExpirationTime spanner.NullTime

		var submitted spanner.NullBool
		// Also validate fields not returned by invocations.Read().
		err = invocations.ReadColumns(ctx, legacyInvID, map[string]any{
			"Type":                              &invocationType,
			"CreateRequestId":                   &legacyCreateRequestID,
			"ExpectedTestResultsExpirationTime": &expectedTestResultsExpirationTime,
			"Submitted":                         &submitted,
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, legacyCreateRequestID.StringVal, should.Equal(row.CreateRequestID))
		assert.Loosely(t, invocationType, should.Equal(invocations.WorkUnit))
		assert.Loosely(t, expectedTestResultsExpirationTime.Time, should.Match(LegacyCreateOptions.ExpectedTestResultsExpirationTime))
		assert.Loosely(t, submitted.Valid, should.BeFalse)

		// Validate IncludedInvocations entry.
		includedIDs, err := invocations.ReadIncluded(ctx, parentID.LegacyInvocationID())
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, includedIDs, should.HaveLength(1))
		assert.That(t, includedIDs.Has(id.LegacyInvocationID()), should.BeTrue)

		includedIDs, err = invocations.ReadIncluded(ctx, parentID.RootInvocationID.LegacyInvocationID())
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, includedIDs, should.HaveLength(1))
		assert.That(t, includedIDs.Has(parentID.LegacyInvocationID()), should.BeTrue)
	})
}

func TestFinalizationMethods(t *testing.T) {
	ftt.Run("TestFinalizationMethods", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		const rootInvocationID = "root-inv-id"

		// Create a root invocation.
		var ms []*spanner.Mutation
		ms = append(ms, rootinvocations.InsertForTesting(rootinvocations.NewBuilder(rootInvocationID).Build())...)
		ms = append(ms, InsertForTesting(NewBuilder(rootInvocationID, "root").Build())...)

		// Create a work unit.
		id := ID{
			RootInvocationID: rootinvocations.ID(rootInvocationID),
			WorkUnitID:       "work-unit-id",
		}
		workUnit := NewBuilder(rootInvocationID, "work-unit-id").WithFinalizationState(pb.WorkUnit_ACTIVE).Build()
		ms = append(ms, InsertForTesting(workUnit)...)

		// Insert rows in the parent RootInvocationShards table.
		testutil.MustApply(ctx, t, ms...)

		t.Run(`MarkFinalized`, func(t *ftt.Test) {
			ct, err := span.Apply(ctx, MarkFinalized(id))
			assert.Loosely(t, err, should.BeNil)

			expectedWU := workUnit.Clone()
			expectedWU.FinalizationState = pb.WorkUnit_FINALIZED
			expectedWU.LastUpdated = ct.In(time.UTC)
			expectedWU.FinalizeTime = spanner.NullTime{Time: ct.In(time.UTC), Valid: true}

			wu, err := Read(span.Single(ctx), id, AllFields)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, wu, should.Match(expectedWU))

			// Check the legacy invocation state is also updated.
			state, err := invocations.ReadState(span.Single(ctx), id.LegacyInvocationID())
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, state, should.Equal(pb.Invocation_FINALIZED))
		})
	})
}

func TestMutationBuilder(t *testing.T) {
	ftt.Run("With mutation builder", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		rootInvID := rootinvocations.ID("root-inv-id")
		// Create a root invocation and root work unit.
		var ms []*spanner.Mutation
		rootInv := rootinvocations.NewBuilder(rootInvID).WithState(pb.RootInvocation_RUNNING).WithFinalizationState(pb.RootInvocation_ACTIVE).Build()
		ms = append(ms, rootinvocations.InsertForTesting(rootInv)...)

		// Create a work unit.
		id := ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "root",
		}
		workUnit := NewBuilder(rootInvID, "root").WithState(pb.WorkUnit_RUNNING).WithFinalizationState(pb.WorkUnit_ACTIVE).Build()
		ms = append(ms, InsertForTesting(workUnit)...)
		testutil.MustApply(ctx, t, ms...)

		b := NewMutationBuilder(id)

		t.Run("UpdateState", func(t *ftt.Test) {
			b.UpdateState(pb.WorkUnit_FAILED)
			ct, err := span.Apply(ctx, b.Build())
			assert.Loosely(t, err, should.BeNil)

			// Check the work unit.
			readWU, err := Read(span.Single(ctx), id, AllFields)
			assert.Loosely(t, err, should.BeNil)
			expectedWU := workUnit.Clone()
			expectedWU.State = pb.WorkUnit_FAILED
			expectedWU.FinalizationState = pb.WorkUnit_FINALIZING
			expectedWU.LastUpdated = ct.In(time.UTC)
			expectedWU.FinalizeStartTime = spanner.NullTime{Time: ct.In(time.UTC), Valid: true}
			expectedWU.FinalizerCandidateTime = spanner.NullTime{Time: ct.In(time.UTC), Valid: true}
			assert.That(t, readWU, should.Match(expectedWU))

			// Check the root invocation, as we updated the root work unit.
			readRootInv, err := rootinvocations.Read(span.Single(ctx), rootInvID)
			assert.Loosely(t, err, should.BeNil)
			expectedRootInv := rootInv.Clone()
			expectedRootInv.State = pb.RootInvocation_FAILED
			expectedRootInv.FinalizationState = pb.RootInvocation_FINALIZING
			expectedRootInv.LastUpdated = ct.In(time.UTC)
			expectedRootInv.FinalizeStartTime = spanner.NullTime{Time: ct.In(time.UTC), Valid: true}
			assert.That(t, readRootInv, should.Match(expectedRootInv))

			// Check the legacy invocation.
			readLegacyInv, err := invocations.Read(span.Single(ctx), id.LegacyInvocationID(), invocations.AllFields)
			assert.Loosely(t, err, should.BeNil)
			expectedLegacyInv := workUnit.ToLegacyInvocationProto()
			expectedLegacyInv.State = pb.Invocation_FINALIZING
			expectedLegacyInv.FinalizeStartTime = timestamppb.New(ct)
			assert.That(t, readLegacyInv, should.Match(expectedLegacyInv))
		})
		t.Run("UpdateSummaryMarkdown", func(t *ftt.Test) {
			b := NewMutationBuilder(id)
			b.UpdateSummaryMarkdown("new summary")
			ct, err := span.Apply(ctx, b.Build())
			assert.Loosely(t, err, should.BeNil)

			// Check the work unit.
			readWU, err := Read(span.Single(ctx), id, AllFields)
			assert.Loosely(t, err, should.BeNil)
			expectedWU := workUnit.Clone()
			expectedWU.SummaryMarkdown = "new summary"
			expectedWU.LastUpdated = ct.In(time.UTC)
			assert.That(t, readWU, should.Match(expectedWU))

			// Check the root invocation.
			readRootInv, err := rootinvocations.Read(span.Single(ctx), rootInvID)
			assert.Loosely(t, err, should.BeNil)
			expectedRootInv := rootInv.Clone()
			expectedRootInv.SummaryMarkdown = "new summary"
			expectedRootInv.LastUpdated = ct.In(time.UTC)
			assert.That(t, readRootInv, should.Match(expectedRootInv))
		})
	})
}

func TestFinalizerCandidateMethods(t *testing.T) {
	ftt.Run("TestFinalizerCandidateMethods", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		const rootInvocationID = "root-inv-id"
		rootwu := NewBuilder(rootInvocationID, "root").Build()

		finalizerCandidateTime := time.Date(2025, time.October, 10, 13, 1, 2, 3, time.UTC)
		wu1 := NewBuilder(rootInvocationID, "work-unit-id").
			WithFinalizationState(pb.WorkUnit_FINALIZING).
			WithFinalizerCandidateTime(finalizerCandidateTime).
			Build()
		testutil.MustApply(ctx, t, testutil.CombineMutations(
			rootinvocations.InsertForTesting(rootinvocations.NewBuilder(rootInvocationID).Build()),
			InsertForTesting(rootwu),
			InsertForTesting(wu1),
		)...)

		t.Run(`SetFinalizerCandidateTime`, func(t *ftt.Test) {
			ct, err := span.Apply(ctx, []*spanner.Mutation{SetFinalizerCandidateTime(wu1.ID)})
			assert.Loosely(t, err, should.BeNil)

			readWU, err := Read(span.Single(ctx), wu1.ID, AllFields)
			assert.Loosely(t, err, should.BeNil)
			expectedWU := wu1.Clone()
			expectedWU.FinalizerCandidateTime = spanner.NullTime{Time: ct.In(time.UTC), Valid: true}
			assert.That(t, readWU, should.Match(expectedWU))
		})
		t.Run(`ResetFinalizerCandidateTime`, func(t *ftt.Test) {
			wu2 := NewBuilder(rootInvocationID, "work-unit-id-2").
				WithFinalizationState(pb.WorkUnit_FINALIZING).
				WithFinalizerCandidateTime(finalizerCandidateTime.Add(time.Millisecond)).
				Build()
			testutil.MustApply(ctx, t, InsertForTesting(wu2)...)
			candidates := []FinalizerCandidate{
				{
					ID:                     wu1.ID,
					FinalizerCandidateTime: finalizerCandidateTime,
				},
				{
					ID:                     wu2.ID,
					FinalizerCandidateTime: finalizerCandidateTime,
				},
			}
			var rowCount int64
			var err error
			_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				rowCount, err = span.Update(ctx, ResetFinalizerCandidateTime(candidates))
				if err != nil {
					return err
				}
				return nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, rowCount, should.Equal(int64(1)))

			// wu1 has been reset.
			wuread, err := Read(span.Single(ctx), wu1.ID, AllFields)
			assert.Loosely(t, err, should.BeNil)
			expectedWU := wu1.Clone()
			expectedWU.FinalizerCandidateTime = spanner.NullTime{Valid: false}
			assert.That(t, wuread, should.Match(expectedWU))

			// wu2 has not been reset.
			wuread, err = Read(span.Single(ctx), wu2.ID, AllFields)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, wuread, should.Match(wu2))
		})
	})
}

func TestWorkUnitUpdateRequestsMethods(t *testing.T) {
	ftt.Run("TestWorkUnitUpdateRequestsMethods", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		rootInvID := rootinvocations.ID("root-inv-id")
		// Create a root invocation and root work unit.
		var ms []*spanner.Mutation
		ms = append(ms, rootinvocations.InsertForTesting(rootinvocations.NewBuilder(rootInvID).Build())...)
		ms = append(ms, InsertForTesting(NewBuilder(rootInvID, "root").Build())...)

		// Create a work unit.
		id := ID{
			RootInvocationID: rootinvocations.ID(rootInvID),
			WorkUnitID:       "work-unit-id",
		}
		workUnit := NewBuilder(rootInvID, "work-unit-id").WithFinalizationState(pb.WorkUnit_ACTIVE).Build()
		ms = append(ms, InsertForTesting(workUnit)...)
		testutil.MustApply(ctx, t, ms...)
		t.Run("CreateWorkUnitUpdateRequest", func(t *ftt.Test) {
			updatedBy := "user:creator@example.com"
			requestID := "req-id-456"

			m := CreateWorkUnitUpdateRequest(id, updatedBy, requestID)
			ct, err := span.Apply(ctx, []*spanner.Mutation{m})
			assert.Loosely(t, err, should.BeNil)
			// Read back the row to confirm it was inserted correctly.
			row, err := span.ReadRow(span.Single(ctx), "WorkUnitUpdateRequests", id.Key(updatedBy, requestID), []string{"CreateTime"})
			assert.Loosely(t, err, should.BeNil)
			var createTime time.Time
			err = row.Column(0, &createTime)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, createTime, should.Match(ct))
		})
	})
}
