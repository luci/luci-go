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

package main

import (
	"context"
	"flag"
	"os/exec"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/lucictx"
	"go.chromium.org/luci/luciexe"

	"go.chromium.org/luci/buildbucket"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

func TestReadyToFinalize(t *testing.T) {
	ctx := memory.Use(context.Background())
	ctx = memlogger.Use(ctx)
	outputFlag := luciexe.AddOutputFlagToSet(&flag.FlagSet{})
	finalBuild := &bbpb.Build{}

	ftt.Run("Ready to finalize: success", t, func(t *ftt.Test) {
		isReady := readyToFinalize(ctx, finalBuild, nil, nil, outputFlag)
		assert.Loosely(t, isReady, should.Equal(true))

	})

	ftt.Run("Not ready to finalize: fatal error not nil", t, func(t *ftt.Test) {
		isReady := readyToFinalize(ctx, finalBuild, errors.New("Fatal Error Happened"), nil, outputFlag)
		assert.Loosely(t, isReady, should.Equal(false))

	})
}

func TestBackFillTaskInfo(t *testing.T) {
	t.Parallel()
	ftt.Run("backFillTaskInfo", t, func(t *ftt.Test) {
		ctx := lucictx.SetSwarming(context.Background(), &lucictx.Swarming{
			Task: &lucictx.Task{
				BotDimensions: []string{
					"cpu:x86",
					"cpu:x86-64",
					"id:bot_id",
					"gcp:google.com:chromecompute",
				},
			},
		})

		t.Run("swarming", func(t *ftt.Test) {
			build := &bbpb.Build{
				Infra: &bbpb.BuildInfra{
					Swarming: &bbpb.BuildInfra_Swarming{},
				},
			}
			input := clientInput{input: &bbpb.BBAgentArgs{Build: build}}

			assert.Loosely(t, backFillTaskInfo(ctx, input), should.BeZero)
			assert.Loosely(t, build.Infra.Swarming.BotDimensions, should.Resemble([]*bbpb.StringPair{
				{
					Key:   "cpu",
					Value: "x86",
				},
				{
					Key:   "cpu",
					Value: "x86-64",
				},
				{
					Key:   "id",
					Value: "bot_id",
				},
				{
					Key:   "gcp",
					Value: "google.com:chromecompute",
				},
			}))
		})

		t.Run("backend", func(t *ftt.Test) {
			build := &bbpb.Build{
				Infra: &bbpb.BuildInfra{
					Backend: &bbpb.BuildInfra_Backend{},
				},
			}
			input := clientInput{input: &bbpb.BBAgentArgs{Build: build}}
			t.Run("fail", func(t *ftt.Test) {
				assert.Loosely(t, backFillTaskInfo(ctx, input), should.Equal(1))
			})

			t.Run("pass", func(t *ftt.Test) {
				build.Infra.Backend.Task = &bbpb.Task{
					Id: &bbpb.TaskID{
						Id:     "id",
						Target: "swarming://target",
					},
				}
				assert.Loosely(t, backFillTaskInfo(ctx, input), should.BeZero)
				actual, err := protoutil.BotDimensionsFromBackend(build)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, actual, should.Resemble([]*bbpb.StringPair{
					{
						Key:   "cpu",
						Value: "x86",
					},
					{
						Key:   "cpu",
						Value: "x86-64",
					},
					{
						Key:   "gcp",
						Value: "google.com:chromecompute",
					},
					{
						Key:   "id",
						Value: "bot_id",
					},
				}))
			})
		})
	})
}

