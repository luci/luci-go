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
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestWriteWorkUnit(t *testing.T) {
	ftt.Run("TestWriteWorkUnit", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		now := testclock.TestRecentTimeUTC
		ctx, _ = testclock.UseTime(ctx, now)

		properties := &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"key": structpb.NewStringValue("value"),
			},
		}
		instructions := &pb.Instructions{
			Instructions: []*pb.Instruction{
				{
					Id:   "step",
					Type: pb.InstructionType_STEP_INSTRUCTION,
					TargetedInstructions: []*pb.TargetedInstruction{
						{
							Targets: []pb.InstructionTarget{
								pb.InstructionTarget_LOCAL,
								pb.InstructionTarget_REMOTE,
							},
							Content: "step instruction",
						},
					},
				},
			},
		}
		extendedProperties := map[string]*structpb.Struct{
			"mykey": {
				Fields: map[string]*structpb.Value{
					"@type":       structpb.NewStringValue("foo.bar.com/x/_some.package.MyMessage"),
					"child_key_1": structpb.NewStringValue("child_value_1"),
				},
			},
		}
		id := ID{
			RootInvocationID: "root-inv-id",
			WorkUnitID:       "work-unit-id",
		}
		row := &WorkUnitRow{
			ID:                 id,
			State:              pb.WorkUnit_ACTIVE,
			Realm:              "testproject:testrealm",
			CreatedBy:          "user:test@example.com",
			Deadline:           now.Add(2 * 24 * time.Hour),
			CreateRequestID:    "test-request-id",
			ProducerResource:   "//builds.example.com/builds/123",
			Tags:               pbutil.StringPairs("k2", "v2", "k1", "v1"),
			Properties:         properties,
			Instructions:       instructions,
			ExtendedProperties: extendedProperties,
		}

		LegacyCreateOptions := LegacyCreateOptions{
			ExpectedTestResultsExpirationTime: now.Add(2 * 24 * time.Hour),
		}

		// Insert rows in the parent RootInvocationShards table.
		testutil.MustApply(
			ctx, t,
			insert.RootInvocationAndShards(rootinvocations.ID("root-inv-id"))...,
		)
		commitTime, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			mutations := Create(row, LegacyCreateOptions)
			span.BufferWrite(ctx, mutations...)
			return nil
		})
		assert.Loosely(t, err, should.BeNil)

		// Validation
		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		// Validate WorkUnits table entry.
		readWorkUnit, err := Read(ctx, id)
		assert.Loosely(t, err, should.BeNil)
		row.CreateTime = commitTime
		row.SecondaryIndexShardID = id.shardID(secondaryIndexShardCount)
		row.Instructions.Instructions[0].Name = "rootInvocations/root-inv-id/workUnits/work-unit-id/instructions/step"
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
	})

}
