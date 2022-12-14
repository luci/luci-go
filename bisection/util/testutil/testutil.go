// Copyright 2022 The LUCI Authors.
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

// Package testutil contains utility functions for test.
package testutil

import (
	"context"
	"fmt"

	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto"
	"go.chromium.org/luci/gae/service/datastore"
)

func CreateBlamelist(nCommits int) *pb.BlameList {
	blamelist := &pb.BlameList{}
	for i := 0; i < nCommits; i++ {
		blamelist.Commits = append(blamelist.Commits, &pb.BlameListSingleCommit{
			Commit: fmt.Sprintf("commit%d", i),
		})
	}
	return blamelist
}

func CreateLuciFailedBuild(c context.Context, id int64) *model.LuciFailedBuild {
	fb := &model.LuciFailedBuild{
		Id: 123,
	}
	So(datastore.Put(c, fb), ShouldBeNil)
	datastore.GetTestable(c).CatchupIndexes()
	return fb
}

func CreateCompileFailure(c context.Context, fb *model.LuciFailedBuild) *model.CompileFailure {
	cf := &model.CompileFailure{
		Id:    fb.Id,
		Build: datastore.KeyForObj(c, fb),
	}
	So(datastore.Put(c, cf), ShouldBeNil)
	datastore.GetTestable(c).CatchupIndexes()
	return cf
}

func CreateCompileFailureAnalysis(c context.Context, id int64, cf *model.CompileFailure) *model.CompileFailureAnalysis {
	cfa := &model.CompileFailureAnalysis{
		Id:             id,
		CompileFailure: datastore.KeyForObj(c, cf),
	}
	So(datastore.Put(c, cfa), ShouldBeNil)
	datastore.GetTestable(c).CatchupIndexes()
	return cfa
}

func CreateCompileFailureAnalysisAnalysisChain(c context.Context, bbid int64, analysisID int64) (*model.LuciFailedBuild, *model.CompileFailure, *model.CompileFailureAnalysis) {
	fb := CreateLuciFailedBuild(c, bbid)
	cf := CreateCompileFailure(c, fb)
	cfa := CreateCompileFailureAnalysis(c, analysisID, cf)
	return fb, cf, cfa
}

func UpdateIndices(c context.Context) {
	datastore.GetTestable(c).AddIndexes(
		&datastore.IndexDefinition{
			Kind: "SingleRerun",
			SortBy: []datastore.IndexColumn{
				{
					Property: "analysis",
				},
				{
					Property: "start_time",
				},
			},
		},
		&datastore.IndexDefinition{
			Kind: "Suspect",
			SortBy: []datastore.IndexColumn{
				{
					Property: "is_revert_created",
				},
				{
					Property: "revert_create_time",
				},
			},
		},
		&datastore.IndexDefinition{
			Kind: "Suspect",
			SortBy: []datastore.IndexColumn{
				{
					Property: "is_revert_committed",
				},
				{
					Property: "revert_commit_time",
				},
			},
		},
		&datastore.IndexDefinition{
			Kind: "SingleRerun",
			SortBy: []datastore.IndexColumn{
				{
					Property: "rerun_build",
				},
				{
					Property: "start_time",
				},
			},
		},
		&datastore.IndexDefinition{
			Kind: "LuciFailedBuild",
			SortBy: []datastore.IndexColumn{
				{
					Property: "project",
				},
				{
					Property: "bucket",
				},
				{
					Property: "builder",
				},
				{
					Property:   "end_time",
					Descending: true,
				},
			},
		},
	)
	datastore.GetTestable(c).CatchupIndexes()
}