func TestDownloadInputs(t *testing.T) {
	resultsFilePath = filepath.Join(t.TempDir(), "cipd_ensure_results.json")
	build := &bbpb.Build{
		Id: 123,
		Infra: &bbpb.BuildInfra{
			Buildbucket: &bbpb.BuildInfra_Buildbucket{
				Agent: &bbpb.BuildInfra_Buildbucket_Agent{
					Input: &bbpb.BuildInfra_Buildbucket_Agent_Input{
						CipdSource: map[string]*bbpb.InputDataRef{
							"cipddir": {
								DataType: &bbpb.InputDataRef_Cipd{
									Cipd: &bbpb.InputDataRef_CIPD{
										Server: "chrome-infra-packages.appspot.com",
										Specs: []*bbpb.InputDataRef_CIPD_PkgSpec{
											{
												Package: "infra/tools/cipd/${platform}",
												Version: "latest",
											},
										},
									},
								},
							},
						},
						Data: map[string]*bbpb.InputDataRef{
							"path_a": {
								DataType: &bbpb.InputDataRef_Cipd{
									Cipd: &bbpb.InputDataRef_CIPD{
										Specs: []*bbpb.InputDataRef_CIPD_PkgSpec{{Package: "pkg_a", Version: "latest"}},
									},
								},
								OnPath: []string{"path_a/bin", "path_a"},
							},
						},
					},
					Output: &bbpb.BuildInfra_Buildbucket_Agent_Output{},
				},
			},
		},
		Input: &bbpb.Build_Input{
			Experiments: []string{"luci.buildbucket.agent.cipd_installation"},
		},
		CancelTime: nil,
	}

	ftt.Run("downloadCipdPackages", t, func(c *ftt.Test) {
		c.Run("success", func(c *ftt.Test) {
			testCase = "success"

			ctx := memory.Use(context.Background())
			ctx = memlogger.Use(ctx)
			execCommandContext = fakeExecCommand
			defer func() { execCommandContext = exec.CommandContext }()
			tempDir := t.TempDir()

			bbclient := &testBBClient{}
			input := &bbpb.BBAgentArgs{Build: build}
			rc := downloadInputs(ctx, tempDir, "cache", clientInput{bbclient, input})

			assert.Loosely(c, rc, should.BeZero)
			assert.Loosely(c, len(bbclient.requests), should.Equal(2))
			assert.Loosely(c, bbclient.requests[0].Build.Infra.Buildbucket.Agent.Output.Status, should.Equal(bbpb.Status_STARTED))
			assert.Loosely(c, bbclient.requests[1].Build.Infra.Buildbucket.Agent.Output.Status, should.Equal(bbpb.Status_SUCCESS))
		})
	})

}

func TestStartBuild(t *testing.T) {
	ftt.Run("startBuild", t, func(t *ftt.Test) {
		ctx := context.Background()
		t.Run("pass", func(t *ftt.Test) {
			bbclient := &testBBClient{}
			res, err := startBuild(ctx, bbclient, 87654321, "pass")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.NotBeNil)
		})

		t.Run("duplicate", func(t *ftt.Test) {
			bbclient := &testBBClient{}
			res, err := startBuild(ctx, bbclient, 87654321, "duplicate")
			assert.Loosely(t, res, should.BeNil)
			assert.Loosely(t, buildbucket.DuplicateTask.In(err), should.BeTrue)
		})
	})
}

func TestChooseCacheDir(t *testing.T) {
	input := &bbpb.BBAgentArgs{
		Build:    &bbpb.Build{},
		CacheDir: "inputCacheDir",
	}

	ftt.Run("use input.CacheDir no backend", t, func(t *ftt.Test) {
		cacheDir := chooseCacheDir(input, "")
		assert.Loosely(t, cacheDir, should.Equal("inputCacheDir"))

	})

	ftt.Run("use input.CacheDir backend exists", t, func(t *ftt.Test) {
		input.Build.Infra = &bbpb.BuildInfra{
			Backend: &bbpb.BuildInfra_Backend{},
		}
		cacheDir := chooseCacheDir(input, "")
		assert.Loosely(t, cacheDir, should.Equal("inputCacheDir"))

	})

	ftt.Run("use cache-base flag backend exists", t, func(t *ftt.Test) {
		input.Build.Infra = &bbpb.BuildInfra{
			Backend: &bbpb.BuildInfra_Backend{},
		}
		cacheDir := chooseCacheDir(input, "cache")
		assert.Loosely(t, cacheDir, should.Equal("cache"))

	})
}
