// Copyright 2023 The LUCI Authors.
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

package pbutil

import (
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestValidate(t *testing.T) {
	t.Parallel()
	Convey(`ValidateGitilesCommit`, t, func() {
		commit := &pb.GitilesCommit{
			Host:       "chromium.googlesource.com",
			Project:    "chromium/src",
			Ref:        "refs/heads/branch",
			CommitHash: "123456789012345678901234567890abcdefabcd",
			Position:   1,
		}
		Convey(`Valid`, func() {
			So(ValidateGitilesCommit(commit), ShouldBeNil)
		})
		Convey(`Nil`, func() {
			So(ValidateGitilesCommit(nil), ShouldErrLike, `unspecified`)
		})
		Convey(`Host`, func() {
			Convey(`Missing`, func() {
				commit.Host = ""
				So(ValidateGitilesCommit(commit), ShouldErrLike, `host: unspecified`)
			})
			Convey(`Invalid format`, func() {
				commit.Host = "https://somehost.com"
				So(ValidateGitilesCommit(commit), ShouldErrLike, `host: does not match`)
			})
			Convey(`Too long`, func() {
				commit.Host = strings.Repeat("a", hostnameMaxLength+1)
				So(ValidateGitilesCommit(commit), ShouldErrLike, `host: exceeds `, ` characters`)
			})
		})
		Convey(`Project`, func() {
			Convey(`Missing`, func() {
				commit.Project = ""
				So(ValidateGitilesCommit(commit), ShouldErrLike, `project: unspecified`)
			})
			Convey(`Too long`, func() {
				commit.Project = strings.Repeat("a", 256)
				So(ValidateGitilesCommit(commit), ShouldErrLike, `project: exceeds 255 characters`)
			})
		})
		Convey(`Refs`, func() {
			Convey(`Missing`, func() {
				commit.Ref = ""
				So(ValidateGitilesCommit(commit), ShouldErrLike, `ref: unspecified`)
			})
			Convey(`Invalid`, func() {
				commit.Ref = "main"
				So(ValidateGitilesCommit(commit), ShouldErrLike, `ref: does not match refs/.*`)
			})
			Convey(`Too long`, func() {
				commit.Ref = "refs/" + strings.Repeat("a", 252)
				So(ValidateGitilesCommit(commit), ShouldErrLike, `ref: exceeds 255 characters`)
			})
		})
		Convey(`Commit Hash`, func() {
			Convey(`Missing`, func() {
				commit.CommitHash = ""
				So(ValidateGitilesCommit(commit), ShouldErrLike, `commit_hash: unspecified`)
			})
			Convey(`Invalid (too long)`, func() {
				commit.CommitHash = strings.Repeat("a", 41)
				So(ValidateGitilesCommit(commit), ShouldErrLike, `commit_hash: does not match "^[a-f0-9]{40}$"`)
			})
			Convey(`Invalid (too short)`, func() {
				commit.CommitHash = strings.Repeat("a", 39)
				So(ValidateGitilesCommit(commit), ShouldErrLike, `commit_hash: does not match "^[a-f0-9]{40}$"`)
			})
			Convey(`Invalid (upper case)`, func() {
				commit.CommitHash = "123456789012345678901234567890ABCDEFABCD"
				So(ValidateGitilesCommit(commit), ShouldErrLike, `commit_hash: does not match "^[a-f0-9]{40}$"`)
			})
		})
		Convey(`Position`, func() {
			Convey(`Missing`, func() {
				commit.Position = 0
				So(ValidateGitilesCommit(commit), ShouldErrLike, `position: unspecified`)
			})
			Convey(`Negative`, func() {
				commit.Position = -1
				So(ValidateGitilesCommit(commit), ShouldErrLike, `position: cannot be negative`)
			})
		})
	})
	Convey(`ValidateGerritChange`, t, func() {
		change := &pb.GerritChange{
			Host:     "chromium-review.googlesource.com",
			Project:  "chromium/src",
			Change:   12345,
			Patchset: 1,
		}
		Convey(`Valid`, func() {
			So(ValidateGerritChange(change), ShouldBeNil)
		})
		Convey(`Nil`, func() {
			So(ValidateGerritChange(nil), ShouldErrLike, `unspecified`)
		})
		Convey(`Host`, func() {
			Convey(`Missing`, func() {
				change.Host = ""
				So(ValidateGerritChange(change), ShouldErrLike, `host: unspecified`)
			})
			Convey(`Invalid format`, func() {
				change.Host = "https://somehost.com"
				So(ValidateGerritChange(change), ShouldErrLike, `host: does not match`)
			})
			Convey(`Too long`, func() {
				change.Host = strings.Repeat("a", hostnameMaxLength+1)
				So(ValidateGerritChange(change), ShouldErrLike, `host: exceeds `, ` characters`)
			})
		})
		Convey(`Project`, func() {
			Convey(`Missing`, func() {
				change.Project = ""
				So(ValidateGerritChange(change), ShouldErrLike, `project: unspecified`)
			})
			Convey(`Too long`, func() {
				change.Project = strings.Repeat("a", 256)
				So(ValidateGerritChange(change), ShouldErrLike, `project: exceeds 255 characters`)
			})
		})
		Convey(`Change`, func() {
			Convey(`Missing`, func() {
				change.Change = 0
				So(ValidateGerritChange(change), ShouldErrLike, `change: unspecified`)
			})
			Convey(`Invalid`, func() {
				change.Change = -1
				So(ValidateGerritChange(change), ShouldErrLike, `change: cannot be negative`)
			})
		})
		Convey(`Patchset`, func() {
			Convey(`Missing`, func() {
				change.Patchset = 0
				So(ValidateGerritChange(change), ShouldErrLike, `patchset: unspecified`)
			})
			Convey(`Invalid`, func() {
				change.Patchset = -1
				So(ValidateGerritChange(change), ShouldErrLike, `patchset: cannot be negative`)
			})
		})
	})

	Convey("ValidateInstruction", t, func() {
		instructions := &pb.Instructions{
			Instructions: []*pb.Instruction{
				{
					Id:              "step_instruction1",
					Type:            pb.InstructionType_STEP_INSTRUCTION,
					DescriptiveName: "Step instruction 1",
					TargetedInstructions: []*pb.TargetedInstruction{
						{
							Targets: []pb.InstructionTarget{
								pb.InstructionTarget_LOCAL,
							},
							Content: "content1",
						},
					},
				},
				{
					Id:              "step_instruction2",
					Type:            pb.InstructionType_STEP_INSTRUCTION,
					DescriptiveName: "Step instruction 2",
					TargetedInstructions: []*pb.TargetedInstruction{
						{
							Targets: []pb.InstructionTarget{
								pb.InstructionTarget_LOCAL,
							},
							Content: "content2",
						},
					},
				},
				{
					Id:              "test_instruction1",
					Type:            pb.InstructionType_TEST_RESULT_INSTRUCTION,
					DescriptiveName: "Test instruction 1",
					TargetedInstructions: []*pb.TargetedInstruction{
						{
							Targets: []pb.InstructionTarget{
								pb.InstructionTarget_LOCAL,
							},
							Content: "content1",
						},
						{
							Targets: []pb.InstructionTarget{
								pb.InstructionTarget_REMOTE,
							},
							Content: "content2",
							Dependencies: []*pb.InstructionDependency{
								{
									InvocationId:  "inv1",
									InstructionId: "anotherinstruction",
								},
							},
						},
					},
					InstructionFilter: &pb.InstructionFilter{
						FilterType: &pb.InstructionFilter_InvocationIds{
							InvocationIds: &pb.InstructionFilterByInvocationID{
								InvocationIds: []string{"swarming-task-1"},
							},
						},
					},
				},
			},
		}
		Convey("Valid instructions", func() {
			So(ValidateInstructions(instructions), ShouldBeNil)
		})
		Convey("Instructions can be nil", func() {
			So(ValidateInstructions(nil), ShouldBeNil)
		})
		Convey("Instructions too big", func() {
			instructions.Instructions[0].Id = strings.Repeat("a", 2*1024*1024)
			So(ValidateInstructions(instructions), ShouldErrLike, "exceeds 1048576 bytes")
		})
		Convey("No ID", func() {
			instructions.Instructions[0].Id = ""
			So(ValidateInstructions(instructions), ShouldErrLike, "instructions[0]: id: unspecified")
		})
		Convey("ID does not match", func() {
			instructions.Instructions[0].Id = "InstructionWithUpperCase"
			So(ValidateInstructions(instructions), ShouldErrLike, "instructions[0]: id: does not match")
		})
		Convey("Duplicate ID", func() {
			instructions.Instructions[0].Id = "step_instruction2"
			So(ValidateInstructions(instructions), ShouldErrLike, `instructions[1]: id: "step_instruction2" is re-used at index 0`)
		})
		Convey("No Type", func() {
			instructions.Instructions[0].Type = pb.InstructionType_INSTRUCTION_TYPE_UNSPECIFIED
			So(ValidateInstructions(instructions), ShouldErrLike, "instructions[0]: type: unspecified")
		})
		Convey("No descriptive name", func() {
			instructions.Instructions[0].DescriptiveName = ""
			So(ValidateInstructions(instructions), ShouldErrLike, "instructions[0]: descriptive_name: unspecified")
		})
		Convey("Descriptive name too long", func() {
			instructions.Instructions[0].DescriptiveName = strings.Repeat("a", 101)
			So(ValidateInstructions(instructions), ShouldErrLike, "instructions[0]: descriptive_name: exceeds 100 characters")
		})
		Convey("Empty target", func() {
			instructions.Instructions[0].TargetedInstructions[0].Targets = []pb.InstructionTarget{}
			So(ValidateInstructions(instructions), ShouldErrLike, "instructions[0]: targeted_instructions[0]: targets: empty")
		})
		Convey("Unspecified target", func() {
			instructions.Instructions[0].TargetedInstructions[0].Targets = []pb.InstructionTarget{
				pb.InstructionTarget_INSTRUCTION_TARGET_UNSPECIFIED,
			}
			So(ValidateInstructions(instructions), ShouldErrLike, "instructions[0]: targeted_instructions[0]: targets[0]: unspecified")
		})
		Convey("Duplicated target", func() {
			instructions.Instructions[2].TargetedInstructions[0].Targets = []pb.InstructionTarget{
				pb.InstructionTarget_REMOTE,
			}
			So(ValidateInstructions(instructions), ShouldErrLike, `instructions[2]: targeted_instructions[1]: targets[0]: duplicated target "REMOTE"`)
		})
		Convey("Content exceeds size limit", func() {
			instructions.Instructions[2].TargetedInstructions[0].Content = strings.Repeat("a", 120000)
			So(ValidateInstructions(instructions), ShouldErrLike, "instructions[2]: targeted_instructions[0]: content: exceeds 10240 characters")
		})
		Convey("More than 1 dependency", func() {
			instructions.Instructions[2].TargetedInstructions[0].Dependencies = []*pb.InstructionDependency{
				{
					InvocationId:  "inv1",
					InstructionId: "instruction",
				},
				{
					InvocationId:  "inv2",
					InstructionId: "instruction",
				},
			}
			So(ValidateInstructions(instructions), ShouldErrLike, "instructions[2]: targeted_instructions[0]: dependencies: more than 1")
		})
		Convey("Dependency invocation id invalid", func() {
			instructions.Instructions[2].TargetedInstructions[0].Dependencies = []*pb.InstructionDependency{
				{
					InvocationId:  strings.Repeat("a", 101),
					InstructionId: "instruction_id",
				},
			}
			So(ValidateInstructions(instructions), ShouldErrLike, "instructions[2]: targeted_instructions[0]: dependencies: [0]: invocation_id")
		})
	})
}
