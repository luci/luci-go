// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package rootinvocations

import (
	"context"
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestWriteRootInvocation(t *testing.T) {
	ftt.Run("WriteRootInvocation", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		now := testclock.TestRecentTimeUTC
		ctx, _ = testclock.UseTime(ctx, now)

		id := "root-inv-id"
		row := NewBuilder("root-inv-id").WithFinalizationState(pb.RootInvocation_ACTIVE).Build()

		commitTime, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			mutations := Create(row)
			span.BufferWrite(ctx, mutations...)
			return nil
		})
		assert.Loosely(t, err, should.BeNil)

		// Validation
		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		// Validate RootInvocations table entry.
		readRootInv, err := Read(ctx, ID(id))
		assert.Loosely(t, err, should.BeNil)
		row.CreateTime = commitTime
		row.LastUpdated = commitTime
		row.SecondaryIndexShardID = row.RootInvocationID.shardID(secondaryIndexShardCount)
		assert.That(t, readRootInv, should.Match(row))

		// Validate Legacy Invocations table entry.
		legacyInvID := invocations.ID(fmt.Sprintf("root:%s", id))
		readLegacyInv, err := invocations.Read(ctx, legacyInvID, invocations.AllFields)
		assert.Loosely(t, err, should.BeNil)

		// The legacy invocation should be a reflection of the root invocation.
		expectedLegacyInv := &pb.Invocation{
			Name:             legacyInvID.Name(),
			State:            pb.Invocation_State(row.FinalizationState),
			Realm:            row.Realm,
			Deadline:         timestamppb.New(time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)),
			CreateTime:       timestamppb.New(commitTime),
			CreatedBy:        row.CreatedBy,
			Tags:             row.Tags,
			ProducerResource: row.ProducerResource,
			Properties:       row.Properties,
			IsExportRoot:     true,
			BaselineId:       row.BaselineID,
			SourceSpec: &pb.SourceSpec{
				Sources: row.Sources,
				Inherit: false,
			},
			IsSourceSpecFinal:      row.StreamingExportState == pb.RootInvocation_METADATA_FINAL,
			TestResultVariantUnion: &pb.Variant{},
		}

		assert.That(t, readLegacyInv, should.Match(expectedLegacyInv))
		var legacyCreateRequestID spanner.NullString
		var invocationType int64
		var expectedTestResultsExpirationTime spanner.NullTime
		var submitted bool
		// Also validate fields not returned by invocations.Read().
		err = invocations.ReadColumns(ctx, legacyInvID, map[string]any{
			"Type":                              &invocationType,
			"CreateRequestId":                   &legacyCreateRequestID,
			"ExpectedTestResultsExpirationTime": &expectedTestResultsExpirationTime,
			"Submitted":                         &submitted,
		})
		assert.Loosely(t, err, should.BeNil)
		assert.That(t, legacyCreateRequestID.StringVal, should.Equal(row.CreateRequestID))
		assert.Loosely(t, invocationType, should.Equal(invocations.Root))
		assert.That(t, expectedTestResultsExpirationTime.Time, should.Match(row.UninterestingTestVerdictsExpirationTime.Time))
		assert.That(t, submitted, should.Equal(row.Submitted))

		// Validate RootInvocationShards table entries.
		for i := 0; i < RootInvocationShardCount; i++ {
			shardID := ShardID{RootInvocationID: ID(id), ShardIndex: i}
			var shardIndex int64
			var rootInvID ID
			var realm string
			var createTime time.Time
			var sourcesCmp spanutil.Compressed
			err := spanutil.ReadRow(ctx, "RootInvocationShards", shardID.Key(), map[string]any{
				"ShardIndex":       &shardIndex,
				"RootInvocationId": &rootInvID,
				"Realm":            &realm,
				"CreateTime":       &createTime,
				"Sources":          &sourcesCmp,
			})
			assert.Loosely(t, err, should.BeNil)

			sources := &pb.Sources{}
			if err := proto.Unmarshal(sourcesCmp, sources); err != nil {
				assert.Loosely(t, err, should.BeNil)
			}

			assert.Loosely(t, shardIndex, should.Equal(i))
			assert.That(t, rootInvID, should.Equal(ID(id)))
			assert.That(t, realm, should.Equal(row.Realm))
			assert.That(t, createTime, should.Match(commitTime))
			assert.That(t, sources, should.Match(row.Sources))
		}
	})
}

