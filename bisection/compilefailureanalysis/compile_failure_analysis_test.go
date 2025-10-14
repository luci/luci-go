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

package compilefailureanalysis

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	lnpb "go.chromium.org/luci/luci_notify/api/service/v1"

	"go.chromium.org/luci/bisection/compilefailureanalysis/llm"
	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/internal/gitiles"
	"go.chromium.org/luci/bisection/internal/logdog"
	"go.chromium.org/luci/bisection/internal/lucinotify"
	"go.chromium.org/luci/bisection/model"
	configpb "go.chromium.org/luci/bisection/proto/config"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/testutil"
)

func TestAnalyzeFailure(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	testutil.UpdateIndices(c)
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	// Setup mock for buildbucket
	ctl := gomock.NewController(t)
	defer ctl.Finish()
	mc := buildbucket.NewMockedClient(c, ctl)
	c = mc.Ctx
	c = gitiles.MockedGitilesClientContext(c, map[string]string{})
	res := &bbpb.Build{
		Input: &bbpb.Build_Input{
			GitilesCommit: &bbpb.GitilesCommit{
				Host:    "host",
				Project: "proj",
				Id:      "id",
				Ref:     "ref",
			},
		},
		Steps: []*bbpb.Step{
			{
				Name: "compile",
				Logs: []*bbpb.Log{
					{
						Name:    "json.output[ninja_info]",
						ViewUrl: "https://logs.chromium.org/logs/ninja_log",
					},
					{
						Name:    "stdout",
						ViewUrl: "https://logs.chromium.org/logs/stdout_log",
					},
				},
			},
		},
	}
	mc.Client.EXPECT().GetBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(res, nil).AnyTimes()

	// Mock logdog
	ninjaLogJson := map[string]any{
		"failures": []map[string]any{
			{
				"output_nodes": []string{
					"obj/net/net_unittests__library/ssl_server_socket_unittest.o",
				},
			},
		},
	}
	ninjaLogStr, _ := json.Marshal(ninjaLogJson)
	c = logdog.MockClientContext(c, map[string]string{
		"https://logs.chromium.org/logs/ninja_log":  string(ninjaLogStr),
		"https://logs.chromium.org/logs/stdout_log": "stdout_log",
	})

	// Mock luci notify
	lnmock := lucinotify.NewMockedClient(c, ctl)
	c = lnmock.Ctx
	resp := &lnpb.CheckTreeCloserResponse{
		IsTreeCloser: true,
	}
	lnmock.Client.EXPECT().CheckTreeCloser(gomock.Any(), gomock.Any(), gomock.Any()).Return(resp, nil).Times(1)

	// Mock LLM client
	mockLLMClient := llm.NewMockClient(ctl)

	// Set up the config
	projectCfg := config.CreatePlaceholderProjectConfig()
	projectCfg.CompileAnalysisConfig.CulpritVerificationEnabled = false
	cfg := map[string]*configpb.ProjectConfig{"chromium": projectCfg}
	assert.Loosely(t, config.SetTestProjectConfig(c, cfg), should.BeNil)

	fb := &model.LuciFailedBuild{
		Id: 88128398584903,
		LuciBuild: model.LuciBuild{
			BuildId:     88128398584903,
			Project:     "chromium",
			Bucket:      "ci",
			Builder:     "android",
			BuildNumber: 123,
			StartTime:   cl.Now(),
			EndTime:     cl.Now(),
			CreateTime:  cl.Now(),
		},
		BuildFailureType: pb.BuildFailureType_COMPILE,
	}
	assert.Loosely(t, datastore.Put(c, fb), should.BeNil)

	cf := testutil.CreateCompileFailure(c, t, fb)

	cfa, err := AnalyzeFailure(c, cf, 123, 456, mockLLMClient)
	assert.Loosely(t, err, should.BeNil)
	datastore.GetTestable(c).CatchupIndexes()
	assert.Loosely(t, cfa.IsTreeCloser, should.BeTrue)

	err = datastore.Get(c, cf)
	assert.Loosely(t, err, should.BeNil)
	assert.Loosely(t, cf.OutputTargets, should.Match([]string{"obj/net/net_unittests__library/ssl_server_socket_unittest.o"}))

	// Make sure that the analysis is created
	q := datastore.NewQuery("CompileFailureAnalysis").Eq("compile_failure", datastore.KeyForObj(c, cf))
	analyses := []*model.CompileFailureAnalysis{}
	datastore.GetAll(c, q, &analyses)
	assert.Loosely(t, len(analyses), should.Equal(1))

	// Make sure the GenAI analysis and nthsection analysis are run
	q = datastore.NewQuery("CompileGenAIAnalysis").Ancestor(datastore.KeyForObj(c, cfa))
	genai_analyses := []*model.CompileGenAIAnalysis{}
	datastore.GetAll(c, q, &genai_analyses)
	assert.Loosely(t, len(genai_analyses), should.Equal(1))

	q = datastore.NewQuery("CompileNthSectionAnalysis").Ancestor(datastore.KeyForObj(c, cfa))
	nthsection_analyses := []*model.CompileNthSectionAnalysis{}
	datastore.GetAll(c, q, &nthsection_analyses)
	assert.Loosely(t, len(nthsection_analyses), should.Equal(1))
}

func TestFindRegressionRange(t *testing.T) {
	t.Parallel()
	// Setup mock for buildbucket
	ctl := gomock.NewController(t)
	defer ctl.Finish()
	c := context.Background()

	ftt.Run("No Gitiles Commit", t, func(t *ftt.Test) {
		mc := buildbucket.NewMockedClient(c, ctl)
		c = mc.Ctx
		res := &bbpb.Build{}
		mc.Client.EXPECT().GetBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(res, nil).AnyTimes()
		_, e := findRegressionRange(c, 8001, 8000)
		assert.Loosely(t, e, should.NotBeNil)
	})

	ftt.Run("Have Gitiles Commit", t, func(t *ftt.Test) {
		mc := buildbucket.NewMockedClient(c, ctl)
		c = mc.Ctx
		res1 := &bbpb.Build{
			Input: &bbpb.Build_Input{
				GitilesCommit: &bbpb.GitilesCommit{
					Host:    "host1",
					Project: "proj1",
					Id:      "id1",
					Ref:     "ref1",
				},
			},
		}

		res2 := &bbpb.Build{
			Input: &bbpb.Build_Input{
				GitilesCommit: &bbpb.GitilesCommit{
					Host:    "host2",
					Project: "proj2",
					Id:      "id2",
					Ref:     "ref2",
				},
			},
		}

		// It is hard to match the exact GetBuildRequest. We use Times() to simulate different response.
		mc.Client.EXPECT().GetBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(res1, nil).Times(1)
		mc.Client.EXPECT().GetBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(res2, nil).Times(1)

		rr, e := findRegressionRange(c, 8001, 8000)
		assert.Loosely(t, e, should.BeNil)

		assert.Loosely(t, rr.FirstFailed, should.Match(&bbpb.GitilesCommit{
			Host:    "host1",
			Project: "proj1",
			Id:      "id1",
			Ref:     "ref1",
		}))

		assert.Loosely(t, rr.LastPassed, should.Match(&bbpb.GitilesCommit{
			Host:    "host2",
			Project: "proj2",
			Id:      "id2",
			Ref:     "ref2",
		}))
	})
}

