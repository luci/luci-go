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
	"os"
	"path/filepath"
	"runtime"
	"testing"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func skipWin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping testing in Win")
	}
}

func TestFindCasClient(t *testing.T) {
	skipWin(t)
	Convey("findCasClient", t, func() {
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
											Specs: []*pb.InputDataRef_CIPD_PkgSpec{{Package: "cas_client", Version: "latest"}},
										},
									},
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
								},
							},
						},
						Output: &pb.BuildInfra_Buildbucket_Agent_Output{},
						Purposes: map[string]pb.BuildInfra_Buildbucket_Agent_Purpose{
							"path_a": pb.BuildInfra_Buildbucket_Agent_PURPOSE_BBAGENT_UTILITY,
							"path_b": pb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
						},
					},
				},
			},
		}

		Convey("pass", func() {
			casClient, err := findCasClient("", build)
			So(err, ShouldBeNil)
			cwd, _ := os.Getwd()
			So(casClient, ShouldEqual, filepath.Join(cwd, "path_a/cas"))
		})

		Convey("fail", func() {
			build.Infra.Buildbucket.Agent.Purposes = nil
			_, err := findCasClient("", build)
			So(err, ShouldErrLike, "Failed to find bbagent utility packages")
		})
	})
}
