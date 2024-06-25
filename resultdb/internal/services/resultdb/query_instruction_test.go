// Copyright 2024 The LUCI Authors.
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

package resultdb

import (
	"testing"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/instructionutil"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateQueryInstructionRequest(t *testing.T) {
	Convey("Validate Query Instruction Dependencies", t, func() {
		Convey("Empty name", func() {
			req := &pb.QueryInstructionRequest{}
			_, _, err := validateQueryInstructionRequest(req)
			So(err, ShouldErrLike, "unspecified")
		})
		Convey("Invalid name", func() {
			req := &pb.QueryInstructionRequest{
				Name: "some invalid name",
			}
			_, _, err := validateQueryInstructionRequest(req)
			So(err, ShouldErrLike, "does not match")
		})
	})
}

func TestAdjustMaxDepth(t *testing.T) {
	Convey("Adjust max depth", t, func() {
		So(adjustMaxDepth(-1), ShouldEqual, 5)
		So(adjustMaxDepth(0), ShouldEqual, 5)
		So(adjustMaxDepth(3), ShouldEqual, 3)
		So(adjustMaxDepth(11), ShouldEqual, 10)
	})
}

func TestQueryInstruction(t *testing.T) {
	Convey(`QueryInstruction`, t, func() {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermGetInstruction},
			},
		})
		srv := newTestResultDBService()

		Convey(`Permission denied`, func() {
			testutil.MustApply(ctx,
				insert.Invocation("build-12345", pb.Invocation_ACTIVE, map[string]any{"Realm": "secretproject:testrealm"}),
			)
			req := &pb.QueryInstructionRequest{
				Name: "invocations/build-12345/instructions/test_instruction",
			}
			_, err := srv.QueryInstruction(ctx, req)
			So(err, ShouldBeRPCPermissionDenied)
		})

		Convey(`Invalid name`, func() {
			req := &pb.QueryInstructionRequest{Name: "Some invalid name"}
			_, err := srv.QueryInstruction(ctx, req)
			So(err, ShouldBeRPCInvalidArgument)
		})

		Convey(`Query instruction`, func() {
			// Insert invocations.
			myInstruction := &pb.Instruction{
				Id:              "my_instruction",
				Type:            pb.InstructionType_TEST_RESULT_INSTRUCTION,
				DescriptiveName: "My instruction",
				TargetedInstructions: []*pb.TargetedInstruction{
					{
						Targets: []pb.InstructionTarget{
							pb.InstructionTarget_LOCAL,
							pb.InstructionTarget_REMOTE,
						},
						Content: "this content",
					},
					{
						Targets: []pb.InstructionTarget{
							pb.InstructionTarget_PREBUILT,
						},
						Content: "content for prebuilt",
					},
				},
			}

			instructionDependingOnRestrictedInvocation := &pb.Instruction{
				Id:              "instruction_that_depends_on_a_restricted_invocation",
				Type:            pb.InstructionType_TEST_RESULT_INSTRUCTION,
				DescriptiveName: "Instruction that depends on a restricted invocation",
				TargetedInstructions: []*pb.TargetedInstruction{
					{
						Targets: []pb.InstructionTarget{
							pb.InstructionTarget_LOCAL,
						},
						Dependencies: []*pb.InstructionDependency{
							{
								InvocationId:  "build-restricted",
								InstructionId: "my_instruction",
							},
						},
					},
				},
			}
			instructionCycle1 := &pb.Instruction{
				Id:              "instruction_cycle_1",
				Type:            pb.InstructionType_STEP_INSTRUCTION,
				DescriptiveName: "Instruction cycle 1",
				TargetedInstructions: []*pb.TargetedInstruction{
					{
						Targets: []pb.InstructionTarget{
							pb.InstructionTarget_LOCAL,
						},
						Dependencies: []*pb.InstructionDependency{
							{
								InvocationId:  "build-12346",
								InstructionId: "instruction_cycle_2",
							},
						},
						Content: "cycle 1",
					},
				},
			}
			instructionCycle2 := &pb.Instruction{
				Id:              "instruction_cycle_2",
				Type:            pb.InstructionType_STEP_INSTRUCTION,
				DescriptiveName: "Instruction cycle 2",
				TargetedInstructions: []*pb.TargetedInstruction{
					{
						Targets: []pb.InstructionTarget{
							pb.InstructionTarget_LOCAL,
						},
						Dependencies: []*pb.InstructionDependency{
							{
								InvocationId:  "build-12345",
								InstructionId: "instruction_cycle_1",
							},
						},
						Content: "cycle 2",
					},
				},
			}
			instructionDepNotFound := &pb.Instruction{
				Id:              "instruction_dep_not_found",
				Type:            pb.InstructionType_STEP_INSTRUCTION,
				DescriptiveName: "Instruction dep not found",
				TargetedInstructions: []*pb.TargetedInstruction{
					{
						Targets: []pb.InstructionTarget{
							pb.InstructionTarget_REMOTE,
						},
						Dependencies: []*pb.InstructionDependency{
							{
								InvocationId:  "build-12345",
								InstructionId: "non_existing_instruction",
							},
						},
					},
				},
			}
			instructionDependingOnNonStepInstruction := &pb.Instruction{
				Id:              "instruction_depending_on_non_step_instruction",
				DescriptiveName: "Instruction depending on non step instruction",
				Type:            pb.InstructionType_TEST_RESULT_INSTRUCTION,
				TargetedInstructions: []*pb.TargetedInstruction{
					{
						Targets: []pb.InstructionTarget{
							pb.InstructionTarget_LOCAL,
						},
						Dependencies: []*pb.InstructionDependency{
							{
								InvocationId:  "build-12345",
								InstructionId: "my_instruction",
							},
						},
					},
				},
			}
			instruction1 := &pb.Instruction{
				Id:              "instruction_1",
				Type:            pb.InstructionType_TEST_RESULT_INSTRUCTION,
				DescriptiveName: "Instruction 1",
				TargetedInstructions: []*pb.TargetedInstruction{
					{
						Targets: []pb.InstructionTarget{
							pb.InstructionTarget_LOCAL,
							pb.InstructionTarget_REMOTE,
						},
						Dependencies: []*pb.InstructionDependency{
							{
								InvocationId:  "build-12346",
								InstructionId: "instruction_2",
							},
						},
					},
				},
			}
			instruction2 := &pb.Instruction{
				Id:              "instruction_2",
				Type:            pb.InstructionType_STEP_INSTRUCTION,
				DescriptiveName: "Instruction 2",
				TargetedInstructions: []*pb.TargetedInstruction{
					{
						Targets: []pb.InstructionTarget{
							pb.InstructionTarget_LOCAL,
						},
						Dependencies: []*pb.InstructionDependency{
							{
								InvocationId:  "build-12346",
								InstructionId: "instruction_3",
							},
						},
						Content: "content of instruction 2",
					},
					{
						Targets: []pb.InstructionTarget{
							pb.InstructionTarget_REMOTE,
						},
						Content: "content of remote target of instruction 2",
					},
				},
			}
			instruction3 := &pb.Instruction{
				Id:              "instruction_3",
				Type:            pb.InstructionType_STEP_INSTRUCTION,
				DescriptiveName: "Instruction 3",
				TargetedInstructions: []*pb.TargetedInstruction{
					{
						Targets: []pb.InstructionTarget{
							pb.InstructionTarget_LOCAL,
						},
						Dependencies: []*pb.InstructionDependency{
							{
								InvocationId:  "build-12346",
								InstructionId: "instruction_4",
							},
						},
						Content: "content of instruction 3",
					},
				},
			}
			instruction4 := &pb.Instruction{
				Id:              "instruction_4",
				Type:            pb.InstructionType_STEP_INSTRUCTION,
				DescriptiveName: "Instruction 4",
				TargetedInstructions: []*pb.TargetedInstruction{
					{
						Targets: []pb.InstructionTarget{
							pb.InstructionTarget_LOCAL,
						},
						Content: "content of instruction 4",
					},
					{
						Targets: []pb.InstructionTarget{
							pb.InstructionTarget_REMOTE,
						},
						Content: "Some other instruction",
					},
				},
			}

			testutil.MustApply(ctx,
				insert.Invocation("build-12345", pb.Invocation_FINALIZED, map[string]any{
					"Realm": "testproject:testrealm",
					"Instructions": spanutil.Compress(pbutil.MustMarshal(&pb.Instructions{
						Instructions: []*pb.Instruction{
							myInstruction,
							instructionDependingOnRestrictedInvocation,
							instructionCycle1,
							instructionDepNotFound,
							instructionDependingOnNonStepInstruction,
							instruction1,
						},
					})),
				}),
				insert.Invocation("build-12346", pb.Invocation_FINALIZED, map[string]any{
					"Realm": "testproject:testrealm",
					"Instructions": spanutil.Compress(pbutil.MustMarshal(&pb.Instructions{
						Instructions: []*pb.Instruction{
							instructionCycle2,
							instruction2,
							instruction3,
							instruction4,
						},
					})),
				}),
				insert.Invocation("build-restricted", pb.Invocation_FINALIZED, map[string]any{
					"Realm": "secretproject:testrealm",
				}),
			)
			Convey("Invocation not found", func() {
				req := &pb.QueryInstructionRequest{
					Name: "invocations/build-xxx/instructions/test_instruction",
				}
				_, err := srv.QueryInstruction(ctx, req)
				So(err, ShouldBeRPCNotFound)
			})

			Convey("Instruction not found", func() {
				req := &pb.QueryInstructionRequest{
					Name: "invocations/build-12345/instructions/not_found",
				}
				_, err := srv.QueryInstruction(ctx, req)
				So(err, ShouldBeRPCNotFound)
			})

			Convey("No dependency", func() {
				req := &pb.QueryInstructionRequest{
					Name: "invocations/build-12345/instructions/my_instruction",
				}
				res, err := srv.QueryInstruction(ctx, req)
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.QueryInstructionResponse{
					Instruction: instructionutil.InstructionWithNames(myInstruction, "build-12345"),
					DependencyChains: []*pb.InstructionDependencyChain{
						{
							Target: pb.InstructionTarget_LOCAL,
							Nodes:  []*pb.InstructionDependencyChain_Node{},
						},
						{
							Target: pb.InstructionTarget_REMOTE,
							Nodes:  []*pb.InstructionDependencyChain_Node{},
						},
						{
							Target: pb.InstructionTarget_PREBUILT,
							Nodes:  []*pb.InstructionDependencyChain_Node{},
						},
					},
				})
			})

			Convey("Instruction the depends on a restricted invocation", func() {
				req := &pb.QueryInstructionRequest{
					Name: "invocations/build-12345/instructions/instruction_that_depends_on_a_restricted_invocation",
				}
				res, err := srv.QueryInstruction(ctx, req)
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.QueryInstructionResponse{
					Instruction: instructionutil.InstructionWithNames(instructionDependingOnRestrictedInvocation, "build-12345"),
					DependencyChains: []*pb.InstructionDependencyChain{
						{
							Target: pb.InstructionTarget_LOCAL,
							Nodes: []*pb.InstructionDependencyChain_Node{
								{
									InstructionName: "invocations/build-restricted/instructions/my_instruction",
									Error:           "caller does not have permission resultdb.instructions.get in realm of invocation build-restricted",
								},
							},
						},
					},
				})
			})

			Convey("Dependency cycle", func() {
				req := &pb.QueryInstructionRequest{
					Name: "invocations/build-12345/instructions/instruction_cycle_1",
				}
				res, err := srv.QueryInstruction(ctx, req)
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.QueryInstructionResponse{
					Instruction: instructionutil.InstructionWithNames(instructionCycle1, "build-12345"),
					DependencyChains: []*pb.InstructionDependencyChain{
						{
							Target: pb.InstructionTarget_LOCAL,
							Nodes: []*pb.InstructionDependencyChain_Node{
								{
									InstructionName: "invocations/build-12346/instructions/instruction_cycle_2",
									Content:         "cycle 2",
									DescriptiveName: "Instruction cycle 2",
								},
								{
									InstructionName: "invocations/build-12345/instructions/instruction_cycle_1",
									Error:           "dependency cycle detected",
									DescriptiveName: "Instruction cycle 1",
								},
							},
						},
					},
				})
			})

			Convey("Dependency target not found", func() {
				req := &pb.QueryInstructionRequest{
					Name: "invocations/build-12345/instructions/instruction_dep_not_found",
				}
				res, err := srv.QueryInstruction(ctx, req)
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.QueryInstructionResponse{
					Instruction: instructionutil.InstructionWithNames(instructionDepNotFound, "build-12345"),
					DependencyChains: []*pb.InstructionDependencyChain{
						{
							Target: pb.InstructionTarget_REMOTE,
							Nodes: []*pb.InstructionDependencyChain_Node{
								{
									InstructionName: "invocations/build-12345/instructions/non_existing_instruction",
									Error:           "targeted instruction not found",
								},
							},
						},
					},
				})
			})

			Convey("Dependency on a non-step instruction", func() {
				req := &pb.QueryInstructionRequest{
					Name: "invocations/build-12345/instructions/instruction_depending_on_non_step_instruction",
				}
				res, err := srv.QueryInstruction(ctx, req)
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.QueryInstructionResponse{
					Instruction: instructionutil.InstructionWithNames(instructionDependingOnNonStepInstruction, "build-12345"),
					DependencyChains: []*pb.InstructionDependencyChain{
						{
							Target: pb.InstructionTarget_LOCAL,
							Nodes: []*pb.InstructionDependencyChain_Node{
								{
									InstructionName: "invocations/build-12345/instructions/my_instruction",
									Error:           "an instruction can only depend on a step instruction",
									DescriptiveName: "My instruction",
								},
							},
						},
					},
				})
			})

			Convey("Dependency chain should stop with limit", func() {
				req := &pb.QueryInstructionRequest{
					Name:               "invocations/build-12345/instructions/instruction_1",
					DependencyMaxDepth: 2,
				}
				res, err := srv.QueryInstruction(ctx, req)
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.QueryInstructionResponse{
					Instruction: instructionutil.InstructionWithNames(instruction1, "build-12345"),
					DependencyChains: []*pb.InstructionDependencyChain{
						{
							Target: pb.InstructionTarget_LOCAL,
							Nodes: []*pb.InstructionDependencyChain_Node{
								{
									InstructionName: "invocations/build-12346/instructions/instruction_2",
									Content:         "content of instruction 2",
									DescriptiveName: "Instruction 2",
								},
								{
									InstructionName: "invocations/build-12346/instructions/instruction_3",
									Content:         "content of instruction 3",
									DescriptiveName: "Instruction 3",
								},
							},
						},
						{
							Target: pb.InstructionTarget_REMOTE,
							Nodes: []*pb.InstructionDependencyChain_Node{
								{
									InstructionName: "invocations/build-12346/instructions/instruction_2",
									Content:         "content of remote target of instruction 2",
									DescriptiveName: "Instruction 2",
								},
							},
						},
					},
				})
			})

			Convey("Full instruction chain", func() {
				req := &pb.QueryInstructionRequest{
					Name: "invocations/build-12345/instructions/instruction_1",
				}
				res, err := srv.QueryInstruction(ctx, req)
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.QueryInstructionResponse{
					Instruction: instructionutil.InstructionWithNames(instruction1, "build-12345"),
					DependencyChains: []*pb.InstructionDependencyChain{
						{
							Target: pb.InstructionTarget_LOCAL,
							Nodes: []*pb.InstructionDependencyChain_Node{
								{
									InstructionName: "invocations/build-12346/instructions/instruction_2",
									Content:         "content of instruction 2",
									DescriptiveName: "Instruction 2",
								},
								{
									InstructionName: "invocations/build-12346/instructions/instruction_3",
									Content:         "content of instruction 3",
									DescriptiveName: "Instruction 3",
								},
								{
									InstructionName: "invocations/build-12346/instructions/instruction_4",
									Content:         "content of instruction 4",
									DescriptiveName: "Instruction 4",
								},
							},
						},
						{
							Target: pb.InstructionTarget_REMOTE,
							Nodes: []*pb.InstructionDependencyChain_Node{
								{
									InstructionName: "invocations/build-12346/instructions/instruction_2",
									Content:         "content of remote target of instruction 2",
									DescriptiveName: "Instruction 2",
								},
							},
						},
					},
				})
			})
		})
	})
}
