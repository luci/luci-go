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

	"google.golang.org/protobuf/types/known/structpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestGetGitilesCommitForBuild(t *testing.T) {
	ftt.Run("Get position from output", t, func(t *ftt.Test) {
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
		assert.Loosely(t, commit, should.Match(&bbpb.GitilesCommit{
			Host:     "chromium.googlesource.com",
			Project:  "chromium/src",
			Id:       "refs/heads/gfiTest",
			Ref:      "1",
			Position: 456,
		}))
	})

	ftt.Run("Output does not match input", t, func(t *ftt.Test) {
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
		assert.Loosely(t, commit, should.Match(&bbpb.GitilesCommit{
			Host:    "chromium.googlesource.com",
			Project: "chromium/src",
			Id:      "refs/heads/gfiTest",
			Ref:     "1",
		}))
	})

	ftt.Run("No output", t, func(t *ftt.Test) {
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
		assert.Loosely(t, commit, should.Match(&bbpb.GitilesCommit{
			Host:    "chromium.googlesource.com",
			Project: "chromium/src",
			Id:      "refs/heads/gfiTest",
			Ref:     "1",
		}))
	})
}

func TestGetSheriffRotationsForBuild(t *testing.T) {
	ftt.Run("No sheriff rotation", t, func(t *ftt.Test) {
		build := &bbpb.Build{}
		rotations := GetSheriffRotationsForBuild(build)
		assert.Loosely(t, rotations, should.Match([]string{}))
	})

	ftt.Run("Has sheriff rotation", t, func(t *ftt.Test) {
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
		assert.Loosely(t, rotations, should.Match([]string{"chromium"}))
	})

}

func TestGetTaskDimensions(t *testing.T) {
	dims := []*bbpb.RequestedDimension{
		{
			Key:   "dimen_key_1",
			Value: "dimen_val_1",
		},
	}
	ftt.Run("from build on swarming", t, func(t *ftt.Test) {
		build := &bbpb.Build{
			Infra: &bbpb.BuildInfra{
				Swarming: &bbpb.BuildInfra_Swarming{
					TaskDimensions: dims,
				},
			},
		}
		got := GetTaskDimensions(build)
		assert.Loosely(t, got, should.Match(dims))
	})

	ftt.Run("from build on backend", t, func(t *ftt.Test) {
		build := &bbpb.Build{
			Infra: &bbpb.BuildInfra{
				Backend: &bbpb.BuildInfra_Backend{
					TaskDimensions: dims,
				},
			},
		}
		got := GetTaskDimensions(build)
		assert.Loosely(t, got, should.Match(dims))
	})

}
