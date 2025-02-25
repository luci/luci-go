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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/instructionutil"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func TestValidateQueryInstructionRequest(t *testing.T) {
	ftt.Run("Validate Query Instruction Dependencies", t, func(t *ftt.Test) {
		t.Run("Empty name", func(t *ftt.Test) {
			req := &pb.QueryInstructionRequest{}
			_, _, err := validateQueryInstructionRequest(req)
			assert.Loosely(t, err, should.ErrLike("unspecified"))
		})
		t.Run("Invalid name", func(t *ftt.Test) {
			req := &pb.QueryInstructionRequest{
				Name: "some invalid name",
			}
			_, _, err := validateQueryInstructionRequest(req)
			assert.Loosely(t, err, should.ErrLike("does not match"))
		})
	})
}

func TestAdjustMaxDepth(t *testing.T) {
	ftt.Run("Adjust max depth", t, func(t *ftt.Test) {
		assert.Loosely(t, adjustMaxDepth(-1), should.Equal(5))
		assert.Loosely(t, adjustMaxDepth(0), should.Equal(5))
		assert.Loosely(t, adjustMaxDepth(3), should.Equal(3))
		assert.Loosely(t, adjustMaxDepth(11), should.Equal(10))
	})
}

func TestQueryInstruction(t *testing.T) {
	ftt.Run(`QueryInstruction`, t, func(t *ftt.Test) {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermGetInstruction},
			},
		})
		srv := newTestResultDBService()

		t.Run(`Permission denied`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Invocation("build-12345", pb.Invocation_ACTIVE, map[string]any{"Realm": "secretproject:testrealm"}),
			)
			req := &pb.QueryInstructionRequest{
				Name: "invocations/build-12345/instructions/test_instruction",
			}
			_, err := srv.QueryInstruction(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
		})

		t.Run(`Invalid name`, func(t *ftt.Test) {
			req := &pb.QueryInstructionRequest{Name: "Some invalid name"}
			_, err := srv.QueryInstruction(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		})

		t.Run(`Query instruction`, func(t *ftt.Test) {
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

			testutil.MustApply(ctx, t,
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
			t.Run("Invocation not found", func(t *ftt.Test) {
				req := &pb.QueryInstructionRequest{
					Name: "invocations/build-xxx/instructions/test_instruction",
				}
				_, err := srv.QueryInstruction(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			})

			t.Run("Instruction not found", func(t *ftt.Test) {
				req := &pb.QueryInstructionRequest{
					Name: "invocations/build-12345/instructions/not_found",
				}
				_, err := srv.QueryInstruction(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			})

			t.Run("No dependency", func(t *ftt.Test) {
				req := &pb.QueryInstructionRequest{
					Name: "invocations/build-12345/instructions/my_instruction",
				}
				res, err := srv.QueryInstruction(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match(&pb.QueryInstructionResponse{
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
				}))
			})

			t.Run("Instruction the depends on a restricted invocation", func(t *ftt.Test) {
				req := &pb.QueryInstructionRequest{
					Name: "invocations/build-12345/instructions/instruction_that_depends_on_a_restricted_invocation",
				}
				res, err := srv.QueryInstruction(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match(&pb.QueryInstructionResponse{
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
				}))
			})

			t.Run("Dependency cycle", func(t *ftt.Test) {
				req := &pb.QueryInstructionRequest{
					Name: "invocations/build-12345/instructions/instruction_cycle_1",
				}
				res, err := srv.QueryInstruction(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match(&pb.QueryInstructionResponse{
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
				}))
			})

			t.Run("Dependency target not found", func(t *ftt.Test) {
				req := &pb.QueryInstructionRequest{
					Name: "invocations/build-12345/instructions/instruction_dep_not_found",
				}
				res, err := srv.QueryInstruction(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match(&pb.QueryInstructionResponse{
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
				}))
			})

			t.Run("Dependency on a non-step instruction", func(t *ftt.Test) {
				req := &pb.QueryInstructionRequest{
					Name: "invocations/build-12345/instructions/instruction_depending_on_non_step_instruction",
				}
				res, err := srv.QueryInstruction(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match(&pb.QueryInstructionResponse{
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
				}))
			})

			t.Run("Dependency chain should stop with limit", func(t *ftt.Test) {
				req := &pb.QueryInstructionRequest{
					Name:               "invocations/build-12345/instructions/instruction_1",
					DependencyMaxDepth: 2,
				}
				res, err := srv.QueryInstruction(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match(&pb.QueryInstructionResponse{
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
				}))
			})

			t.Run("Full instruction chain", func(t *ftt.Test) {
				req := &pb.QueryInstructionRequest{
					Name: "invocations/build-12345/instructions/instruction_1",
				}
				res, err := srv.QueryInstruction(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match(&pb.QueryInstructionResponse{
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
				}))
			})
		})
	})
}
