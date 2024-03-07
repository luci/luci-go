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

package util

import (
	"testing"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"google.golang.org/protobuf/types/known/structpb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGetGitilesCommitForBuild(t *testing.T) {
	Convey("Get position from output", t, func() {
		build := &bbpb.Build{
			Input: &bbpb.Build_Input{
				GitilesCommit: &bbpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Id:      "refs/heads/gfiTest",
					Ref:     "1",
				},
			},
			Output: &bbpb.Build_Output{
				GitilesCommit: &bbpb.GitilesCommit{
					Host:     "chromium.googlesource.com",
					Project:  "chromium/src",
					Id:       "refs/heads/gfiTest",
					Ref:      "1",
					Position: 456,
				},
			},
		}
		commit := GetGitilesCommitForBuild(build)
		So(commit, ShouldResembleProto, &bbpb.GitilesCommit{
			Host:     "chromium.googlesource.com",
			Project:  "chromium/src",
			Id:       "refs/heads/gfiTest",
			Ref:      "1",
			Position: 456,
		})
	})

	Convey("Output does not match input", t, func() {
		build := &bbpb.Build{
			Input: &bbpb.Build_Input{
				GitilesCommit: &bbpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Id:      "refs/heads/gfiTest",
					Ref:     "1",
				},
			},
			Output: &bbpb.Build_Output{
				GitilesCommit: &bbpb.GitilesCommit{
					Host:     "chromium1.googlesource.com",
					Project:  "chromium/src",
					Id:       "refs/heads/gfiTest",
					Ref:      "1",
					Position: 456,
				},
			},
		}
		commit := GetGitilesCommitForBuild(build)
		So(commit, ShouldResembleProto, &bbpb.GitilesCommit{
			Host:    "chromium.googlesource.com",
			Project: "chromium/src",
			Id:      "refs/heads/gfiTest",
			Ref:     "1",
		})
	})

	Convey("No output", t, func() {
		build := &bbpb.Build{
			Input: &bbpb.Build_Input{
				GitilesCommit: &bbpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Id:      "refs/heads/gfiTest",
					Ref:     "1",
				},
			},
		}
		commit := GetGitilesCommitForBuild(build)
		So(commit, ShouldResembleProto, &bbpb.GitilesCommit{
			Host:    "chromium.googlesource.com",
			Project: "chromium/src",
			Id:      "refs/heads/gfiTest",
			Ref:     "1",
		})
	})
}

func TestGetSheriffRotationsForBuild(t *testing.T) {
	Convey("No sheriff rotation", t, func() {
		build := &bbpb.Build{}
		rotations := GetSheriffRotationsForBuild(build)
		So(rotations, ShouldResemble, []string{})
	})

	Convey("Has sheriff rotation", t, func() {
		build := &bbpb.Build{
			Input: &bbpb.Build_Input{
				Properties: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"sheriff_rotations": structpb.NewListValue(&structpb.ListValue{
							Values: []*structpb.Value{
								structpb.NewStringValue("chromium"),
							},
						}),
					},
				},
			},
		}
		rotations := GetSheriffRotationsForBuild(build)
		So(rotations, ShouldResemble, []string{"chromium"})
	})

}

func TestGetTaskDimensions(t *testing.T) {
	dims := []*bbpb.RequestedDimension{
		{
			Key:   "dimen_key_1",
			Value: "dimen_val_1",
		},
	}
	Convey("from build on swarming", t, func() {
		build := &bbpb.Build{
			Infra: &bbpb.BuildInfra{
				Swarming: &bbpb.BuildInfra_Swarming{
					TaskDimensions: dims,
				},
			},
		}
		got := GetTaskDimensions(build)
		So(got, ShouldResembleProto, dims)
	})

	Convey("from build on backend", t, func() {
		build := &bbpb.Build{
			Infra: &bbpb.BuildInfra{
				Backend: &bbpb.BuildInfra_Backend{
					TaskDimensions: dims,
				},
			},
		}
		got := GetTaskDimensions(build)
		So(got, ShouldResembleProto, dims)
	})

}