func TestEtag(t *testing.T) {
	t.Parallel()
	ftt.Run("TestEtag", t, func(t *ftt.Test) {
		lastUpdatedTime := time.Date(2025, 4, 26, 1, 2, 3, 4000, time.UTC)
		ri := &RootInvocationRow{LastUpdated: lastUpdatedTime}

		t.Run("Etag", func(t *ftt.Test) {
			etag := Etag(ri)
			assert.That(t, etag, should.Equal(`W/"2025-04-26T01:02:03.000004Z"`))
		})

		t.Run("ParseEtag", func(t *ftt.Test) {
			t.Run("valid", func(t *ftt.Test) {
				etag := `W/"2025-04-26T01:02:03.000004Z"`
				lastUpdated, err := ParseEtag(etag)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, lastUpdated, should.Equal("2025-04-26T01:02:03.000004Z"))
			})
			t.Run("malformed", func(t *ftt.Test) {
				etag := `W/+l/"malformed"`
				_, err := ParseEtag(etag)
				assert.Loosely(t, err, should.ErrLike("malformated etag"))
			})
		})

		t.Run("IsEtagMatch", func(t *ftt.Test) {
			t.Run("round-trip", func(t *ftt.Test) {
				etag := Etag(ri)
				match, err := IsEtagMatch(ri, etag)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, match, should.BeTrue)
			})
			t.Run("not match", func(t *ftt.Test) {
				etag := Etag(ri)
				ri.LastUpdated = ri.LastUpdated.Add(time.Second)

				match, err := IsEtagMatch(ri, etag)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, match, should.BeFalse)
			})
		})
	})
}

func TestFinalizationMethods(t *testing.T) {
	ftt.Run("TestFinalizationMethods", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		const rootInvocationID = ID("root-inv-id")

		// Create a root invocation.
		rootInvocation := NewBuilder(rootInvocationID).WithFinalizationState(pb.RootInvocation_ACTIVE).Build()
		testutil.MustApply(ctx, t, InsertForTesting(rootInvocation)...)

		t.Run(`MarkFinalized`, func(t *ftt.Test) {
			ct, err := span.Apply(ctx, MarkFinalized(rootInvocationID))
			assert.Loosely(t, err, should.BeNil)

			expected := rootInvocation.Clone()
			expected.FinalizationState = pb.RootInvocation_FINALIZED
			expected.LastUpdated = ct.In(time.UTC)
			expected.FinalizeTime = spanner.NullTime{Time: ct.In(time.UTC), Valid: true}

			ri, err := Read(span.Single(ctx), rootInvocationID)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, ri, should.Match(expected))

			// Check the legacy invocation state is also updated.
			state, err := invocations.ReadState(span.Single(ctx), rootInvocationID.LegacyInvocationID())
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, state, should.Equal(pb.Invocation_FINALIZED))
		})
	})
}

