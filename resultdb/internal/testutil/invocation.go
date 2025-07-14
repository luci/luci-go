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

package testutil

import (
	"google.golang.org/protobuf/types/known/structpb"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestProperties() *structpb.Struct {
	return &structpb.Struct{
		Fields: map[string]*structpb.Value{
			"key_1": structpb.NewStringValue("value_1"),
			"key_2": structpb.NewStructValue(&structpb.Struct{
				Fields: map[string]*structpb.Value{
					"child_key": structpb.NewNumberValue(1),
				},
			}),
		},
	}
}

func TestSources() *pb.Sources {
	return TestSourcesWithChangelistNumbers(567)
}

func TestSourcesWithChangelistNumbers(changelistNumbers ...int) *pb.Sources {
	result := &pb.Sources{
		GitilesCommit: &pb.GitilesCommit{
			Host:       "chromium.googlesource.com",
			Project:    "infra/infra",
			Ref:        "refs/heads/main",
			CommitHash: "1234567890abcdefabcd1234567890abcdefabcd",
			Position:   12345,
		},
		IsDirty: true,
	}
	for _, cl := range changelistNumbers {
		result.Changelists = append(result.Changelists, &pb.GerritChange{
			Host:     "chromium-review.googlesource.com",
			Project:  "infra/luci-go",
			Change:   int64(cl),
			Patchset: 321,
		})
	}
	return result
}

func TestInvocationExtendedProperties() map[string]*structpb.Struct {
	return map[string]*structpb.Struct{
		"key_1": {
			Fields: map[string]*structpb.Value{
				"@type":       structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
				"child_key_1": structpb.NewStringValue("child_value_1"),
			},
		},
		"key_2": {
			Fields: map[string]*structpb.Value{
				"@type":       structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
				"child_key_1": structpb.NewStringValue("child_value_2"),
			},
		},
	}
}

func TestInstructions() *pb.Instructions {
	return &pb.Instructions{
		Instructions: []*pb.Instruction{
			{
				Id:              "step",
				Type:            pb.InstructionType_STEP_INSTRUCTION,
				DescriptiveName: "Step Instruction",
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
		},
	}
}
