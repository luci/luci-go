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
	"os"
	"path/filepath"
	"strings"
	"testing"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPrependPath(t *testing.T) {
	originalPathEnv := os.Getenv("PATH")
	Convey("prependPath", t, func() {
		defer func() {
			_ = os.Setenv("PATH", originalPathEnv)
		}()

		build := &pb.Build{
			Id: 123,
			Infra: &pb.BuildInfra{
				Buildbucket: &pb.BuildInfra_Buildbucket{
					Agent: &pb.BuildInfra_Buildbucket_Agent{
						Input: &pb.BuildInfra_Buildbucket_Agent_Input{
							Data: map[string]*pb.InputDataRef{
								"path_a": {
									DataType: &pb.InputDataRef_Cipd{
										Cipd: &pb.InputDataRef_CIPD{
											Specs: []*pb.InputDataRef_CIPD_PkgSpec{{Package: "pkg_a", Version: "latest"}},
										},
									},
									OnPath: []string{"path_a/bin", "path_a"},
								},
								"path_b": {
									DataType: &pb.InputDataRef_Cas{
										Cas: &pb.InputDataRef_CAS{
											CasInstance: "projects/project/instances/instance",
											Digest: &pb.InputDataRef_CAS_Digest{
												Hash:      "hash",
												SizeBytes: 1,
											},
										},
									},
									OnPath: []string{"path_b/bin", "path_b"},
								},
							},
						},
						Output: &pb.BuildInfra_Buildbucket_Agent_Output{},
					},
				},
			},
			Input: &pb.Build_Input{
				Experiments: []string{"luci.buildbucket.agent.cipd_installation"},
			},
		}

		cwd, err := os.Getwd()
		So(err, ShouldBeNil)
		So(prependPath(build, cwd), ShouldBeNil)
		pathEnv := os.Getenv("PATH")
		var expectedPath []string
		for _, p := range []string{"path_a", "path_a/bin", "path_b", "path_b/bin"} {
			expectedPath = append(expectedPath, filepath.Join(cwd, p))
		}
		So(strings.Contains(pathEnv, strings.Join(expectedPath, string(os.PathListSeparator))), ShouldBeTrue)
	})
}
