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

package rpc

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/tq"

	// TODO(crbug/1242998): Remove once safe get becomes datastore default.
	_ "go.chromium.org/luci/gae/service/datastore/crbug1242998safeget"

	bb "go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestScheduleBuild(t *testing.T) {
	t.Parallel()

	Convey("builderMatches", t, func() {
		Convey("nil", func() {
			So(builderMatches("", nil), ShouldBeTrue)
			So(builderMatches("project/bucket/builder", nil), ShouldBeTrue)
		})

		Convey("empty", func() {
			p := &pb.BuilderPredicate{}
			So(builderMatches("", p), ShouldBeTrue)
			So(builderMatches("project/bucket/builder", p), ShouldBeTrue)
		})

		Convey("regex", func() {
			p := &pb.BuilderPredicate{
				Regex: []string{
					"project/bucket/.+",
				},
			}
			So(builderMatches("", p), ShouldBeFalse)
			So(builderMatches("project/bucket/builder", p), ShouldBeTrue)
			So(builderMatches("project/other/builder", p), ShouldBeFalse)
		})

		Convey("regex exclude", func() {
			p := &pb.BuilderPredicate{
				RegexExclude: []string{
					"project/bucket/.+",
				},
			}
			So(builderMatches("", p), ShouldBeTrue)
			So(builderMatches("project/bucket/builder", p), ShouldBeFalse)
			So(builderMatches("project/other/builder", p), ShouldBeTrue)
		})

		Convey("regex exclude > regex", func() {
			p := &pb.BuilderPredicate{
				Regex: []string{
					"project/bucket/.+",
				},
				RegexExclude: []string{
					"project/bucket/builder",
				},
			}
			So(builderMatches("", p), ShouldBeFalse)
			So(builderMatches("project/bucket/builder", p), ShouldBeFalse)
			So(builderMatches("project/bucket/other", p), ShouldBeTrue)
		})
	})

	Convey("fetchBuilderConfigs", t, func() {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		So(datastore.Put(ctx, &model.Builder{
			Parent: model.BucketKey(ctx, "project", "bucket 1"),
			ID:     "builder 1",
			Config: pb.Builder{
				Name: "builder 1",
			},
		}), ShouldBeNil)
		So(datastore.Put(ctx, &model.Builder{
			Parent: model.BucketKey(ctx, "project", "bucket 1"),
			ID:     "builder 2",
			Config: pb.Builder{
				Name: "builder 2",
			},
		}), ShouldBeNil)

		So(datastore.Put(ctx, &model.Bucket{
			Parent: model.ProjectKey(ctx, "project"),
			ID:     "bucket 1",
			Proto: pb.Bucket{
				Name:     "bucket 1",
				Swarming: &pb.Swarming{},
			},
		}), ShouldBeNil)
		So(datastore.Put(ctx, &model.Bucket{
			Parent: model.ProjectKey(ctx, "project"),
			ID:     "bucket 2",
			Proto: pb.Bucket{
				Name: "bucket 2",
			},
		}), ShouldBeNil)

		Convey("bucket not found", func() {
			reqs := []*pb.ScheduleBuildRequest{
				{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket 3",
						Builder: "builder 1",
					},
				},
			}
			bldrs, err := fetchBuilderConfigs(ctx, reqs)
			So(err, ShouldErrLike, "bucket not found")
			So(bldrs, ShouldBeNil)
		})

		Convey("builder not found", func() {
			reqs := []*pb.ScheduleBuildRequest{
				{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket 1",
						Builder: "builder 3",
					},
				},
			}
			bldrs, err := fetchBuilderConfigs(ctx, reqs)
			So(err, ShouldErrLike, "builder not found")
			So(bldrs, ShouldBeNil)
		})

		Convey("dynamic", func() {
			reqs := []*pb.ScheduleBuildRequest{
				{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket 2",
						Builder: "builder 1",
					},
				},
			}
			bldrs, err := fetchBuilderConfigs(ctx, reqs)
			So(err, ShouldBeNil)
			So(bldrs["project/bucket 2"]["builder 1"], ShouldBeNil)
		})

		Convey("one", func() {
			reqs := []*pb.ScheduleBuildRequest{
				{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket 1",
						Builder: "builder 1",
					},
				},
			}
			bldrs, err := fetchBuilderConfigs(ctx, reqs)
			So(err, ShouldBeNil)
			So(bldrs["project/bucket 1"]["builder 1"], ShouldResembleProto, &pb.Builder{
				Name: "builder 1",
			})
		})

		Convey("many", func() {
			reqs := []*pb.ScheduleBuildRequest{
				{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket 1",
						Builder: "builder 1",
					},
				},
				{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket 1",
						Builder: "builder 2",
					},
				},
				{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket 2",
						Builder: "builder 1",
					},
				},
			}
			bldrs, err := fetchBuilderConfigs(ctx, reqs)
			So(err, ShouldBeNil)
			So(bldrs["project/bucket 1"]["builder 1"], ShouldResembleProto, &pb.Builder{
				Name: "builder 1",
			})
			So(bldrs["project/bucket 1"]["builder 2"], ShouldResembleProto, &pb.Builder{
				Name: "builder 2",
			})
			So(bldrs["project/bucket 2"]["builder 1"], ShouldBeNil)
		})
	})

	Convey("generateBuildNumbers", t, func() {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		Convey("one", func() {
			blds := []*model.Build{
				{
					Proto: pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					},
				},
			}
			err := generateBuildNumbers(ctx, blds)
			So(err, ShouldBeNil)
			So(blds, ShouldResemble, []*model.Build{
				{
					Proto: pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Number: 1,
					},
					Tags: []string{
						"build_address:luci.project.bucket/builder/1",
					},
				},
			})
		})

		Convey("many", func() {
			blds := []*model.Build{
				{
					Proto: pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder1",
						},
					},
				},
				{
					Proto: pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder2",
						},
					},
				},
				{
					Proto: pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder1",
						},
					},
				},
			}
			err := generateBuildNumbers(ctx, blds)
			So(err, ShouldBeNil)
			So(blds, ShouldResemble, []*model.Build{
				{
					Proto: pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder1",
						},
						Number: 1,
					},
					Tags: []string{
						"build_address:luci.project.bucket/builder1/1",
					},
				},
				{
					Proto: pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder2",
						},
						Number: 1,
					},
					Tags: []string{
						"build_address:luci.project.bucket/builder2/1",
					},
				},
				{
					Proto: pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder1",
						},
						Number: 2,
					},
					Tags: []string{
						"build_address:luci.project.bucket/builder1/2",
					},
				},
			})
		})
	})

	Convey("scheduleBuilds", t, func() {
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		ctx = mathrand.Set(ctx, rand.New(rand.NewSource(0)))
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx, sch := tq.TestingContext(ctx, nil)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		store := tsmon.Store(ctx)
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

		// stripProtos strips the Proto field from each of the given *model.Builds,
		// returning a slice whose ith index is the stripped *pb.Build value.
		// Needed because model.Build.Proto can only be compared with ShouldResembleProto
		// while model.Build can only be compared with ShouldResemble.
		stripProtos := func(builds []*model.Build) []*pb.Build {
			ret := make([]*pb.Build, len(builds))
			for i, b := range builds {
				p := b.Proto
				ret[i] = &p
				b.Proto = pb.Build{}
			}
			return ret
		}

		Convey("builder not found", func() {
			Convey("error", func() {
				req := &pb.ScheduleBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				}

				blds, err := scheduleBuilds(ctx, req)
				So(err, ShouldErrLike, "error fetching builders")
				So(blds, ShouldBeNil)
				So(sch.Tasks(), ShouldBeEmpty)
				So(store.Get(ctx, buildCountCreated, time.Time{}, fv("")), ShouldBeNil)
			})

			Convey("dynamic", func() {
				So(datastore.Put(ctx, &model.Bucket{
					Parent: model.ProjectKey(ctx, "project"),
					ID:     "bucket",
					Proto: pb.Bucket{
						Name: "bucket",
					},
				}), ShouldBeNil)
				req := &pb.ScheduleBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					DryRun: true,
				}

				blds, err := scheduleBuilds(ctx, req)
				So(err, ShouldBeNil)
				So(stripProtos(blds), ShouldResembleProto, []*pb.Build{
					{
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
								CacheDir: "cache",
								Input: &pb.BuildInfra_BBAgent_Input{
									CipdPackages: []*pb.BuildInfra_BBAgent_Input_CIPDPackage{
										{
											Name:    "bbagent",
											Path:    ".",
											Version: "bbagent-version",
										},
										{
											Name:    "kitchen",
											Path:    ".",
											Version: "kitchen-version",
										},
										{
											Path: "kitchen-checkout",
										},
									},
								},
								PayloadPath: "kitchen-checkout",
							},
							Buildbucket: &pb.BuildInfra_Buildbucket{},
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
								Priority: 30,
							},
						},
						Input: &pb.Build_Input{
							Properties: &structpb.Struct{},
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
					},
				})
				So(sch.Tasks(), ShouldBeEmpty)
			})
		})

		Convey("dry run", func() {
			So(datastore.Put(ctx, &model.Builder{
				Parent: model.BucketKey(ctx, "project", "bucket"),
				ID:     "builder",
				Config: pb.Builder{
					Name: "builder",
				},
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.Bucket{
				Parent: model.ProjectKey(ctx, "project"),
				ID:     "bucket",
				Proto: pb.Bucket{
					Name: "bucket",
				},
			}), ShouldBeNil)

			Convey("mixed", func() {
				reqs := []*pb.ScheduleBuildRequest{
					{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					},
					{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						DryRun: true,
					},
					{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						DryRun: false,
					},
				}

				blds, err := scheduleBuilds(ctx, reqs...)
				So(err, ShouldErrLike, "all requests must have the same dry_run value")
				So(blds, ShouldBeNil)
				So(sch.Tasks(), ShouldBeEmpty)

				// dry-run should not increase the build creation counter metric.
				So(store.Get(ctx, buildCountCreated, time.Time{}, fv("")), ShouldBeNil)
			})

			Convey("one", func() {
				req := &pb.ScheduleBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					DryRun: true,
				}

				blds, err := scheduleBuilds(ctx, req)
				So(err, ShouldBeNil)
				So(stripProtos(blds), ShouldResembleProto, []*pb.Build{
					{
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
								CacheDir: "cache",
								Input: &pb.BuildInfra_BBAgent_Input{
									CipdPackages: []*pb.BuildInfra_BBAgent_Input_CIPDPackage{
										{
											Name:    "bbagent",
											Path:    ".",
											Version: "bbagent-version",
										},
										{
											Name:    "kitchen",
											Path:    ".",
											Version: "kitchen-version",
										},
										{
											Path: "kitchen-checkout",
										},
									},
								},
								PayloadPath: "kitchen-checkout",
							},
							Buildbucket: &pb.BuildInfra_Buildbucket{},
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
								Priority: 30,
							},
						},
						Input: &pb.Build_Input{
							Properties: &structpb.Struct{},
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
					},
				})
				So(sch.Tasks(), ShouldBeEmpty)
			})
		})

		Convey("zero", func() {
			blds, err := scheduleBuilds(ctx)
			So(err, ShouldBeNil)
			So(blds, ShouldBeEmpty)
			So(sch.Tasks(), ShouldBeEmpty)
		})

		Convey("one", func() {
			req := &pb.ScheduleBuildRequest{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Notify: &pb.NotificationConfig{
					PubsubTopic: "topic",
					UserData:    []byte("data"),
				},
				Tags: []*pb.StringPair{
					{
						Key:   "buildset",
						Value: "buildset",
					},
					{
						Key:   "user_agent",
						Value: "gerrit",
					},
				},
			}
			So(datastore.Put(ctx, &model.Builder{
				Parent: model.BucketKey(ctx, "project", "bucket"),
				ID:     "builder",
				Config: pb.Builder{
					Name: "builder",
				},
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.Bucket{
				Parent: model.ProjectKey(ctx, "project"),
				ID:     "bucket",
				Proto: pb.Bucket{
					Name: "bucket",
				},
			}), ShouldBeNil)

			blds, err := scheduleBuilds(ctx, req)
			So(err, ShouldBeNil)
			So(store.Get(ctx, buildCountCreated, time.Time{}, fv("gerrit")), ShouldEqual, 1)
			So(stripProtos(blds), ShouldResembleProto, []*pb.Build{
				{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					CreatedBy:  "anonymous:anonymous",
					CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
					Exe: &pb.Executable{
						Cmd: []string{"recipes"},
					},
					ExecutionTimeout: &durationpb.Duration{
						Seconds: 10800,
					},
					GracePeriod: &durationpb.Duration{
						Seconds: 30,
					},
					Id: 9021868963221667745,
					Infra: &pb.BuildInfra{
						Bbagent: &pb.BuildInfra_BBAgent{
							CacheDir: "cache",
							Input: &pb.BuildInfra_BBAgent_Input{
								CipdPackages: []*pb.BuildInfra_BBAgent_Input_CIPDPackage{
									{
										Name:    "bbagent",
										Path:    ".",
										Version: "bbagent-version",
									},
									{
										Name:    "kitchen",
										Path:    ".",
										Version: "kitchen-version",
									},
									{
										Path: "kitchen-checkout",
									},
								},
							},
							PayloadPath: "kitchen-checkout",
						},
						Buildbucket: &pb.BuildInfra_Buildbucket{},
						Logdog: &pb.BuildInfra_LogDog{
							Prefix:  "buildbucket/app/9021868963221667745",
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
							Priority: 30,
						},
					},
					Input: &pb.Build_Input{
						Properties: &structpb.Struct{},
					},
					SchedulingTimeout: &durationpb.Duration{
						Seconds: 21600,
					},
					Status: pb.Status_SCHEDULED,
					Tags: []*pb.StringPair{
						{
							Key:   "builder",
							Value: "builder",
						},
						{
							Key:   "buildset",
							Value: "buildset",
						},
						{
							Key:   "user_agent",
							Value: "gerrit",
						},
					},
				},
			})
			So(blds, ShouldResemble, []*model.Build{
				{
					ID:         9021868963221667745,
					BucketID:   "project/bucket",
					BuilderID:  "project/bucket/builder",
					CreatedBy:  "anonymous:anonymous",
					CreateTime: testclock.TestRecentTimeUTC,
					Experiments: []string{
						"-" + bb.ExperimentBBAgentGetBuild,
						"-" + bb.ExperimentBBCanarySoftware,
						"-" + bb.ExperimentBBAgent,
						"-" + bb.ExperimentRecipePY3,
						"-" + bb.ExperimentUseRealms,
					},
					Incomplete: true,
					IsLuci:     true,
					Status:     pb.Status_SCHEDULED,
					Tags: []string{
						"builder:builder",
						"buildset:buildset",
						"user_agent:gerrit",
					},
					Project: "project",
					PubSubCallback: model.PubSubCallback{
						Topic:    "topic",
						UserData: []byte("data"),
					},
					LegacyProperties: model.LegacyProperties{
						Status: model.Scheduled,
					},
				},
			})
			So(sch.Tasks(), ShouldHaveLength, 1)
			So(datastore.Get(ctx, blds), ShouldBeNil)

			ind, err := model.SearchTagIndex(ctx, "buildset", "buildset")
			So(err, ShouldBeNil)
			So(ind, ShouldResemble, []*model.TagIndexEntry{
				{
					BuildID:     9021868963221667745,
					BucketID:    "project/bucket",
					CreatedTime: datastore.RoundTime(testclock.TestRecentTimeUTC),
				},
			})
		})

		Convey("many", func() {
			reqs := []*pb.ScheduleBuildRequest{
				{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "static bucket",
						Builder: "static builder",
					},
					Critical: pb.Trinary_UNSET,
				},
				{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "static bucket",
						Builder: "static builder",
					},
					Critical: pb.Trinary_YES,
				},
				{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "dynamic bucket",
						Builder: "dynamic builder",
					},
					Critical: pb.Trinary_NO,
				},
			}
			So(datastore.Put(ctx, &model.Builder{
				Parent: model.BucketKey(ctx, "project", "static bucket"),
				ID:     "static builder",
				Config: pb.Builder{
					Name: "static builder",
				},
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.Bucket{
				Parent: model.ProjectKey(ctx, "project"),
				ID:     "static bucket",
				Proto: pb.Bucket{
					Name:     "static bucket",
					Swarming: &pb.Swarming{},
				},
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.Bucket{
				Parent: model.ProjectKey(ctx, "project"),
				ID:     "dynamic bucket",
				Proto: pb.Bucket{
					Name: "dynamic bucket",
				},
			}), ShouldBeNil)

			blds, err := scheduleBuilds(ctx, reqs...)
			So(err, ShouldBeNil)

			fvs := []interface{}{"luci.project.static bucket", "static builder", ""}
			So(store.Get(ctx, buildCountCreated, time.Time{}, fvs), ShouldEqual, 2)
			fvs = []interface{}{"luci.project.dynamic bucket", "dynamic builder", ""}
			So(store.Get(ctx, buildCountCreated, time.Time{}, fvs), ShouldEqual, 1)

			So(stripProtos(blds), ShouldResembleProto, []*pb.Build{
				{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "static bucket",
						Builder: "static builder",
					},
					CreatedBy:  "anonymous:anonymous",
					CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
					Exe: &pb.Executable{
						Cmd: []string{"recipes"},
					},
					ExecutionTimeout: &durationpb.Duration{
						Seconds: 10800,
					},
					GracePeriod: &durationpb.Duration{
						Seconds: 30,
					},
					Id: 9021868963221610337,
					Infra: &pb.BuildInfra{
						Bbagent: &pb.BuildInfra_BBAgent{
							CacheDir: "cache",
							Input: &pb.BuildInfra_BBAgent_Input{
								CipdPackages: []*pb.BuildInfra_BBAgent_Input_CIPDPackage{
									{
										Name:    "bbagent",
										Path:    ".",
										Version: "bbagent-version",
									},
									{
										Name:    "kitchen",
										Path:    ".",
										Version: "kitchen-version",
									},
									{
										Path: "kitchen-checkout",
									},
								},
							},
							PayloadPath: "kitchen-checkout",
						},
						Buildbucket: &pb.BuildInfra_Buildbucket{},
						Logdog: &pb.BuildInfra_LogDog{
							Prefix:  "buildbucket/app/9021868963221610337",
							Project: "project",
						},
						Resultdb: &pb.BuildInfra_ResultDB{
							Hostname: "rdbHost",
						},
						Swarming: &pb.BuildInfra_Swarming{
							Caches: []*pb.BuildInfra_Swarming_CacheEntry{
								{
									Name: "builder_943d53aa636f1497a9367662af111471018b08dcd116ae5405ff9fab3b2d5682_v2",
									Path: "builder",
									WaitForWarmCache: &durationpb.Duration{
										Seconds: 240,
									},
								},
							},
							Priority: 30,
						},
					},
					Input: &pb.Build_Input{
						Properties: &structpb.Struct{},
					},
					SchedulingTimeout: &durationpb.Duration{
						Seconds: 21600,
					},
					Status: pb.Status_SCHEDULED,
					Tags: []*pb.StringPair{
						{
							Key:   "builder",
							Value: "static builder",
						},
					},
				},
				{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "static bucket",
						Builder: "static builder",
					},
					CreatedBy:  "anonymous:anonymous",
					CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
					Exe: &pb.Executable{
						Cmd: []string{"recipes"},
					},
					Critical: pb.Trinary_YES,
					ExecutionTimeout: &durationpb.Duration{
						Seconds: 10800,
					},
					GracePeriod: &durationpb.Duration{
						Seconds: 30,
					},
					Id: 9021868963221610321,
					Infra: &pb.BuildInfra{
						Bbagent: &pb.BuildInfra_BBAgent{
							CacheDir: "cache",
							Input: &pb.BuildInfra_BBAgent_Input{
								CipdPackages: []*pb.BuildInfra_BBAgent_Input_CIPDPackage{
									{
										Name:    "bbagent",
										Path:    ".",
										Version: "bbagent-version",
									},
									{
										Name:    "kitchen",
										Path:    ".",
										Version: "kitchen-version",
									},
									{
										Path: "kitchen-checkout",
									},
								},
							},
							PayloadPath: "kitchen-checkout",
						},
						Buildbucket: &pb.BuildInfra_Buildbucket{},
						Logdog: &pb.BuildInfra_LogDog{
							Prefix:  "buildbucket/app/9021868963221610321",
							Project: "project",
						},
						Resultdb: &pb.BuildInfra_ResultDB{
							Hostname: "rdbHost",
						},
						Swarming: &pb.BuildInfra_Swarming{
							Caches: []*pb.BuildInfra_Swarming_CacheEntry{
								{
									Name: "builder_943d53aa636f1497a9367662af111471018b08dcd116ae5405ff9fab3b2d5682_v2",
									Path: "builder",
									WaitForWarmCache: &durationpb.Duration{
										Seconds: 240,
									},
								},
							},
							Priority: 30,
						},
					},
					Input: &pb.Build_Input{
						Properties: &structpb.Struct{},
					},
					SchedulingTimeout: &durationpb.Duration{
						Seconds: 21600,
					},
					Status: pb.Status_SCHEDULED,
					Tags: []*pb.StringPair{
						{
							Key:   "builder",
							Value: "static builder",
						},
					},
				},
				{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "dynamic bucket",
						Builder: "dynamic builder",
					},
					CreatedBy:  "anonymous:anonymous",
					CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
					Exe: &pb.Executable{
						Cmd: []string{"recipes"},
					},
					Critical: pb.Trinary_NO,
					ExecutionTimeout: &durationpb.Duration{
						Seconds: 10800,
					},
					GracePeriod: &durationpb.Duration{
						Seconds: 30,
					},
					Id: 9021868963221610305,
					Infra: &pb.BuildInfra{
						Bbagent: &pb.BuildInfra_BBAgent{
							CacheDir: "cache",
							Input: &pb.BuildInfra_BBAgent_Input{
								CipdPackages: []*pb.BuildInfra_BBAgent_Input_CIPDPackage{
									{
										Name:    "bbagent",
										Path:    ".",
										Version: "bbagent-version",
									},
									{
										Name:    "kitchen",
										Path:    ".",
										Version: "kitchen-version",
									},
									{
										Path: "kitchen-checkout",
									},
								},
							},
							PayloadPath: "kitchen-checkout",
						},
						Buildbucket: &pb.BuildInfra_Buildbucket{},
						Logdog: &pb.BuildInfra_LogDog{
							Prefix:  "buildbucket/app/9021868963221610305",
							Project: "project",
						},
						Resultdb: &pb.BuildInfra_ResultDB{
							Hostname: "rdbHost",
						},
						Swarming: &pb.BuildInfra_Swarming{
							Caches: []*pb.BuildInfra_Swarming_CacheEntry{
								{
									Name: "builder_e229fa0169afaeb5fa8340560ffb3c5fe529169e0207f7378bd115cd74977bd2_v2",
									Path: "builder",
									WaitForWarmCache: &durationpb.Duration{
										Seconds: 240,
									},
								},
							},
							Priority: 30,
						},
					},
					Input: &pb.Build_Input{
						Properties: &structpb.Struct{},
					},
					SchedulingTimeout: &durationpb.Duration{
						Seconds: 21600,
					},
					Status: pb.Status_SCHEDULED,
					Tags: []*pb.StringPair{
						{
							Key:   "builder",
							Value: "dynamic builder",
						},
					},
				},
			})
			So(blds, ShouldResemble, []*model.Build{
				{
					ID:         9021868963221610337,
					BucketID:   "project/static bucket",
					BuilderID:  "project/static bucket/static builder",
					CreatedBy:  "anonymous:anonymous",
					CreateTime: testclock.TestRecentTimeUTC,
					Experiments: []string{
						"-" + bb.ExperimentBBAgentGetBuild,
						"-" + bb.ExperimentBBCanarySoftware,
						"-" + bb.ExperimentBBAgent,
						"-" + bb.ExperimentRecipePY3,
						"-" + bb.ExperimentUseRealms,
					},
					Incomplete: true,
					IsLuci:     true,
					Status:     pb.Status_SCHEDULED,
					Tags: []string{
						"builder:static builder",
					},
					Project: "project",
					LegacyProperties: model.LegacyProperties{
						Status: model.Scheduled,
					},
				},
				{
					ID:         9021868963221610321,
					BucketID:   "project/static bucket",
					BuilderID:  "project/static bucket/static builder",
					CreatedBy:  "anonymous:anonymous",
					CreateTime: testclock.TestRecentTimeUTC,
					Experiments: []string{
						"-" + bb.ExperimentBBAgentGetBuild,
						"-" + bb.ExperimentBBCanarySoftware,
						"-" + bb.ExperimentBBAgent,
						"-" + bb.ExperimentRecipePY3,
						"-" + bb.ExperimentUseRealms,
					},
					Incomplete: true,
					IsLuci:     true,
					Status:     pb.Status_SCHEDULED,
					Tags: []string{
						"builder:static builder",
					},
					Project: "project",
					LegacyProperties: model.LegacyProperties{
						Status: model.Scheduled,
					},
				},
				{
					ID:         9021868963221610305,
					BucketID:   "project/dynamic bucket",
					BuilderID:  "project/dynamic bucket/dynamic builder",
					CreatedBy:  "anonymous:anonymous",
					CreateTime: testclock.TestRecentTimeUTC,
					Experiments: []string{
						"-" + bb.ExperimentBBAgentGetBuild,
						"-" + bb.ExperimentBBCanarySoftware,
						"-" + bb.ExperimentBBAgent,
						"-" + bb.ExperimentRecipePY3,
						"-" + bb.ExperimentUseRealms,
					},
					Incomplete: true,
					IsLuci:     false,
					Status:     pb.Status_SCHEDULED,
					Tags: []string{
						"builder:dynamic builder",
					},
					Project: "project",
					LegacyProperties: model.LegacyProperties{
						Status: model.Scheduled,
					},
				},
			})
			So(sch.Tasks(), ShouldHaveLength, 2)
			So(datastore.Get(ctx, blds), ShouldBeNil)
		})
	})

	Convey("scheduleRequestFromTemplate", t, func() {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:caller@example.com",
		})

		So(datastore.Put(ctx, &model.Bucket{
			ID:     "bucket",
			Parent: model.ProjectKey(ctx, "project"),
			Proto: pb.Bucket{
				Acls: []*pb.Acl{
					{
						Identity: "user:caller@example.com",
						Role:     pb.Acl_READER,
					},
				},
			},
		}), ShouldBeNil)

		Convey("nil", func() {
			ret, err := scheduleRequestFromTemplate(ctx, nil)
			So(err, ShouldBeNil)
			So(ret, ShouldBeNil)
		})

		Convey("empty", func() {
			req := &pb.ScheduleBuildRequest{}
			ret, err := scheduleRequestFromTemplate(ctx, req)
			So(err, ShouldBeNil)
			So(req, ShouldResembleProto, &pb.ScheduleBuildRequest{})
			So(ret, ShouldResembleProto, &pb.ScheduleBuildRequest{})
		})

		Convey("not found", func() {
			req := &pb.ScheduleBuildRequest{
				TemplateBuildId: 1,
			}
			ret, err := scheduleRequestFromTemplate(ctx, req)
			So(err, ShouldErrLike, "not found")
			So(req, ShouldResembleProto, &pb.ScheduleBuildRequest{
				TemplateBuildId: 1,
			})
			So(ret, ShouldBeNil)
		})

		Convey("permission denied", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:unauthorized@example.com",
			})
			So(datastore.Put(ctx, &model.Build{
				Proto: pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
			}), ShouldBeNil)
			req := &pb.ScheduleBuildRequest{
				TemplateBuildId: 1,
			}
			ret, err := scheduleRequestFromTemplate(ctx, req)
			So(err, ShouldErrLike, "not found")
			So(req, ShouldResembleProto, &pb.ScheduleBuildRequest{
				TemplateBuildId: 1,
			})
			So(ret, ShouldBeNil)
		})

		Convey("canary", func() {
			Convey("false default", func() {
				So(datastore.Put(ctx, &model.Build{
					Proto: pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					},
					Experiments: []string{
						"-" + bb.ExperimentBBCanarySoftware,
					},
				}), ShouldBeNil)

				Convey("merge", func() {
					req := &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						Experiments:     map[string]bool{bb.ExperimentBBCanarySoftware: true},
					}
					ret, err := scheduleRequestFromTemplate(ctx, req)
					So(err, ShouldBeNil)
					So(req, ShouldResembleProto, &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						Experiments:     map[string]bool{bb.ExperimentBBCanarySoftware: true},
					})
					So(ret, ShouldResembleProto, &pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: true,
							bb.ExperimentNonProduction:    false,
						},
						Properties: &structpb.Struct{},
					})
				})

				Convey("ok", func() {
					req := &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
					}
					ret, err := scheduleRequestFromTemplate(ctx, req)
					So(err, ShouldBeNil)
					So(req, ShouldResembleProto, &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
					})
					So(ret, ShouldResembleProto, &pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: false,
							bb.ExperimentNonProduction:    false,
						},
						Properties: &structpb.Struct{},
					})
				})
			})

			Convey("true default", func() {
				So(datastore.Put(ctx, &model.Build{
					Proto: pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					},
					Experiments: []string{
						"+" + bb.ExperimentBBCanarySoftware,
					},
				}), ShouldBeNil)

				Convey("merge", func() {
					req := &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: false,
						},
					}
					ret, err := scheduleRequestFromTemplate(ctx, req)
					So(err, ShouldBeNil)
					So(req, ShouldResembleProto, &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: false,
						},
					})
					So(ret, ShouldResembleProto, &pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Properties: &structpb.Struct{},
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: false,
							bb.ExperimentNonProduction:    false,
						},
					})
				})

				Convey("ok", func() {
					req := &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
					}
					ret, err := scheduleRequestFromTemplate(ctx, req)
					So(err, ShouldBeNil)
					So(req, ShouldResembleProto, &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
					})
					So(ret, ShouldResembleProto, &pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: true,
							bb.ExperimentNonProduction:    false,
						},
						Properties: &structpb.Struct{},
					})
				})
			})
		})

		Convey("critical", func() {
			So(datastore.Put(ctx, &model.Build{
				Proto: pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Critical: pb.Trinary_YES,
				},
				Experiments: []string{"-" + bb.ExperimentBBCanarySoftware},
			}), ShouldBeNil)

			Convey("merge", func() {
				req := &pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
					Critical:        pb.Trinary_NO,
				}
				ret, err := scheduleRequestFromTemplate(ctx, req)
				So(err, ShouldBeNil)
				So(req, ShouldResembleProto, &pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
					Critical:        pb.Trinary_NO,
				})
				So(ret, ShouldResembleProto, &pb.ScheduleBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Critical: pb.Trinary_NO,
					Experiments: map[string]bool{
						bb.ExperimentBBCanarySoftware: false,
						bb.ExperimentNonProduction:    false,
					},
					Properties: &structpb.Struct{},
				})
			})

			Convey("ok", func() {
				req := &pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
				}
				ret, err := scheduleRequestFromTemplate(ctx, req)
				So(err, ShouldBeNil)
				So(req, ShouldResembleProto, &pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
				})
				So(ret, ShouldResembleProto, &pb.ScheduleBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Critical: pb.Trinary_YES,
					Experiments: map[string]bool{
						bb.ExperimentBBCanarySoftware: false,
						bb.ExperimentNonProduction:    false,
					},
					Properties: &structpb.Struct{},
				})
			})
		})

		Convey("exe", func() {
			So(datastore.Put(ctx, &model.Build{
				Proto: pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Exe: &pb.Executable{
						CipdPackage: "package",
						CipdVersion: "version",
					},
				},
				Experiments: []string{"-" + bb.ExperimentBBCanarySoftware},
			}), ShouldBeNil)

			Convey("merge", func() {
				Convey("empty", func() {
					req := &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						Exe:             &pb.Executable{},
					}
					ret, err := scheduleRequestFromTemplate(ctx, req)
					So(err, ShouldBeNil)
					So(req, ShouldResembleProto, &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						Exe:             &pb.Executable{},
					})
					So(ret, ShouldResembleProto, &pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Exe: &pb.Executable{},
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: false,
							bb.ExperimentNonProduction:    false,
						},
						Properties: &structpb.Struct{},
					})
				})

				Convey("non-empty", func() {
					req := &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						Exe: &pb.Executable{
							CipdPackage: "package",
							CipdVersion: "new",
						},
					}
					ret, err := scheduleRequestFromTemplate(ctx, req)
					So(err, ShouldBeNil)
					So(req, ShouldResembleProto, &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						Exe: &pb.Executable{
							CipdPackage: "package",
							CipdVersion: "new",
						},
					})
					So(ret, ShouldResembleProto, &pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: false,
							bb.ExperimentNonProduction:    false,
						},
						Exe: &pb.Executable{
							CipdPackage: "package",
							CipdVersion: "new",
						},
						Properties: &structpb.Struct{},
					})
				})
			})

			Convey("ok", func() {
				req := &pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
				}
				ret, err := scheduleRequestFromTemplate(ctx, req)
				So(err, ShouldBeNil)
				So(req, ShouldResembleProto, &pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
				})
				So(ret, ShouldResembleProto, &pb.ScheduleBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Experiments: map[string]bool{
						bb.ExperimentBBCanarySoftware: false,
						bb.ExperimentNonProduction:    false,
					},
					Exe: &pb.Executable{
						CipdPackage: "package",
						CipdVersion: "version",
					},
					Properties: &structpb.Struct{},
				})
			})
		})

		Convey("gerrit changes", func() {
			So(datastore.Put(ctx, &model.Build{
				Proto: pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Input: &pb.Build_Input{
						GerritChanges: []*pb.GerritChange{
							{
								Host:     "example.com",
								Project:  "project",
								Change:   1,
								Patchset: 1,
							},
						},
					},
				},
				Experiments: []string{"-" + bb.ExperimentBBCanarySoftware},
			}), ShouldBeNil)

			Convey("merge", func() {
				Convey("empty", func() {
					req := &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						GerritChanges:   []*pb.GerritChange{},
					}
					ret, err := scheduleRequestFromTemplate(ctx, req)
					So(err, ShouldBeNil)
					So(req, ShouldResembleProto, &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						GerritChanges:   []*pb.GerritChange{},
					})
					So(ret, ShouldResembleProto, &pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: false,
							bb.ExperimentNonProduction:    false,
						},
						GerritChanges: []*pb.GerritChange{
							{
								Host:     "example.com",
								Project:  "project",
								Change:   1,
								Patchset: 1,
							},
						},
						Properties: &structpb.Struct{},
					})
				})

				Convey("non-empty", func() {
					req := &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						GerritChanges: []*pb.GerritChange{
							{
								Host:     "example.com",
								Project:  "project",
								Change:   1,
								Patchset: 2,
							},
						},
					}
					ret, err := scheduleRequestFromTemplate(ctx, req)
					So(err, ShouldBeNil)
					So(req, ShouldResembleProto, &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						GerritChanges: []*pb.GerritChange{
							{
								Host:     "example.com",
								Project:  "project",
								Change:   1,
								Patchset: 2,
							},
						},
					})
					So(ret, ShouldResembleProto, &pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: false,
							bb.ExperimentNonProduction:    false,
						},
						GerritChanges: []*pb.GerritChange{
							{
								Host:     "example.com",
								Project:  "project",
								Change:   1,
								Patchset: 2,
							},
						},
						Properties: &structpb.Struct{},
					})
				})
			})

			Convey("ok", func() {
				req := &pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
				}
				ret, err := scheduleRequestFromTemplate(ctx, req)
				So(err, ShouldBeNil)
				So(req, ShouldResembleProto, &pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
				})
				So(ret, ShouldResembleProto, &pb.ScheduleBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Experiments: map[string]bool{
						bb.ExperimentBBCanarySoftware: false,
						bb.ExperimentNonProduction:    false,
					},
					GerritChanges: []*pb.GerritChange{
						{
							Host:     "example.com",
							Project:  "project",
							Change:   1,
							Patchset: 1,
						},
					},
					Properties: &structpb.Struct{},
				})
			})
		})

		Convey("gitiles commit", func() {
			So(datastore.Put(ctx, &model.Build{
				Proto: pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Input: &pb.Build_Input{
						GitilesCommit: &pb.GitilesCommit{
							Host:    "example.com",
							Project: "project",
							Ref:     "refs/heads/master",
						},
					},
				},
				Experiments: []string{"-" + bb.ExperimentBBCanarySoftware},
			}), ShouldBeNil)
			req := &pb.ScheduleBuildRequest{
				TemplateBuildId: 1,
			}
			ret, err := scheduleRequestFromTemplate(ctx, req)
			So(err, ShouldBeNil)
			So(req, ShouldResembleProto, &pb.ScheduleBuildRequest{
				TemplateBuildId: 1,
			})
			So(ret, ShouldResembleProto, &pb.ScheduleBuildRequest{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Experiments: map[string]bool{
					bb.ExperimentBBCanarySoftware: false,
					bb.ExperimentNonProduction:    false,
				},
				GitilesCommit: &pb.GitilesCommit{
					Host:    "example.com",
					Project: "project",
					Ref:     "refs/heads/master",
				},
				Properties: &structpb.Struct{},
			})
		})

		Convey("input properties", func() {
			So(datastore.Put(ctx, &model.Build{
				Proto: pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
				Experiments: []string{"-" + bb.ExperimentBBCanarySoftware},
			}), ShouldBeNil)

			Convey("empty", func() {
				So(datastore.Put(ctx, &model.BuildInputProperties{
					Build: datastore.MakeKey(ctx, "Build", 1),
				}), ShouldBeNil)

				Convey("merge", func() {
					req := &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"input": {
									Kind: &structpb.Value_StringValue{
										StringValue: "input value",
									},
								},
							},
						},
					}
					ret, err := scheduleRequestFromTemplate(ctx, req)
					So(err, ShouldBeNil)
					So(req, ShouldResembleProto, &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"input": {
									Kind: &structpb.Value_StringValue{
										StringValue: "input value",
									},
								},
							},
						},
					})
					So(ret, ShouldResembleProto, &pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: false,
							bb.ExperimentNonProduction:    false,
						},
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"input": {
									Kind: &structpb.Value_StringValue{
										StringValue: "input value",
									},
								},
							},
						},
					})
				})

				Convey("ok", func() {
					req := &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
					}
					ret, err := scheduleRequestFromTemplate(ctx, req)
					So(err, ShouldBeNil)
					So(req, ShouldResembleProto, &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
					})
					So(ret, ShouldResembleProto, &pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: false,
							bb.ExperimentNonProduction:    false,
						},
						Properties: &structpb.Struct{},
					})
				})
			})

			Convey("non-empty", func() {
				So(datastore.Put(ctx, &model.BuildInputProperties{
					Build: datastore.MakeKey(ctx, "Build", 1),
					Proto: model.DSStruct{
						Struct: structpb.Struct{
							Fields: map[string]*structpb.Value{
								"input": {
									Kind: &structpb.Value_StringValue{
										StringValue: "input value",
									},
								},
							},
						},
					},
				}), ShouldBeNil)

				Convey("merge", func() {
					Convey("empty", func() {
						req := &pb.ScheduleBuildRequest{
							TemplateBuildId: 1,
							Properties:      &structpb.Struct{},
						}
						ret, err := scheduleRequestFromTemplate(ctx, req)
						So(err, ShouldBeNil)
						So(req, ShouldResembleProto, &pb.ScheduleBuildRequest{
							TemplateBuildId: 1,
							Properties:      &structpb.Struct{},
						})
						So(ret, ShouldResembleProto, &pb.ScheduleBuildRequest{
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket",
								Builder: "builder",
							},
							Experiments: map[string]bool{
								bb.ExperimentBBCanarySoftware: false,
								bb.ExperimentNonProduction:    false,
							},
							Properties: &structpb.Struct{},
						})
					})

					Convey("non-empty", func() {
						req := &pb.ScheduleBuildRequest{
							TemplateBuildId: 1,
							Properties: &structpb.Struct{
								Fields: map[string]*structpb.Value{
									"other": {
										Kind: &structpb.Value_StringValue{
											StringValue: "other value",
										},
									},
								},
							},
						}
						ret, err := scheduleRequestFromTemplate(ctx, req)
						So(err, ShouldBeNil)
						So(req, ShouldResembleProto, &pb.ScheduleBuildRequest{
							TemplateBuildId: 1,
							Properties: &structpb.Struct{
								Fields: map[string]*structpb.Value{
									"other": {
										Kind: &structpb.Value_StringValue{
											StringValue: "other value",
										},
									},
								},
							},
						})
						So(ret, ShouldResembleProto, &pb.ScheduleBuildRequest{
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket",
								Builder: "builder",
							},
							Experiments: map[string]bool{
								bb.ExperimentBBCanarySoftware: false,
								bb.ExperimentNonProduction:    false,
							},
							Properties: &structpb.Struct{
								Fields: map[string]*structpb.Value{
									"other": {
										Kind: &structpb.Value_StringValue{
											StringValue: "other value",
										},
									},
								},
							},
						})
					})
				})

				Convey("ok", func() {
					req := &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
					}
					ret, err := scheduleRequestFromTemplate(ctx, req)
					So(err, ShouldBeNil)
					So(req, ShouldResembleProto, &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
					})
					So(ret, ShouldResembleProto, &pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: false,
							bb.ExperimentNonProduction:    false,
						},
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"input": {
									Kind: &structpb.Value_StringValue{
										StringValue: "input value",
									},
								},
							},
						},
					})
				})
			})
		})

		Convey("tags", func() {
			So(datastore.Put(ctx, &model.Build{
				Proto: pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
				Tags: []string{
					"key:value",
				},
				Experiments: []string{"-" + bb.ExperimentBBCanarySoftware},
			}), ShouldBeNil)

			Convey("merge", func() {
				Convey("empty", func() {
					req := &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						Tags:            []*pb.StringPair{},
					}
					ret, err := scheduleRequestFromTemplate(ctx, req)
					So(err, ShouldBeNil)
					So(req, ShouldResembleProto, &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						Tags:            []*pb.StringPair{},
					})
					So(ret, ShouldResembleProto, &pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: false,
							bb.ExperimentNonProduction:    false,
						},
						Properties: &structpb.Struct{},
						Tags: []*pb.StringPair{
							{
								Key:   "key",
								Value: "value",
							},
						},
					})
				})

				Convey("non-empty", func() {
					req := &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						Tags: []*pb.StringPair{
							{
								Key:   "other",
								Value: "other",
							},
						},
					}
					ret, err := scheduleRequestFromTemplate(ctx, req)
					So(err, ShouldBeNil)
					So(req, ShouldResembleProto, &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						Tags: []*pb.StringPair{
							{
								Key:   "other",
								Value: "other",
							},
						},
					})
					So(ret, ShouldResembleProto, &pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: false,
							bb.ExperimentNonProduction:    false,
						},
						Properties: &structpb.Struct{},
						Tags: []*pb.StringPair{
							{
								Key:   "other",
								Value: "other",
							},
						},
					})
				})
			})

			Convey("ok", func() {
				req := &pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
				}
				ret, err := scheduleRequestFromTemplate(ctx, req)
				So(err, ShouldBeNil)
				So(req, ShouldResembleProto, &pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
				})
				So(ret, ShouldResembleProto, &pb.ScheduleBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Experiments: map[string]bool{
						bb.ExperimentBBCanarySoftware: false,
						bb.ExperimentNonProduction:    false,
					},
					Properties: &structpb.Struct{},
					Tags: []*pb.StringPair{
						{
							Key:   "key",
							Value: "value",
						},
					},
				})
			})
		})

		Convey("ok", func() {
			So(datastore.Put(ctx, &model.Build{
				Proto: pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
				Experiments: []string{"-" + bb.ExperimentBBCanarySoftware},
			}), ShouldBeNil)
			req := &pb.ScheduleBuildRequest{
				TemplateBuildId: 1,
			}
			ret, err := scheduleRequestFromTemplate(ctx, req)
			So(err, ShouldBeNil)
			So(req, ShouldResembleProto, &pb.ScheduleBuildRequest{
				TemplateBuildId: 1,
			})
			So(ret, ShouldResembleProto, &pb.ScheduleBuildRequest{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Experiments: map[string]bool{
					bb.ExperimentBBCanarySoftware: false,
					bb.ExperimentNonProduction:    false,
				},
				Properties: &structpb.Struct{},
			})
		})
	})

	Convey("setCIPDPackages", t, func() {
		Convey("base packages", func() {
			b := &pb.Build{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Exe: &pb.Executable{
					CipdPackage: "exe",
					CipdVersion: "exe-version",
				},
				Infra: &pb.BuildInfra{
					Bbagent: &pb.BuildInfra_BBAgent{
						Input: &pb.BuildInfra_BBAgent_Input{},
					},
				},
			}
			s := &pb.SettingsCfg{
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
			}

			setCIPDPackages(b, s)
			So(b, ShouldResemble, &pb.Build{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Exe: &pb.Executable{
					CipdPackage: "exe",
					CipdVersion: "exe-version",
				},
				Infra: &pb.BuildInfra{
					Bbagent: &pb.BuildInfra_BBAgent{
						Input: &pb.BuildInfra_BBAgent_Input{
							CipdPackages: []*pb.BuildInfra_BBAgent_Input_CIPDPackage{
								{
									Name:    "bbagent",
									Path:    ".",
									Version: "bbagent-version",
								},
								{
									Name:    "kitchen",
									Path:    ".",
									Version: "kitchen-version",
								},
								{
									Name:    "exe",
									Path:    "kitchen-checkout",
									Version: "exe-version",
								},
							},
						},
					},
				},
			})
		})

		Convey("user packages", func() {
			b := &pb.Build{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Canary: true,
				Exe: &pb.Executable{
					CipdPackage: "exe",
					CipdVersion: "exe-version",
				},
				Infra: &pb.BuildInfra{
					Bbagent: &pb.BuildInfra_BBAgent{
						Input: &pb.BuildInfra_BBAgent_Input{},
					},
				},
			}
			s := &pb.SettingsCfg{
				Swarming: &pb.SwarmingSettings{
					BbagentPackage: &pb.SwarmingSettings_Package{
						PackageName:   "bbagent",
						Version:       "bbagent-version",
						VersionCanary: "canary-version",
					},
					KitchenPackage: &pb.SwarmingSettings_Package{
						PackageName:   "kitchen",
						Version:       "kitchen-version",
						VersionCanary: "canary-version",
					},
					UserPackages: []*pb.SwarmingSettings_Package{
						{
							PackageName:   "include",
							Version:       "version",
							VersionCanary: "canary-version",
						},
						{
							Builders: &pb.BuilderPredicate{
								RegexExclude: []string{
									".*",
								},
							},
							PackageName:   "exclude",
							Version:       "version",
							VersionCanary: "canary-version",
						},
						{
							Builders: &pb.BuilderPredicate{
								Regex: []string{
									".*",
								},
							},
							PackageName:   "subdir",
							Subdir:        "subdir",
							Version:       "version",
							VersionCanary: "canary-version",
						},
					},
				},
			}

			setCIPDPackages(b, s)
			So(b, ShouldResemble, &pb.Build{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Canary: true,
				Exe: &pb.Executable{
					CipdPackage: "exe",
					CipdVersion: "exe-version",
				},
				Infra: &pb.BuildInfra{
					Bbagent: &pb.BuildInfra_BBAgent{
						Input: &pb.BuildInfra_BBAgent_Input{
							CipdPackages: []*pb.BuildInfra_BBAgent_Input_CIPDPackage{
								{
									Name:    "bbagent",
									Path:    ".",
									Version: "canary-version",
								},
								{
									Name:    "kitchen",
									Path:    ".",
									Version: "canary-version",
								},
								{
									Name:    "exe",
									Path:    "kitchen-checkout",
									Version: "exe-version",
								},
								{
									Name:    "include",
									Path:    "cipd_bin_packages",
									Version: "canary-version",
								},
								{
									Name:    "subdir",
									Path:    "cipd_bin_packages/subdir",
									Version: "canary-version",
								},
							},
						},
					},
				},
			})
		})
	})

	Convey("setDimensions", t, func() {
		Convey("config", func() {
			Convey("omit", func() {
				cfg := &pb.Builder{
					Dimensions: []string{
						"key:",
					},
				}
				b := &pb.Build{
					Infra: &pb.BuildInfra{
						Swarming: &pb.BuildInfra_Swarming{},
					},
				}

				setDimensions(nil, cfg, b)
				So(b.Infra.Swarming, ShouldResembleProto, &pb.BuildInfra_Swarming{})
			})

			Convey("simple", func() {
				cfg := &pb.Builder{
					Dimensions: []string{
						"key:value",
					},
				}
				b := &pb.Build{
					Infra: &pb.BuildInfra{
						Swarming: &pb.BuildInfra_Swarming{},
					},
				}

				setDimensions(nil, cfg, b)
				So(b.Infra.Swarming, ShouldResembleProto, &pb.BuildInfra_Swarming{
					TaskDimensions: []*pb.RequestedDimension{
						{
							Key:   "key",
							Value: "value",
						},
					},
				})
			})

			Convey("expiration", func() {
				cfg := &pb.Builder{
					Dimensions: []string{
						"1:key:value",
					},
				}
				b := &pb.Build{
					Infra: &pb.BuildInfra{
						Swarming: &pb.BuildInfra_Swarming{},
					},
				}

				setDimensions(nil, cfg, b)
				So(b.Infra.Swarming, ShouldResembleProto, &pb.BuildInfra_Swarming{
					TaskDimensions: []*pb.RequestedDimension{
						{
							Expiration: &durationpb.Duration{
								Seconds: 1,
							},
							Key:   "key",
							Value: "value",
						},
					},
				})
			})

			Convey("many", func() {
				cfg := &pb.Builder{
					Dimensions: []string{
						"key:",
						"key:value",
						"key:value:",
						"key:val:ue",
						"0:key:",
						"0:key:value",
						"0:key:value:",
						"0:key:val:ue",
						"1:key:",
						"1:key:value",
						"1:key:value:",
						"1:key:val:ue",
					},
				}
				b := &pb.Build{
					Infra: &pb.BuildInfra{
						Swarming: &pb.BuildInfra_Swarming{},
					},
				}

				setDimensions(nil, cfg, b)
				So(b.Infra, ShouldResembleProto, &pb.BuildInfra{
					Swarming: &pb.BuildInfra_Swarming{
						TaskDimensions: []*pb.RequestedDimension{
							{
								Key:   "key",
								Value: "value",
							},
							{
								Key:   "key",
								Value: "value:",
							},
							{
								Key:   "key",
								Value: "val:ue",
							},
							{
								Key:   "key",
								Value: "value",
							},
							{
								Key:   "key",
								Value: "value:",
							},
							{
								Key:   "key",
								Value: "val:ue",
							},
							{
								Expiration: &durationpb.Duration{
									Seconds: 1,
								},
								Key:   "key",
								Value: "value",
							},
							{
								Expiration: &durationpb.Duration{
									Seconds: 1,
								},
								Key:   "key",
								Value: "value:",
							},
							{
								Expiration: &durationpb.Duration{
									Seconds: 1,
								},
								Key:   "key",
								Value: "val:ue",
							},
						},
					},
				})
			})

			Convey("auto builder", func() {
				cfg := &pb.Builder{
					AutoBuilderDimension: pb.Toggle_YES,
					Name:                 "builder",
				}
				b := &pb.Build{
					Infra: &pb.BuildInfra{
						Swarming: &pb.BuildInfra_Swarming{},
					},
				}

				setDimensions(nil, cfg, b)
				So(b.Infra.Swarming, ShouldResembleProto, &pb.BuildInfra_Swarming{
					TaskDimensions: []*pb.RequestedDimension{
						{
							Key:   "builder",
							Value: "builder",
						},
					},
				})
			})

			Convey("builder > auto builder", func() {
				cfg := &pb.Builder{
					AutoBuilderDimension: pb.Toggle_YES,
					Dimensions: []string{
						"1:builder:cfg builder",
					},
					Name: "auto builder",
				}
				b := &pb.Build{
					Infra: &pb.BuildInfra{
						Swarming: &pb.BuildInfra_Swarming{},
					},
				}

				setDimensions(nil, cfg, b)
				So(b.Infra.Swarming, ShouldResembleProto, &pb.BuildInfra_Swarming{
					TaskDimensions: []*pb.RequestedDimension{
						{
							Expiration: &durationpb.Duration{
								Seconds: 1,
							},
							Key:   "builder",
							Value: "cfg builder",
						},
					},
				})
			})

			Convey("omit builder > auto builder", func() {
				cfg := &pb.Builder{
					AutoBuilderDimension: pb.Toggle_YES,
					Dimensions: []string{
						"builder:",
					},
					Name: "auto builder",
				}
				b := &pb.Build{
					Infra: &pb.BuildInfra{
						Swarming: &pb.BuildInfra_Swarming{},
					},
				}

				setDimensions(nil, cfg, b)
				So(b.Infra.Swarming, ShouldResembleProto, &pb.BuildInfra_Swarming{})
			})
		})

		Convey("request", func() {
			req := &pb.ScheduleBuildRequest{
				Dimensions: []*pb.RequestedDimension{
					{
						Expiration: &durationpb.Duration{
							Seconds: 1,
						},
						Key:   "key",
						Value: "value",
					},
				},
			}
			b := &pb.Build{
				Infra: &pb.BuildInfra{
					Swarming: &pb.BuildInfra_Swarming{},
				},
			}

			setDimensions(req, nil, b)
			So(b.Infra.Swarming, ShouldResembleProto, &pb.BuildInfra_Swarming{
				TaskDimensions: []*pb.RequestedDimension{
					{
						Expiration: &durationpb.Duration{
							Seconds: 1,
						},
						Key:   "key",
						Value: "value",
					},
				},
			})
		})

		Convey("request > config", func() {
			req := &pb.ScheduleBuildRequest{
				Dimensions: []*pb.RequestedDimension{
					{
						Expiration: &durationpb.Duration{
							Seconds: 1,
						},
						Key:   "req only",
						Value: "req value",
					},
					{
						Key:   "req only",
						Value: "req value",
					},
					{
						Key:   "key",
						Value: "req value",
					},
				},
			}
			cfg := &pb.Builder{
				AutoBuilderDimension: pb.Toggle_YES,
				Dimensions: []string{
					"1:cfg only:cfg value",
					"cfg only:cfg value",
					"cfg only:",
					"1:key:cfg value",
				},
				Name: "auto builder",
			}
			b := &pb.Build{
				Infra: &pb.BuildInfra{
					Swarming: &pb.BuildInfra_Swarming{},
				},
			}

			setDimensions(req, cfg, b)
			So(b.Infra.Swarming, ShouldResembleProto, &pb.BuildInfra_Swarming{
				TaskDimensions: []*pb.RequestedDimension{
					{
						Key:   "builder",
						Value: "auto builder",
					},
					{
						Key:   "cfg only",
						Value: "cfg value",
					},
					{
						Expiration: &durationpb.Duration{
							Seconds: 1,
						},
						Key:   "cfg only",
						Value: "cfg value",
					},
					{
						Key:   "key",
						Value: "req value",
					},
					{
						Key:   "req only",
						Value: "req value",
					},
					{
						Expiration: &durationpb.Duration{
							Seconds: 1,
						},
						Key:   "req only",
						Value: "req value",
					},
				},
			})
		})
	})

	Convey("setExecutable", t, func() {
		Convey("nil", func() {
			b := &pb.Build{}

			setExecutable(nil, nil, b)
			So(b.Exe, ShouldResembleProto, &pb.Executable{})
		})

		Convey("request only", func() {
			req := &pb.ScheduleBuildRequest{
				Exe: &pb.Executable{
					CipdPackage: "package",
					CipdVersion: "version",
					Cmd:         []string{"command"},
				},
			}
			b := &pb.Build{}

			setExecutable(req, nil, b)
			So(b.Exe, ShouldResembleProto, &pb.Executable{
				CipdVersion: "version",
			})
		})

		Convey("config only", func() {
			Convey("exe", func() {
				cfg := &pb.Builder{
					Exe: &pb.Executable{
						CipdPackage: "package",
						CipdVersion: "version",
						Cmd:         []string{"command"},
					},
				}
				b := &pb.Build{}

				setExecutable(nil, cfg, b)
				So(b.Exe, ShouldResembleProto, &pb.Executable{
					CipdPackage: "package",
					CipdVersion: "version",
					Cmd:         []string{"command"},
				})
			})

			Convey("recipe", func() {
				cfg := &pb.Builder{
					Exe: &pb.Executable{
						CipdPackage: "package 1",
						CipdVersion: "version 1",
						Cmd:         []string{"command"},
					},
					Recipe: &pb.Builder_Recipe{
						CipdPackage: "package 2",
						CipdVersion: "version 2",
					},
				}
				b := &pb.Build{}

				setExecutable(nil, cfg, b)
				So(b.Exe, ShouldResembleProto, &pb.Executable{
					CipdPackage: "package 2",
					CipdVersion: "version 2",
					Cmd:         []string{"command"},
				})
			})
		})

		Convey("request > config", func() {
			req := &pb.ScheduleBuildRequest{
				Exe: &pb.Executable{
					CipdPackage: "package 1",
					CipdVersion: "version 1",
					Cmd:         []string{"command 1"},
				},
			}
			cfg := &pb.Builder{
				Exe: &pb.Executable{
					CipdPackage: "package 2",
					CipdVersion: "version 2",
					Cmd:         []string{"command 2"},
				},
			}
			b := &pb.Build{}

			setExecutable(req, cfg, b)
			So(b.Exe, ShouldResembleProto, &pb.Executable{
				CipdPackage: "package 2",
				CipdVersion: "version 1",
				Cmd:         []string{"command 2"},
			})
		})
	})

	Convey("setExperiments", t, func() {
		ctx := mathrand.Set(memory.Use(context.Background()), rand.New(rand.NewSource(0)))

		Convey("nil", func() {
			ent := &model.Build{
				Proto: pb.Build{
					Exe: &pb.Executable{},
					Infra: &pb.BuildInfra{
						Swarming: &pb.BuildInfra_Swarming{},
					},
					Input: &pb.Build_Input{},
				},
			}

			setExperiments(ctx, nil, nil, &ent.Proto)
			setExperimentsFromProto(nil, nil, ent)
			So(ent, ShouldResemble, &model.Build{
				Experiments: []string{
					"-" + bb.ExperimentBBAgentGetBuild,
					"-" + bb.ExperimentBBCanarySoftware,
					"-" + bb.ExperimentBBAgent,
					"-" + bb.ExperimentRecipePY3,
					"-" + bb.ExperimentUseRealms,
				},
				Proto: pb.Build{
					Exe: &pb.Executable{
						Cmd: []string{"recipes"},
					},
					Infra: &pb.BuildInfra{
						Swarming: &pb.BuildInfra_Swarming{},
					},
					Input: &pb.Build_Input{},
				},
			})
		})

		Convey("command", func() {
			Convey("recipes", func() {
				req := &pb.ScheduleBuildRequest{
					Experiments: map[string]bool{
						bb.ExperimentBBAgent: false,
					},
				}
				normalizeSchedule(req)
				ent := &model.Build{
					Proto: pb.Build{
						Exe: &pb.Executable{},
						Infra: &pb.BuildInfra{
							Swarming: &pb.BuildInfra_Swarming{},
						},
						Input: &pb.Build_Input{},
					},
				}

				setExperiments(ctx, req, nil, &ent.Proto)
				setExperimentsFromProto(req, nil, ent)
				So(ent.Proto.Exe, ShouldResembleProto, &pb.Executable{
					Cmd: []string{"recipes"},
				})
			})

			Convey("luciexe", func() {
				req := &pb.ScheduleBuildRequest{
					Experiments: map[string]bool{
						bb.ExperimentBBAgent: true,
					},
				}
				normalizeSchedule(req)
				ent := &model.Build{
					Proto: pb.Build{
						Exe: &pb.Executable{},
						Infra: &pb.BuildInfra{
							Swarming: &pb.BuildInfra_Swarming{},
						},
						Input: &pb.Build_Input{},
					},
				}

				setExperiments(ctx, req, nil, &ent.Proto)
				setExperimentsFromProto(req, nil, ent)
				So(ent.Proto.Exe, ShouldResembleProto, &pb.Executable{
					Cmd: []string{"luciexe"},
				})
			})

			Convey("cmd > experiment", func() {
				req := &pb.ScheduleBuildRequest{
					Experiments: map[string]bool{
						bb.ExperimentBBAgent: true,
					},
				}
				normalizeSchedule(req)
				ent := &model.Build{
					Proto: pb.Build{
						Exe: &pb.Executable{
							Cmd: []string{"command"},
						},
						Infra: &pb.BuildInfra{
							Swarming: &pb.BuildInfra_Swarming{},
						},
						Input: &pb.Build_Input{},
					},
				}

				setExperiments(ctx, req, nil, &ent.Proto)
				setExperimentsFromProto(req, nil, ent)
				So(ent.Proto.Exe, ShouldResembleProto, &pb.Executable{
					Cmd: []string{"command"},
				})
			})
		})

		Convey("priority", func() {
			Convey("production", func() {
				req := &pb.ScheduleBuildRequest{
					Experiments: map[string]bool{
						bb.ExperimentNonProduction: false,
					},
				}
				normalizeSchedule(req)
				ent := &model.Build{
					Proto: pb.Build{
						Exe: &pb.Executable{},
						Infra: &pb.BuildInfra{
							Swarming: &pb.BuildInfra_Swarming{
								Priority: 1,
							},
						},
						Input: &pb.Build_Input{},
					},
				}

				setExperiments(ctx, req, nil, &ent.Proto)
				setExperimentsFromProto(req, nil, ent)
				So(ent.Proto.Infra.Swarming.Priority, ShouldEqual, 1)
				So(ent.Proto.Input.Experimental, ShouldBeFalse)
				So(ent.Experimental, ShouldBeFalse)
			})

			Convey("non-production", func() {
				req := &pb.ScheduleBuildRequest{
					Experiments: map[string]bool{
						bb.ExperimentNonProduction: true,
					},
				}
				normalizeSchedule(req)
				ent := &model.Build{
					Proto: pb.Build{
						Exe: &pb.Executable{},
						Infra: &pb.BuildInfra{
							Swarming: &pb.BuildInfra_Swarming{
								Priority: 1,
							},
						},
						Input: &pb.Build_Input{},
					},
				}

				setExperiments(ctx, req, nil, &ent.Proto)
				setExperimentsFromProto(req, nil, ent)
				So(ent.Proto.Infra.Swarming.Priority, ShouldEqual, 255)
				So(ent.Proto.Input.Experimental, ShouldBeTrue)
				So(ent.Experimental, ShouldBeTrue)
			})

			Convey("req > experiment", func() {
				req := &pb.ScheduleBuildRequest{
					Experiments: map[string]bool{
						bb.ExperimentNonProduction: true,
					},
					Priority: 1,
				}
				normalizeSchedule(req)
				ent := &model.Build{
					Proto: pb.Build{
						Exe: &pb.Executable{},
						Infra: &pb.BuildInfra{
							Swarming: &pb.BuildInfra_Swarming{
								Priority: 1,
							},
						},
						Input: &pb.Build_Input{},
					},
				}

				setExperiments(ctx, req, nil, &ent.Proto)
				setExperimentsFromProto(req, nil, ent)
				So(ent.Proto.Infra.Swarming.Priority, ShouldEqual, 1)
			})
		})

		Convey("request only", func() {
			req := &pb.ScheduleBuildRequest{
				Experiments: map[string]bool{
					"experiment1": true,
					"experiment2": false,
				},
			}
			ent := &model.Build{
				Proto: pb.Build{
					Exe: &pb.Executable{},
					Infra: &pb.BuildInfra{
						Swarming: &pb.BuildInfra_Swarming{},
					},
					Input: &pb.Build_Input{},
				},
			}

			setExperiments(ctx, req, nil, &ent.Proto)
			setExperimentsFromProto(req, nil, ent)
			So(ent, ShouldResemble, &model.Build{
				Experiments: []string{
					"+experiment1",
					"-experiment2",
					"-" + bb.ExperimentBBAgentGetBuild,
					"-" + bb.ExperimentBBCanarySoftware,
					"-" + bb.ExperimentBBAgent,
					"-" + bb.ExperimentRecipePY3,
					"-" + bb.ExperimentUseRealms,
				},
				Proto: pb.Build{
					Exe: &pb.Executable{
						Cmd: []string{"recipes"},
					},
					Infra: &pb.BuildInfra{
						Swarming: &pb.BuildInfra_Swarming{},
					},
					Input: &pb.Build_Input{
						Experiments: []string{
							"experiment1",
						},
					},
				},
			})
		})

		Convey("legacy only", func() {
			req := &pb.ScheduleBuildRequest{
				Canary:       pb.Trinary_YES,
				Experimental: pb.Trinary_NO,
			}
			normalizeSchedule(req)
			ent := &model.Build{
				Proto: pb.Build{
					Exe: &pb.Executable{},
					Infra: &pb.BuildInfra{
						Swarming: &pb.BuildInfra_Swarming{},
					},
					Input: &pb.Build_Input{},
				},
			}

			setExperiments(ctx, req, nil, &ent.Proto)
			setExperimentsFromProto(req, nil, ent)
			So(ent, ShouldResemble, &model.Build{
				Canary: true,
				Experiments: []string{
					"+" + bb.ExperimentBBCanarySoftware,
					"-" + bb.ExperimentBBAgentGetBuild,
					"-" + bb.ExperimentBBAgent,
					"-" + bb.ExperimentRecipePY3,
					"-" + bb.ExperimentUseRealms,
				},
				Proto: pb.Build{
					Canary: true,
					Exe: &pb.Executable{
						Cmd: []string{"recipes"},
					},
					Infra: &pb.BuildInfra{
						Swarming: &pb.BuildInfra_Swarming{},
					},
					Input: &pb.Build_Input{
						Experiments: []string{
							bb.ExperimentBBCanarySoftware,
						},
					},
				},
			})
		})

		Convey("config only", func() {
			ent := &model.Build{
				Proto: pb.Build{
					Exe: &pb.Executable{},
					Infra: &pb.BuildInfra{
						Swarming: &pb.BuildInfra_Swarming{},
					},
					Input: &pb.Build_Input{},
				},
			}
			cfg := &pb.Builder{
				Experiments: map[string]int32{
					"experiment1": 100,
					"experiment2": 0,
				},
			}

			setExperiments(ctx, nil, cfg, &ent.Proto)
			setExperimentsFromProto(nil, cfg, ent)
			So(ent, ShouldResemble, &model.Build{
				Experiments: []string{
					"+experiment1",
					"-experiment2",
					"-" + bb.ExperimentBBAgentGetBuild,
					"-" + bb.ExperimentBBCanarySoftware,
					"-" + bb.ExperimentBBAgent,
					"-" + bb.ExperimentRecipePY3,
					"-" + bb.ExperimentUseRealms,
				},
				Proto: pb.Build{
					Exe: &pb.Executable{
						Cmd: []string{"recipes"},
					},
					Infra: &pb.BuildInfra{
						Swarming: &pb.BuildInfra_Swarming{},
					},
					Input: &pb.Build_Input{
						Experiments: []string{
							"experiment1",
						},
					},
				},
			})
		})

		Convey("override", func() {
			Convey("request > legacy", func() {
				req := &pb.ScheduleBuildRequest{
					Canary:       pb.Trinary_YES,
					Experimental: pb.Trinary_NO,
					Experiments: map[string]bool{
						bb.ExperimentBBCanarySoftware: false,
						bb.ExperimentNonProduction:    true,
					},
				}
				normalizeSchedule(req)
				ent := &model.Build{
					Proto: pb.Build{
						Exe: &pb.Executable{},
						Infra: &pb.BuildInfra{
							Swarming: &pb.BuildInfra_Swarming{},
						},
						Input: &pb.Build_Input{},
					},
				}

				setExperiments(ctx, req, nil, &ent.Proto)
				setExperimentsFromProto(req, nil, ent)
				So(ent, ShouldResemble, &model.Build{
					Experimental: true,
					Experiments: []string{
						"+" + bb.ExperimentNonProduction,
						"-" + bb.ExperimentBBAgentGetBuild,
						"-" + bb.ExperimentBBCanarySoftware,
						"-" + bb.ExperimentBBAgent,
						"-" + bb.ExperimentRecipePY3,
						"-" + bb.ExperimentUseRealms,
					},
					Proto: pb.Build{
						Exe: &pb.Executable{
							Cmd: []string{"recipes"},
						},
						Infra: &pb.BuildInfra{
							Swarming: &pb.BuildInfra_Swarming{
								Priority: 255,
							},
						},
						Input: &pb.Build_Input{
							Experimental: true,
							Experiments: []string{
								bb.ExperimentNonProduction,
							},
						},
					},
				})
			})

			Convey("legacy > config", func() {
				req := &pb.ScheduleBuildRequest{
					Canary:       pb.Trinary_YES,
					Experimental: pb.Trinary_NO,
				}
				normalizeSchedule(req)
				cfg := &pb.Builder{
					Experiments: map[string]int32{
						bb.ExperimentBBCanarySoftware: 0,
						bb.ExperimentNonProduction:    100,
					},
				}
				ent := &model.Build{
					Proto: pb.Build{
						Exe: &pb.Executable{},
						Infra: &pb.BuildInfra{
							Swarming: &pb.BuildInfra_Swarming{},
						},
						Input: &pb.Build_Input{},
					},
				}

				setExperiments(ctx, req, cfg, &ent.Proto)
				setExperimentsFromProto(req, cfg, ent)
				So(ent, ShouldResemble, &model.Build{
					Canary: true,
					Experiments: []string{
						"+" + bb.ExperimentBBCanarySoftware,
						"-" + bb.ExperimentBBAgentGetBuild,
						"-" + bb.ExperimentBBAgent,
						"-" + bb.ExperimentRecipePY3,
						"-" + bb.ExperimentUseRealms,
					},
					Proto: pb.Build{
						Canary: true,
						Exe: &pb.Executable{
							Cmd: []string{"recipes"},
						},
						Infra: &pb.BuildInfra{
							Swarming: &pb.BuildInfra_Swarming{},
						},
						Input: &pb.Build_Input{
							Experiments: []string{
								bb.ExperimentBBCanarySoftware,
							},
						},
					},
				})
			})

			Convey("request > config", func() {
				req := &pb.ScheduleBuildRequest{
					Experiments: map[string]bool{
						"experiment1": true,
						"experiment2": false,
					},
				}
				normalizeSchedule(req)
				cfg := &pb.Builder{
					Experiments: map[string]int32{
						"experiment1": 0,
						"experiment2": 100,
					},
				}
				ent := &model.Build{
					Proto: pb.Build{
						Exe: &pb.Executable{},
						Infra: &pb.BuildInfra{
							Swarming: &pb.BuildInfra_Swarming{},
						},
						Input: &pb.Build_Input{},
					},
				}

				setExperiments(ctx, req, cfg, &ent.Proto)
				setExperimentsFromProto(req, cfg, ent)
				So(ent, ShouldResemble, &model.Build{
					Experiments: []string{
						"+experiment1",
						"-experiment2",
						"-" + bb.ExperimentBBAgentGetBuild,
						"-" + bb.ExperimentBBCanarySoftware,
						"-" + bb.ExperimentBBAgent,
						"-" + bb.ExperimentRecipePY3,
						"-" + bb.ExperimentUseRealms,
					},
					Proto: pb.Build{
						Exe: &pb.Executable{
							Cmd: []string{"recipes"},
						},
						Infra: &pb.BuildInfra{
							Swarming: &pb.BuildInfra_Swarming{},
						},
						Input: &pb.Build_Input{
							Experiments: []string{
								"experiment1",
							},
						},
					},
				})
			})

			Convey("request > legacy > config", func() {
				req := &pb.ScheduleBuildRequest{
					Canary:       pb.Trinary_YES,
					Experimental: pb.Trinary_NO,
					Experiments: map[string]bool{
						bb.ExperimentBBCanarySoftware: false,
						bb.ExperimentNonProduction:    true,
						"experiment1":                 true,
						"experiment2":                 false,
					},
				}
				normalizeSchedule(req)
				cfg := &pb.Builder{
					Experiments: map[string]int32{
						bb.ExperimentBBCanarySoftware: 100,
						bb.ExperimentNonProduction:    100,
						"experiment1":                 0,
						"experiment2":                 0,
					},
				}
				ent := &model.Build{
					Proto: pb.Build{
						Exe: &pb.Executable{},
						Infra: &pb.BuildInfra{
							Swarming: &pb.BuildInfra_Swarming{},
						},
						Input: &pb.Build_Input{},
					},
				}

				setExperiments(ctx, req, cfg, &ent.Proto)
				setExperimentsFromProto(req, cfg, ent)
				So(ent, ShouldResemble, &model.Build{
					Experimental: true,
					Experiments: []string{
						"+experiment1",
						"+" + bb.ExperimentNonProduction,
						"-experiment2",
						"-" + bb.ExperimentBBAgentGetBuild,
						"-" + bb.ExperimentBBCanarySoftware,
						"-" + bb.ExperimentBBAgent,
						"-" + bb.ExperimentRecipePY3,
						"-" + bb.ExperimentUseRealms,
					},
					Proto: pb.Build{
						Exe: &pb.Executable{
							Cmd: []string{"recipes"},
						},
						Infra: &pb.BuildInfra{
							Swarming: &pb.BuildInfra_Swarming{
								Priority: 255,
							},
						},
						Input: &pb.Build_Input{
							Experimental: true,
							Experiments: []string{
								"experiment1",
								bb.ExperimentNonProduction,
							},
						},
					},
				})
			})
		})
	})

	Convey("setInfra", t, func() {
		Convey("nil", func() {
			b := &pb.Build{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			}

			setInfra(nil, nil, b, nil)
			So(b.Infra, ShouldResembleProto, &pb.BuildInfra{
				Bbagent: &pb.BuildInfra_BBAgent{
					CacheDir:    "cache",
					Input:       &pb.BuildInfra_BBAgent_Input{},
					PayloadPath: "kitchen-checkout",
				},
				Buildbucket: &pb.BuildInfra_Buildbucket{},
				Logdog: &pb.BuildInfra_LogDog{
					Project: "project",
				},
				Resultdb: &pb.BuildInfra_ResultDB{},
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
					Priority: 30,
				},
			})
		})

		Convey("bbagent", func() {
			b := &pb.Build{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			}
			s := &pb.SettingsCfg{
				KnownPublicGerritHosts: []string{
					"host",
				},
			}

			setInfra(nil, nil, b, s)
			So(b.Infra, ShouldResembleProto, &pb.BuildInfra{
				Bbagent: &pb.BuildInfra_BBAgent{
					CacheDir: "cache",
					Input:    &pb.BuildInfra_BBAgent_Input{},
					KnownPublicGerritHosts: []string{
						"host",
					},
					PayloadPath: "kitchen-checkout",
				},
				Buildbucket: &pb.BuildInfra_Buildbucket{},
				Logdog: &pb.BuildInfra_LogDog{
					Project: "project",
				},
				Resultdb: &pb.BuildInfra_ResultDB{},
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
					Priority: 30,
				},
			})
		})

		Convey("logdog", func() {
			b := &pb.Build{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			}
			s := &pb.SettingsCfg{
				Logdog: &pb.LogDogSettings{
					Hostname: "host",
				},
			}

			setInfra(nil, nil, b, s)
			So(b.Infra, ShouldResembleProto, &pb.BuildInfra{
				Bbagent: &pb.BuildInfra_BBAgent{
					CacheDir:    "cache",
					Input:       &pb.BuildInfra_BBAgent_Input{},
					PayloadPath: "kitchen-checkout",
				},
				Buildbucket: &pb.BuildInfra_Buildbucket{},
				Logdog: &pb.BuildInfra_LogDog{
					Hostname: "host",
					Project:  "project",
				},
				Resultdb: &pb.BuildInfra_ResultDB{},
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
					Priority: 30,
				},
			})
		})

		Convey("resultdb", func() {
			b := &pb.Build{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Id: 1,
			}
			s := &pb.SettingsCfg{
				Resultdb: &pb.ResultDBSettings{
					Hostname: "host",
				},
			}

			setInfra(nil, nil, b, s)
			So(b.Infra, ShouldResembleProto, &pb.BuildInfra{
				Bbagent: &pb.BuildInfra_BBAgent{
					CacheDir:    "cache",
					Input:       &pb.BuildInfra_BBAgent_Input{},
					PayloadPath: "kitchen-checkout",
				},
				Buildbucket: &pb.BuildInfra_Buildbucket{},
				Logdog: &pb.BuildInfra_LogDog{
					Hostname: "",
					Project:  "project",
				},
				Resultdb: &pb.BuildInfra_ResultDB{
					Hostname: "host",
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
					Priority: 30,
				},
			})
		})

		Convey("config", func() {
			Convey("recipe", func() {
				cfg := &pb.Builder{
					Recipe: &pb.Builder_Recipe{
						CipdPackage: "package",
						Name:        "name",
					},
				}
				b := &pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				}

				setInfra(nil, cfg, b, nil)
				So(b.Infra, ShouldResembleProto, &pb.BuildInfra{
					Bbagent: &pb.BuildInfra_BBAgent{
						CacheDir:    "cache",
						Input:       &pb.BuildInfra_BBAgent_Input{},
						PayloadPath: "kitchen-checkout",
					},
					Buildbucket: &pb.BuildInfra_Buildbucket{},
					Logdog: &pb.BuildInfra_LogDog{
						Project: "project",
					},
					Recipe: &pb.BuildInfra_Recipe{
						CipdPackage: "package",
						Name:        "name",
					},
					Resultdb: &pb.BuildInfra_ResultDB{},
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
						Priority: 30,
					},
				})
			})

			Convey("swarming", func() {
				Convey("no dimensions", func() {
					cfg := &pb.Builder{
						Priority:       1,
						ServiceAccount: "account",
						SwarmingHost:   "host",
					}
					b := &pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					}

					setInfra(nil, cfg, b, nil)
					So(b.Infra, ShouldResembleProto, &pb.BuildInfra{
						Bbagent: &pb.BuildInfra_BBAgent{
							CacheDir:    "cache",
							Input:       &pb.BuildInfra_BBAgent_Input{},
							PayloadPath: "kitchen-checkout",
						},
						Buildbucket: &pb.BuildInfra_Buildbucket{},
						Logdog: &pb.BuildInfra_LogDog{
							Project: "project",
						},
						Resultdb: &pb.BuildInfra_ResultDB{},
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
							Hostname:           "host",
							Priority:           1,
							TaskServiceAccount: "account",
						},
					})
				})

				Convey("caches", func() {
					Convey("nil", func() {
						b := &pb.Build{
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket",
								Builder: "builder",
							},
						}

						setInfra(nil, nil, b, nil)
						So(b.Infra, ShouldResembleProto, &pb.BuildInfra{
							Bbagent: &pb.BuildInfra_BBAgent{
								CacheDir:    "cache",
								Input:       &pb.BuildInfra_BBAgent_Input{},
								PayloadPath: "kitchen-checkout",
							},
							Buildbucket: &pb.BuildInfra_Buildbucket{},
							Logdog: &pb.BuildInfra_LogDog{
								Project: "project",
							},
							Resultdb: &pb.BuildInfra_ResultDB{},
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
								Priority: 30,
							},
						})
					})

					Convey("global", func() {
						b := &pb.Build{
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket",
								Builder: "builder",
							},
						}
						s := &pb.SettingsCfg{
							Swarming: &pb.SwarmingSettings{
								GlobalCaches: []*pb.Builder_CacheEntry{
									{
										Path: "cache",
									},
								},
							},
						}

						setInfra(nil, nil, b, s)
						So(b.Infra, ShouldResembleProto, &pb.BuildInfra{
							Bbagent: &pb.BuildInfra_BBAgent{
								CacheDir:    "cache",
								Input:       &pb.BuildInfra_BBAgent_Input{},
								PayloadPath: "kitchen-checkout",
							},
							Buildbucket: &pb.BuildInfra_Buildbucket{},
							Logdog: &pb.BuildInfra_LogDog{
								Project: "project",
							},
							Resultdb: &pb.BuildInfra_ResultDB{},
							Swarming: &pb.BuildInfra_Swarming{
								Caches: []*pb.BuildInfra_Swarming_CacheEntry{
									{
										Name: "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
										Path: "builder",
										WaitForWarmCache: &durationpb.Duration{
											Seconds: 240,
										},
									},
									{
										Name: "cache",
										Path: "cache",
									},
								},
								Priority: 30,
							},
						})
					})

					Convey("config", func() {
						cfg := &pb.Builder{
							Caches: []*pb.Builder_CacheEntry{
								{
									Path: "cache",
								},
							},
						}
						b := &pb.Build{
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket",
								Builder: "builder",
							},
						}

						setInfra(nil, cfg, b, nil)
						So(b.Infra, ShouldResembleProto, &pb.BuildInfra{
							Bbagent: &pb.BuildInfra_BBAgent{
								CacheDir:    "cache",
								Input:       &pb.BuildInfra_BBAgent_Input{},
								PayloadPath: "kitchen-checkout",
							},
							Buildbucket: &pb.BuildInfra_Buildbucket{},
							Logdog: &pb.BuildInfra_LogDog{
								Project: "project",
							},
							Resultdb: &pb.BuildInfra_ResultDB{},
							Swarming: &pb.BuildInfra_Swarming{
								Caches: []*pb.BuildInfra_Swarming_CacheEntry{
									{
										Name: "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
										Path: "builder",
										WaitForWarmCache: &durationpb.Duration{
											Seconds: 240,
										},
									},
									{
										Name: "cache",
										Path: "cache",
									},
								},
								Priority: 30,
							},
						})
					})

					Convey("config > global", func() {
						cfg := &pb.Builder{
							Caches: []*pb.Builder_CacheEntry{
								{
									Name: "builder only name",
									Path: "builder only path",
								},
								{
									Name: "name",
									Path: "builder path",
								},
								{
									Name: "builder name",
									Path: "path",
								},
								{
									EnvVar: "builder env",
									Path:   "env",
								},
							},
						}
						b := &pb.Build{
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket",
								Builder: "builder",
							},
						}
						s := &pb.SettingsCfg{
							Swarming: &pb.SwarmingSettings{
								GlobalCaches: []*pb.Builder_CacheEntry{
									{
										Name: "global only name",
										Path: "global only path",
									},
									{
										Name: "name",
										Path: "global path",
									},
									{
										Name: "global name",
										Path: "path",
									},
									{
										EnvVar: "global env",
										Path:   "path",
									},
								},
							},
						}

						setInfra(nil, cfg, b, s)
						So(b.Infra, ShouldResembleProto, &pb.BuildInfra{
							Bbagent: &pb.BuildInfra_BBAgent{
								CacheDir:    "cache",
								Input:       &pb.BuildInfra_BBAgent_Input{},
								PayloadPath: "kitchen-checkout",
							},
							Buildbucket: &pb.BuildInfra_Buildbucket{},
							Logdog: &pb.BuildInfra_LogDog{
								Project: "project",
							},
							Resultdb: &pb.BuildInfra_ResultDB{},
							Swarming: &pb.BuildInfra_Swarming{
								Caches: []*pb.BuildInfra_Swarming_CacheEntry{
									{
										Name: "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
										Path: "builder",
										WaitForWarmCache: &durationpb.Duration{
											Seconds: 240,
										},
									},
									{
										Name: "builder only name",
										Path: "builder only path",
									},
									{
										Name: "name",
										Path: "builder path",
									},
									{
										EnvVar: "builder env",
										Name:   "env",
										Path:   "env",
									},
									{
										Name: "global only name",
										Path: "global only path",
									},
									{
										Name: "builder name",
										Path: "path",
									},
								},
								Priority: 30,
							},
						})
					})
				})
			})
		})

		Convey("request", func() {
			Convey("dimensions", func() {
				req := &pb.ScheduleBuildRequest{
					Dimensions: []*pb.RequestedDimension{
						{
							Expiration: &durationpb.Duration{
								Seconds: 1,
							},
							Key:   "key",
							Value: "value",
						},
					},
				}
				b := &pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				}

				setInfra(req, nil, b, nil)
				So(b.Infra, ShouldResembleProto, &pb.BuildInfra{
					Bbagent: &pb.BuildInfra_BBAgent{
						CacheDir:    "cache",
						Input:       &pb.BuildInfra_BBAgent_Input{},
						PayloadPath: "kitchen-checkout",
					},
					Buildbucket: &pb.BuildInfra_Buildbucket{
						RequestedDimensions: []*pb.RequestedDimension{
							{
								Expiration: &durationpb.Duration{
									Seconds: 1,
								},
								Key:   "key",
								Value: "value",
							},
						},
					},
					Logdog: &pb.BuildInfra_LogDog{
						Project: "project",
					},
					Resultdb: &pb.BuildInfra_ResultDB{},
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
						Priority: 30,
						TaskDimensions: []*pb.RequestedDimension{
							{
								Expiration: &durationpb.Duration{
									Seconds: 1,
								},
								Key:   "key",
								Value: "value",
							},
						},
					},
				})
			})

			Convey("properties", func() {
				req := &pb.ScheduleBuildRequest{
					Properties: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key": {
								Kind: &structpb.Value_StringValue{
									StringValue: "value",
								},
							},
						},
					},
				}
				b := &pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				}

				setInfra(req, nil, b, nil)
				So(b.Infra, ShouldResembleProto, &pb.BuildInfra{
					Bbagent: &pb.BuildInfra_BBAgent{
						CacheDir:    "cache",
						Input:       &pb.BuildInfra_BBAgent_Input{},
						PayloadPath: "kitchen-checkout",
					},
					Buildbucket: &pb.BuildInfra_Buildbucket{
						RequestedProperties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"key": {
									Kind: &structpb.Value_StringValue{
										StringValue: "value",
									},
								},
							},
						},
					},
					Logdog: &pb.BuildInfra_LogDog{
						Project: "project",
					},
					Resultdb: &pb.BuildInfra_ResultDB{},
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
						Priority: 30,
					},
				})
			})

			Convey("parent run id", func() {
				req := &pb.ScheduleBuildRequest{
					Swarming: &pb.ScheduleBuildRequest_Swarming{
						ParentRunId: "id",
					},
				}
				b := &pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				}

				setInfra(req, nil, b, nil)
				So(b.Infra, ShouldResembleProto, &pb.BuildInfra{
					Bbagent: &pb.BuildInfra_BBAgent{
						CacheDir:    "cache",
						Input:       &pb.BuildInfra_BBAgent_Input{},
						PayloadPath: "kitchen-checkout",
					},
					Buildbucket: &pb.BuildInfra_Buildbucket{},
					Logdog: &pb.BuildInfra_LogDog{
						Project: "project",
					},
					Resultdb: &pb.BuildInfra_ResultDB{},
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
						ParentRunId: "id",
						Priority:    30,
					},
				})
			})

			Convey("priority", func() {
				req := &pb.ScheduleBuildRequest{
					Priority: 1,
				}
				b := &pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				}

				setInfra(req, nil, b, nil)
				So(b.Infra, ShouldResembleProto, &pb.BuildInfra{
					Bbagent: &pb.BuildInfra_BBAgent{
						CacheDir:    "cache",
						Input:       &pb.BuildInfra_BBAgent_Input{},
						PayloadPath: "kitchen-checkout",
					},
					Buildbucket: &pb.BuildInfra_Buildbucket{},
					Logdog: &pb.BuildInfra_LogDog{
						Project: "project",
					},
					Resultdb: &pb.BuildInfra_ResultDB{},
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
						Priority: 1,
					},
				})
			})
		})
	})

	Convey("setInput", t, func() {
		Convey("nil", func() {
			b := &pb.Build{}

			setInput(nil, nil, b)
			So(b.Input, ShouldResembleProto, &pb.Build_Input{
				Properties: &structpb.Struct{},
			})
		})

		Convey("request", func() {
			Convey("properties", func() {
				Convey("empty", func() {
					req := &pb.ScheduleBuildRequest{}
					b := &pb.Build{}

					setInput(req, nil, b)
					So(b.Input, ShouldResembleProto, &pb.Build_Input{
						Properties: &structpb.Struct{},
					})
				})

				Convey("non-empty", func() {
					req := &pb.ScheduleBuildRequest{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"int": {
									Kind: &structpb.Value_NumberValue{
										NumberValue: 1,
									},
								},
								"str": {
									Kind: &structpb.Value_StringValue{
										StringValue: "value",
									},
								},
							},
						},
					}
					b := &pb.Build{}

					setInput(req, nil, b)
					So(b.Input, ShouldResembleProto, &pb.Build_Input{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"int": {
									Kind: &structpb.Value_NumberValue{
										NumberValue: 1,
									},
								},
								"str": {
									Kind: &structpb.Value_StringValue{
										StringValue: "value",
									},
								},
							},
						},
					})
				})
			})
		})

		Convey("config", func() {
			Convey("properties", func() {
				cfg := &pb.Builder{
					Properties: "{\"int\": 1, \"str\": \"value\"}",
				}
				b := &pb.Build{}

				setInput(nil, cfg, b)
				So(b.Input, ShouldResembleProto, &pb.Build_Input{
					Properties: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"int": {
								Kind: &structpb.Value_NumberValue{
									NumberValue: 1,
								},
							},
							"str": {
								Kind: &structpb.Value_StringValue{
									StringValue: "value",
								},
							},
						},
					},
				})
			})

			Convey("recipe", func() {
				Convey("empty", func() {
					cfg := &pb.Builder{
						Recipe: &pb.Builder_Recipe{},
					}
					b := &pb.Build{}

					setInput(nil, cfg, b)
					So(b.Input, ShouldResembleProto, &pb.Build_Input{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"recipe": {
									Kind: &structpb.Value_StringValue{},
								},
							},
						},
					})
				})

				Convey("properties", func() {
					cfg := &pb.Builder{
						Recipe: &pb.Builder_Recipe{
							Properties: []string{
								"key:value",
							},
						},
					}
					b := &pb.Build{}

					setInput(nil, cfg, b)
					So(b.Input, ShouldResembleProto, &pb.Build_Input{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"key": {
									Kind: &structpb.Value_StringValue{
										StringValue: "value",
									},
								},
								"recipe": {
									Kind: &structpb.Value_StringValue{
										StringValue: "",
									},
								},
							},
						},
					})
				})

				Convey("properties json", func() {
					cfg := &pb.Builder{
						Recipe: &pb.Builder_Recipe{
							PropertiesJ: []string{
								"str:\"value\"",
								"int:1",
							},
						},
					}
					b := &pb.Build{}

					setInput(nil, cfg, b)
					So(b.Input, ShouldResembleProto, &pb.Build_Input{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"int": {
									Kind: &structpb.Value_NumberValue{
										NumberValue: 1,
									},
								},
								"recipe": {
									Kind: &structpb.Value_StringValue{},
								},
								"str": {
									Kind: &structpb.Value_StringValue{
										StringValue: "value",
									},
								},
							},
						},
					})
				})

				Convey("recipe", func() {
					cfg := &pb.Builder{
						Recipe: &pb.Builder_Recipe{
							Name: "recipe",
						},
					}
					b := &pb.Build{}

					setInput(nil, cfg, b)
					So(b.Input, ShouldResembleProto, &pb.Build_Input{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"recipe": {
									Kind: &structpb.Value_StringValue{
										StringValue: "recipe",
									},
								},
							},
						},
					})
				})

				Convey("properties json > properties", func() {
					cfg := &pb.Builder{
						Recipe: &pb.Builder_Recipe{
							Properties: []string{
								"key:value",
							},
							PropertiesJ: []string{
								"key:1",
							},
						},
					}
					b := &pb.Build{}

					setInput(nil, cfg, b)
					So(b.Input, ShouldResembleProto, &pb.Build_Input{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"key": {
									Kind: &structpb.Value_NumberValue{
										NumberValue: 1,
									},
								},
								"recipe": {
									Kind: &structpb.Value_StringValue{
										StringValue: "",
									},
								},
							},
						},
					})
				})

				Convey("recipe > properties", func() {
					cfg := &pb.Builder{
						Recipe: &pb.Builder_Recipe{
							Name: "recipe",
							Properties: []string{
								"recipe:value",
							},
						},
					}
					b := &pb.Build{}

					setInput(nil, cfg, b)
					So(b.Input, ShouldResembleProto, &pb.Build_Input{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"recipe": {
									Kind: &structpb.Value_StringValue{
										StringValue: "recipe",
									},
								},
							},
						},
					})
				})

				Convey("recipe > properties json", func() {
					cfg := &pb.Builder{
						Recipe: &pb.Builder_Recipe{
							Name: "recipe",
							PropertiesJ: []string{
								"recipe:\"value\"",
							},
						},
					}
					b := &pb.Build{}

					setInput(nil, cfg, b)
					So(b.Input, ShouldResembleProto, &pb.Build_Input{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"recipe": {
									Kind: &structpb.Value_StringValue{
										StringValue: "recipe",
									},
								},
							},
						},
					})
				})
			})
		})

		Convey("request > config", func() {
			req := &pb.ScheduleBuildRequest{
				Properties: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"override": {
							Kind: &structpb.Value_StringValue{
								StringValue: "req value",
							},
						},
						"req key": {
							Kind: &structpb.Value_StringValue{
								StringValue: "req value",
							},
						},
					},
				},
			}
			cfg := &pb.Builder{
				Properties: "{\"override\": \"cfg value\", \"cfg key\": \"cfg value\"}",
			}
			b := &pb.Build{}

			setInput(req, cfg, b)
			So(b.Input, ShouldResembleProto, &pb.Build_Input{
				Properties: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"cfg key": {
							Kind: &structpb.Value_StringValue{
								StringValue: "cfg value",
							},
						},
						"override": {
							Kind: &structpb.Value_StringValue{
								StringValue: "req value",
							},
						},
						"req key": {
							Kind: &structpb.Value_StringValue{
								StringValue: "req value",
							},
						},
					},
				},
			})
		})
	})

	Convey("setTags", t, func() {
		Convey("nil", func() {
			b := &pb.Build{}

			setTags(nil, b)
			So(b.Tags, ShouldResemble, []*pb.StringPair{})
		})

		Convey("request", func() {
			req := &pb.ScheduleBuildRequest{
				Tags: []*pb.StringPair{
					{
						Key:   "key2",
						Value: "value2",
					},
					{
						Key:   "key1",
						Value: "value1",
					},
				},
			}
			normalizeSchedule(req)
			b := &pb.Build{}

			setTags(req, b)
			So(b.Tags, ShouldResemble, []*pb.StringPair{
				{
					Key:   "key1",
					Value: "value1",
				},
				{
					Key:   "key2",
					Value: "value2",
				},
			})
		})

		Convey("builder", func() {
			req := &pb.ScheduleBuildRequest{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			}
			normalizeSchedule(req)
			b := &pb.Build{}

			setTags(req, b)
			So(b.Tags, ShouldResemble, []*pb.StringPair{
				{
					Key:   "builder",
					Value: "builder",
				},
			})
		})

		Convey("gitiles commit", func() {
			req := &pb.ScheduleBuildRequest{
				GitilesCommit: &pb.GitilesCommit{
					Host:    "host",
					Project: "project",
					Id:      "id",
					Ref:     "ref",
				},
			}
			normalizeSchedule(req)
			b := &pb.Build{}

			setTags(req, b)
			So(b.Tags, ShouldResemble, []*pb.StringPair{
				{
					Key:   "buildset",
					Value: "commit/gitiles/host/project/+/id",
				},
				{
					Key:   "gitiles_ref",
					Value: "ref",
				},
			})
		})

		Convey("partial gitiles commit", func() {
			req := &pb.ScheduleBuildRequest{
				GitilesCommit: &pb.GitilesCommit{
					Host:    "host",
					Project: "project",
					Ref:     "ref",
				},
			}
			normalizeSchedule(req)
			b := &pb.Build{}

			setTags(req, b)
			So(b.Tags, ShouldResemble, []*pb.StringPair{
				{
					Key:   "gitiles_ref",
					Value: "ref",
				},
			})
		})

		Convey("gerrit changes", func() {
			Convey("one", func() {
				req := &pb.ScheduleBuildRequest{
					GerritChanges: []*pb.GerritChange{
						{
							Host:     "host",
							Change:   1,
							Patchset: 2,
						},
					},
				}
				normalizeSchedule(req)
				b := &pb.Build{}

				setTags(req, b)
				So(b.Tags, ShouldResemble, []*pb.StringPair{
					{
						Key:   "buildset",
						Value: "patch/gerrit/host/1/2",
					},
				})
			})

			Convey("many", func() {
				req := &pb.ScheduleBuildRequest{
					GerritChanges: []*pb.GerritChange{
						{
							Host:     "host",
							Change:   3,
							Patchset: 4,
						},
						{
							Host:     "host",
							Change:   1,
							Patchset: 2,
						},
					},
				}
				normalizeSchedule(req)
				b := &pb.Build{}

				setTags(req, b)
				So(b.Tags, ShouldResemble, []*pb.StringPair{
					{
						Key:   "buildset",
						Value: "patch/gerrit/host/1/2",
					},
					{
						Key:   "buildset",
						Value: "patch/gerrit/host/3/4",
					},
				})
			})
		})

		Convey("various", func() {
			req := &pb.ScheduleBuildRequest{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				GerritChanges: []*pb.GerritChange{
					{
						Host:     "host",
						Change:   3,
						Patchset: 4,
					},
					{
						Host:     "host",
						Change:   1,
						Patchset: 2,
					},
				},
				GitilesCommit: &pb.GitilesCommit{
					Host:    "host",
					Project: "project",
					Id:      "id",
					Ref:     "ref",
				},
				Tags: []*pb.StringPair{
					{
						Key:   "key2",
						Value: "value2",
					},
					{
						Key:   "key1",
						Value: "value1",
					},
				},
			}
			normalizeSchedule(req)
			b := &pb.Build{}

			setTags(req, b)
			So(b.Tags, ShouldResemble, []*pb.StringPair{
				{
					Key:   "builder",
					Value: "builder",
				},
				{
					Key:   "buildset",
					Value: "commit/gitiles/host/project/+/id",
				},
				{
					Key:   "buildset",
					Value: "patch/gerrit/host/1/2",
				},
				{
					Key:   "buildset",
					Value: "patch/gerrit/host/3/4",
				},
				{
					Key:   "gitiles_ref",
					Value: "ref",
				},
				{
					Key:   "key1",
					Value: "value1",
				},
				{
					Key:   "key2",
					Value: "value2",
				},
			})
		})
	})

	Convey("setTimeouts", t, func() {
		Convey("nil", func() {
			b := &pb.Build{}

			setTimeouts(nil, nil, b)
			So(b.ExecutionTimeout, ShouldResembleProto, &durationpb.Duration{
				Seconds: 10800,
			})
			So(b.GracePeriod, ShouldResembleProto, &durationpb.Duration{
				Seconds: 30,
			})
			So(b.SchedulingTimeout, ShouldResembleProto, &durationpb.Duration{
				Seconds: 21600,
			})
		})

		Convey("request only", func() {
			req := &pb.ScheduleBuildRequest{
				ExecutionTimeout: &durationpb.Duration{
					Seconds: 1,
				},
				GracePeriod: &durationpb.Duration{
					Seconds: 2,
				},
				SchedulingTimeout: &durationpb.Duration{
					Seconds: 3,
				},
			}
			normalizeSchedule(req)
			b := &pb.Build{}

			setTimeouts(req, nil, b)
			So(b.ExecutionTimeout, ShouldResembleProto, &durationpb.Duration{
				Seconds: 1,
			})
			So(b.GracePeriod, ShouldResembleProto, &durationpb.Duration{
				Seconds: 2,
			})
			So(b.SchedulingTimeout, ShouldResembleProto, &durationpb.Duration{
				Seconds: 3,
			})
		})

		Convey("config only", func() {
			cfg := &pb.Builder{
				ExecutionTimeoutSecs: 1,
				ExpirationSecs:       3,
				GracePeriod: &durationpb.Duration{
					Seconds: 2,
				},
			}
			b := &pb.Build{}

			setTimeouts(nil, cfg, b)
			So(b.ExecutionTimeout, ShouldResembleProto, &durationpb.Duration{
				Seconds: 1,
			})
			So(b.GracePeriod, ShouldResembleProto, &durationpb.Duration{
				Seconds: 2,
			})
			So(b.SchedulingTimeout, ShouldResembleProto, &durationpb.Duration{
				Seconds: 3,
			})
		})

		Convey("override", func() {
			req := &pb.ScheduleBuildRequest{
				ExecutionTimeout: &durationpb.Duration{
					Seconds: 1,
				},
				GracePeriod: &durationpb.Duration{
					Seconds: 2,
				},
				SchedulingTimeout: &durationpb.Duration{
					Seconds: 3,
				},
			}
			normalizeSchedule(req)
			cfg := &pb.Builder{
				ExecutionTimeoutSecs: 4,
				ExpirationSecs:       6,
				GracePeriod: &durationpb.Duration{
					Seconds: 5,
				},
			}
			b := &pb.Build{}

			setTimeouts(req, cfg, b)
			So(b.ExecutionTimeout, ShouldResembleProto, &durationpb.Duration{
				Seconds: 1,
			})
			So(b.GracePeriod, ShouldResembleProto, &durationpb.Duration{
				Seconds: 2,
			})
			So(b.SchedulingTimeout, ShouldResembleProto, &durationpb.Duration{
				Seconds: 3,
			})
		})
	})

	Convey("ScheduleBuild", t, func() {
		srv := &Builds{}
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		ctx = mathrand.Set(ctx, rand.New(rand.NewSource(0)))
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx, sch := tq.TestingContext(ctx, nil)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:caller@example.com",
		})
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

		Convey("builder", func() {
			Convey("not found", func() {
				req := &pb.ScheduleBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				}
				rsp, err := srv.ScheduleBuild(ctx, req)
				So(err, ShouldErrLike, "not found")
				So(rsp, ShouldBeNil)
				So(sch.Tasks(), ShouldBeEmpty)
			})

			Convey("permission denied", func() {
				So(datastore.Put(ctx, &model.Build{
					Proto: pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					},
				}), ShouldBeNil)
				req := &pb.ScheduleBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				}
				rsp, err := srv.ScheduleBuild(ctx, req)
				So(err, ShouldErrLike, "not found")
				So(rsp, ShouldBeNil)
				So(sch.Tasks(), ShouldBeEmpty)
			})

			Convey("dynamic", func() {
				So(datastore.Put(ctx, &model.Bucket{
					ID:     "bucket",
					Parent: model.ProjectKey(ctx, "project"),
					Proto: pb.Bucket{
						Acls: []*pb.Acl{
							{
								Identity: "user:caller@example.com",
								Role:     pb.Acl_SCHEDULER,
							},
						},
						Name: "bucket",
					},
				}), ShouldBeNil)
				req := &pb.ScheduleBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				}

				rsp, err := srv.ScheduleBuild(ctx, req)
				So(err, ShouldBeNil)
				So(rsp, ShouldResembleProto, &pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					CreatedBy:  "user:caller@example.com",
					CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
					Id:         9021868963221667745,
					Input:      &pb.Build_Input{},
					Status:     pb.Status_SCHEDULED,
				})
				So(sch.Tasks(), ShouldBeEmpty)
			})

			Convey("static", func() {
				So(datastore.Put(ctx, &model.Bucket{
					ID:     "bucket",
					Parent: model.ProjectKey(ctx, "project"),
					Proto: pb.Bucket{
						Acls: []*pb.Acl{
							{
								Identity: "user:caller@example.com",
								Role:     pb.Acl_SCHEDULER,
							},
						},
						Name:     "bucket",
						Swarming: &pb.Swarming{},
					},
				}), ShouldBeNil)
				req := &pb.ScheduleBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				}

				Convey("not found", func() {
					rsp, err := srv.ScheduleBuild(ctx, req)
					So(err, ShouldErrLike, "error fetching builders")
					So(rsp, ShouldBeNil)
					So(sch.Tasks(), ShouldBeEmpty)
				})

				Convey("exists", func() {
					So(datastore.Put(ctx, &model.Builder{
						Parent: model.BucketKey(ctx, "project", "bucket"),
						ID:     "builder",
						Config: pb.Builder{
							Name: "builder",
						},
					}), ShouldBeNil)
					So(datastore.Put(ctx, &model.Build{
						ID: 9021868963221667745,
						Proto: pb.Build{
							Id: 9021868963221667745,
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket",
								Builder: "builder",
							},
						},
					}), ShouldBeNil)

					rsp, err := srv.ScheduleBuild(ctx, req)
					So(err, ShouldErrLike, "build already exists")
					So(rsp, ShouldBeNil)
					So(sch.Tasks(), ShouldBeEmpty)
				})

				Convey("ok", func() {
					So(datastore.Put(ctx, &model.Builder{
						Parent: model.BucketKey(ctx, "project", "bucket"),
						ID:     "builder",
						Config: pb.Builder{
							BuildNumbers: pb.Toggle_YES,
							Name:         "builder",
						},
					}), ShouldBeNil)

					rsp, err := srv.ScheduleBuild(ctx, req)
					So(err, ShouldBeNil)
					So(rsp, ShouldResembleProto, &pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						CreatedBy:  "user:caller@example.com",
						CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
						Id:         9021868963221667745,
						Input:      &pb.Build_Input{},
						Number:     1,
						Status:     pb.Status_SCHEDULED,
					})
					So(sch.Tasks(), ShouldHaveLength, 1)
				})

				Convey("request ID", func() {
					req := &pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						RequestId: "id",
					}
					So(datastore.Put(ctx, &model.Builder{
						Parent: model.BucketKey(ctx, "project", "bucket"),
						ID:     "builder",
						Config: pb.Builder{
							Name: "builder",
						},
					}), ShouldBeNil)

					Convey("deduplication", func() {
						So(datastore.Put(ctx, &model.RequestID{
							ID:      "6d03f5c780125e74ac6cb0f25c5e0b6467ff96c96d98bfb41ba382863ba7707a",
							BuildID: 1,
						}), ShouldBeNil)

						Convey("not found", func() {
							rsp, err := srv.ScheduleBuild(ctx, req)
							So(err, ShouldErrLike, "no such entity")
							So(rsp, ShouldBeNil)
							So(sch.Tasks(), ShouldBeEmpty)
						})

						Convey("ok", func() {
							So(datastore.Put(ctx, &model.Build{
								ID: 1,
								Proto: pb.Build{
									Builder: &pb.BuilderID{
										Project: "project",
										Bucket:  "bucket",
										Builder: "builder",
									},
									Id: 1,
								},
							}), ShouldBeNil)

							rsp, err := srv.ScheduleBuild(ctx, req)
							So(err, ShouldBeNil)
							So(rsp, ShouldResembleProto, &pb.Build{
								Builder: &pb.BuilderID{
									Project: "project",
									Bucket:  "bucket",
									Builder: "builder",
								},
								Id:    1,
								Input: &pb.Build_Input{},
							})
							So(sch.Tasks(), ShouldBeEmpty)
						})
					})

					Convey("ok", func() {
						rsp, err := srv.ScheduleBuild(ctx, req)
						So(err, ShouldBeNil)
						So(rsp, ShouldResembleProto, &pb.Build{
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket",
								Builder: "builder",
							},
							CreatedBy:  "user:caller@example.com",
							CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
							Id:         9021868963221667745,
							Input:      &pb.Build_Input{},
							Status:     pb.Status_SCHEDULED,
						})
						So(sch.Tasks(), ShouldHaveLength, 1)

						r := &model.RequestID{
							ID: "6d03f5c780125e74ac6cb0f25c5e0b6467ff96c96d98bfb41ba382863ba7707a",
						}
						So(datastore.Get(ctx, r), ShouldBeNil)
						So(r, ShouldResemble, &model.RequestID{
							ID:         "6d03f5c780125e74ac6cb0f25c5e0b6467ff96c96d98bfb41ba382863ba7707a",
							BuildID:    9021868963221667745,
							CreatedBy:  "user:caller@example.com",
							CreateTime: datastore.RoundTime(testclock.TestRecentTimeUTC),
							RequestID:  "id",
						})
					})
				})
			})
		})

		Convey("template build ID", func() {
			Convey("not found", func() {
				req := &pb.ScheduleBuildRequest{
					TemplateBuildId: 1000,
				}
				rsp, err := srv.ScheduleBuild(ctx, req)
				So(err, ShouldErrLike, "not found")
				So(rsp, ShouldBeNil)
				So(sch.Tasks(), ShouldBeEmpty)
			})

			Convey("permission denied", func() {
				So(datastore.Put(ctx, &model.Build{
					ID: 1000,
					Proto: pb.Build{
						Id: 1000,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					},
				}), ShouldBeNil)
				req := &pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
				}
				rsp, err := srv.ScheduleBuild(ctx, req)
				So(err, ShouldErrLike, "not found")
				So(rsp, ShouldBeNil)
				So(sch.Tasks(), ShouldBeEmpty)
			})

			Convey("ok", func() {
				So(datastore.Put(ctx, &model.Bucket{
					ID:     "bucket",
					Parent: model.ProjectKey(ctx, "project"),
					Proto: pb.Bucket{
						Acls: []*pb.Acl{
							{
								Identity: "user:caller@example.com",
								Role:     pb.Acl_SCHEDULER,
							},
						},
						Name:     "bucket",
						Swarming: &pb.Swarming{},
					},
				}), ShouldBeNil)
				So(datastore.Put(ctx, &model.Build{
					ID: 1000,
					Proto: pb.Build{
						Id: 1000,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					},
				}), ShouldBeNil)
				req := &pb.ScheduleBuildRequest{
					TemplateBuildId: 1000,
				}

				Convey("not found", func() {
					rsp, err := srv.ScheduleBuild(ctx, req)
					So(err, ShouldErrLike, "error fetching builders")
					So(rsp, ShouldBeNil)
					So(sch.Tasks(), ShouldBeEmpty)
				})

				Convey("ok", func() {
					So(datastore.Put(ctx, &model.Builder{
						Parent: model.BucketKey(ctx, "project", "bucket"),
						ID:     "builder",
						Config: pb.Builder{
							BuildNumbers: pb.Toggle_YES,
							Name:         "builder",
						},
					}), ShouldBeNil)

					rsp, err := srv.ScheduleBuild(ctx, req)
					So(err, ShouldBeNil)
					So(rsp, ShouldResembleProto, &pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						CreatedBy:  "user:caller@example.com",
						CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
						Id:         9021868963221667745,
						Input:      &pb.Build_Input{},
						Number:     1,
						Status:     pb.Status_SCHEDULED,
					})
					So(sch.Tasks(), ShouldHaveLength, 1)
				})
			})
		})
	})

	Convey("scheduleBuilds", t, func() {
		srv := &Builds{}
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		ctx = mathrand.Set(ctx, rand.New(rand.NewSource(0)))
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx, sch := tq.TestingContext(ctx, nil)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:caller@example.com",
		})
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

		So(datastore.Put(ctx, &model.Bucket{
			ID:     "bucket",
			Parent: model.ProjectKey(ctx, "project"),
			Proto: pb.Bucket{
				Acls: []*pb.Acl{
					{
						Identity: "user:caller@example.com",
						Role:     pb.Acl_SCHEDULER,
					},
				},
				Name:     "bucket",
				Swarming: &pb.Swarming{},
			},
		}), ShouldBeNil)
		So(datastore.Put(ctx, &model.Build{
			ID: 1000,
			Proto: pb.Build{
				Id: 1000,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			},
		}), ShouldBeNil)
		So(datastore.Put(ctx, &model.Builder{
			Parent: model.BucketKey(ctx, "project", "bucket"),
			ID:     "builder",
			Config: pb.Builder{
				BuildNumbers: pb.Toggle_YES,
				Name:         "builder",
			},
		}), ShouldBeNil)

		Convey("one", func() {
			reqs := []*pb.ScheduleBuildRequest{
				{
					TemplateBuildId: 1000,
					Tags: []*pb.StringPair{
						{
							Key:   "buildset",
							Value: "buildset",
						},
					},
				},
			}

			rsp, err := srv.scheduleBuilds(ctx, reqs)
			So(err, ShouldBeNil)
			So(rsp, ShouldHaveLength, 1)
			So(rsp[0], ShouldResembleProto, &pb.Build{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				CreatedBy:  "user:caller@example.com",
				CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
				Id:         9021868963221667745,
				Input:      &pb.Build_Input{},
				Number:     1,
				Status:     pb.Status_SCHEDULED,
			})
			So(sch.Tasks(), ShouldHaveLength, 1)

			ind, err := model.SearchTagIndex(ctx, "buildset", "buildset")
			So(err, ShouldBeNil)
			So(ind, ShouldResemble, []*model.TagIndexEntry{
				{
					BuildID:     9021868963221667745,
					BucketID:    "project/bucket",
					CreatedTime: datastore.RoundTime(testclock.TestRecentTimeUTC),
				},
			})
		})

		Convey("many", func() {
			Convey("not found", func() {
				reqs := []*pb.ScheduleBuildRequest{
					{
						TemplateBuildId: 1000,
						Tags: []*pb.StringPair{
							{
								Key:   "buildset",
								Value: "buildset",
							},
						},
					},
					{
						TemplateBuildId: 1001,
						Tags: []*pb.StringPair{
							{
								Key:   "buildset",
								Value: "buildset",
							},
						},
					},
				}

				rsp, err := srv.scheduleBuilds(ctx, reqs)
				So(err, ShouldNotBeNil)
				So(err, ShouldHaveSameTypeAs, errors.MultiError{})
				So(err.(errors.MultiError)[0], ShouldErrLike, "error in schedule batch")
				So(err.(errors.MultiError)[1], ShouldErrLike, "not found")
				So(rsp, ShouldBeEmpty)
				So(sch.Tasks(), ShouldBeEmpty)

				ind, err := model.SearchTagIndex(ctx, "buildset", "buildset")
				So(err, ShouldBeNil)
				So(ind, ShouldBeEmpty)
			})

			Convey("ok", func() {
				reqs := []*pb.ScheduleBuildRequest{
					{
						TemplateBuildId: 1000,
						Tags: []*pb.StringPair{
							{
								Key:   "buildset",
								Value: "buildset",
							},
						},
					},
					{
						TemplateBuildId: 1000,
						Tags: []*pb.StringPair{
							{
								Key:   "buildset",
								Value: "buildset",
							},
						},
					},
				}

				rsp, err := srv.scheduleBuilds(ctx, reqs)
				So(err, ShouldBeNil)
				So(rsp, ShouldHaveLength, 2)
				So(rsp[0], ShouldResembleProto, &pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					CreatedBy:  "user:caller@example.com",
					CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
					Id:         9021868963222163313,
					Input:      &pb.Build_Input{},
					Number:     1,
					Status:     pb.Status_SCHEDULED,
				})
				So(rsp[1], ShouldResembleProto, &pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					CreatedBy:  "user:caller@example.com",
					CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
					Id:         9021868963222163297,
					Input:      &pb.Build_Input{},
					Number:     2,
					Status:     pb.Status_SCHEDULED,
				})
				So(sch.Tasks(), ShouldHaveLength, 2)

				ind, err := model.SearchTagIndex(ctx, "buildset", "buildset")
				So(err, ShouldBeNil)
				So(ind, ShouldResemble, []*model.TagIndexEntry{
					{
						BuildID:     9021868963222163313,
						BucketID:    "project/bucket",
						CreatedTime: datastore.RoundTime(testclock.TestRecentTimeUTC),
					},
					{
						BuildID:     9021868963222163297,
						BucketID:    "project/bucket",
						CreatedTime: datastore.RoundTime(testclock.TestRecentTimeUTC),
					},
				})
			})
		})
	})

	Convey("structContains", t, func() {
		Convey("nil", func() {
			So(structContains(nil, nil), ShouldBeTrue)
		})

		Convey("nil struct", func() {
			path := []string{"path"}
			So(structContains(nil, path), ShouldBeFalse)
		})

		Convey("nil path", func() {
			s := &structpb.Struct{}
			So(structContains(s, nil), ShouldBeTrue)
		})

		Convey("one component", func() {
			s := &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"key": {
						Kind: &structpb.Value_StringValue{
							StringValue: "value",
						},
					},
				},
			}
			path := []string{"key"}
			So(structContains(s, path), ShouldBeTrue)
		})

		Convey("many components", func() {
			s := &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"key1": {
						Kind: &structpb.Value_StructValue{
							StructValue: &structpb.Struct{
								Fields: map[string]*structpb.Value{
									"key2": {
										Kind: &structpb.Value_StructValue{
											StructValue: &structpb.Struct{
												Fields: map[string]*structpb.Value{
													"key3": {
														Kind: &structpb.Value_StringValue{
															StringValue: "value",
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}
			path := []string{"key1", "key2", "key3"}
			So(structContains(s, path), ShouldBeTrue)
		})

		Convey("excess component", func() {
			s := &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"key1": {
						Kind: &structpb.Value_StructValue{
							StructValue: &structpb.Struct{
								Fields: map[string]*structpb.Value{
									"key2": {
										Kind: &structpb.Value_StringValue{
											StringValue: "value",
										},
									},
								},
							},
						},
					},
				},
			}
			path := []string{"key1"}
			So(structContains(s, path), ShouldBeTrue)
		})
	})

	Convey("validateSchedule", t, func() {
		Convey("nil", func() {
			err := validateSchedule(nil)
			So(err, ShouldErrLike, "builder or template_build_id is required")
		})

		Convey("empty", func() {
			req := &pb.ScheduleBuildRequest{}
			err := validateSchedule(req)
			So(err, ShouldErrLike, "builder or template_build_id is required")
		})

		Convey("request ID", func() {
			req := &pb.ScheduleBuildRequest{
				RequestId:       "request/id",
				TemplateBuildId: 1,
			}
			err := validateSchedule(req)
			So(err, ShouldErrLike, "request_id cannot contain")
		})

		Convey("builder ID", func() {
			req := &pb.ScheduleBuildRequest{
				Builder: &pb.BuilderID{},
			}
			err := validateSchedule(req)
			So(err, ShouldErrLike, "project must match")
		})

		Convey("dimensions", func() {
			Convey("empty", func() {
				req := &pb.ScheduleBuildRequest{
					Dimensions: []*pb.RequestedDimension{
						{},
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(req)
				So(err, ShouldErrLike, "dimensions")
			})

			Convey("expiration", func() {
				Convey("empty", func() {
					req := &pb.ScheduleBuildRequest{
						Dimensions: []*pb.RequestedDimension{
							{
								Expiration: &durationpb.Duration{},
								Key:        "key",
								Value:      "value",
							},
						},
						TemplateBuildId: 1,
					}
					err := validateSchedule(req)
					So(err, ShouldBeNil)
				})

				Convey("nanos", func() {
					req := &pb.ScheduleBuildRequest{
						Dimensions: []*pb.RequestedDimension{
							{
								Expiration: &durationpb.Duration{
									Nanos: 1,
								},
								Key:   "key",
								Value: "value",
							},
						},
						TemplateBuildId: 1,
					}
					err := validateSchedule(req)
					So(err, ShouldErrLike, "nanos must not be specified")
				})

				Convey("seconds", func() {
					Convey("negative", func() {
						req := &pb.ScheduleBuildRequest{
							Dimensions: []*pb.RequestedDimension{
								{
									Expiration: &durationpb.Duration{
										Seconds: -60,
									},
									Key:   "key",
									Value: "value",
								},
							},
							TemplateBuildId: 1,
						}
						err := validateSchedule(req)
						So(err, ShouldErrLike, "seconds must not be negative")
					})

					Convey("whole minute", func() {
						req := &pb.ScheduleBuildRequest{
							Dimensions: []*pb.RequestedDimension{
								{
									Expiration: &durationpb.Duration{
										Seconds: 1,
									},
									Key:   "key",
									Value: "value",
								},
							},
							TemplateBuildId: 1,
						}
						err := validateSchedule(req)
						So(err, ShouldErrLike, "seconds must be a multiple of 60")
					})
				})

				Convey("ok", func() {
					req := &pb.ScheduleBuildRequest{
						Dimensions: []*pb.RequestedDimension{
							{
								Expiration: &durationpb.Duration{
									Seconds: 60,
								},
								Key:   "key",
								Value: "value",
							},
						},
						TemplateBuildId: 1,
					}
					err := validateSchedule(req)
					So(err, ShouldBeNil)
				})
			})

			Convey("key", func() {
				Convey("empty", func() {
					req := &pb.ScheduleBuildRequest{
						Dimensions: []*pb.RequestedDimension{
							{
								Value: "value",
							},
						},
						TemplateBuildId: 1,
					}
					err := validateSchedule(req)
					So(err, ShouldErrLike, "key must be specified")
				})

				Convey("caches", func() {
					req := &pb.ScheduleBuildRequest{
						Dimensions: []*pb.RequestedDimension{
							{
								Key:   "caches",
								Value: "value",
							},
						},
						TemplateBuildId: 1,
					}
					err := validateSchedule(req)
					So(err, ShouldErrLike, "caches may only be specified in builder configs")
				})

				Convey("pool", func() {
					req := &pb.ScheduleBuildRequest{
						Dimensions: []*pb.RequestedDimension{
							{
								Key:   "pool",
								Value: "value",
							},
						},
						TemplateBuildId: 1,
					}
					err := validateSchedule(req)
					So(err, ShouldErrLike, "pool may only be specified in builder configs")
				})

				Convey("ok", func() {
					req := &pb.ScheduleBuildRequest{
						Dimensions: []*pb.RequestedDimension{
							{
								Key:   "key",
								Value: "value",
							},
						},
						TemplateBuildId: 1,
					}
					err := validateSchedule(req)
					So(err, ShouldBeNil)
				})
			})

			Convey("value", func() {
				req := &pb.ScheduleBuildRequest{
					Dimensions: []*pb.RequestedDimension{
						{
							Key: "key",
						},
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(req)
				So(err, ShouldErrLike, "value must be specified")
			})

			Convey("ok", func() {
				req := &pb.ScheduleBuildRequest{
					Dimensions: []*pb.RequestedDimension{
						{
							Key:   "key",
							Value: "value",
						},
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(req)
				So(err, ShouldBeNil)
			})
		})

		Convey("exe", func() {
			Convey("empty", func() {
				req := &pb.ScheduleBuildRequest{
					Exe:             &pb.Executable{},
					TemplateBuildId: 1,
				}
				err := validateSchedule(req)
				So(err, ShouldBeNil)
			})

			Convey("package", func() {
				req := &pb.ScheduleBuildRequest{
					Exe: &pb.Executable{
						CipdPackage: "package",
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(req)
				So(err, ShouldErrLike, "cipd_package must not be specified")
			})

			Convey("version", func() {
				Convey("invalid", func() {
					req := &pb.ScheduleBuildRequest{
						Exe: &pb.Executable{
							CipdVersion: "invalid!",
						},
						TemplateBuildId: 1,
					}
					err := validateSchedule(req)
					So(err, ShouldErrLike, "cipd_version")
				})

				Convey("valid", func() {
					req := &pb.ScheduleBuildRequest{
						Exe: &pb.Executable{
							CipdVersion: "valid",
						},
						TemplateBuildId: 1,
					}
					err := validateSchedule(req)
					So(err, ShouldBeNil)
				})
			})
		})

		Convey("gerrit changes", func() {
			Convey("empty", func() {
				req := &pb.ScheduleBuildRequest{
					GerritChanges:   []*pb.GerritChange{},
					TemplateBuildId: 1,
				}
				err := validateSchedule(req)
				So(err, ShouldBeNil)
			})

			Convey("unspecified", func() {
				req := &pb.ScheduleBuildRequest{
					GerritChanges: []*pb.GerritChange{
						{},
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(req)
				So(err, ShouldErrLike, "gerrit_changes")
			})

			Convey("change", func() {
				req := &pb.ScheduleBuildRequest{
					GerritChanges: []*pb.GerritChange{
						{
							Host:     "host",
							Patchset: 1,
							Project:  "project",
						},
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(req)
				So(err, ShouldErrLike, "change must be specified")
			})

			Convey("host", func() {
				req := &pb.ScheduleBuildRequest{
					GerritChanges: []*pb.GerritChange{
						{
							Change:   1,
							Patchset: 1,
							Project:  "project",
						},
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(req)
				So(err, ShouldErrLike, "host must be specified")
			})

			Convey("patchset", func() {
				req := &pb.ScheduleBuildRequest{
					GerritChanges: []*pb.GerritChange{
						{
							Change:  1,
							Host:    "host",
							Project: "project",
						},
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(req)
				So(err, ShouldErrLike, "patchset must be specified")
			})

			Convey("project", func() {
				req := &pb.ScheduleBuildRequest{
					GerritChanges: []*pb.GerritChange{
						{
							Change:   1,
							Host:     "host",
							Patchset: 1,
						},
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(req)
				So(err, ShouldErrLike, "project must be specified")
			})

			Convey("ok", func() {
				req := &pb.ScheduleBuildRequest{
					GerritChanges: []*pb.GerritChange{
						{
							Change:   1,
							Host:     "host",
							Patchset: 1,
							Project:  "project",
						},
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(req)
				So(err, ShouldBeNil)
			})
		})

		Convey("gitiles commit", func() {
			req := &pb.ScheduleBuildRequest{
				GitilesCommit: &pb.GitilesCommit{
					Host: "example.com",
				},
				TemplateBuildId: 1,
			}
			err := validateSchedule(req)
			So(err, ShouldErrLike, "gitiles_commit")
		})

		Convey("notify", func() {
			Convey("empty", func() {
				req := &pb.ScheduleBuildRequest{
					Notify:          &pb.NotificationConfig{},
					TemplateBuildId: 1,
				}
				err := validateSchedule(req)
				So(err, ShouldErrLike, "notify")
			})

			Convey("pubsub topic", func() {
				req := &pb.ScheduleBuildRequest{
					Notify: &pb.NotificationConfig{
						UserData: []byte("user data"),
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(req)
				So(err, ShouldErrLike, "pubsub_topic")
			})

			Convey("user data", func() {
				req := &pb.ScheduleBuildRequest{
					Notify: &pb.NotificationConfig{
						PubsubTopic: "topic",
						UserData:    make([]byte, 4097),
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(req)
				So(err, ShouldErrLike, "user_data")
			})

			Convey("ok", func() {
				req := &pb.ScheduleBuildRequest{
					Notify: &pb.NotificationConfig{
						PubsubTopic: "topic",
						UserData:    []byte("user data"),
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(req)
				So(err, ShouldBeNil)
			})
		})

		Convey("priority", func() {
			Convey("negative", func() {
				req := &pb.ScheduleBuildRequest{
					Priority:        -1,
					TemplateBuildId: 1,
				}
				err := validateSchedule(req)
				So(err, ShouldErrLike, "priority must be in")
			})

			Convey("excessive", func() {
				req := &pb.ScheduleBuildRequest{
					Priority:        256,
					TemplateBuildId: 1,
				}
				err := validateSchedule(req)
				So(err, ShouldErrLike, "priority must be in")
			})
		})

		Convey("properties", func() {
			Convey("prohibited", func() {
				req := &pb.ScheduleBuildRequest{
					Properties: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"buildbucket": {
								Kind: &structpb.Value_StringValue{},
							},
						},
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(req)
				So(err, ShouldErrLike, "must not be specified")
			})

			Convey("ok", func() {
				req := &pb.ScheduleBuildRequest{
					Properties: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key": {
								Kind: &structpb.Value_StringValue{},
							},
						},
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(req)
				So(err, ShouldBeNil)
			})
		})

		Convey("tags", func() {
			req := &pb.ScheduleBuildRequest{
				Tags: []*pb.StringPair{
					{
						Key: "key:value",
					},
				},
				TemplateBuildId: 1,
			}
			err := validateSchedule(req)
			So(err, ShouldErrLike, "tags")
		})

		Convey("experiments", func() {
			Convey("ok", func() {
				req := &pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
					Experiments: map[string]bool{
						"luci.use_realms":       true,
						"cool.experiment_thing": true,
					},
				}
				So(validateSchedule(req), ShouldBeNil)
			})

			Convey("bad name", func() {
				req := &pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
					Experiments: map[string]bool{
						"bad name": true,
					},
				}
				So(validateSchedule(req), ShouldErrLike, "does not match")
			})

			Convey("bad reserved", func() {
				req := &pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
					Experiments: map[string]bool{
						"luci.use_ralms": true,
					},
				}
				So(validateSchedule(req), ShouldErrLike, "unknown experiment has reserved prefix")
			})
		})
	})
}
