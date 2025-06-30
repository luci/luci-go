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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestValidate(t *testing.T) {
	t.Parallel()
	ftt.Run(`ValidateGitilesCommit`, t, func(t *ftt.Test) {
		commit := &pb.GitilesCommit{
			Host:       "chromium.googlesource.com",
			Project:    "chromium/src",
			Ref:        "refs/heads/branch",
			CommitHash: "123456789012345678901234567890abcdefabcd",
			Position:   1,
		}
		t.Run(`Valid`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateGitilesCommit(commit), should.BeNil)
		})
		t.Run(`Nil`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateGitilesCommit(nil), should.ErrLike(`unspecified`))
		})
		t.Run(`Host`, func(t *ftt.Test) {
			t.Run(`Missing`, func(t *ftt.Test) {
				commit.Host = ""
				assert.Loosely(t, ValidateGitilesCommit(commit), should.ErrLike(`host: unspecified`))
			})
			t.Run(`Invalid format`, func(t *ftt.Test) {
				commit.Host = "https://somehost.com"
				assert.Loosely(t, ValidateGitilesCommit(commit), should.ErrLike(`host: does not match`))
			})
			t.Run(`Too long`, func(t *ftt.Test) {
				commit.Host = strings.Repeat("a", hostnameMaxLength+1)
				assert.Loosely(t, ValidateGitilesCommit(commit), should.ErrLike(`host: exceeds `))
				assert.Loosely(t, ValidateGitilesCommit(commit), should.ErrLike(` characters`))
			})
		})
		t.Run(`Project`, func(t *ftt.Test) {
			t.Run(`Missing`, func(t *ftt.Test) {
				commit.Project = ""
				assert.Loosely(t, ValidateGitilesCommit(commit), should.ErrLike(`project: unspecified`))
			})
			t.Run(`Too long`, func(t *ftt.Test) {
				commit.Project = strings.Repeat("a", 256)
				assert.Loosely(t, ValidateGitilesCommit(commit), should.ErrLike(`project: exceeds 255 characters`))
			})
		})
		t.Run(`Refs`, func(t *ftt.Test) {
			t.Run(`Missing`, func(t *ftt.Test) {
				commit.Ref = ""
				assert.Loosely(t, ValidateGitilesCommit(commit), should.ErrLike(`ref: unspecified`))
			})
			t.Run(`Invalid`, func(t *ftt.Test) {
				commit.Ref = "main"
				assert.Loosely(t, ValidateGitilesCommit(commit), should.ErrLike(`ref: does not match refs/.*`))
			})
			t.Run(`Too long`, func(t *ftt.Test) {
				commit.Ref = "refs/" + strings.Repeat("a", 252)
				assert.Loosely(t, ValidateGitilesCommit(commit), should.ErrLike(`ref: exceeds 255 characters`))
			})
		})
		t.Run(`Commit Hash`, func(t *ftt.Test) {
			t.Run(`Missing`, func(t *ftt.Test) {
				commit.CommitHash = ""
				assert.Loosely(t, ValidateGitilesCommit(commit), should.ErrLike(`commit_hash: unspecified`))
			})
			t.Run(`Invalid (too long)`, func(t *ftt.Test) {
				commit.CommitHash = strings.Repeat("a", 41)
				assert.Loosely(t, ValidateGitilesCommit(commit), should.ErrLike(`commit_hash: does not match "^[a-f0-9]{40}$"`))
			})
			t.Run(`Invalid (too short)`, func(t *ftt.Test) {
				commit.CommitHash = strings.Repeat("a", 39)
				assert.Loosely(t, ValidateGitilesCommit(commit), should.ErrLike(`commit_hash: does not match "^[a-f0-9]{40}$"`))
			})
			t.Run(`Invalid (upper case)`, func(t *ftt.Test) {
				commit.CommitHash = "123456789012345678901234567890ABCDEFABCD"
				assert.Loosely(t, ValidateGitilesCommit(commit), should.ErrLike(`commit_hash: does not match "^[a-f0-9]{40}$"`))
			})
		})
		t.Run(`Position`, func(t *ftt.Test) {
			t.Run(`Missing`, func(t *ftt.Test) {
				commit.Position = 0
				assert.Loosely(t, ValidateGitilesCommit(commit), should.ErrLike(`position: unspecified`))
			})
			t.Run(`Negative`, func(t *ftt.Test) {
				commit.Position = -1
				assert.Loosely(t, ValidateGitilesCommit(commit), should.ErrLike(`position: cannot be negative`))
			})
		})
	})
	ftt.Run(`ValidateGerritChange`, t, func(t *ftt.Test) {
		change := &pb.GerritChange{
			Host:     "chromium-review.googlesource.com",
			Project:  "chromium/src",
			Change:   12345,
			Patchset: 1,
		}
		t.Run(`Valid`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateGerritChange(change), should.BeNil)
		})
		t.Run(`Nil`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateGerritChange(nil), should.ErrLike(`unspecified`))
		})
		t.Run(`Host`, func(t *ftt.Test) {
			t.Run(`Missing`, func(t *ftt.Test) {
				change.Host = ""
				assert.Loosely(t, ValidateGerritChange(change), should.ErrLike(`host: unspecified`))
			})
			t.Run(`Invalid format`, func(t *ftt.Test) {
				change.Host = "https://somehost.com"
				assert.Loosely(t, ValidateGerritChange(change), should.ErrLike(`host: does not match`))
			})
			t.Run(`Too long`, func(t *ftt.Test) {
				change.Host = strings.Repeat("a", hostnameMaxLength+1)
				assert.Loosely(t, ValidateGerritChange(change), should.ErrLike(`host: exceeds `))
				assert.Loosely(t, ValidateGerritChange(change), should.ErrLike(` characters`))
			})
		})
		t.Run(`Project`, func(t *ftt.Test) {
			t.Run(`Missing`, func(t *ftt.Test) {
				change.Project = ""
				assert.Loosely(t, ValidateGerritChange(change), should.ErrLike(`project: unspecified`))
			})
			t.Run(`Too long`, func(t *ftt.Test) {
				change.Project = strings.Repeat("a", 256)
				assert.Loosely(t, ValidateGerritChange(change), should.ErrLike(`project: exceeds 255 characters`))
			})
		})
		t.Run(`Change`, func(t *ftt.Test) {
			t.Run(`Missing`, func(t *ftt.Test) {
				change.Change = 0
				assert.Loosely(t, ValidateGerritChange(change), should.ErrLike(`change: unspecified`))
			})
			t.Run(`Invalid`, func(t *ftt.Test) {
				change.Change = -1
				assert.Loosely(t, ValidateGerritChange(change), should.ErrLike(`change: cannot be negative`))
			})
		})
		t.Run(`Patchset`, func(t *ftt.Test) {
			t.Run(`Missing`, func(t *ftt.Test) {
				change.Patchset = 0
				assert.Loosely(t, ValidateGerritChange(change), should.ErrLike(`patchset: unspecified`))
			})
			t.Run(`Invalid`, func(t *ftt.Test) {
				change.Patchset = -1
				assert.Loosely(t, ValidateGerritChange(change), should.ErrLike(`patchset: cannot be negative`))
			})
		})
	})

	ftt.Run("ValidateInstruction", t, func(t *ftt.Test) {
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
		t.Run("Valid instructions", func(t *ftt.Test) {
			assert.Loosely(t, ValidateInstructions(instructions), should.BeNil)
		})
		t.Run("Instructions can be nil", func(t *ftt.Test) {
			assert.Loosely(t, ValidateInstructions(nil), should.BeNil)
		})
		t.Run("Instructions too big", func(t *ftt.Test) {
			instructions.Instructions[0].Id = strings.Repeat("a", 2*1024*1024)
			assert.Loosely(t, ValidateInstructions(instructions), should.ErrLike("exceeds 1048576 bytes"))
		})
		t.Run("No ID", func(t *ftt.Test) {
			instructions.Instructions[0].Id = ""
			assert.Loosely(t, ValidateInstructions(instructions), should.ErrLike("instructions[0]: id: unspecified"))
		})
		t.Run("ID does not match", func(t *ftt.Test) {
			instructions.Instructions[0].Id = "InstructionWithUpperCase"
			assert.Loosely(t, ValidateInstructions(instructions), should.ErrLike("instructions[0]: id: does not match"))
		})
		t.Run("Duplicate ID", func(t *ftt.Test) {
			instructions.Instructions[0].Id = "step_instruction2"
			assert.Loosely(t, ValidateInstructions(instructions), should.ErrLike(`instructions[1]: id: "step_instruction2" is re-used at index 0`))
		})
		t.Run("No Type", func(t *ftt.Test) {
			instructions.Instructions[0].Type = pb.InstructionType_INSTRUCTION_TYPE_UNSPECIFIED
			assert.Loosely(t, ValidateInstructions(instructions), should.ErrLike("instructions[0]: type: unspecified"))
		})
		t.Run("No descriptive name", func(t *ftt.Test) {
			instructions.Instructions[0].DescriptiveName = ""
			assert.Loosely(t, ValidateInstructions(instructions), should.ErrLike("instructions[0]: descriptive_name: unspecified"))
		})
		t.Run("Descriptive name too long", func(t *ftt.Test) {
			instructions.Instructions[0].DescriptiveName = strings.Repeat("a", 101)
			assert.Loosely(t, ValidateInstructions(instructions), should.ErrLike("instructions[0]: descriptive_name: exceeds 100 characters"))
		})
		t.Run("Empty target", func(t *ftt.Test) {
			instructions.Instructions[0].TargetedInstructions[0].Targets = []pb.InstructionTarget{}
			assert.Loosely(t, ValidateInstructions(instructions), should.ErrLike("instructions[0]: targeted_instructions[0]: targets: empty"))
		})
		t.Run("Unspecified target", func(t *ftt.Test) {
			instructions.Instructions[0].TargetedInstructions[0].Targets = []pb.InstructionTarget{
				pb.InstructionTarget_INSTRUCTION_TARGET_UNSPECIFIED,
			}
			assert.Loosely(t, ValidateInstructions(instructions), should.ErrLike("instructions[0]: targeted_instructions[0]: targets[0]: unspecified"))
		})
		t.Run("Duplicated target", func(t *ftt.Test) {
			instructions.Instructions[2].TargetedInstructions[0].Targets = []pb.InstructionTarget{
				pb.InstructionTarget_REMOTE,
			}
			assert.Loosely(t, ValidateInstructions(instructions), should.ErrLike(`instructions[2]: targeted_instructions[1]: targets[0]: duplicated target "REMOTE"`))
		})
		t.Run("Content exceeds size limit", func(t *ftt.Test) {
			instructions.Instructions[2].TargetedInstructions[0].Content = strings.Repeat("a", 120000)
			assert.Loosely(t, ValidateInstructions(instructions), should.ErrLike("instructions[2]: targeted_instructions[0]: content: exceeds 10240 characters"))
		})
		t.Run("More than 1 dependency", func(t *ftt.Test) {
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
			assert.Loosely(t, ValidateInstructions(instructions), should.ErrLike("instructions[2]: targeted_instructions[0]: dependencies: more than 1"))
		})
		t.Run("Dependency invocation id invalid", func(t *ftt.Test) {
			instructions.Instructions[2].TargetedInstructions[0].Dependencies = []*pb.InstructionDependency{
				{
					InvocationId:  strings.Repeat("a", 101),
					InstructionId: "instruction_id",
				},
			}
			assert.Loosely(t, ValidateInstructions(instructions), should.ErrLike("instructions[2]: targeted_instructions[0]: dependencies: [0]: invocation_id"))
		})
	})
	ftt.Run(`ValidateSources`, t, func(t *ftt.Test) {
		sources := &pb.Sources{
			GitilesCommit: &pb.GitilesCommit{
				Host:       "chromium.googlesource.com",
				Project:    "chromium/src",
				Ref:        "refs/heads/branch",
				CommitHash: "123456789012345678901234567890abcdefabcd",
				Position:   1,
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
		t.Run(`Valid with sources`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateSources(sources), should.BeNil)
		})
		t.Run(`Nil`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateSources(nil), should.ErrLike(`unspecified`))
		})
		t.Run(`Gitiles commit`, func(t *ftt.Test) {
			t.Run(`Missing`, func(t *ftt.Test) {
				sources.GitilesCommit = nil
				assert.Loosely(t, ValidateSources(sources), should.ErrLike(`gitiles_commit: unspecified`))
			})
			t.Run(`Invalid`, func(t *ftt.Test) {
				// protocol prefix should not be included.
				sources.GitilesCommit.Host = "https://service"
				assert.Loosely(t, ValidateSources(sources), should.ErrLike(`gitiles_commit: host: does not match`))
			})
		})
		t.Run(`Changelists`, func(t *ftt.Test) {
			t.Run(`Zero length`, func(t *ftt.Test) {
				sources.Changelists = nil
				assert.Loosely(t, ValidateSources(sources), should.BeNil)
			})
			t.Run(`Invalid`, func(t *ftt.Test) {
				sources.Changelists[0].Change = -1
				assert.Loosely(t, ValidateSources(sources), should.ErrLike(`changelists[0]: change: cannot be negative`))
			})
			t.Run(`Too many`, func(t *ftt.Test) {
				sources.Changelists = nil
				for i := 0; i < 11; i++ {
					sources.Changelists = append(sources.Changelists,
						&pb.GerritChange{
							Host:     "chromium-review.googlesource.com",
							Project:  "infra/luci-go",
							Change:   int64(i + 1),
							Patchset: 321,
						},
					)
				}
				assert.Loosely(t, ValidateSources(sources), should.ErrLike(`changelists: exceeds maximum of 10 changelists`))
			})
			t.Run(`Duplicates`, func(t *ftt.Test) {
				sources.Changelists = nil
				for i := 0; i < 2; i++ {
					sources.Changelists = append(sources.Changelists,
						&pb.GerritChange{
							Host:     "chromium-review.googlesource.com",
							Project:  "infra/luci-go",
							Change:   12345,
							Patchset: int64(i + 1),
						},
					)
				}
				assert.Loosely(t, ValidateSources(sources), should.ErrLike(`changelists[1]: duplicate change modulo patchset number; same change at changelists[0]`))
			})
		})
	})
	ftt.Run(`ValidateSourceSpec`, t, func(t *ftt.Test) {
		sourceSpec := &pb.SourceSpec{
			Sources: &pb.Sources{
				GitilesCommit: &pb.GitilesCommit{
					Host:       "chromium.googlesource.com",
					Project:    "chromium/src",
					Ref:        "refs/heads/branch",
					CommitHash: "123456789012345678901234567890abcdefabcd",
					Position:   1,
				},
			},
			Inherit: false,
		}
		t.Run(`Valid`, func(t *ftt.Test) {
			t.Run(`Sources only`, func(t *ftt.Test) {
				assert.Loosely(t, ValidateSourceSpec(sourceSpec), should.BeNil)
			})
			t.Run(`Empty`, func(t *ftt.Test) {
				sourceSpec.Sources = nil
				assert.Loosely(t, ValidateSourceSpec(sourceSpec), should.BeNil)
			})
			t.Run(`Inherit only`, func(t *ftt.Test) {
				sourceSpec.Sources = nil
				sourceSpec.Inherit = true
				assert.Loosely(t, ValidateSourceSpec(sourceSpec), should.BeNil)
			})
			t.Run(`Nil`, func(t *ftt.Test) {
				assert.Loosely(t, ValidateSourceSpec(nil), should.BeNil)
			})
		})
		t.Run(`Cannot specify inherit concurrently with sources`, func(t *ftt.Test) {
			assert.Loosely(t, sourceSpec.Sources, should.NotBeNil)
			sourceSpec.Inherit = true
			assert.Loosely(t, ValidateSourceSpec(sourceSpec), should.ErrLike(`only one of inherit and sources may be set`))
		})
		t.Run(`Invalid Sources`, func(t *ftt.Test) {
			sourceSpec.Sources.GitilesCommit.Host = "b@d"
			assert.Loosely(t, ValidateSourceSpec(sourceSpec), should.ErrLike(`sources: gitiles_commit: host: does not match`))
		})
	})
}
