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
			WithState(pb.WorkUnit_ACTIVE).
			WithParentWorkUnitID(parentID.WorkUnitID).
			Build()
		parentRow := NewBuilder("root-inv-id", "root").WithState(pb.WorkUnit_ACTIVE).Build()

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
		row.SecondaryIndexShardID = id.shardID(secondaryIndexShardCount)
		assert.Loosely(t, readWorkUnit, should.Match(row))

		// Validate Legacy Invocations table entry.
		legacyInvID := invocations.ID("workunit:root-inv-id:work-unit-id")
		readLegacyInv, err := invocations.Read(ctx, legacyInvID, invocations.AllFields)
		assert.Loosely(t, err, should.BeNil)

		expectedLegacyInv := &pb.Invocation{
			Name:                   legacyInvID.Name(),
			State:                  pb.Invocation_State(row.State),
			Realm:                  row.Realm,
			Deadline:               pbutil.MustTimestampProto(row.Deadline),
			CreateTime:             timestamppb.New(commitTime),
			CreatedBy:              row.CreatedBy,
			Tags:                   row.Tags,
			ProducerResource:       row.ProducerResource,
			Properties:             row.Properties,
			Instructions:           row.Instructions,
			ExtendedProperties:     row.ExtendedProperties,
			IsExportRoot:           false,
			TestResultVariantUnion: &pb.Variant{},
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
		workUnit := NewBuilder(rootInvocationID, "work-unit-id").WithState(pb.WorkUnit_ACTIVE).Build()
		ms = append(ms, InsertForTesting(workUnit)...)

		// Insert rows in the parent RootInvocationShards table.
		testutil.MustApply(ctx, t, ms...)

		t.Run(`MarkFinalizing`, func(t *ftt.Test) {
			ct, err := span.Apply(ctx, MarkFinalizing(id))
			assert.Loosely(t, err, should.BeNil)

			expectedWU := workUnit.Clone()
			expectedWU.State = pb.WorkUnit_FINALIZING
			expectedWU.FinalizeStartTime = spanner.NullTime{Time: ct.In(time.UTC), Valid: true}

			wu, err := Read(span.Single(ctx), id, AllFields)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, wu, should.Match(expectedWU))

			// Check the legacy invocation state is also updated.
			state, err := invocations.ReadState(span.Single(ctx), id.LegacyInvocationID())
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, state, should.Equal(pb.Invocation_FINALIZING))
		})
		t.Run(`MarkFinalized`, func(t *ftt.Test) {
			ct, err := span.Apply(ctx, MarkFinalized(id))
			assert.Loosely(t, err, should.BeNil)

			expectedWU := workUnit.Clone()
			expectedWU.State = pb.WorkUnit_FINALIZED
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
