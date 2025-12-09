// Copyright 2025 The LUCI Authors.
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

package nthsection

import (
	"context"
	"testing"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/bisection/nthsectionsnapshot"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/testutil"
)

func TestCreateSnapshot(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	testutil.UpdateIndices(c)

	ftt.Run("Create Snapshot", t, func(t *ftt.Test) {
		tfa := &model.TestFailureAnalysis{}
		assert.Loosely(t, datastore.Put(c, tfa), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()
		blamelist := testutil.CreateBlamelist(5)
		nsa := &model.TestNthSectionAnalysis{
			BlameList:         blamelist,
			ParentAnalysisKey: datastore.KeyForObj(c, tfa),
		}
		assert.Loosely(t, datastore.Put(c, nsa), should.BeNil)

		rerun1 := &model.TestSingleRerun{
			Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
			LUCIBuild: model.LUCIBuild{
				GitilesCommit: &bbpb.GitilesCommit{
					Id: "commit1",
				},
			},
			NthSectionAnalysisKey: datastore.KeyForObj(c, nsa),
		}

		assert.Loosely(t, datastore.Put(c, rerun1), should.BeNil)

		rerun2 := &model.TestSingleRerun{
			Status: pb.RerunStatus_RERUN_STATUS_FAILED,
			LUCIBuild: model.LUCIBuild{
				GitilesCommit: &bbpb.GitilesCommit{
					Id: "commit3",
				},
			},
			NthSectionAnalysisKey: datastore.KeyForObj(c, nsa),
		}
		assert.Loosely(t, datastore.Put(c, rerun2), should.BeNil)

		rerun3 := &model.TestSingleRerun{
			Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
			LUCIBuild: model.LUCIBuild{
				GitilesCommit: &bbpb.GitilesCommit{
					Id: "commit0",
				},
			},
			NthSectionAnalysisKey: datastore.KeyForObj(c, nsa),
		}

		assert.Loosely(t, datastore.Put(c, rerun3), should.BeNil)

		rerun4 := &model.TestSingleRerun{
			Status: pb.RerunStatus_RERUN_STATUS_INFRA_FAILED,
			LUCIBuild: model.LUCIBuild{
				GitilesCommit: &bbpb.GitilesCommit{
					Id: "commit2",
				},
			},
			NthSectionAnalysisKey: datastore.KeyForObj(c, nsa),
		}
		assert.Loosely(t, datastore.Put(c, rerun4), should.BeNil)

		rerun5 := &model.TestSingleRerun{
			Status: pb.RerunStatus_RERUN_STATUS_TEST_SKIPPED,
			LUCIBuild: model.LUCIBuild{
				GitilesCommit: &bbpb.GitilesCommit{
					Id: "commit4",
				},
			},
			NthSectionAnalysisKey: datastore.KeyForObj(c, nsa),
		}
		assert.Loosely(t, datastore.Put(c, rerun5), should.BeNil)

		datastore.GetTestable(c).CatchupIndexes()

		snapshot, err := CreateSnapshot(c, nsa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, snapshot.BlameList, should.Match(blamelist))
		assert.Loosely(t, snapshot.NumInProgress, should.Equal(2))
		assert.Loosely(t, snapshot.NumInfraFailed, should.Equal(1))
		assert.Loosely(t, snapshot.NumTestSkipped, should.Equal(1))
		assert.Loosely(t, snapshot.Runs, should.Match([]*nthsectionsnapshot.Run{
			{
				Index:  0,
				Commit: "commit0",
				Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				Type:   model.RerunBuildType_NthSection,
			},
			{
				Index:  1,
				Commit: "commit1",
				Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				Type:   model.RerunBuildType_NthSection,
			},
			{
				Index:  2,
				Commit: "commit2",
				Status: pb.RerunStatus_RERUN_STATUS_INFRA_FAILED,
				Type:   model.RerunBuildType_NthSection,
			},
			{
				Index:  3,
				Commit: "commit3",
				Status: pb.RerunStatus_RERUN_STATUS_FAILED,
				Type:   model.RerunBuildType_NthSection,
			},
			{
				Index:  4,
				Commit: "commit4",
				Status: pb.RerunStatus_RERUN_STATUS_TEST_SKIPPED,
				Type:   model.RerunBuildType_NthSection,
			},
		}))
	})
}
