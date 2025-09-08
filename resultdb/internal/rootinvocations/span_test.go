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
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestWriteRootInvocation(t *testing.T) {
	ftt.Run("WriteRootInvocation", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		now := testclock.TestRecentTimeUTC
		ctx, _ = testclock.UseTime(ctx, now)

		id := "root-inv-id"
		row := NewBuilder("root-inv-id").WithState(pb.RootInvocation_ACTIVE).Build()

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
			State:            pb.Invocation_State(row.State),
			Realm:            row.Realm,
			Deadline:         pbutil.MustTimestampProto(row.Deadline),
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
			IsSourceSpecFinal:      row.IsSourcesFinal,
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
			var state pb.RootInvocation_State
			var realm string
			var createTime time.Time
			var sourcesCmp spanutil.Compressed
			var sourcesFinal bool
			err := spanutil.ReadRow(ctx, "RootInvocationShards", shardID.Key(), map[string]any{
				"ShardIndex":       &shardIndex,
				"RootInvocationId": &rootInvID,
				"State":            &state,
				"Realm":            &realm,
				"CreateTime":       &createTime,
				"Sources":          &sourcesCmp,
				"IsSourcesFinal":   &sourcesFinal,
			})
			assert.Loosely(t, err, should.BeNil)

			sources := &pb.Sources{}
			if err := proto.Unmarshal(sourcesCmp, sources); err != nil {
				assert.Loosely(t, err, should.BeNil)
			}

			assert.Loosely(t, shardIndex, should.Equal(i))
			assert.That(t, rootInvID, should.Equal(ID(id)))
			assert.That(t, state, should.Equal(row.State))
			assert.That(t, realm, should.Equal(row.Realm))
			assert.That(t, createTime, should.Match(commitTime))
			assert.That(t, sources, should.Match(row.Sources))
			assert.That(t, sourcesFinal, should.Equal(row.IsSourcesFinal))
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
		rootInvocation := NewBuilder(rootInvocationID).WithState(pb.RootInvocation_ACTIVE).Build()
		testutil.MustApply(ctx, t, InsertForTesting(rootInvocation)...)

		t.Run(`MarkFinalizing`, func(t *ftt.Test) {
			ct, err := span.Apply(ctx, MarkFinalizing(rootInvocationID))
			assert.Loosely(t, err, should.BeNil)

			expected := rootInvocation.Clone()
			expected.State = pb.RootInvocation_FINALIZING
			expected.LastUpdated = ct.In(time.UTC)
			expected.FinalizeStartTime = spanner.NullTime{Time: ct.In(time.UTC), Valid: true}

			ri, err := Read(span.Single(ctx), rootInvocationID)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, ri, should.Match(expected))

			// Check RootInvocationShards state is also updated.
			for i := 0; i < RootInvocationShardCount; i++ {
				var state pb.RootInvocation_State
				shardID := ShardID{RootInvocationID: rootInvocationID, ShardIndex: i}
				err := readColumnsFromShard(span.Single(ctx), shardID, map[string]any{
					"State": &state,
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, state, should.Equal(pb.RootInvocation_FINALIZING))
			}

			// Check the legacy invocation state is also updated.
			state, err := invocations.ReadState(span.Single(ctx), rootInvocationID.LegacyInvocationID())
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, state, should.Equal(pb.Invocation_FINALIZING))
		})
		t.Run(`MarkFinalized`, func(t *ftt.Test) {
			ct, err := span.Apply(ctx, MarkFinalized(rootInvocationID))
			assert.Loosely(t, err, should.BeNil)

			expected := rootInvocation.Clone()
			expected.State = pb.RootInvocation_FINALIZED
			expected.LastUpdated = ct.In(time.UTC)
			expected.FinalizeTime = spanner.NullTime{Time: ct.In(time.UTC), Valid: true}

			ri, err := Read(span.Single(ctx), rootInvocationID)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, ri, should.Match(expected))

			// Check RootInvocationShards state is also updated.
			for i := 0; i < RootInvocationShardCount; i++ {
				var state pb.RootInvocation_State
				shardID := ShardID{RootInvocationID: rootInvocationID, ShardIndex: i}
				err := readColumnsFromShard(span.Single(ctx), shardID, map[string]any{
					"State": &state,
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, state, should.Equal(pb.RootInvocation_FINALIZED))
			}

			// Check the legacy invocation state is also updated.
			state, err := invocations.ReadState(span.Single(ctx), rootInvocationID.LegacyInvocationID())
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, state, should.Equal(pb.Invocation_FINALIZED))
		})
	})
}
