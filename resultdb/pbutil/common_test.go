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
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

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
	ftt.Run(`ValidateExtraBuildDescriptors`, t, func(t *ftt.Test) {
		builds := []*pb.BuildDescriptor{
			{
				Definition: &pb.BuildDescriptor_AndroidBuild{
					AndroidBuild: &pb.AndroidBuildDescriptor{
						DataRealm:   "prod",
						Branch:      "git_main",
						BuildTarget: "aosp_arm64-userdebug",
						BuildId:     "L1234567890",
					},
				},
			},
			{
				Definition: &pb.BuildDescriptor_AndroidBuild{
					AndroidBuild: &pb.AndroidBuildDescriptor{
						DataRealm:   "test",
						Branch:      "git_main",
						BuildTarget: "aosp_arm64-userdebug",
						BuildId:     "E1234567890",
					},
				},
			},
		}
		t.Run(`Valid`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateExtraBuildDescriptors(builds), should.BeNil)
		})
		t.Run(`Invalid`, func(t *ftt.Test) {
			builds[0] = &pb.BuildDescriptor{
				Definition: &pb.BuildDescriptor_AndroidBuild{
					AndroidBuild: &pb.AndroidBuildDescriptor{
						DataRealm:   "invalid",
						Branch:      "git_main",
						BuildTarget: "aosp_arm64-userdebug",
						BuildId:     "L1234567890",
					},
				},
			}
			assert.Loosely(t, ValidateExtraBuildDescriptors(builds), should.ErrLike(`[0]: android_build: data_realm: unknown data realm "invalid"`))
		})
		t.Run(`Too large`, func(t *ftt.Test) {
			builds = make([]*pb.BuildDescriptor, 11)
			for i := 0; i < 11; i++ {
				builds[i] = &pb.BuildDescriptor{
					Definition: &pb.BuildDescriptor_AndroidBuild{
						AndroidBuild: &pb.AndroidBuildDescriptor{
							DataRealm:   "prod",
							Branch:      "git_main",
							BuildTarget: "aosp_arm64-userdebug",
							BuildId:     fmt.Sprintf("L%d", i),
						},
					},
				}
			}
			assert.Loosely(t, ValidateExtraBuildDescriptors(builds), should.ErrLike(`exceeds maximum of 10 extra builds`))
		})
	})
	ftt.Run(`ValidateBuildDescriptorsUniquenessAndOrder`, t, func(t *ftt.Test) {
		builds := []*pb.BuildDescriptor{
			{
				Definition: &pb.BuildDescriptor_AndroidBuild{
					AndroidBuild: &pb.AndroidBuildDescriptor{
						DataRealm:   "prod",
						Branch:      "git_main",
						BuildTarget: "aosp_arm64-userdebug",
						BuildId:     "L1234567890",
					},
				},
			},
			{
				Definition: &pb.BuildDescriptor_AndroidBuild{
					AndroidBuild: &pb.AndroidBuildDescriptor{
						DataRealm:   "test",
						Branch:      "git_main",
						BuildTarget: "aosp_arm64-userdebug",
						BuildId:     "E1234567890",
					},
				},
			},
		}
		primaryBuild := &pb.BuildDescriptor{
			Definition: &pb.BuildDescriptor_AndroidBuild{
				AndroidBuild: &pb.AndroidBuildDescriptor{
					DataRealm:   "prod",
					Branch:      "git_main",
					BuildTarget: "aosp_arm64-userdebug",
					BuildId:     "P1234567890",
				},
			},
		}
		t.Run(`Valid`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateBuildDescriptorsUniquenessAndOrder(builds, primaryBuild), should.BeNil)
		})
		t.Run(`Nil primary build, no extra builds`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateBuildDescriptorsUniquenessAndOrder(nil, nil), should.BeNil)
		})
		t.Run(`Nil primary build, with extra builds`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateBuildDescriptorsUniquenessAndOrder(builds, nil), should.ErrLike(`may not be specified unless primary build is set`))
		})
		t.Run(`Duplicates of primary build`, func(t *ftt.Test) {
			primaryBuild = proto.Clone(builds[1]).(*pb.BuildDescriptor)
			assert.Loosely(t, ValidateBuildDescriptorsUniquenessAndOrder(builds, primaryBuild), should.ErrLike(`[1]: duplicate of primary_build`))
		})
		t.Run(`Duplicate within extra builds`, func(t *ftt.Test) {
			builds[1] = builds[0]
			assert.Loosely(t, ValidateBuildDescriptorsUniquenessAndOrder(builds, primaryBuild), should.ErrLike(`[1]: duplicate of extra_builds[0]`))
		})
	})
	ftt.Run(`ValidateBuildDescriptor`, t, func(t *ftt.Test) {
		build := &pb.BuildDescriptor{
			Definition: &pb.BuildDescriptor_AndroidBuild{
				AndroidBuild: &pb.AndroidBuildDescriptor{
					DataRealm:   "prod",
					Branch:      "git_main",
					BuildTarget: "aosp_arm64-userdebug",
					BuildId:     "L1234567890",
				},
			},
		}
		t.Run(`Valid`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateBuildDescriptor(build), should.BeNil)
		})
		t.Run(`Nil`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateBuildDescriptor(nil), should.ErrLike(`unspecified`))
		})
		t.Run(`Definition`, func(t *ftt.Test) {
			t.Run(`Missing`, func(t *ftt.Test) {
				build.Definition = nil
				assert.Loosely(t, ValidateBuildDescriptor(build), should.ErrLike(`definition: unspecified`))
			})
			t.Run(`AndroidBuild`, func(t *ftt.Test) {
				ab := &pb.AndroidBuildDescriptor{
					DataRealm:   "test",
					Branch:      "git_main",
					BuildTarget: "aosp_arm64-userdebug",
					BuildId:     "E1234567890",
				}
				build.Definition = &pb.BuildDescriptor_AndroidBuild{
					AndroidBuild: ab,
				}
				t.Run(`Valid`, func(t *ftt.Test) {
					assert.Loosely(t, ValidateBuildDescriptor(build), should.BeNil)
				})
				t.Run(`Invalid`, func(t *ftt.Test) {
					ab.DataRealm = "invalid"
					assert.Loosely(t, ValidateBuildDescriptor(build), should.ErrLike(`android_build: data_realm: unknown data realm "invalid"`))
				})
			})
		})
	})
	ftt.Run(`ValidateAndroidBuildDescriptor`, t, func(t *ftt.Test) {
		build := &pb.AndroidBuildDescriptor{
			DataRealm:   "prod",
			Branch:      "git_main",
			BuildTarget: "aosp_arm64-userdebug",
			BuildId:     "P1234567890",
		}
		t.Run(`Valid`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateAndroidBuildDescriptor(build), should.BeNil)
		})
		t.Run(`Nil`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateAndroidBuildDescriptor(nil), should.ErrLike(`unspecified`))
		})
		t.Run(`Data Realm`, func(t *ftt.Test) {
			t.Run(`Missing`, func(t *ftt.Test) {
				build.DataRealm = ""
				assert.Loosely(t, ValidateAndroidBuildDescriptor(build), should.ErrLike(`data_realm: unspecified`))
			})
			t.Run(`Invalid`, func(t *ftt.Test) {
				build.DataRealm = "invalid"
				assert.Loosely(t, ValidateAndroidBuildDescriptor(build), should.ErrLike(`data_realm: unknown data realm "invalid"`))
			})
		})
		t.Run(`Branch`, func(t *ftt.Test) {
			t.Run(`Missing`, func(t *ftt.Test) {
				build.Branch = ""
				assert.Loosely(t, ValidateAndroidBuildDescriptor(build), should.ErrLike(`branch: unspecified`))
			})
			t.Run(`Too long`, func(t *ftt.Test) {
				build.Branch = strings.Repeat("a", MaxAndroidBranchLength+1)
				assert.Loosely(t, ValidateAndroidBuildDescriptor(build), should.ErrLike(`branch: longer than 255 bytes`))
			})
			t.Run(`Non-Printable`, func(t *ftt.Test) {
				build.Branch = "abc\u0000def"
				assert.Loosely(t, ValidateAndroidBuildDescriptor(build), should.ErrLike(`branch: non-printable rune`))
			})
			t.Run(`Not in Unicode Normal Form C`, func(t *ftt.Test) {
				build.Branch = "e\u0301" // Normal Form C would be \u00e9 (é).
				assert.Loosely(t, ValidateAndroidBuildDescriptor(build), should.ErrLike(`branch: not in unicode normalized form C`))
			})
			t.Run(`Not valid UTF-8`, func(t *ftt.Test) {
				build.Branch = "\xbd"
				assert.Loosely(t, ValidateAndroidBuildDescriptor(build), should.ErrLike(`branch: not a valid utf8 string`))
			})
			t.Run(`Error rune`, func(t *ftt.Test) {
				build.Branch = "aa\ufffd"
				assert.Loosely(t, ValidateAndroidBuildDescriptor(build), should.ErrLike(`branch: unicode replacement character (U+FFFD) at byte index 2`))
			})
		})
		t.Run(`Build Target`, func(t *ftt.Test) {
			t.Run(`Missing`, func(t *ftt.Test) {
				build.BuildTarget = ""
				assert.Loosely(t, ValidateAndroidBuildDescriptor(build), should.ErrLike(`build_target: unspecified`))
			})
			t.Run(`Too long`, func(t *ftt.Test) {
				build.BuildTarget = strings.Repeat("a", MaxAndroidBuildTargetLength+1)
				assert.Loosely(t, ValidateAndroidBuildDescriptor(build), should.ErrLike(`build_target: longer than 255 bytes`))
			})
			t.Run(`Non-Printable`, func(t *ftt.Test) {
				build.BuildTarget = "abc\u0000def"
				assert.Loosely(t, ValidateAndroidBuildDescriptor(build), should.ErrLike(`build_target: non-printable rune`))
			})
			t.Run(`Not in Unicode Normal Form C`, func(t *ftt.Test) {
				build.BuildTarget = "e\u0301" // Normal Form C would be \u00e9 (é).
				assert.Loosely(t, ValidateAndroidBuildDescriptor(build), should.ErrLike(`build_target: not in unicode normalized form C`))
			})
			t.Run(`Not valid UTF-8`, func(t *ftt.Test) {
				build.BuildTarget = "\xbd"
				assert.Loosely(t, ValidateAndroidBuildDescriptor(build), should.ErrLike(`build_target: not a valid utf8 string`))
			})
			t.Run(`Error rune`, func(t *ftt.Test) {
				build.BuildTarget = "aa\ufffd"
				assert.Loosely(t, ValidateAndroidBuildDescriptor(build), should.ErrLike(`build_target: unicode replacement character (U+FFFD) at byte index 2`))
			})
		})
		t.Run(`Build ID`, func(t *ftt.Test) {
			t.Run(`Missing`, func(t *ftt.Test) {
				build.BuildId = ""
				assert.Loosely(t, ValidateAndroidBuildDescriptor(build), should.ErrLike(`build_id: unspecified`))
			})
			t.Run(`Too long`, func(t *ftt.Test) {
				build.BuildId = strings.Repeat("1", MaxAndroidBuildLength+1)
				assert.Loosely(t, ValidateAndroidBuildDescriptor(build), should.ErrLike(`build_id: must be at most 32 bytes`))
			})
			t.Run(`Invalid format`, func(t *ftt.Test) {
				build.BuildId = "abc"
				assert.Loosely(t, ValidateAndroidBuildDescriptor(build), should.ErrLike(`build_id: does not match`))
			})
		})
	})
	ftt.Run("ValidateSubmittedAndroidBuild", t, func(t *ftt.Test) {
		buildID := &pb.SubmittedAndroidBuild{
			DataRealm: "prod",
			Branch:    "git_main",
			BuildId:   1234567890,
		}
		t.Run("Valid", func(t *ftt.Test) {
			assert.Loosely(t, ValidateSubmittedAndroidBuild(buildID), should.BeNil)
		})
		t.Run("Nil", func(t *ftt.Test) {
			assert.Loosely(t, ValidateSubmittedAndroidBuild(nil), should.ErrLike("unspecified"))
		})
		t.Run("Data Realm", func(t *ftt.Test) {
			t.Run("Missing", func(t *ftt.Test) {
				buildID.DataRealm = ""
				assert.Loosely(t, ValidateSubmittedAndroidBuild(buildID), should.ErrLike("data_realm: unspecified"))
			})
			t.Run("Invalid", func(t *ftt.Test) {
				buildID.DataRealm = "invalid"
				assert.Loosely(t, ValidateSubmittedAndroidBuild(buildID), should.ErrLike(`data_realm: unknown data realm "invalid"`))
			})
		})
		t.Run("Branch", func(t *ftt.Test) {
			t.Run("Missing", func(t *ftt.Test) {
				buildID.Branch = ""
				assert.Loosely(t, ValidateSubmittedAndroidBuild(buildID), should.ErrLike("branch: unspecified"))
			})
			t.Run("Too long", func(t *ftt.Test) {
				buildID.Branch = strings.Repeat("a", MaxAndroidBranchLength+1)
				assert.Loosely(t, ValidateSubmittedAndroidBuild(buildID), should.ErrLike("branch: longer than 255 bytes"))
			})
			t.Run("Non-Printable", func(t *ftt.Test) {
				buildID.Branch = "abc\u0000def"
				assert.Loosely(t, ValidateSubmittedAndroidBuild(buildID), should.ErrLike("branch: non-printable rune"))
			})
			t.Run("Not in Unicode Normal Form C", func(t *ftt.Test) {
				buildID.Branch = "e\u0301" // Normal Form C would be \u00e9 (é).
				assert.Loosely(t, ValidateSubmittedAndroidBuild(buildID), should.ErrLike("branch: not in unicode normalized form C"))
			})
			t.Run("Not valid UTF-8", func(t *ftt.Test) {
				buildID.Branch = "\xbd"
				assert.Loosely(t, ValidateSubmittedAndroidBuild(buildID), should.ErrLike("branch: not a valid utf8 string"))
			})
			t.Run("Error rune", func(t *ftt.Test) {
				buildID.Branch = "aa\ufffd"
				assert.Loosely(t, ValidateSubmittedAndroidBuild(buildID), should.ErrLike("branch: unicode replacement character (U+FFFD) at byte index 2"))
			})
		})
		t.Run("Build ID", func(t *ftt.Test) {
			t.Run("Missing", func(t *ftt.Test) {
				buildID.BuildId = 0
				assert.Loosely(t, ValidateSubmittedAndroidBuild(buildID), should.ErrLike("build_id: unspecified"))
			})
			t.Run("Negative", func(t *ftt.Test) {
				buildID.BuildId = -1
				assert.Loosely(t, ValidateSubmittedAndroidBuild(buildID), should.ErrLike("build_id: cannot be negative"))
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
			BaseSources: &pb.Sources_GitilesCommit{
				GitilesCommit: &pb.GitilesCommit{
					Host:       "chromium.googlesource.com",
					Project:    "chromium/src",
					Ref:        "refs/heads/branch",
					CommitHash: "123456789012345678901234567890abcdefabcd",
					Position:   1,
				},
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
		t.Run(`Base sources`, func(t *ftt.Test) {
			t.Run(`Missing and IsDirty is false`, func(t *ftt.Test) {
				sources.BaseSources = nil
				sources.IsDirty = false
				assert.Loosely(t, ValidateSources(sources), should.ErrLike(`base_sources: unspecified; if you really have no sources (e.g. due to running tests locally) and are OK with the loss of test history for these results, set is_dirty to true`))
			})
			t.Run(`Missing and IsDirty is true`, func(t *ftt.Test) {
				// This is valid. It represents testing on sources which could not be precisely determined,
				// such as can occur with a local checkout.
				sources.BaseSources = nil
				sources.IsDirty = true
				assert.Loosely(t, ValidateSources(sources), should.BeNil)
			})
			t.Run(`Gitiles commit`, func(t *ftt.Test) {
				gc := &pb.GitilesCommit{
					Host:       "chromium.googlesource.com",
					Project:    "chromium/src",
					Ref:        "refs/heads/branch",
					CommitHash: "123456789012345678901234567890abcdefabcd",
					Position:   1,
				}
				sources.BaseSources = &pb.Sources_GitilesCommit{
					GitilesCommit: gc,
				}
				t.Run(`Valid`, func(t *ftt.Test) {
					assert.Loosely(t, ValidateSources(sources), should.BeNil)
				})
				t.Run(`Invalid`, func(t *ftt.Test) {
					// protocol prefix should not be included.
					gc.Host = "https://service"
					assert.Loosely(t, ValidateSources(sources), should.ErrLike(`gitiles_commit: host: does not match`))
				})
			})
			t.Run(`Submitted Android build`, func(t *ftt.Test) {
				ab := &pb.SubmittedAndroidBuild{
					DataRealm: "prod",
					Branch:    "git_main",
					BuildId:   1234567890,
				}
				sources.BaseSources = &pb.Sources_SubmittedAndroidBuild{
					SubmittedAndroidBuild: ab,
				}
				t.Run(`Valid`, func(t *ftt.Test) {
					assert.Loosely(t, ValidateSources(sources), should.BeNil)
				})
				t.Run(`Invalid`, func(t *ftt.Test) {
					ab.DataRealm = "invalid"
					assert.Loosely(t, ValidateSources(sources), should.ErrLike(`submitted_android_build: data_realm: unknown data realm "invalid"`))
				})
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
				BaseSources: &pb.Sources_GitilesCommit{
					GitilesCommit: &pb.GitilesCommit{
						Host:       "chromium.googlesource.com",
						Project:    "chromium/src",
						Ref:        "refs/heads/branch",
						CommitHash: "123456789012345678901234567890abcdefabcd",
						Position:   1,
					},
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
			sourceSpec.Sources.Changelists = []*pb.GerritChange{
				{
					Host:     "chromium.googlesource.com",
					Project:  "infra/luci-go",
					Change:   -1,
					Patchset: 321,
				},
			}
			assert.Loosely(t, ValidateSourceSpec(sourceSpec), should.ErrLike(`sources: changelists[0]: change: cannot be negative`))
		})
	})
	ftt.Run(`ValidateFullResourceName`, t, func(t *ftt.Test) {
		t.Run("Valid", func(t *ftt.Test) {
			assert.Loosely(t, ValidateFullResourceName("//chromium-swarm.appspot.com/tasks/deadbeef"), should.BeNil)
			assert.Loosely(t, ValidateFullResourceName("//cr-buildbucket.appspot.com/builds/1234567890"), should.BeNil)
			assert.Loosely(t, ValidateFullResourceName("//resultdb.api.luci.app/rootInvocations/inv/workUnits/root"), should.BeNil)
			assert.Loosely(t, ValidateFullResourceName("//some-service.googleapis.com/some/resource/name"), should.BeNil)
			// "café" in NFC
			assert.Loosely(t, ValidateFullResourceName("//service/r/caf\u00e9"), should.BeNil)
		})
		t.Run("Invalid", func(t *ftt.Test) {
			t.Run("Does not start with //", func(t *ftt.Test) {
				err := ValidateFullResourceName("resultdb.api.luci.app/rootInvocations/inv")
				assert.Loosely(t, err, should.ErrLike(`resource name "resultdb.api.luci.app/rootInvocations/inv" does not start with '//'`))
			})
			t.Run("Too long", func(t *ftt.Test) {
				longName := "//" + strings.Repeat("a", 2000) + "/a"
				err := ValidateFullResourceName(longName)
				assert.Loosely(t, err, should.ErrLike(`resource name exceeds 2000 characters`))
			})
			t.Run("Missing service name", func(t *ftt.Test) {
				err := ValidateFullResourceName("///invocations/inv")
				assert.Loosely(t, err, should.ErrLike(`resource name "///invocations/inv" is missing a service name`))
			})
			t.Run("Missing resource path", func(t *ftt.Test) {
				err := ValidateFullResourceName("//resultdb.googleapis.com")
				assert.Loosely(t, err, should.ErrLike(`resource name "//resultdb.googleapis.com" is missing a resource path`))
			})
			t.Run("Missing resource path with slash", func(t *ftt.Test) {
				err := ValidateFullResourceName("//resultdb.googleapis.com/")
				assert.Loosely(t, err, should.ErrLike(`resource name "//resultdb.googleapis.com/" is missing a resource path`))
			})
			t.Run("Not in NFC", func(t *ftt.Test) {
				// "e" followed by a combining acute accent.
				name := "//service/r/cafe\u0301"
				err := ValidateFullResourceName(name)
				assert.Loosely(t, err, should.ErrLike(`is not in Unicode Normal Form C`))
			})
		})
	})
}

func TestSourceModel(t *testing.T) {
	gitilesCommit := &pb.GitilesCommit{
		Host:       "host",
		Project:    "project",
		Ref:        "ref",
		CommitHash: "commit_hash",
		Position:   123,
	}
	submittedAndroidBuild := &pb.SubmittedAndroidBuild{
		DataRealm: "prod",
		Branch:    "branch",
		BuildId:   1234567890,
	}

	ftt.Run("SourceRefFromSources", t, func(t *ftt.Test) {
		t.Run("Gitiles", func(t *ftt.Test) {
			src := &pb.Sources{
				BaseSources: &pb.Sources_GitilesCommit{
					GitilesCommit: gitilesCommit,
				},
			}
			expected := &pb.SourceRef{
				System: &pb.SourceRef_Gitiles{
					Gitiles: &pb.GitilesRef{
						Host:    "host",
						Project: "project",
						Ref:     "ref",
					},
				},
			}
			assert.Loosely(t, SourceRefFromSources(src), should.Match(expected))
		})
		t.Run("AndroidBuild", func(t *ftt.Test) {
			src := &pb.Sources{
				BaseSources: &pb.Sources_SubmittedAndroidBuild{
					SubmittedAndroidBuild: submittedAndroidBuild,
				},
			}
			expected := &pb.SourceRef{
				System: &pb.SourceRef_AndroidBuild{
					AndroidBuild: &pb.AndroidBuildBranch{
						DataRealm: "prod",
						Branch:    "branch",
					},
				},
			}
			assert.Loosely(t, SourceRefFromSources(src), should.Match(expected))
		})
	})
	ftt.Run("SourcePositionFromSources", t, func(t *ftt.Test) {
		t.Run("Gitiles", func(t *ftt.Test) {
			src := &pb.Sources{
				BaseSources: &pb.Sources_GitilesCommit{
					GitilesCommit: gitilesCommit,
				},
			}
			assert.Loosely(t, SourcePosition(src), should.Equal(int64(123)))
		})
		t.Run("AndroidBuild", func(t *ftt.Test) {
			src := &pb.Sources{
				BaseSources: &pb.Sources_SubmittedAndroidBuild{
					SubmittedAndroidBuild: submittedAndroidBuild,
				},
			}
			assert.Loosely(t, SourcePosition(src), should.Equal(int64(1234567890)))
		})
	})
	ftt.Run("SourceRefHash", t, func(t *ftt.Test) {
		t.Run("Gitiles", func(t *ftt.Test) {
			ref := &pb.SourceRef{
				System: &pb.SourceRef_Gitiles{
					Gitiles: &pb.GitilesRef{
						Host:    "project.googlesource.com",
						Project: "myproject/src",
						Ref:     "refs/heads/main",
					},
				},
			}
			hash := SourceRefHash(ref)
			assert.Loosely(t, hex.EncodeToString(hash), should.Equal(`5d47c679cf080cb5`))
		})
		t.Run("AndroidBuild", func(t *ftt.Test) {
			sr := &pb.SourceRef{
				System: &pb.SourceRef_AndroidBuild{
					AndroidBuild: &pb.AndroidBuildBranch{
						DataRealm: "prod",
						Branch:    "branch",
					},
				},
			}
			assert.Loosely(t, hex.EncodeToString(SourceRefHash(sr)), should.Equal("7abd7eb5810c61dd"))
		})
	})
}
