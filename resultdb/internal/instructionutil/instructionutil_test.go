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

package instructionutil

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestRemoveInstructionsContent(t *testing.T) {
	ftt.Run("Remove instructions content", t, func(t *ftt.Test) {
		t.Run("Success", func(t *ftt.Test) {
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
							},
						},
					},
				},
			}
			result := RemoveInstructionsContent(instructions)
			assert.Loosely(t, result, should.Match(&pb.Instructions{
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
							},
						},
					},
				},
			}))
		})
	})
}

func TestFilterInstructionType(t *testing.T) {
	ftt.Run("Filter instruction types", t, func(t *ftt.Test) {
		t.Run("Success", func(t *ftt.Test) {
			instructions := &pb.Instructions{
				Instructions: []*pb.Instruction{
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
							},
						},
					},
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
					{
						Id:   "test 1",
						Type: pb.InstructionType_TEST_RESULT_INSTRUCTION,
						TargetedInstructions: []*pb.TargetedInstruction{
							{
								Targets: []pb.InstructionTarget{
									pb.InstructionTarget_LOCAL,
									pb.InstructionTarget_REMOTE,
								},
								Content: "test instruction 1",
							},
						},
					},
				},
			}
			result := FilterInstructionType(instructions, pb.InstructionType_TEST_RESULT_INSTRUCTION)
			assert.Loosely(t, result, should.Match(&pb.Instructions{
				Instructions: []*pb.Instruction{
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
							},
						},
					},
					{
						Id:   "test 1",
						Type: pb.InstructionType_TEST_RESULT_INSTRUCTION,
						TargetedInstructions: []*pb.TargetedInstruction{
							{
								Targets: []pb.InstructionTarget{
									pb.InstructionTarget_LOCAL,
									pb.InstructionTarget_REMOTE,
								},
								Content: "test instruction 1",
							},
						},
					},
				},
			}))
		})
	})
}

func TestInstructionsName(t *testing.T) {
	ftt.Run("Instructions name", t, func(t *ftt.Test) {
		t.Run("Instructions is nil", func(t *ftt.Test) {
			assert.Loosely(t, InstructionsWithNames(nil, "inv"), should.BeNil)
		})
		t.Run("Success", func(t *ftt.Test) {
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
							},
						},
					},
				},
			}
			result := InstructionsWithNames(instructions, "invocations/inv")
			assert.Loosely(t, result, should.Match(&pb.Instructions{
				Instructions: []*pb.Instruction{
					{
						Id:   "step",
						Type: pb.InstructionType_STEP_INSTRUCTION,
						Name: "invocations/inv/instructions/step",
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
					{
						Id:   "test",
						Type: pb.InstructionType_TEST_RESULT_INSTRUCTION,
						Name: "invocations/inv/instructions/test",
						TargetedInstructions: []*pb.TargetedInstruction{
							{
								Targets: []pb.InstructionTarget{
									pb.InstructionTarget_LOCAL,
									pb.InstructionTarget_REMOTE,
								},
								Content: "test instruction",
							},
						},
					},
				},
			}))
		})
	})
}

func TestRemoveInstructionsName(t *testing.T) {
	ftt.Run("Instructions name", t, func(t *ftt.Test) {
		t.Run("Instructions is nil", func(t *ftt.Test) {
			assert.Loosely(t, RemoveInstructionsName(nil), should.BeNil)
		})
		t.Run("Success", func(t *ftt.Test) {
			instructions := &pb.Instructions{
				Instructions: []*pb.Instruction{
					{
						Id:   "step",
						Type: pb.InstructionType_STEP_INSTRUCTION,
						Name: "name",
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
					{
						Id:   "test",
						Type: pb.InstructionType_TEST_RESULT_INSTRUCTION,
						Name: "name",
						TargetedInstructions: []*pb.TargetedInstruction{
							{
								Targets: []pb.InstructionTarget{
									pb.InstructionTarget_LOCAL,
									pb.InstructionTarget_REMOTE,
								},
								Content: "test instruction",
							},
						},
					},
				},
			}
			result := RemoveInstructionsName(instructions)
			assert.Loosely(t, result, should.Match(&pb.Instructions{
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
							},
						},
					},
				},
			}))
		})
	})
}
