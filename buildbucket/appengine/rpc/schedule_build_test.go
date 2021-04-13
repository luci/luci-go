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

	structpb "github.com/golang/protobuf/ptypes/struct"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/tq"

	bb "go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestScheduleBuild(t *testing.T) {
	t.Parallel()

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
		So(datastore.Put(ctx, &model.Builder{
			Parent: model.BucketKey(ctx, "project", "bucket 2"),
			ID:     "builder 1",
			Config: pb.Builder{
				Name: "builder 1",
			},
		}), ShouldBeNil)

		Convey("not found", func() {
			reqs := []*pb.ScheduleBuildRequest{
				{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
			}
			bldrs, err := fetchBuilderConfigs(ctx, reqs)
			So(err, ShouldErrLike, "no such entity")
			So(bldrs, ShouldBeNil)
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
			So(bldrs["project/bucket 2"]["builder 1"], ShouldResembleProto, &pb.Builder{
				Name: "builder 1",
			})
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
		ctx, _ := testclock.UseTime(mathrand.Set(txndefer.FilterRDS(memory.Use(context.Background())), rand.New(rand.NewSource(0))), testclock.TestRecentTimeUTC)
		ctx, sch := tq.TestingContext(ctx, nil)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

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
			}
			So(datastore.Put(ctx, &model.Builder{
				Parent: model.BucketKey(ctx, "project", "bucket"),
				ID:     "builder",
			}), ShouldBeNil)

			blds, err := scheduleBuilds(ctx, req)
			So(err, ShouldBeNil)
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
					Id:    9021868963221667745,
					Input: &pb.Build_Input{},
					SchedulingTimeout: &durationpb.Duration{
						Seconds: 21600,
					},
					Status: pb.Status_SCHEDULED,
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
						"-" + bb.ExperimentBBCanarySoftware,
						"-" + bb.ExperimentBBAgent,
						"-" + bb.ExperimentUseRealms,
					},
					Incomplete: true,
					Status:     pb.Status_SCHEDULED,
					Tags: []string{
						"builder:builder",
					},
					Project: "project",
				},
			})
			So(sch.Tasks(), ShouldHaveLength, 1)
		})

		Convey("many", func() {
			reqs := []*pb.ScheduleBuildRequest{
				{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Critical: pb.Trinary_UNSET,
				},
				{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Critical: pb.Trinary_YES,
				},
				{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Critical: pb.Trinary_NO,
				},
			}
			So(datastore.Put(ctx, &model.Builder{
				Parent: model.BucketKey(ctx, "project", "bucket"),
				ID:     "builder",
			}), ShouldBeNil)

			blds, err := scheduleBuilds(ctx, reqs...)
			So(err, ShouldBeNil)
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
					Id:    9021868963221610337,
					Input: &pb.Build_Input{},
					SchedulingTimeout: &durationpb.Duration{
						Seconds: 21600,
					},
					Status: pb.Status_SCHEDULED,
				},
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
					Critical: pb.Trinary_YES,
					ExecutionTimeout: &durationpb.Duration{
						Seconds: 10800,
					},
					GracePeriod: &durationpb.Duration{
						Seconds: 30,
					},
					Id:    9021868963221610321,
					Input: &pb.Build_Input{},
					SchedulingTimeout: &durationpb.Duration{
						Seconds: 21600,
					},
					Status: pb.Status_SCHEDULED,
				},
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
					Critical: pb.Trinary_NO,
					ExecutionTimeout: &durationpb.Duration{
						Seconds: 10800,
					},
					GracePeriod: &durationpb.Duration{
						Seconds: 30,
					},
					Id:    9021868963221610305,
					Input: &pb.Build_Input{},
					SchedulingTimeout: &durationpb.Duration{
						Seconds: 21600,
					},
					Status: pb.Status_SCHEDULED,
				},
			})
			So(blds, ShouldResemble, []*model.Build{
				{
					ID:         9021868963221610337,
					BucketID:   "project/bucket",
					BuilderID:  "project/bucket/builder",
					CreatedBy:  "anonymous:anonymous",
					CreateTime: testclock.TestRecentTimeUTC,
					Experiments: []string{
						"-" + bb.ExperimentBBCanarySoftware,
						"-" + bb.ExperimentBBAgent,
						"-" + bb.ExperimentUseRealms,
					},
					Incomplete: true,
					Status:     pb.Status_SCHEDULED,
					Tags: []string{
						"builder:builder",
					},
					Project: "project",
				},
				{
					ID:         9021868963221610321,
					BucketID:   "project/bucket",
					BuilderID:  "project/bucket/builder",
					CreatedBy:  "anonymous:anonymous",
					CreateTime: testclock.TestRecentTimeUTC,
					Experiments: []string{
						"-" + bb.ExperimentBBCanarySoftware,
						"-" + bb.ExperimentBBAgent,
						"-" + bb.ExperimentUseRealms,
					},
					Incomplete: true,
					Status:     pb.Status_SCHEDULED,
					Tags: []string{
						"builder:builder",
					},
					Project: "project",
				},
				{
					ID:         9021868963221610305,
					BucketID:   "project/bucket",
					BuilderID:  "project/bucket/builder",
					CreatedBy:  "anonymous:anonymous",
					CreateTime: testclock.TestRecentTimeUTC,
					Experiments: []string{
						"-" + bb.ExperimentBBCanarySoftware,
						"-" + bb.ExperimentBBAgent,
						"-" + bb.ExperimentUseRealms,
					},
					Incomplete: true,
					Status:     pb.Status_SCHEDULED,
					Tags: []string{
						"builder:builder",
					},
					Project: "project",
				},
			})
			So(sch.Tasks(), ShouldHaveLength, 3)
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

	Convey("setExecutable", t, func() {
		Convey("nil", func() {
			ent := &model.Build{}

			setExecutable(nil, nil, ent)
			So(ent.Proto.Exe, ShouldResembleProto, &pb.Executable{
				Cmd: []string{"recipes"},
			})
		})

		Convey("command", func() {
			Convey("recipes", func() {
				ent := &model.Build{}

				setExecutable(nil, nil, ent)
				So(ent.Proto.Exe, ShouldResembleProto, &pb.Executable{
					Cmd: []string{"recipes"},
				})
			})

			Convey("luciexe", func() {
				ent := &model.Build{
					Experiments: []string{
						"+" + bb.ExperimentBBAgent,
					},
				}

				setExecutable(nil, nil, ent)
				So(ent.Proto.Exe, ShouldResembleProto, &pb.Executable{
					Cmd: []string{"luciexe"},
				})
			})
		})

		Convey("request only", func() {
			req := &pb.ScheduleBuildRequest{
				Exe: &pb.Executable{
					CipdPackage: "package",
					CipdVersion: "version",
					Cmd:         []string{"command"},
				},
			}
			ent := &model.Build{}

			setExecutable(req, nil, ent)
			So(ent.Proto.Exe, ShouldResembleProto, &pb.Executable{
				CipdVersion: "version",
				Cmd:         []string{"recipes"},
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
				ent := &model.Build{}

				setExecutable(nil, cfg, ent)
				So(ent.Proto.Exe, ShouldResembleProto, &pb.Executable{
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
				ent := &model.Build{}

				setExecutable(nil, cfg, ent)
				So(ent.Proto.Exe, ShouldResembleProto, &pb.Executable{
					CipdPackage: "package 2",
					CipdVersion: "version 2",
					Cmd:         []string{"command"},
				})
			})

			Convey("command", func() {
				Convey("recipes", func() {
					cfg := &pb.Builder{
						Exe: &pb.Executable{
							CipdPackage: "package",
							CipdVersion: "version",
						},
					}
					ent := &model.Build{
						Experiments: []string{
							"-" + bb.ExperimentBBAgent,
						},
					}

					setExecutable(nil, cfg, ent)
					So(ent.Proto.Exe, ShouldResembleProto, &pb.Executable{
						CipdPackage: "package",
						CipdVersion: "version",
						Cmd:         []string{"recipes"},
					})
				})

				Convey("luciexe", func() {
					cfg := &pb.Builder{
						Exe: &pb.Executable{
							CipdPackage: "package",
							CipdVersion: "version",
						},
					}
					ent := &model.Build{
						Experiments: []string{
							"+" + bb.ExperimentBBAgent,
						},
					}

					setExecutable(nil, cfg, ent)
					So(ent.Proto.Exe, ShouldResembleProto, &pb.Executable{
						CipdPackage: "package",
						CipdVersion: "version",
						Cmd:         []string{"luciexe"},
					})
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
			ent := &model.Build{}

			setExecutable(req, cfg, ent)
			So(ent.Proto.Exe, ShouldResembleProto, &pb.Executable{
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
					Input: &pb.Build_Input{},
				},
			}

			setExperiments(ctx, nil, nil, ent)
			So(ent.Experiments, ShouldResemble, []string{
				"-" + bb.ExperimentBBCanarySoftware,
				"-" + bb.ExperimentBBAgent,
				"-" + bb.ExperimentUseRealms,
			})
			So(ent.Proto.Input.Experiments, ShouldBeNil)
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
					Input: &pb.Build_Input{},
				},
			}

			setExperiments(ctx, req, nil, ent)
			So(ent, ShouldResemble, &model.Build{
				Experiments: []string{
					"+experiment1",
					"-experiment2",
					"-" + bb.ExperimentBBCanarySoftware,
					"-" + bb.ExperimentBBAgent,
					"-" + bb.ExperimentUseRealms,
				},
				Proto: pb.Build{
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
					Input: &pb.Build_Input{},
				},
			}

			setExperiments(ctx, req, nil, ent)
			So(ent, ShouldResemble, &model.Build{
				Canary: true,
				Experiments: []string{
					"+" + bb.ExperimentBBCanarySoftware,
					"-" + bb.ExperimentBBAgent,
					"-" + bb.ExperimentUseRealms,
				},
				Proto: pb.Build{
					Canary: true,
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
					Input: &pb.Build_Input{},
				},
			}
			cfg := &pb.Builder{
				Experiments: map[string]int32{
					"experiment1": 100,
					"experiment2": 0,
				},
			}

			setExperiments(ctx, nil, cfg, ent)
			So(ent, ShouldResemble, &model.Build{
				Experiments: []string{
					"+experiment1",
					"-experiment2",
					"-" + bb.ExperimentBBCanarySoftware,
					"-" + bb.ExperimentBBAgent,
					"-" + bb.ExperimentUseRealms,
				},
				Proto: pb.Build{
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
						Input: &pb.Build_Input{},
					},
				}

				setExperiments(ctx, req, nil, ent)
				So(ent, ShouldResemble, &model.Build{
					Experimental: true,
					Experiments: []string{
						"+" + bb.ExperimentNonProduction,
						"-" + bb.ExperimentBBCanarySoftware,
						"-" + bb.ExperimentBBAgent,
						"-" + bb.ExperimentUseRealms,
					},
					Proto: pb.Build{
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
						Input: &pb.Build_Input{},
					},
				}

				setExperiments(ctx, req, cfg, ent)
				So(ent, ShouldResemble, &model.Build{
					Canary: true,
					Experiments: []string{
						"+" + bb.ExperimentBBCanarySoftware,
						"-" + bb.ExperimentBBAgent,
						"-" + bb.ExperimentUseRealms,
					},
					Proto: pb.Build{
						Canary: true,
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
						Input: &pb.Build_Input{},
					},
				}

				setExperiments(ctx, req, cfg, ent)
				So(ent, ShouldResemble, &model.Build{
					Experiments: []string{
						"+experiment1",
						"-experiment2",
						"-" + bb.ExperimentBBCanarySoftware,
						"-" + bb.ExperimentBBAgent,
						"-" + bb.ExperimentUseRealms,
					},
					Proto: pb.Build{
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
						Input: &pb.Build_Input{},
					},
				}

				setExperiments(ctx, req, cfg, ent)
				So(ent, ShouldResemble, &model.Build{
					Experimental: true,
					Experiments: []string{
						"+experiment1",
						"+" + bb.ExperimentNonProduction,
						"-experiment2",
						"-" + bb.ExperimentBBCanarySoftware,
						"-" + bb.ExperimentBBAgent,
						"-" + bb.ExperimentUseRealms,
					},
					Proto: pb.Build{
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

	Convey("setTags", t, func() {
		Convey("nil", func() {
			ent := &model.Build{}

			setTags(nil, ent)
			So(ent.Tags, ShouldBeEmpty)
			So(ent.Proto.Tags, ShouldBeNil)
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
			ent := &model.Build{}

			setTags(req, ent)
			So(ent.Tags, ShouldResemble, []string{
				"key1:value1",
				"key2:value2",
			})
			So(ent.Proto.Tags, ShouldBeNil)
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
			ent := &model.Build{}

			setTags(req, ent)
			So(ent.Tags, ShouldResemble, []string{
				"builder:builder",
			})
			So(ent.Proto.Tags, ShouldBeNil)
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
			ent := &model.Build{}

			setTags(req, ent)
			So(ent.Tags, ShouldResemble, []string{
				"buildset:commit/gitiles/host/project/+/id",
				"gitiles_ref:ref",
			})
			So(ent.Proto.Tags, ShouldBeNil)
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
				ent := &model.Build{}

				setTags(req, ent)
				So(ent.Tags, ShouldResemble, []string{
					"buildset:patch/gerrit/host/1/2",
				})
				So(ent.Proto.Tags, ShouldBeNil)
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
				ent := &model.Build{}

				setTags(req, ent)
				So(ent.Tags, ShouldResemble, []string{
					"buildset:patch/gerrit/host/1/2",
					"buildset:patch/gerrit/host/3/4",
				})
				So(ent.Proto.Tags, ShouldBeNil)
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
			ent := &model.Build{}

			setTags(req, ent)
			So(ent.Tags, ShouldResemble, []string{
				"builder:builder",
				"buildset:commit/gitiles/host/project/+/id",
				"buildset:patch/gerrit/host/1/2",
				"buildset:patch/gerrit/host/3/4",
				"gitiles_ref:ref",
				"key1:value1",
				"key2:value2",
			})
			So(ent.Proto.Tags, ShouldBeNil)
		})
	})

	Convey("setTimeouts", t, func() {
		Convey("nil", func() {
			ent := &model.Build{}

			setTimeouts(nil, nil, ent)
			So(ent.Proto.ExecutionTimeout, ShouldResembleProto, &durationpb.Duration{
				Seconds: 10800,
			})
			So(ent.Proto.GracePeriod, ShouldResembleProto, &durationpb.Duration{
				Seconds: 30,
			})
			So(ent.Proto.SchedulingTimeout, ShouldResembleProto, &durationpb.Duration{
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
			ent := &model.Build{}

			setTimeouts(req, nil, ent)
			So(ent.Proto.ExecutionTimeout, ShouldResembleProto, &durationpb.Duration{
				Seconds: 1,
			})
			So(ent.Proto.GracePeriod, ShouldResembleProto, &durationpb.Duration{
				Seconds: 2,
			})
			So(ent.Proto.SchedulingTimeout, ShouldResembleProto, &durationpb.Duration{
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
			ent := &model.Build{}

			setTimeouts(nil, cfg, ent)
			So(ent.Proto.ExecutionTimeout, ShouldResembleProto, &durationpb.Duration{
				Seconds: 1,
			})
			So(ent.Proto.GracePeriod, ShouldResembleProto, &durationpb.Duration{
				Seconds: 2,
			})
			So(ent.Proto.SchedulingTimeout, ShouldResembleProto, &durationpb.Duration{
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
			ent := &model.Build{}

			setTimeouts(req, cfg, ent)
			So(ent.Proto.ExecutionTimeout, ShouldResembleProto, &durationpb.Duration{
				Seconds: 1,
			})
			So(ent.Proto.GracePeriod, ShouldResembleProto, &durationpb.Duration{
				Seconds: 2,
			})
			So(ent.Proto.SchedulingTimeout, ShouldResembleProto, &durationpb.Duration{
				Seconds: 3,
			})
		})
	})

	Convey("ScheduleBuild", t, func() {
		srv := &Builds{}
		ctx, _ := testclock.UseTime(mathrand.Set(txndefer.FilterRDS(memory.Use(context.Background())), rand.New(rand.NewSource(0))), testclock.TestRecentTimeUTC)
		ctx, sch := tq.TestingContext(ctx, nil)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:caller@example.com",
		})

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
