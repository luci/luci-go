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

	"google.golang.org/protobuf/types/known/structpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/luciexe/exe"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"
)

func TestBBEnsureBasics(t *testing.T) {
	t.Parallel()

	ftt.Run(`Buildbucket.EnsureBasics`, t, func(t *ftt.Test) {
		jd := testBBJob(false)
		assert.Loosely(t, jd.GetBuildbucket().GetBbagentArgs().GetBuild(), should.BeNil)

		jd.GetBuildbucket().EnsureBasics()

		assert.Loosely(t, jd.GetBuildbucket().BbagentArgs.Build.Infra, should.NotBeNil)
	})
}

func TestWriteProperties(t *testing.T) {
	t.Parallel()

	ftt.Run(`Buildbucket.WriteProperties`, t, func(t *ftt.Test) {
		jd := testBBJob(false)
		assert.Loosely(t, jd.GetBuildbucket().GetBbagentArgs().GetBuild().GetInput().GetProperties(), should.BeNil)

		jd.GetBuildbucket().WriteProperties(map[string]any{
			"hello": "world",
		})
		assert.Loosely(t, jd.GetBuildbucket().GetBbagentArgs().GetBuild().GetInput().GetProperties(), should.Match(&structpb.Struct{
			Fields: map[string]*structpb.Value{
				"hello": {Kind: &structpb.Value_StringValue{StringValue: "world"}},
			},
		}))
	})
}

func TestUpdateBuildFromBbagentArgs(t *testing.T) {
	t.Parallel()

	ftt.Run(`UpdateBuildFromBbagentArgs`, t, func(t *ftt.Test) {
		bb := testBBJob(false).GetBuildbucket()
		assert.Loosely(t, bb.GetBbagentArgs().GetBuild().GetInfra().GetBuildbucket().GetAgent(), should.BeNil)

		bb.BbagentArgs = &bbpb.BBAgentArgs{
			PayloadPath:            "payload_path",
			KnownPublicGerritHosts: []string{"host"},
		}
		bb.UpdateBuildFromBbagentArgs()

		assert.Loosely(t, bb.GetBbagentArgs().GetBuild().GetInfra().GetBuildbucket(), should.Match(
			&bbpb.BuildInfra_Buildbucket{
				Agent: &bbpb.BuildInfra_Buildbucket_Agent{
					Purposes: map[string]bbpb.BuildInfra_Buildbucket_Agent_Purpose{
						"payload_path": bbpb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
					},
				},
				KnownPublicGerritHosts: []string{"host"},
			}))
	})
}

func TestUpdatePayloadPath(t *testing.T) {
	t.Parallel()

	ftt.Run(`UpdatePayloadPath`, t, func(t *ftt.Test) {
		bb := testBBJob(false).GetBuildbucket()

		bb.BbagentArgs = &bbpb.BBAgentArgs{
			PayloadPath: "payload_path",
		}
		bb.UpdateBuildFromBbagentArgs()
		bb.UpdatePayloadPath("new_path")

		assert.Loosely(t, bb.GetBbagentArgs().GetPayloadPath(), should.Equal("new_path"))
		assert.Loosely(t, bb.GetBbagentArgs().GetBuild().GetInfra().GetBuildbucket(), should.Match(
			&bbpb.BuildInfra_Buildbucket{
				Agent: &bbpb.BuildInfra_Buildbucket_Agent{
					Purposes: map[string]bbpb.BuildInfra_Buildbucket_Agent_Purpose{
						"new_path": bbpb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
					},
				},
			}))
	})
}

func TestUpdateLedProperties(t *testing.T) {
	t.Parallel()

	ftt.Run(`UpdateLedProperties`, t, func(t *ftt.Test) {
		bb := testBBJob(false).GetBuildbucket()
		bb.EnsureBasics()
		t.Run(`cas input`, func(t *ftt.Test) {
			bld := bb.BbagentArgs.Build
			bld.Infra.Buildbucket.Agent = &bbpb.BuildInfra_Buildbucket_Agent{
				Input: &bbpb.BuildInfra_Buildbucket_Agent_Input{
					Data: map[string]*bbpb.InputDataRef{
						"kitchen-checkout": {
							DataType: &bbpb.InputDataRef_Cas{
								Cas: &bbpb.InputDataRef_CAS{
									CasInstance: "projects/project/instances/instance",
									Digest: &bbpb.InputDataRef_CAS_Digest{
										Hash:      "hash",
										SizeBytes: 1,
									},
								},
							},
						},
					},
				},
				Purposes: map[string]bbpb.BuildInfra_Buildbucket_Agent_Purpose{
					"kitchen-checkout": bbpb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
				},
			}
			bb.WriteProperties(map[string]any{
				"$recipe_engine/led": &ledProperties{
					ShadowedBucket: "bucket",
				},
			})

			err := bb.UpdateLedProperties()
			assert.Loosely(t, err, should.BeNil)
			newProps := ledProperties{}
			err = exe.ParseProperties(bld.Input.Properties, map[string]any{
				"$recipe_engine/led": &newProps,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, newProps, should.Match(ledProperties{
				RbeCasInput: &swarmingpb.CASReference{
					CasInstance: "projects/project/instances/instance",
					Digest: &swarmingpb.Digest{
						Hash:      "hash",
						SizeBytes: 1,
					},
				},
				ShadowedBucket: "bucket",
			}))
		})
		t.Run(`cipd input`, func(t *ftt.Test) {
			bld := bb.BbagentArgs.Build
			bld.Infra.Buildbucket.Agent = &bbpb.BuildInfra_Buildbucket_Agent{
				Input: &bbpb.BuildInfra_Buildbucket_Agent_Input{
					Data: map[string]*bbpb.InputDataRef{
						"kitchen-checkout": {
							DataType: &bbpb.InputDataRef_Cipd{
								Cipd: &bbpb.InputDataRef_CIPD{
									Specs: []*bbpb.InputDataRef_CIPD_PkgSpec{
										{
											Package: "package",
											Version: "version"},
									},
								},
							},
						},
					},
				},
				Purposes: map[string]bbpb.BuildInfra_Buildbucket_Agent_Purpose{
					"kitchen-checkout": bbpb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
				},
			}

			err := bb.UpdateLedProperties()
			assert.Loosely(t, err, should.BeNil)
			newProps := ledProperties{}
			err = exe.ParseProperties(bld.Input.Properties, map[string]any{
				"$recipe_engine/led": &newProps,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, newProps, should.Match(ledProperties{
				CIPDInput: &cipdInput{
					Package: "package",
					Version: "version",
				},
			}))
		})
	})
}
