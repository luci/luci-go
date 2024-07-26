// Copyright 2024 The LUCI Authors.
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

package host

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/lucictx"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDownloadInputs(t *testing.T) {
	resultsFilePath = filepath.Join(t.TempDir(), "cipd_ensure_results.json")
	Convey(`test download agent inputs`, t, func() {
		ctx, closer := testCtx()
		defer closer()

		execCommandContext = fakeExecCommand
		defer func() { execCommandContext = exec.CommandContext }()

		opts := &Options{
			BaseBuild: &bbpb.Build{
				Infra: &bbpb.BuildInfra{
					Buildbucket: &bbpb.BuildInfra_Buildbucket{},
				},
			},
			DownloadAgentInputs: true,
		}

		Convey(`empty`, func() {
			ch, err := Run(ctx, opts, func(ctx context.Context, _ Options, _ <-chan lucictx.DeadlineEvent, _ func()) {
				So("unreachable", ShouldBeEmpty)
			})

			So(err, ShouldBeNil)
			build := opts.BaseBuild
			for build = range ch {
			}
			So(build.Output.Status, ShouldEqual, bbpb.Status_INFRA_FAILURE)
			So(build.Infra.Buildbucket.Agent.Output.Status, ShouldEqual, bbpb.Status_FAILURE)
			So(build.Infra.Buildbucket.Agent.Output.SummaryMarkdown, ShouldContainSubstring, "Build Agent field is not set")
		})

		Convey(`success`, func() {
			testCase = "success"
			opts.BaseBuild.Infra.Buildbucket.Agent = &bbpb.BuildInfra_Buildbucket_Agent{
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
			}
			ch, err := Run(ctx, opts, func(ctx context.Context, _ Options, _ <-chan lucictx.DeadlineEvent, _ func()) {
			})

			So(err, ShouldBeNil)
			build := <-ch
			So(build.Infra.Buildbucket.Agent.Output.Status, ShouldEqual, bbpb.Status_STARTED)
			for build = range ch {
			}
			So(build.Infra.Buildbucket.Agent.Output.Status, ShouldEqual, bbpb.Status_SUCCESS)
		})

		Convey(`failure`, func() {
			testCase = "failure"
			opts.BaseBuild.Infra.Buildbucket.Agent = &bbpb.BuildInfra_Buildbucket_Agent{
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
			}
			ch, err := Run(ctx, opts, func(ctx context.Context, _ Options, _ <-chan lucictx.DeadlineEvent, _ func()) {
			})

			So(err, ShouldBeNil)
			build := opts.BaseBuild
			for build = range ch {
			}
			So(build.Infra.Buildbucket.Agent.Output.Status, ShouldEqual, bbpb.Status_FAILURE)
			So(build.Infra.Buildbucket.Agent.Output.SummaryMarkdown, ShouldContainSubstring, "cipd ensure")
		})
	})
}
