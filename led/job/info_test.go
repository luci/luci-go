// Copyright 2020 The LUCI Authors.
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

package job

import (
	"testing"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGetCurrentIsolated(t *testing.T) {
	t.Parallel()

	Convey(`GetCurrentIsolated`, t, func() {
		Convey(`none (bb)`, func() {
			current, err := testBBJob(false).Info().CurrentIsolated()
			So(err, ShouldBeNil)
			So(current, ShouldBeNil)
		})

		Convey(`CasUserPayload`, func() {
			jd := testBBJob(false)
			jd.GetBuildbucket().BbagentArgs = &bbpb.BBAgentArgs{
				Build: &bbpb.Build{
					Infra: &bbpb.BuildInfra{
						Buildbucket: &bbpb.BuildInfra_Buildbucket{
							Agent: &bbpb.BuildInfra_Buildbucket_Agent{
								Input: &bbpb.BuildInfra_Buildbucket_Agent_Input{
									Data: map[string]*bbpb.InputDataRef{
										"payload_path": {
											DataType: &bbpb.InputDataRef_Cas{
												Cas: &bbpb.InputDataRef_CAS{
													Digest: &bbpb.InputDataRef_CAS_Digest{
														Hash: "hash",
													},
												},
											},
										},
									},
								},
								Purposes: map[string]bbpb.BuildInfra_Buildbucket_Agent_Purpose{
									"payload_path": bbpb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
								},
							},
						},
					},
				},
			}
			current, err := jd.Info().CurrentIsolated()
			So(err, ShouldBeNil)
			So(current, ShouldResembleProto, &swarmingpb.CASReference{Digest: &swarmingpb.Digest{Hash: "hash"}})
		})

		Convey(`Swarming`, func() {
			Convey(`rbe-cas`, func() {
				Convey(`one slice`, func() {
					jd := testSWJob()
					jd.GetSwarming().Task = &swarmingpb.NewTaskRequest{
						TaskSlices: []*swarmingpb.TaskSlice{
							{
								Properties: &swarmingpb.TaskProperties{CasInputRoot: &swarmingpb.CASReference{
									Digest: &swarmingpb.Digest{Hash: "hash"},
								}},
							},
						},
					}
					current, err := jd.Info().CurrentIsolated()
					So(err, ShouldBeNil)
					So(current, ShouldResembleProto, &swarmingpb.CASReference{Digest: &swarmingpb.Digest{Hash: "hash"}})
				})

				Convey(`slice+CasUserPayload (match)`, func() {
					jd := testSWJob()
					jd.GetSwarming().CasUserPayload = &swarmingpb.CASReference{CasInstance: "instance"}
					jd.GetSwarming().Task = &swarmingpb.NewTaskRequest{
						TaskSlices: []*swarmingpb.TaskSlice{
							{
								Properties: &swarmingpb.TaskProperties{CasInputRoot: &swarmingpb.CASReference{
									Digest: &swarmingpb.Digest{Hash: "hash"},
								}},
							},
						},
					}
					current, err := jd.Info().CurrentIsolated()
					So(err, ShouldBeNil)
					So(current, ShouldResembleProto, &swarmingpb.CASReference{Digest: &swarmingpb.Digest{Hash: "hash"}})
				})

				Convey(`slice+CasUserPayload (mismatch)`, func() {
					jd := testSWJob()
					jd.GetSwarming().CasUserPayload = &swarmingpb.CASReference{Digest: &swarmingpb.Digest{Hash: "new hash"}}
					jd.GetSwarming().Task = &swarmingpb.NewTaskRequest{
						TaskSlices: []*swarmingpb.TaskSlice{
							{
								Properties: &swarmingpb.TaskProperties{CasInputRoot: &swarmingpb.CASReference{
									Digest: &swarmingpb.Digest{Hash: "hash"},
								}},
							},
						},
					}
					_, err := jd.Info().CurrentIsolated()
					So(err, ShouldErrLike, "Definition contains multiple RBE-CAS inputs")
				})
			})
		})
	})
}
