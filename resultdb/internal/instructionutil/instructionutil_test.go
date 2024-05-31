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

	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRemoveInstructionsContent(t *testing.T) {
	Convey("Remove instructions content", t, func() {
		Convey("Success", func() {
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
			So(result, ShouldResembleProto, &pb.Instructions{
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
			})
		})
	})
}

func TestFilterInstructionType(t *testing.T) {
	Convey("Filter instruction types", t, func() {
		Convey("Success", func() {
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
			So(result, ShouldResembleProto, &pb.Instructions{
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
			})
		})
	})
}
