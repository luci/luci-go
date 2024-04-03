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
		stepInstructions := &pb.Instructions{
			Instructions: []*pb.Instruction{
				{
					Id: "instruction1",
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
					Id: "instruction2",
					TargetedInstructions: []*pb.TargetedInstruction{
						{
							Targets: []pb.InstructionTarget{
								pb.InstructionTarget_LOCAL,
							},
							Content: "content2",
						},
					},
				},
			},
		}
		Convey("Valid Step Instructions", func() {
			So(ValidateStepInstructions(stepInstructions), ShouldBeNil)
		})
		Convey("Step Instruction can be nil", func() {
			So(ValidateStepInstructions(nil), ShouldBeNil)
		})
		Convey("Step Instruction too big", func() {
			stepInstructions.Instructions[0].Id = stringWithLength(2 * 1024 * 1024)
			So(ValidateStepInstructions(stepInstructions), ShouldErrLike, "step instructions: bigger than 1048576 bytes")
		})
		Convey("No ID", func() {
			stepInstructions.Instructions[0].Id = ""
			So(ValidateStepInstructions(stepInstructions), ShouldErrLike, "step instructions: unspecified id")
		})
		Convey("Duplicate ID", func() {
			stepInstructions.Instructions[0].Id = "instruction2"
			So(ValidateStepInstructions(stepInstructions), ShouldErrLike, `step instructions: ID "instruction2" is re-used at index 0 and 1`)
		})

		instruction := &pb.Instruction{
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
					Dependency: []*pb.InstructionDependency{
						{
							BuildId:  "8000",
							StepName: "compile",
						},
					},
				},
			},
		}
		Convey("Valid Test Instruction", func() {
			So(ValidateTestInstruction(instruction), ShouldBeNil)
		})
		Convey("Test instruction can be nil", func() {
			So(ValidateTestInstruction(nil), ShouldBeNil)
		})
		Convey("Empty target", func() {
			instruction.TargetedInstructions[0].Targets = []pb.InstructionTarget{}
			So(ValidateInstruction(instruction), ShouldErrLike, "target: empty")
		})
		Convey("Unspecified target", func() {
			instruction.TargetedInstructions[0].Targets = []pb.InstructionTarget{
				pb.InstructionTarget_INSTRUCTION_TARGET_UNSPECIFIED,
			}
			So(ValidateInstruction(instruction), ShouldErrLike, "target: unspecified")
		})
		Convey("Duplicated target", func() {
			instruction.TargetedInstructions[0].Targets = []pb.InstructionTarget{
				pb.InstructionTarget_REMOTE,
			}
			So(ValidateInstruction(instruction), ShouldErrLike, `target: duplicated "REMOTE"`)
		})
		Convey("Content exceeds size limit", func() {
			content := ""
			for i := 0; i < 120000; i++ {
				content += "a"
			}
			instruction.TargetedInstructions[0].Content = content
			So(ValidateInstruction(instruction), ShouldErrLike, "content: longer than 10240 bytes")
		})
		Convey("More than 1 dependency", func() {
			instruction.TargetedInstructions[0].Dependency = []*pb.InstructionDependency{
				{
					BuildId:  "8000",
					StepName: "dep1",
				},
				{
					BuildId:  "8000",
					StepName: "dep2",
				},
			}
			So(ValidateInstruction(instruction), ShouldErrLike, "dependency: more than 1")
		})
		Convey("Dependency buildID exceeds limit", func() {
			instruction.TargetedInstructions[0].Dependency = []*pb.InstructionDependency{
				{
					BuildId:  stringWithLength(101),
					StepName: "dep",
				},
			}
			So(ValidateInstruction(instruction), ShouldErrLike, "dependencies[0]: build_id: longer than 100 bytes")
		})
		Convey("Dependency empty step name", func() {
			instruction.TargetedInstructions[0].Dependency = []*pb.InstructionDependency{
				{
					BuildId: "8000",
				},
			}
			So(ValidateInstruction(instruction), ShouldErrLike, "dependencies[0]: step_name: empty")
		})
		Convey("Dependency step name exceeds limit", func() {
			instruction.TargetedInstructions[0].Dependency = []*pb.InstructionDependency{
				{
					BuildId:  "8000",
					StepName: stringWithLength(1025),
				},
			}
			So(ValidateInstruction(instruction), ShouldErrLike, "dependencies[0]: step_name: longer than 1024 bytes")
		})

		Convey("Dependency step tag key exceeds limit", func() {
			instruction.TargetedInstructions[0].Dependency = []*pb.InstructionDependency{
				{
					BuildId:  "8000",
					StepName: "step",
					StepTag: &pb.StringPair{
						Key:   stringWithLength(257),
						Value: "val",
					},
				},
			}
			So(ValidateInstruction(instruction), ShouldErrLike, "dependencies[0]: step_tag_key: longer than 256 bytes")
		})

		Convey("Dependency step tag value exceeds limit", func() {
			instruction.TargetedInstructions[0].Dependency = []*pb.InstructionDependency{
				{
					BuildId:  "8000",
					StepName: "step",
					StepTag: &pb.StringPair{
						Key:   "key",
						Value: stringWithLength(1025),
					},
				},
			}
			So(ValidateInstruction(instruction), ShouldErrLike, "dependencies[0]: step_tag_val: longer than 1024 bytes")
		})
	})
}

func stringWithLength(l int) string {
	return strings.Repeat("a", l)
}
