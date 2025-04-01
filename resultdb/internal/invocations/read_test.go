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

package invocations

import (
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/instructionutil"
	"go.chromium.org/luci/resultdb/internal/invocations/invocationspb"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestRead(t *testing.T) {
	ctx := testutil.SpannerTestContext(t)
	start := testclock.TestRecentTimeUTC

	properties := &structpb.Struct{
		Fields: map[string]*structpb.Value{
			"key_1": structpb.NewStringValue("value_1"),
			"key_2": structpb.NewStructValue(&structpb.Struct{
				Fields: map[string]*structpb.Value{
					"child_key": structpb.NewNumberValue(1),
				},
			}),
		},
	}
	sources := &pb.Sources{
		GitilesCommit: &pb.GitilesCommit{
			Host:       "chromium.googlesource.com",
			Project:    "infra/infra",
			Ref:        "refs/heads/main",
			CommitHash: "1234567890abcdefabcd1234567890abcdefabcd",
			Position:   567,
		},
		Changelists: []*pb.GerritChange{
			{
				Host:     "chromium-review.googlesource.com",
				Project:  "infra/luci-go",
				Change:   12345,
				Patchset: 321,
			},
		},
		IsDirty: true,
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
						Dependencies: []*pb.InstructionDependency{
							{
								InvocationId:  "dep_inv_id",
								InstructionId: "dep_ins_id",
							},
						},
					},
				},
			},
			{
				Id:   "test",
				Type: pb.InstructionType_TEST_RESULT_INSTRUCTION,
				TargetedInstructions: []*pb.TargetedInstruction{
					{
						Targets: []pb.InstructionTarget{
							pb.InstructionTarget_LOCAL,
							pb.InstructionTarget_REMOTE,
						},
						Content: "test instruction",
						Dependencies: []*pb.InstructionDependency{
							{
								InvocationId:  "dep_inv_id",
								InstructionId: "dep_ins_id",
							},
						},
					},
				},
				InstructionFilter: &pb.InstructionFilter{
					FilterType: &pb.InstructionFilter_InvocationIds{
						InvocationIds: &pb.InstructionFilterByInvocationID{
							InvocationIds: []string{"swarming_task_1"},
						},
					},
				},
			},
		},
	}

	extendedProperties := testutil.TestInvocationExtendedProperties()
	internalExtendedProperties := &invocationspb.ExtendedProperties{
		ExtendedProperties: extendedProperties,
	}

	// Insert some Invocations.
	testutil.MustApply(ctx, t,
		insertInvocation("including", map[string]any{
			"State":              pb.Invocation_ACTIVE,
			"CreateTime":         start,
			"Deadline":           start.Add(time.Hour),
			"IsExportRoot":       spanner.NullBool{Bool: true, Valid: true},
			"Properties":         spanutil.Compressed(pbutil.MustMarshal(properties)),
			"Sources":            spanutil.Compressed(pbutil.MustMarshal(sources)),
			"InheritSources":     spanner.NullBool{Bool: true, Valid: true},
			"IsSourceSpecFinal":  spanner.NullBool{Bool: true, Valid: true},
			"BaselineId":         "try:linux-rel",
			"Instructions":       spanutil.Compressed(pbutil.MustMarshal(instructions)),
			"ExtendedProperties": spanutil.Compressed(pbutil.MustMarshal(internalExtendedProperties)),
		}),
		insertInvocation("included0", nil),
		insertInvocation("included1", nil),
		insertInclusion("including", "included0"),
		insertInclusion("including", "included1"),
	)

	ftt.Run(`TestReadAll`, t, func(t *ftt.Test) {
		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		// Fetch back the top-level Invocation.
		inv, err := Read(ctx, "including", AllFields)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, inv, should.Match(&pb.Invocation{
			Name:                "invocations/including",
			State:               pb.Invocation_ACTIVE,
			CreateTime:          pbutil.MustTimestampProto(start),
			Deadline:            pbutil.MustTimestampProto(start.Add(time.Hour)),
			IsExportRoot:        true,
			IncludedInvocations: []string{"invocations/included0", "invocations/included1"},
			Properties:          properties,
			SourceSpec: &pb.SourceSpec{
				Inherit: true,
				Sources: sources,
			},
			IsSourceSpecFinal:  true,
			BaselineId:         "try:linux-rel",
			Instructions:       instructionutil.InstructionsWithNames(instructions, "including"),
			ExtendedProperties: extendedProperties,
		}))
	})

	ftt.Run(`TestReadExcludeExtendedProperties`, t, func(t *ftt.Test) {
		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		// Fetch back the top-level Invocation.
		inv, err := Read(ctx, "including", ExcludeExtendedProperties)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, inv, should.Match(&pb.Invocation{
			Name:                "invocations/including",
			State:               pb.Invocation_ACTIVE,
			CreateTime:          pbutil.MustTimestampProto(start),
			Deadline:            pbutil.MustTimestampProto(start.Add(time.Hour)),
			IsExportRoot:        true,
			IncludedInvocations: []string{"invocations/included0", "invocations/included1"},
			Properties:          properties,
			SourceSpec: &pb.SourceSpec{
				Inherit: true,
				Sources: sources,
			},
			IsSourceSpecFinal: true,
			BaselineId:        "try:linux-rel",
			Instructions:      instructionutil.InstructionsWithNames(instructions, "including"),
		}))
		// Double check the ExtendedProperties is nil
		assert.Loosely(t, inv.ExtendedProperties, should.BeNil)
	})
}

