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

	"github.com/google/go-cmp/cmp"
	. "github.com/smartystreets/goconvey/convey"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"google.golang.org/protobuf/proto"
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
		diff := cmp.Diff(commit, &bbpb.GitilesCommit{
			Host:     "chromium.googlesource.com",
			Project:  "chromium/src",
			Id:       "refs/heads/gfiTest",
			Ref:      "1",
			Position: 456,
		}, cmp.Comparer(proto.Equal))
		So(diff, ShouldEqual, "")
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
		diff := cmp.Diff(commit, &bbpb.GitilesCommit{
			Host:    "chromium.googlesource.com",
			Project: "chromium/src",
			Id:      "refs/heads/gfiTest",
			Ref:     "1",
		}, cmp.Comparer(proto.Equal))
		So(diff, ShouldEqual, "")
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
		diff := cmp.Diff(commit, &bbpb.GitilesCommit{
			Host:    "chromium.googlesource.com",
			Project: "chromium/src",
			Id:      "refs/heads/gfiTest",
			Ref:     "1",
		}, cmp.Comparer(proto.Equal))
		So(diff, ShouldEqual, "")
	})
}
