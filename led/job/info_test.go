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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"
)

func TestGetCurrentIsolated(t *testing.T) {
	t.Parallel()

	ftt.Run(`GetCurrentIsolated`, t, func(t *ftt.Test) {
		t.Run(`none (bb)`, func(t *ftt.Test) {
			current, err := testBBJob(false).Info().CurrentIsolated()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, current, should.BeNil)
		})

		t.Run(`CasUserPayload`, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, current, should.Match(&swarmingpb.CASReference{Digest: &swarmingpb.Digest{Hash: "hash"}}))
		})

		t.Run(`Swarming`, func(t *ftt.Test) {
			t.Run(`rbe-cas`, func(t *ftt.Test) {
				t.Run(`one slice`, func(t *ftt.Test) {
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
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, current, should.Match(&swarmingpb.CASReference{Digest: &swarmingpb.Digest{Hash: "hash"}}))
				})

				t.Run(`slice+CasUserPayload (match)`, func(t *ftt.Test) {
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
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, current, should.Match(&swarmingpb.CASReference{Digest: &swarmingpb.Digest{Hash: "hash"}}))
				})

				t.Run(`slice+CasUserPayload (mismatch)`, func(t *ftt.Test) {
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
					assert.Loosely(t, err, should.ErrLike("Definition contains multiple RBE-CAS inputs"))
				})
			})
		})
	})
}
