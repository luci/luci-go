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

package rpc

import (
	"context"
	"math/rand"
	"testing"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateSynthesize(t *testing.T) {
	t.Parallel()

	Convey("validateSynthesize", t, func() {
		Convey("nil", func() {
			So(validateSynthesize(&pb.SynthesizeBuildRequest{}), ShouldErrLike, "builder or template_build_id is required")
		})

		Convey("invalid Builder", func() {
			req := &pb.SynthesizeBuildRequest{
				Builder: &pb.BuilderID{
					Project: "project",
					Builder: "builder",
				},
			}
			So(validateSynthesize(req), ShouldErrLike, "builder:")
		})
	})

}

func TestSynthesizeBuild(t *testing.T) {
	const userID = identity.Identity("user:user@example.com")

	Convey("SynthesizeBuild", t, func() {
		srv := &Builds{}
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		ctx = mathrand.Set(ctx, rand.New(rand.NewSource(0)))
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		So(config.SetTestSettingsCfg(ctx, &pb.SettingsCfg{
			Resultdb: &pb.ResultDBSettings{
				Hostname: "rdbHost",
			},
			Swarming: &pb.SwarmingSettings{
				BbagentPackage: &pb.SwarmingSettings_Package{
					PackageName: "bbagent",
					Version:     "bbagent-version",
				},
				KitchenPackage: &pb.SwarmingSettings_Package{
					PackageName: "kitchen",
					Version:     "kitchen-version",
				},
			},
		}), ShouldBeNil)

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: userID,
			FakeDB: authtest.NewFakeDB(
				authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
			),
		})
		So(datastore.Put(ctx, &model.Bucket{
			ID:     "bucket",
			Parent: model.ProjectKey(ctx, "project"),
			Proto:  &pb.Bucket{},
		}), ShouldBeNil)

		Convey("fail", func() {
			Convey("permission denied for getting builder", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:unauthorized@example.com",
				})
				req := &pb.SynthesizeBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				}
				_, err := srv.SynthesizeBuild(ctx, req)
				So(err, ShouldErrLike, "not found")
			})
			Convey("permission denied for getting template build", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:unauthorized@example.com",
				})
				So(datastore.Put(ctx, &model.Build{
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					},
				}), ShouldBeNil)

				req := &pb.SynthesizeBuildRequest{
					TemplateBuildId: 1,
				}
				_, err := srv.SynthesizeBuild(ctx, req)
				So(err, ShouldErrLike, "not found")
			})
		})

		Convey("pass", func() {
			So(datastore.Put(ctx, &model.Builder{
				Parent: model.BucketKey(ctx, "project", "bucket"),
				ID:     "builder",
				Config: &pb.BuilderConfig{
					Name:           "builder",
					ServiceAccount: "sa@chops-service-accounts.iam.gserviceaccount.com",
					Dimensions:     []string{"pool:pool1"},
					Properties:     `{"a":"b","b":"b"}`,
					ShadowBuilderAdjustments: &pb.BuilderConfig_ShadowBuilderAdjustments{
						ServiceAccount: "shadow@chops-service-accounts.iam.gserviceaccount.com",
						Pool:           "pool2",
						Properties:     `{"a":"b2","c":"c"}`,
						Dimensions: []string{
							"pool:pool2",
						},
					},
				},
			}), ShouldBeNil)

			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: userID,
				FakeDB: authtest.NewFakeDB(
					authtest.MockPermission(userID, "project:bucket", bbperms.BuildersGet),
					authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
				),
			})

			Convey("template build", func() {
				So(datastore.Put(ctx, &model.Build{
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Input: &pb.Build_Input{
							GerritChanges: []*pb.GerritChange{
								{
									Host:     "host",
									Patchset: 1,
									Project:  "project",
								},
							},
						},
						// Non-retriable build can still be synthesized.
						Retriable: pb.Trinary_NO,
					},
				}), ShouldBeNil)

				req := &pb.SynthesizeBuildRequest{
					TemplateBuildId: 1,
				}
				b, err := srv.SynthesizeBuild(ctx, req)
				So(err, ShouldBeNil)

				expected := &pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Exe: &pb.Executable{
						Cmd: []string{"recipes"},
					},
					ExecutionTimeout: &durationpb.Duration{
						Seconds: 10800,
					},
					GracePeriod: &durationpb.Duration{
						Seconds: 30,
					},
					Infra: &pb.BuildInfra{
						Bbagent: &pb.BuildInfra_BBAgent{
							CacheDir:    "cache",
							PayloadPath: "kitchen-checkout",
						},
						Buildbucket: &pb.BuildInfra_Buildbucket{
							Hostname: "app.appspot.com",
							Agent: &pb.BuildInfra_Buildbucket_Agent{
								Input: &pb.BuildInfra_Buildbucket_Agent_Input{},
								Purposes: map[string]pb.BuildInfra_Buildbucket_Agent_Purpose{
									"kitchen-checkout": pb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
								},
							},
						},
						Logdog: &pb.BuildInfra_LogDog{
							Project: "project",
						},
						Resultdb: &pb.BuildInfra_ResultDB{
							Hostname: "rdbHost",
						},
						Swarming: &pb.BuildInfra_Swarming{
							Caches: []*pb.BuildInfra_Swarming_CacheEntry{
								{
									Name: "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
									Path: "builder",
									WaitForWarmCache: &durationpb.Duration{
										Seconds: 240,
									},
								},
							},
							Priority:           30,
							TaskServiceAccount: "sa@chops-service-accounts.iam.gserviceaccount.com",
							TaskDimensions: []*pb.RequestedDimension{
								{
									Key:   "pool",
									Value: "pool1",
								},
							},
						},
					},
					Input: &pb.Build_Input{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"a": {
									Kind: &structpb.Value_StringValue{
										StringValue: "b",
									},
								},
								"b": {
									Kind: &structpb.Value_StringValue{
										StringValue: "b",
									},
								},
							},
						},
						GerritChanges: []*pb.GerritChange{
							{
								Host:     "host",
								Patchset: 1,
								Project:  "project",
							},
						},
					},
					SchedulingTimeout: &durationpb.Duration{
						Seconds: 21600,
					},
					Tags: []*pb.StringPair{
						{
							Key:   "builder",
							Value: "builder",
						},
						{
							Key:   "buildset",
							Value: "patch/gerrit/host/0/1",
						},
					},
				}
				So(b, ShouldResembleProto, expected)
			})

			Convey("builder", func() {
				So(datastore.Put(ctx, &model.Bucket{
					ID:     "bucket",
					Parent: model.ProjectKey(ctx, "project"),
					Proto: &pb.Bucket{
						Acls: []*pb.Acl{
							{
								Identity: "user:caller@example.com",
								Role:     pb.Acl_READER,
							},
						},
						Shadow: "bucket.shadow",
					},
				}), ShouldBeNil)
				expected := &pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket.shadow",
						Builder: "builder",
					},
					Exe: &pb.Executable{
						Cmd: []string{"recipes"},
					},
					ExecutionTimeout: &durationpb.Duration{
						Seconds: 10800,
					},
					GracePeriod: &durationpb.Duration{
						Seconds: 30,
					},
					Infra: &pb.BuildInfra{
						Bbagent: &pb.BuildInfra_BBAgent{
							CacheDir:    "cache",
							PayloadPath: "kitchen-checkout",
						},
						Buildbucket: &pb.BuildInfra_Buildbucket{
							Hostname: "app.appspot.com",
							Agent: &pb.BuildInfra_Buildbucket_Agent{
								Input: &pb.BuildInfra_Buildbucket_Agent_Input{},
								Purposes: map[string]pb.BuildInfra_Buildbucket_Agent_Purpose{
									"kitchen-checkout": pb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
								},
							},
						},
						Logdog: &pb.BuildInfra_LogDog{
							Project: "project",
						},
						Resultdb: &pb.BuildInfra_ResultDB{
							Hostname: "rdbHost",
						},
						Swarming: &pb.BuildInfra_Swarming{
							Caches: []*pb.BuildInfra_Swarming_CacheEntry{
								{
									Name: "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
									Path: "builder",
									WaitForWarmCache: &durationpb.Duration{
										Seconds: 240,
									},
								},
							},
							Priority:           30,
							TaskServiceAccount: "shadow@chops-service-accounts.iam.gserviceaccount.com",
							TaskDimensions: []*pb.RequestedDimension{
								{
									Key:   "pool",
									Value: "pool2",
								},
							},
						},
						Led: &pb.BuildInfra_Led{
							ShadowedBucket: "bucket",
						},
					},
					Input: &pb.Build_Input{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"$recipe_engine/led": {
									Kind: &structpb.Value_StructValue{
										StructValue: &structpb.Struct{
											Fields: map[string]*structpb.Value{
												"shadowed_bucket": {
													Kind: &structpb.Value_StringValue{
														StringValue: "bucket",
													},
												},
											},
										},
									},
								},
								"a": {
									Kind: &structpb.Value_StringValue{
										StringValue: "b2",
									},
								},
								"b": {
									Kind: &structpb.Value_StringValue{
										StringValue: "b",
									},
								},
								"c": {
									Kind: &structpb.Value_StringValue{
										StringValue: "c",
									},
								},
							},
						},
					},
					SchedulingTimeout: &durationpb.Duration{
						Seconds: 21600,
					},
					Tags: []*pb.StringPair{
						{
							Key:   "builder",
							Value: "builder",
						},
					},
				}
				req := &pb.SynthesizeBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				}
				b, err := srv.SynthesizeBuild(ctx, req)
				So(err, ShouldBeNil)

				So(b, ShouldResembleProto, expected)
			})

			Convey("set experiments", func() {
				So(datastore.Put(ctx, &model.Bucket{
					ID:     "bucket",
					Parent: model.ProjectKey(ctx, "project"),
					Proto: &pb.Bucket{
						Acls: []*pb.Acl{
							{
								Identity: "user:caller@example.com",
								Role:     pb.Acl_READER,
							},
						},
						Shadow: "bucket.shadow",
					},
				}), ShouldBeNil)
				expected := &pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket.shadow",
						Builder: "builder",
					},
					Exe: &pb.Executable{
						Cmd: []string{"recipes"},
					},
					ExecutionTimeout: &durationpb.Duration{
						Seconds: 10800,
					},
					GracePeriod: &durationpb.Duration{
						Seconds: 30,
					},
					Infra: &pb.BuildInfra{
						Bbagent: &pb.BuildInfra_BBAgent{
							CacheDir:    "cache",
							PayloadPath: "kitchen-checkout",
						},
						Buildbucket: &pb.BuildInfra_Buildbucket{
							Hostname: "app.appspot.com",
							ExperimentReasons: map[string]pb.BuildInfra_Buildbucket_ExperimentReason{
								"cool.experiment_thing":     pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_REQUESTED,
								"disabled.experiment_thing": pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_REQUESTED,
							},
							Agent: &pb.BuildInfra_Buildbucket_Agent{
								Input: &pb.BuildInfra_Buildbucket_Agent_Input{},
								Purposes: map[string]pb.BuildInfra_Buildbucket_Agent_Purpose{
									"kitchen-checkout": pb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
								},
							},
						},
						Logdog: &pb.BuildInfra_LogDog{
							Project: "project",
						},
						Resultdb: &pb.BuildInfra_ResultDB{
							Hostname: "rdbHost",
						},
						Swarming: &pb.BuildInfra_Swarming{
							Caches: []*pb.BuildInfra_Swarming_CacheEntry{
								{
									Name: "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
									Path: "builder",
									WaitForWarmCache: &durationpb.Duration{
										Seconds: 240,
									},
								},
							},
							Priority:           30,
							TaskServiceAccount: "shadow@chops-service-accounts.iam.gserviceaccount.com",
							TaskDimensions: []*pb.RequestedDimension{
								{
									Key:   "pool",
									Value: "pool2",
								},
							},
						},
						Led: &pb.BuildInfra_Led{
							ShadowedBucket: "bucket",
						},
					},
					Input: &pb.Build_Input{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"$recipe_engine/led": {
									Kind: &structpb.Value_StructValue{
										StructValue: &structpb.Struct{
											Fields: map[string]*structpb.Value{
												"shadowed_bucket": {
													Kind: &structpb.Value_StringValue{
														StringValue: "bucket",
													},
												},
											},
										},
									},
								},
								"a": {
									Kind: &structpb.Value_StringValue{
										StringValue: "b2",
									},
								},
								"b": {
									Kind: &structpb.Value_StringValue{
										StringValue: "b",
									},
								},
								"c": {
									Kind: &structpb.Value_StringValue{
										StringValue: "c",
									},
								},
							},
						},
						Experiments: []string{
							"cool.experiment_thing",
						},
					},
					SchedulingTimeout: &durationpb.Duration{
						Seconds: 21600,
					},
					Tags: []*pb.StringPair{
						{
							Key:   "builder",
							Value: "builder",
						},
					},
				}
				req := &pb.SynthesizeBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Experiments: map[string]bool{
						"cool.experiment_thing":     true,
						"disabled.experiment_thing": false,
					},
				}
				b, err := srv.SynthesizeBuild(ctx, req)
				So(err, ShouldBeNil)

				So(b, ShouldResembleProto, expected)
			})

		})
	})
}
