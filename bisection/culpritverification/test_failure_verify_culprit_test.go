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

// Package culpritverification performs culprit verification for test failures.
package culpritverification

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/golang/mock/gomock"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/hosts"
	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/internal/gitiles"
	"go.chromium.org/luci/bisection/model"
	configpb "go.chromium.org/luci/bisection/proto/config"
	pb "go.chromium.org/luci/bisection/proto/v1"
	tpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/util/testutil"
)

func TestProcessTestFailureTask(t *testing.T) {
	t.Parallel()
	c := context.Background()
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)
	c = hosts.UseHosts(c, hosts.ModuleOptions{
		APIHost: "test-bisection-host",
	})

	// Setup mock for buildbucket
	ctl := gomock.NewController(t)
	defer ctl.Finish()
	mc := buildbucket.NewMockedClient(c, ctl)
	c = mc.Ctx
	build1 := buildbucket.MockScheduleBuild(mc, 123, "3425")
	build2 := buildbucket.MockScheduleBuild(mc, 456, "3424")
	buildbucket.MockGetBuild(mc)

	c = memory.Use(c)
	testutil.UpdateIndices(c)

	// Setup config.
	projectCfg := config.CreatePlaceholderProjectConfig()
	cfg := map[string]*configpb.ProjectConfig{"chromium": projectCfg}
	assert.Loosely(t, config.SetTestProjectConfig(c, cfg), should.BeNil)

	tfa := testutil.CreateTestFailureAnalysis(c, t, nil)
	nsa := testutil.CreateTestNthSectionAnalysis(c, t, &testutil.TestNthSectionAnalysisCreationOption{
		ParentAnalysisKey: datastore.KeyForObj(c, tfa),
	})
	suspect := testutil.CreateSuspect(c, t, &testutil.SuspectCreationOption{
		ParentKey: datastore.KeyForObj(c, nsa),
		CommitID:  "3425",
	})
	tf := testutil.CreateTestFailure(c, t, &testutil.TestFailureCreationOption{IsPrimary: true, Analysis: tfa})
	gitilesResponse := model.ChangeLogResponse{
		Log: []*model.ChangeLog{
			{
				Commit: "3424",
			},
		},
	}
	gitilesResponseStr, _ := json.Marshal(gitilesResponse)
	c = gitiles.MockedGitilesClientContext(c, map[string]string{
		"https://chromium.googlesource.com/chromium/src/+log/3425~2..3425^": string(gitilesResponseStr),
	})
	task := &tpb.TestFailureCulpritVerificationTask{
		AnalysisId: tfa.ID,
	}

	err := processTestFailureTask(c, task)
	assert.Loosely(t, err, should.BeNil)
	datastore.GetTestable(c).CatchupIndexes()
	// Check suspect updated.
	assert.Loosely(t, datastore.Get(c, suspect), should.BeNil)
	assert.Loosely(t, suspect.VerificationStatus, should.Equal(model.SuspectVerificationStatus_UnderVerification))
	// Check rerun saved.
	suspectRerun := &model.TestSingleRerun{
		ID: suspect.SuspectRerunBuild.IntID(),
	}
	err = datastore.Get(c, suspectRerun)
	assert.Loosely(t, err, should.BeNil)
	expectedRerun := &model.TestSingleRerun{
		ID: build1.Id,
		LUCIBuild: model.LUCIBuild{
			BuildID:       build1.Id,
			Project:       build1.Builder.Project,
			Bucket:        build1.Builder.Bucket,
			Builder:       build1.Builder.Builder,
			GitilesCommit: build1.Input.GitilesCommit,
			CreateTime:    build1.CreateTime.AsTime(),
			EndTime:       time.Time{},
			StartTime:     build1.StartTime.AsTime(),
			Status:        bbpb.Status_STARTED,
		},
		Type:        model.RerunBuildType_CulpritVerification,
		AnalysisKey: datastore.KeyForObj(c, tfa),
		CulpritKey:  datastore.KeyForObj(c, suspect),
		TestResults: model.RerunTestResults{
			IsFinalized: false,
			Results: []model.RerunSingleTestResult{{
				TestFailureKey: datastore.KeyForObj(c, tf),
			}},
		},
		Dimensions: &pb.Dimensions{},
		Status:     pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
	}
	assert.Loosely(t, suspectRerun, should.Match(expectedRerun))
	parentRerun := &model.TestSingleRerun{
		ID: suspect.ParentRerunBuild.IntID(),
	}
	err = datastore.Get(c, parentRerun)
	assert.Loosely(t, err, should.BeNil)
	expectedRerun.ID = build2.Id
	expectedRerun.LUCIBuild.BuildID = build2.Id
	expectedRerun.LUCIBuild.GitilesCommit = build2.Input.GitilesCommit
	assert.Loosely(t, parentRerun, should.Match(expectedRerun))
}