func TestMutationBuilder(t *testing.T) {
	ftt.Run("With mutation builder", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		id := ID("root-inv-id")
		rootInv := NewBuilder(id).WithState(pb.RootInvocation_RUNNING).WithFinalizationState(pb.RootInvocation_ACTIVE).Build()
		testutil.MustApply(ctx, t, InsertForTesting(rootInv)...)

		b := NewMutationBuilder(id)

		t.Run("UpdateState", func(t *ftt.Test) {
			b.UpdateState(pb.RootInvocation_FAILED)
			ct, err := span.Apply(ctx, b.Build())
			assert.Loosely(t, err, should.BeNil)

			// Check the root invocation.
			readRootInv, err := Read(span.Single(ctx), id)
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
			expectedLegacyInv := rootInv.ToLegacyInvocationProto()
			expectedLegacyInv.State = pb.Invocation_FINALIZING
			expectedLegacyInv.FinalizeStartTime = timestamppb.New(ct)
			expectedLegacyInv.CreateTime = timestamppb.New(rootInv.CreateTime)
			assert.That(t, readLegacyInv, should.Match(expectedLegacyInv))
		})
		t.Run("UpdateSummaryMarkdown", func(t *ftt.Test) {
			b := NewMutationBuilder(id)
			b.UpdateSummaryMarkdown("new summary")
			ct, err := span.Apply(ctx, b.Build())
			assert.Loosely(t, err, should.BeNil)

			// Check the root invocation.
			readRootInv, err := Read(span.Single(ctx), id)
			assert.Loosely(t, err, should.BeNil)
			expectedRootInv := rootInv.Clone()
			expectedRootInv.SummaryMarkdown = "new summary"
			expectedRootInv.LastUpdated = ct.In(time.UTC)
			assert.That(t, readRootInv, should.Match(expectedRootInv))
		})
	})
}

func TestFinalizerTaskStateMethods(t *testing.T) {
	ftt.Run("TestFinalizerTaskStateMethods", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		const rootInvocationID = ID("root-inv-id")
		// Create a root invocation.
		rootInvocation := NewBuilder(rootInvocationID).
			WithFinalizationState(pb.RootInvocation_ACTIVE).
			WithFinalizerPending(false).
			WithFinalizerSequence(1).
			Build()
		testutil.MustApply(ctx, t, InsertForTesting(rootInvocation)...)

		t.Run(`SetFinalizerPending`, func(t *ftt.Test) {
			newSeq := int64(123)
			_, err := span.Apply(ctx, []*spanner.Mutation{SetFinalizerPending(rootInvocationID, newSeq)})
			assert.Loosely(t, err, should.BeNil)

			taskState, err := ReadFinalizerTaskState(span.Single(ctx), rootInvocationID)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, taskState.Pending, should.BeTrue)
			assert.That(t, taskState.Sequence, should.Equal(newSeq))

			t.Run(`ResetFinalizerPending`, func(t *ftt.Test) {
				_, err := span.Apply(ctx, []*spanner.Mutation{ResetFinalizerPending(rootInvocationID)})
				assert.Loosely(t, err, should.BeNil)

				taskState, err := ReadFinalizerTaskState(span.Single(ctx), rootInvocationID)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, taskState.Pending, should.BeFalse)
				assert.That(t, taskState.Sequence, should.Equal(newSeq))
			})
		})
	})
}

func TestCreateRootInvocationUpdateRequest(t *testing.T) {
	ftt.Run("TestCreateRootInvocationUpdateRequest", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		rootInvID := ID("root-inv-id")
		updatedBy := "user:someone@example.com"
		requestID := "test-request-id"

		// Create a root invocation.
		rootInvocation := NewBuilder(rootInvID).WithFinalizationState(pb.RootInvocation_ACTIVE).Build()
		testutil.MustApply(ctx, t, InsertForTesting(rootInvocation)...)

		t.Run(`ReadRootInvocationUpdateRequest`, func(t *ftt.Test) {
			m := CreateRootInvocationUpdateRequest(rootInvID, updatedBy, requestID)
			ct, err := span.Apply(ctx, []*spanner.Mutation{m})
			assert.Loosely(t, err, should.BeNil)
			// Read back the row to confirm it was inserted correctly.
			var readCreateTime time.Time
			err = spanutil.ReadRow(span.Single(ctx), "RootInvocationUpdateRequests", rootInvID.Key(updatedBy, requestID), map[string]any{
				"CreateTime": &readCreateTime,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, readCreateTime, should.Match(ct))
		})
	})
}
