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

	"go.chromium.org/luci/buildbucket"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/lucictx"
	"go.chromium.org/luci/luciexe"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestReadyToFinalize(t *testing.T) {
	ctx := memory.Use(context.Background())
	ctx = memlogger.Use(ctx)
	outputFlag := luciexe.AddOutputFlagToSet(&flag.FlagSet{})
	finalBuild := &bbpb.Build{}

	Convey("Ready to finalize: success", t, func() {
		isReady := readyToFinalize(ctx, finalBuild, nil, nil, outputFlag)
		So(isReady, ShouldEqual, true)

	})

	Convey("Not ready to finalize: fatal error not nil", t, func() {
		isReady := readyToFinalize(ctx, finalBuild, errors.New("Fatal Error Happened"), nil, outputFlag)
		So(isReady, ShouldEqual, false)

	})
}

func TestBackFillTaskInfo(t *testing.T) {
	Convey("backFillTaskInfo", t, func() {
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

		build := &bbpb.Build{
			Infra: &bbpb.BuildInfra{
				Swarming: &bbpb.BuildInfra_Swarming{},
			},
		}
		input := clientInput{input: &bbpb.BBAgentArgs{Build: build}}

		So(backFillTaskInfo(ctx, input), ShouldEqual, 0)
		So(build.Infra.Swarming.BotDimensions, ShouldResembleProto, []*bbpb.StringPair{
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

	Convey("downloadCipdPackages", t, func(c C) {
		Convey("success", func() {
			testCase = "success"

			ctx := memory.Use(context.Background())
			ctx = memlogger.Use(ctx)
			execCommandContext = fakeExecCommand
			defer func() { execCommandContext = exec.CommandContext }()
			tempDir := t.TempDir()

			bbclient := &testBBClient{}
			input := &bbpb.BBAgentArgs{Build: build}
			rc := downloadInputs(ctx, tempDir, "cache", clientInput{bbclient, input})

			So(rc, ShouldEqual, 0)
			So(len(bbclient.requests), ShouldEqual, 2)
			So(bbclient.requests[0].Build.Infra.Buildbucket.Agent.Output.Status, ShouldEqual, bbpb.Status_STARTED)
			So(bbclient.requests[1].Build.Infra.Buildbucket.Agent.Output.Status, ShouldEqual, bbpb.Status_SUCCESS)
		})
	})

}

func TestStartBuild(t *testing.T) {
	Convey("startBuild", t, func() {
		ctx := context.Background()
		Convey("pass", func() {
			bbclient := &testBBClient{}
			res, err := startBuild(ctx, bbclient, 87654321, "pass")
			So(err, ShouldBeNil)
			So(res, ShouldNotBeNil)
		})

		Convey("duplicate", func() {
			bbclient := &testBBClient{}
			res, err := startBuild(ctx, bbclient, 87654321, "duplicate")
			So(res, ShouldBeNil)
			So(buildbucket.DuplicateTask.In(err), ShouldBeTrue)
		})
	})
}

func TestChooseCacheDir(t *testing.T) {
	input := &bbpb.BBAgentArgs{
		Build:    &bbpb.Build{},
		CacheDir: "inputCacheDir",
	}

	Convey("use input.CacheDir no backend", t, func() {
		cacheDir := chooseCacheDir(input, "")
		So(cacheDir, ShouldEqual, "inputCacheDir")

	})

	Convey("use input.CacheDir backend exists", t, func() {
		input.Build.Infra = &bbpb.BuildInfra{
			Backend: &bbpb.BuildInfra_Backend{},
		}
		cacheDir := chooseCacheDir(input, "")
		So(cacheDir, ShouldEqual, "inputCacheDir")

	})

	Convey("use cache-base flag backend exists", t, func() {
		input.Build.Infra = &bbpb.BuildInfra{
			Backend: &bbpb.BuildInfra_Backend{},
		}
		cacheDir := chooseCacheDir(input, "cache")
		So(cacheDir, ShouldEqual, "cache")

	})
}
