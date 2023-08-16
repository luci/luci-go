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
	"go.chromium.org/luci/luciexe/exe"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestBBEnsureBasics(t *testing.T) {
	t.Parallel()

	Convey(`Buildbucket.EnsureBasics`, t, func() {
		jd := testBBJob(false)
		So(jd.GetBuildbucket().GetBbagentArgs().GetBuild(), ShouldBeNil)

		jd.GetBuildbucket().EnsureBasics()

		So(jd.GetBuildbucket().BbagentArgs.Build.Infra, ShouldNotBeNil)
	})
}

func TestWriteProperties(t *testing.T) {
	t.Parallel()

	Convey(`Buildbucket.WriteProperties`, t, func() {
		jd := testBBJob(false)
		So(jd.GetBuildbucket().GetBbagentArgs().GetBuild().GetInput().GetProperties(), ShouldBeNil)

		jd.GetBuildbucket().WriteProperties(map[string]any{
			"hello": "world",
		})
		So(jd.GetBuildbucket().GetBbagentArgs().GetBuild().GetInput().GetProperties(), ShouldResembleProto, &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"hello": {Kind: &structpb.Value_StringValue{StringValue: "world"}},
			},
		})
	})
}

func TestUpdateBuildFromBbagentArgs(t *testing.T) {
	t.Parallel()

	Convey(`UpdateBuildFromBbagentArgs`, t, func() {
		bb := testBBJob(false).GetBuildbucket()
		So(bb.GetBbagentArgs().GetBuild().GetInfra().GetBuildbucket().GetAgent(), ShouldBeNil)

		bb.BbagentArgs = &bbpb.BBAgentArgs{
			PayloadPath:            "payload_path",
			KnownPublicGerritHosts: []string{"host"},
		}
		bb.UpdateBuildFromBbagentArgs()

		So(bb.GetBbagentArgs().GetBuild().GetInfra().GetBuildbucket(), ShouldResembleProto,
			&bbpb.BuildInfra_Buildbucket{
				Agent: &bbpb.BuildInfra_Buildbucket_Agent{
					Purposes: map[string]bbpb.BuildInfra_Buildbucket_Agent_Purpose{
						"payload_path": bbpb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
					},
				},
				KnownPublicGerritHosts: []string{"host"},
			})
	})
}

func TestUpdatePayloadPath(t *testing.T) {
	t.Parallel()

	Convey(`UpdatePayloadPath`, t, func() {
		bb := testBBJob(false).GetBuildbucket()

		bb.BbagentArgs = &bbpb.BBAgentArgs{
			PayloadPath: "payload_path",
		}
		bb.UpdateBuildFromBbagentArgs()
		bb.UpdatePayloadPath("new_path")

		So(bb.GetBbagentArgs().GetPayloadPath(), ShouldEqual, "new_path")
		So(bb.GetBbagentArgs().GetBuild().GetInfra().GetBuildbucket(), ShouldResembleProto,
			&bbpb.BuildInfra_Buildbucket{
				Agent: &bbpb.BuildInfra_Buildbucket_Agent{
					Purposes: map[string]bbpb.BuildInfra_Buildbucket_Agent_Purpose{
						"new_path": bbpb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
					},
				},
			})
	})
}

func TestUpdateLedProperties(t *testing.T) {
	t.Parallel()

	Convey(`UpdateLedProperties`, t, func() {
		bb := testBBJob(false).GetBuildbucket()
		bb.EnsureBasics()
		Convey(`cas input`, func() {
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
			So(err, ShouldBeNil)
			newProps := ledProperties{}
			err = exe.ParseProperties(bld.Input.Properties, map[string]any{
				"$recipe_engine/led": &newProps,
			})
			So(err, ShouldBeNil)
			So(newProps, ShouldResemble, ledProperties{
				RbeCasInput: &swarmingpb.CASReference{
					CasInstance: "projects/project/instances/instance",
					Digest: &swarmingpb.Digest{
						Hash:      "hash",
						SizeBytes: 1,
					},
				},
				ShadowedBucket: "bucket",
			})
		})
		Convey(`cipd input`, func() {
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
			So(err, ShouldBeNil)
			newProps := ledProperties{}
			err = exe.ParseProperties(bld.Input.Properties, map[string]any{
				"$recipe_engine/led": &newProps,
			})
			So(err, ShouldBeNil)
			So(newProps, ShouldResemble, ledProperties{
				CIPDInput: &cipdInput{
					Package: "package",
					Version: "version",
				},
			})
		})
	})
}
