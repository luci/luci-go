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
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRead(t *testing.T) {
	Convey(`Read`, t, func() {
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

		testInstruction := &pb.Instruction{
			TargetedInstructions: []*pb.TargetedInstruction{
				{
					Targets: []pb.InstructionTarget{
						pb.InstructionTarget_LOCAL,
						pb.InstructionTarget_REMOTE,
					},
					Content: "test instruction",
					Dependency: []*pb.InstructionDependency{
						{
							BuildId:  "8000",
							StepName: "step",
						},
					},
				},
			},
		}

		stepInstructions := &pb.Instructions{
			Instructions: []*pb.Instruction{
				{
					Id: "step",
					TargetedInstructions: []*pb.TargetedInstruction{
						{
							Targets: []pb.InstructionTarget{
								pb.InstructionTarget_LOCAL,
								pb.InstructionTarget_REMOTE,
							},
							Content: "step instruction",
							Dependency: []*pb.InstructionDependency{
								{
									BuildId:  "8001",
									StepName: "dep_step",
								},
							},
						},
					},
				},
			},
		}

		// Insert some Invocations.
		testutil.MustApply(ctx,
			insertInvocation("including", map[string]any{
				"State":             pb.Invocation_ACTIVE,
				"CreateTime":        start,
				"Deadline":          start.Add(time.Hour),
				"Properties":        spanutil.Compressed(pbutil.MustMarshal(properties)),
				"Sources":           spanutil.Compressed(pbutil.MustMarshal(sources)),
				"InheritSources":    spanner.NullBool{Bool: true, Valid: true},
				"IsSourceSpecFinal": spanner.NullBool{Bool: true, Valid: true},
				"BaselineId":        "try:linux-rel",
				"TestInstruction":   spanutil.Compressed(pbutil.MustMarshal(testInstruction)),
				"StepInstructions":  spanutil.Compressed(pbutil.MustMarshal(stepInstructions)),
			}),
			insertInvocation("included0", nil),
			insertInvocation("included1", nil),
			insertInclusion("including", "included0"),
			insertInclusion("including", "included1"),
		)

		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		// Fetch back the top-level Invocation.
		inv, err := Read(ctx, "including")
		So(err, ShouldBeNil)
		So(inv, ShouldResembleProto, &pb.Invocation{
			Name:                "invocations/including",
			State:               pb.Invocation_ACTIVE,
			CreateTime:          pbutil.MustTimestampProto(start),
			Deadline:            pbutil.MustTimestampProto(start.Add(time.Hour)),
			IncludedInvocations: []string{"invocations/included0", "invocations/included1"},
			Properties:          properties,
			SourceSpec: &pb.SourceSpec{
				Inherit: true,
				Sources: sources,
			},
			IsSourceSpecFinal: true,
			BaselineId:        "try:linux-rel",
			TestInstruction:   testInstruction,
			StepInstructions:  stepInstructions,
		})
	})
}

func TestReadBatch(t *testing.T) {
	Convey(`TestReadBatch`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		testutil.MustApply(ctx,
			insertInvocation("inv0", nil),
			insertInvocation("inv1", nil),
			insertInvocation("inv2", nil),
		)

		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		Convey(`One name`, func() {
			invs, err := ReadBatch(ctx, NewIDSet("inv1"))
			So(err, ShouldBeNil)
			So(invs, ShouldHaveLength, 1)
			So(invs, ShouldContainKey, ID("inv1"))
			So(invs["inv1"].Name, ShouldEqual, "invocations/inv1")
			So(invs["inv1"].State, ShouldEqual, pb.Invocation_FINALIZED)
		})

		Convey(`Two names`, func() {
			invs, err := ReadBatch(ctx, NewIDSet("inv0", "inv1"))
			So(err, ShouldBeNil)
			So(invs, ShouldHaveLength, 2)
			So(invs, ShouldContainKey, ID("inv0"))
			So(invs, ShouldContainKey, ID("inv1"))
			So(invs["inv0"].Name, ShouldEqual, "invocations/inv0")
			So(invs["inv0"].State, ShouldEqual, pb.Invocation_FINALIZED)
		})

		Convey(`Not found`, func() {
			_, err := ReadBatch(ctx, NewIDSet("inv0", "x"))
			So(err, ShouldErrLike, `invocations/x not found`)
		})
	})
}

func TestQueryRealms(t *testing.T) {
	Convey(`TestQueryRealms`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		Convey(`Works`, func() {
			testutil.MustApply(ctx,
				insertInvocation("inv0", map[string]any{"Realm": "0"}),
				insertInvocation("inv1", map[string]any{"Realm": "1"}),
				insertInvocation("inv2", map[string]any{"Realm": "2"}),
			)

			realms, err := QueryRealms(span.Single(ctx), NewIDSet("inv0", "inv1", "inv2"))
			So(err, ShouldBeNil)
			So(realms, ShouldResemble, map[ID]string{
				"inv0": "0",
				"inv1": "1",
				"inv2": "2",
			})
		})
		Convey(`Valid with missing invocation `, func() {
			testutil.MustApply(ctx,
				insertInvocation("inv0", map[string]any{"Realm": "0"}),
			)

			realms, err := QueryRealms(span.Single(ctx), NewIDSet("inv0", "inv1"))
			So(err, ShouldBeNil)
			So(realms, ShouldResemble, map[ID]string{
				"inv0": "0",
			})
		})
	})
}

func TestReadRealms(t *testing.T) {
	Convey(`TestReadRealms`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		Convey(`Works`, func() {
			testutil.MustApply(ctx,
				insertInvocation("inv0", map[string]any{"Realm": "0"}),
				insertInvocation("inv1", map[string]any{"Realm": "1"}),
				insertInvocation("inv2", map[string]any{"Realm": "2"}),
			)

			realms, err := ReadRealms(span.Single(ctx), NewIDSet("inv0", "inv1", "inv2"))
			So(err, ShouldBeNil)
			So(realms, ShouldResemble, map[ID]string{
				"inv0": "0",
				"inv1": "1",
				"inv2": "2",
			})
		})
		Convey(`NotFound`, func() {
			testutil.MustApply(ctx,
				insertInvocation("inv0", map[string]any{"Realm": "0"}),
			)

			_, err := ReadRealms(span.Single(ctx), NewIDSet("inv0", "inv1"))
			So(err, ShouldHaveAppStatus, codes.NotFound, "invocations/inv1 not found")
		})
	})
}

func TestReadSubmitted(t *testing.T) {
	Convey(`TestReadSubmitted`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		Convey(`Valid`, func() {
			testutil.MustApply(ctx,
				insertInvocation("inv0", map[string]any{"Submitted": true}),
			)

			submitted, err := ReadSubmitted(span.Single(ctx), ID("inv0"))
			So(err, ShouldBeNil)
			So(submitted, ShouldBeTrue)
		})

		Convey(`Not Found`, func() {
			testutil.MustApply(ctx,
				insertInvocation("inv0", map[string]any{"Submitted": true}),
			)
			_, err := ReadSubmitted(span.Single(ctx), ID("inv1"))
			So(err, ShouldHaveAppStatus, codes.NotFound, "invocations/inv1 not found")
		})

		Convey(`Nil`, func() {
			testutil.MustApply(ctx,
				insertInvocation("inv0", map[string]any{}),
			)
			submitted, err := ReadSubmitted(span.Single(ctx), ID("inv0"))

			So(err, ShouldBeNil)
			So(submitted, ShouldBeFalse)
		})
	})
}