func TestReadBatch(t *testing.T) {
	ftt.Run(`TestReadBatch`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		testutil.MustApply(ctx, t,
			insertInvocation("inv0", nil),
			insertInvocation("inv1", nil),
			insertInvocation("inv2", nil),
		)

		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		t.Run(`One name`, func(t *ftt.Test) {
			invs, err := ReadBatch(ctx, NewIDSet("inv1"), AllFields)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, invs, should.HaveLength(1))
			assert.Loosely(t, invs, should.ContainKey(ID("inv1")))
			assert.Loosely(t, invs["inv1"].Name, should.Equal("invocations/inv1"))
			assert.Loosely(t, invs["inv1"].State, should.Equal(pb.Invocation_FINALIZED))
		})

		t.Run(`Two names`, func(t *ftt.Test) {
			invs, err := ReadBatch(ctx, NewIDSet("inv0", "inv1"), AllFields)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, invs, should.HaveLength(2))
			assert.Loosely(t, invs, should.ContainKey(ID("inv0")))
			assert.Loosely(t, invs, should.ContainKey(ID("inv1")))
			assert.Loosely(t, invs["inv0"].Name, should.Equal("invocations/inv0"))
			assert.Loosely(t, invs["inv0"].State, should.Equal(pb.Invocation_FINALIZED))
		})

		t.Run(`Not found`, func(t *ftt.Test) {
			_, err := ReadBatch(ctx, NewIDSet("inv0", "x"), AllFields)
			assert.Loosely(t, err, should.ErrLike(`invocations/x not found`))
		})
	})
}

func TestQueryRealms(t *testing.T) {
	ftt.Run(`TestQueryRealms`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		t.Run(`Works`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insertInvocation("inv0", map[string]any{"Realm": "0"}),
				insertInvocation("inv1", map[string]any{"Realm": "1"}),
				insertInvocation("inv2", map[string]any{"Realm": "2"}),
			)

			realms, err := QueryRealms(span.Single(ctx), NewIDSet("inv0", "inv1", "inv2"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, realms, should.Match(map[ID]string{
				"inv0": "0",
				"inv1": "1",
				"inv2": "2",
			}))
		})
		t.Run(`Valid with missing invocation `, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insertInvocation("inv0", map[string]any{"Realm": "0"}),
			)

			realms, err := QueryRealms(span.Single(ctx), NewIDSet("inv0", "inv1"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, realms, should.Match(map[ID]string{
				"inv0": "0",
			}))
		})
	})
}

func TestReadRealms(t *testing.T) {
	ftt.Run(`TestReadRealms`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		t.Run(`Works`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insertInvocation("inv0", map[string]any{"Realm": "0"}),
				insertInvocation("inv1", map[string]any{"Realm": "1"}),
				insertInvocation("inv2", map[string]any{"Realm": "2"}),
			)

			realms, err := ReadRealms(span.Single(ctx), NewIDSet("inv0", "inv1", "inv2"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, realms, should.Match(map[ID]string{
				"inv0": "0",
				"inv1": "1",
				"inv2": "2",
			}))
		})
		t.Run(`NotFound`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insertInvocation("inv0", map[string]any{"Realm": "0"}),
			)

			_, err := ReadRealms(span.Single(ctx), NewIDSet("inv0", "inv1"))
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.NotFound))
			assert.That(t, err, should.ErrLike("invocations/inv1 not found"))
		})
	})
}

func TestReadSubmitted(t *testing.T) {
	ftt.Run(`TestReadSubmitted`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		t.Run(`Valid`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insertInvocation("inv0", map[string]any{"Submitted": true}),
			)

			submitted, err := ReadSubmitted(span.Single(ctx), ID("inv0"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, submitted, should.BeTrue)
		})

		t.Run(`Not Found`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insertInvocation("inv0", map[string]any{"Submitted": true}),
			)
			_, err := ReadSubmitted(span.Single(ctx), ID("inv1"))

			as, ok := appstatus.Get(err)
			assert.That(t, ok, should.BeTrue)
			assert.That(t, as.Code(), should.Equal(codes.NotFound))
			assert.That(t, as.Message(), should.ContainSubstring("invocations/inv1 not found"))
		})

		t.Run(`Nil`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insertInvocation("inv0", map[string]any{}),
			)
			submitted, err := ReadSubmitted(span.Single(ctx), ID("inv0"))

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, submitted, should.BeFalse)
		})
	})
}
