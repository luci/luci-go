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
	"google.golang.org/protobuf/types/known/structpb"
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

		properties := &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"key": structpb.NewStringValue("value"),
			},
		}
		sources := &pb.Sources{
			GitilesCommit: &pb.GitilesCommit{
				Host:       "chromium.googlesource.com",
				Project:    "chromium/src",
				Ref:        "refs/heads/main",
				CommitHash: "1234567890abcdef1234567890abcdef12345678",
				Position:   123,
			},
		}
		id := "root-inv-id"
		row := &RootInvocationRow{
			RootInvocationID:                        ID(id),
			State:                                   pb.RootInvocation_ACTIVE,
			Realm:                                   "testproject:testrealm",
			CreatedBy:                               "user:test@example.com",
			Deadline:                                now.Add(2 * 24 * time.Hour),
			UninterestingTestVerdictsExpirationTime: spanner.NullTime{Valid: true, Time: now.Add(2 * 24 * time.Hour)},
			CreateRequestID:                         "test-request-id",
			ProducerResource:                        "//builds.example.com/builds/123",
			Tags:                                    pbutil.StringPairs("k2", "v2", "k1", "v1"),
			Properties:                              properties,
			Sources:                                 sources,
			IsSourcesFinal:                          true,
			BaselineID:                              "try:linux-rel",
			Submitted:                               false,
		}
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
		row.SecondaryIndexShardID = row.RootInvocationID.shardID(secondaryIndexShardCount)
		assert.Loosely(t, readRootInv, should.Match(row))

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
		assert.Loosely(t, legacyCreateRequestID.StringVal, should.Equal(row.CreateRequestID))
		assert.Loosely(t, invocationType, should.Equal(invocations.Root))
		assert.Loosely(t, expectedTestResultsExpirationTime.Time, should.Match(row.UninterestingTestVerdictsExpirationTime.Time))
		assert.Loosely(t, submitted, should.Equal(row.Submitted))

		// Validate RootInvocationShards table entries.
		for i := 0; i < RootInvocationShardCount; i++ {
			shardID := ComputeRootInvocationShardID(ID(id), i)
			var shardIndex int64
			var rootInvID ID
			var createTime time.Time
			err := spanutil.ReadRow(ctx, "RootInvocationShards", spanner.Key{shardID}, map[string]any{
				"ShardIndex":       &shardIndex,
				"RootInvocationId": &rootInvID,
				"CreateTime":       &createTime,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, shardIndex, should.Equal(i))
			assert.Loosely(t, createTime, should.Match(commitTime))
			assert.That(t, rootInvID, should.Equal(ID(id)))
		}
	})
}

func TestComputeRootInvocationShardID(t *testing.T) {
	ftt.Run("ComputeRootInvocationShardID", t, func(t *ftt.Test) {
		t.Run(`Works`, func(t *ftt.Test) {
			assert.That(t, ComputeRootInvocationShardID("abc", 0), should.Equal("0a7816bf:abc"))
			assert.That(t, ComputeRootInvocationShardID("abc", 1), should.Equal("1a7816bf:abc"))
			assert.That(t, ComputeRootInvocationShardID("abc", 2), should.Equal("2a7816bf:abc"))
			assert.That(t, ComputeRootInvocationShardID("abc", 3), should.Equal("3a7816bf:abc"))
			assert.That(t, ComputeRootInvocationShardID("abc", 10), should.Equal("aa7816bf:abc"))
			assert.That(t, ComputeRootInvocationShardID("abc", 11), should.Equal("ba7816bf:abc"))
			assert.That(t, ComputeRootInvocationShardID("abc", 15), should.Equal("fa7816bf:abc"))
		})
	})
}
