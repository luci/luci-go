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

package model

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func mustStruct(data map[string]any) *structpb.Struct {
	ret, err := structpb.NewStruct(data)
	if err != nil {
		panic(err)
	}
	return ret
}

func TestBuild(t *testing.T) {
	t.Parallel()

	Convey("Build", t, func() {
		ctx := memory.Use(context.Background())
		ctx, tclock := testclock.UseTime(ctx, testclock.TestRecentTimeUTC)

		t0 := tclock.Now()
		t0pb := timestamppb.New(t0)

		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		m := NoopBuildMask

		Convey("read/write", func() {
			So(datastore.Put(ctx, &Build{
				ID: 1,
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status:      pb.Status_SUCCESS,
					CreateTime:  t0pb,
					UpdateTime:  t0pb,
					AncestorIds: []int64{2, 3, 4},
				},
			}), ShouldBeNil)

			b := &Build{
				ID: 1,
			}
			So(datastore.Get(ctx, b), ShouldBeNil)
			p := proto.Clone(b.Proto).(*pb.Build)
			b.Proto = &pb.Build{}
			b.NextBackendSyncTime = ""
			So(b, ShouldResemble, &Build{
				ID:                1,
				Proto:             &pb.Build{},
				BucketID:          "project/bucket",
				BuilderID:         "project/bucket/builder",
				Canary:            false,
				CreateTime:        datastore.RoundTime(t0),
				StatusChangedTime: datastore.RoundTime(t0),
				Experimental:      false,
				Incomplete:        false,
				Status:            pb.Status_SUCCESS,
				Project:           "project",
				LegacyProperties: LegacyProperties{
					Result: Success,
					Status: Completed,
				},
				AncestorIds: []int64{2, 3, 4},
				ParentID:    4,
			})
			So(p, ShouldResembleProto, &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status:      pb.Status_SUCCESS,
				CreateTime:  t0pb,
				UpdateTime:  t0pb,
				AncestorIds: []int64{2, 3, 4},
			})
		})

		Convey("legacy", func() {
			Convey("infra failure", func() {
				So(datastore.Put(ctx, &Build{
					ID: 1,
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Status:     pb.Status_INFRA_FAILURE,
						CreateTime: t0pb,
						UpdateTime: t0pb,
					},
				}), ShouldBeNil)

				b := &Build{
					ID: 1,
				}
				So(datastore.Get(ctx, b), ShouldBeNil)
				p := proto.Clone(b.Proto).(*pb.Build)
				b.Proto = &pb.Build{}
				b.NextBackendSyncTime = ""
				So(b, ShouldResemble, &Build{
					ID:                1,
					Proto:             &pb.Build{},
					BucketID:          "project/bucket",
					BuilderID:         "project/bucket/builder",
					Canary:            false,
					CreateTime:        datastore.RoundTime(t0),
					StatusChangedTime: datastore.RoundTime(t0),
					Experimental:      false,
					Incomplete:        false,
					Status:            pb.Status_INFRA_FAILURE,
					Project:           "project",
					LegacyProperties: LegacyProperties{
						FailureReason: InfraFailure,
						Result:        Failure,
						Status:        Completed,
					},
				})
				So(p, ShouldResembleProto, &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status:     pb.Status_INFRA_FAILURE,
					CreateTime: t0pb,
					UpdateTime: t0pb,
				})
			})

			Convey("timeout", func() {
				So(datastore.Put(ctx, &Build{
					ID: 1,
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Status: pb.Status_INFRA_FAILURE,
						StatusDetails: &pb.StatusDetails{
							Timeout: &pb.StatusDetails_Timeout{},
						},
						CreateTime: t0pb,
						UpdateTime: t0pb,
					},
				}), ShouldBeNil)

				b := &Build{
					ID: 1,
				}
				So(datastore.Get(ctx, b), ShouldBeNil)
				p := proto.Clone(b.Proto).(*pb.Build)
				b.Proto = &pb.Build{}
				b.NextBackendSyncTime = ""
				So(b, ShouldResemble, &Build{
					ID:                1,
					Proto:             &pb.Build{},
					BucketID:          "project/bucket",
					BuilderID:         "project/bucket/builder",
					Canary:            false,
					CreateTime:        datastore.RoundTime(t0),
					StatusChangedTime: datastore.RoundTime(t0),
					Experimental:      false,
					Incomplete:        false,
					Status:            pb.Status_INFRA_FAILURE,
					Project:           "project",
					LegacyProperties: LegacyProperties{
						CancelationReason: TimeoutCanceled,
						Result:            Canceled,
						Status:            Completed,
					},
				})
				So(p, ShouldResembleProto, &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status: pb.Status_INFRA_FAILURE,
					StatusDetails: &pb.StatusDetails{
						Timeout: &pb.StatusDetails_Timeout{},
					},
					CreateTime: t0pb,
					UpdateTime: t0pb,
				})
			})

			Convey("canceled", func() {
				So(datastore.Put(ctx, &Build{
					ID: 1,
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Status:     pb.Status_CANCELED,
						CreateTime: t0pb,
						UpdateTime: t0pb,
					},
				}), ShouldBeNil)

				b := &Build{
					ID: 1,
				}
				So(datastore.Get(ctx, b), ShouldBeNil)
				p := proto.Clone(b.Proto).(*pb.Build)
				b.Proto = &pb.Build{}
				b.NextBackendSyncTime = ""
				So(b, ShouldResemble, &Build{
					ID:                1,
					Proto:             &pb.Build{},
					BucketID:          "project/bucket",
					BuilderID:         "project/bucket/builder",
					Canary:            false,
					CreateTime:        datastore.RoundTime(t0),
					StatusChangedTime: datastore.RoundTime(t0),
					Experimental:      false,
					Incomplete:        false,
					Status:            pb.Status_CANCELED,
					Project:           "project",
					LegacyProperties: LegacyProperties{
						CancelationReason: ExplicitlyCanceled,
						Result:            Canceled,
						Status:            Completed,
					},
				})
				So(p, ShouldResembleProto, &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status:     pb.Status_CANCELED,
					CreateTime: t0pb,
					UpdateTime: t0pb,
				})
			})
		})

		Convey("Realm", func() {
			b := &Build{
				ID: 1,
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
			}
			So(b.Realm(), ShouldEqual, "project:bucket")
		})

		Convey("ToProto", func() {
			b := &Build{
				ID: 1,
				Proto: &pb.Build{
					Id: 1,
				},
				Tags: []string{
					"key1:value1",
					"builder:hidden",
					"key2:value2",
				},
			}
			key := datastore.KeyForObj(ctx, b)
			So(datastore.Put(ctx, &BuildInfra{
				Build: key,
				Proto: &pb.BuildInfra{
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "example.com",
					},
				},
			}), ShouldBeNil)
			So(datastore.Put(ctx, &BuildInputProperties{
				Build: key,
				Proto: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"input": {
							Kind: &structpb.Value_StringValue{
								StringValue: "input value",
							},
						},
					},
				},
			}), ShouldBeNil)

			Convey("mask", func() {
				Convey("include", func() {
					m := HardcodedBuildMask("id")
					p, err := b.ToProto(ctx, m, nil)
					So(err, ShouldBeNil)
					So(p.Id, ShouldEqual, 1)
				})

				Convey("exclude", func() {
					m := HardcodedBuildMask("builder")
					p, err := b.ToProto(ctx, m, nil)
					So(err, ShouldBeNil)
					So(p.Id, ShouldEqual, 0)
				})
			})

			Convey("tags", func() {
				p, err := b.ToProto(ctx, m, nil)
				So(err, ShouldBeNil)
				So(p.Tags, ShouldResembleProto, []*pb.StringPair{
					{
						Key:   "key1",
						Value: "value1",
					},
					{
						Key:   "key2",
						Value: "value2",
					},
				})
				So(b.Proto.Tags, ShouldBeEmpty)
			})

			Convey("infra", func() {
				p, err := b.ToProto(ctx, m, nil)
				So(err, ShouldBeNil)
				So(p.Infra, ShouldResembleProto, &pb.BuildInfra{
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "example.com",
					},
				})
				So(b.Proto.Infra, ShouldBeNil)
			})

			Convey("input properties", func() {
				p, err := b.ToProto(ctx, m, nil)
				So(err, ShouldBeNil)
				So(p.Input.Properties, ShouldResembleProto, mustStruct(map[string]any{
					"input": "input value",
				}))
				So(b.Proto.Input, ShouldBeNil)
			})

			Convey("output properties", func() {
				So(datastore.Put(ctx, &BuildOutputProperties{
					Build: key,
					Proto: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"output": {
								Kind: &structpb.Value_StringValue{
									StringValue: "output value",
								},
							},
						},
					},
				}), ShouldBeNil)
				p, err := b.ToProto(ctx, m, nil)
				So(err, ShouldBeNil)
				So(p.Output.Properties, ShouldResembleProto, mustStruct(map[string]any{
					"output": "output value",
				}))
				So(b.Proto.Output, ShouldBeNil)

				Convey("one missing, one found", func() {
					b1 := &pb.Build{
						Id: 1,
					}
					b2 := &pb.Build{
						Id: 2,
					}
					m := HardcodedBuildMask("output.properties")
					So(LoadBuildDetails(ctx, m, nil, b1, b2), ShouldBeNil)
					So(b1.Output.Properties, ShouldResembleProto, mustStruct(map[string]any{
						"output": "output value",
					}))
					So(b2.Output.GetProperties(), ShouldBeNil)
				})
			})

			Convey("output properties(large)", func() {
				largeProps, err := structpb.NewStruct(map[string]any{})
				So(err, ShouldBeNil)
				k := "laaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaarge_key"
				v := "laaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaarge_value"
				for i := 0; i < 10000; i++ {
					largeProps.Fields[k+strconv.Itoa(i)] = &structpb.Value{
						Kind: &structpb.Value_StringValue{
							StringValue: v,
						},
					}
				}
				outProp := &BuildOutputProperties{
					Build: key,
					Proto: largeProps,
				}
				So(outProp.Put(ctx), ShouldBeNil)

				p, err := b.ToProto(ctx, m, nil)
				So(err, ShouldBeNil)
				So(p.Output.Properties, ShouldResembleProto, largeProps)
				So(b.Proto.Output, ShouldBeNil)
			})

			Convey("steps", func() {
				s, err := proto.Marshal(&pb.Build{
					Steps: []*pb.Step{
						{
							Name: "step",
						},
					},
				})
				So(err, ShouldBeNil)
				So(datastore.Put(ctx, &BuildSteps{
					Build:    key,
					Bytes:    s,
					IsZipped: false,
				}), ShouldBeNil)
				p, err := b.ToProto(ctx, m, nil)
				So(err, ShouldBeNil)
				So(p.Steps, ShouldResembleProto, []*pb.Step{
					{
						Name: "step",
					},
				})
				So(b.Proto.Steps, ShouldBeEmpty)
			})
		})

		Convey("ToSimpleBuildProto", func() {
			b := &Build{
				ID: 1,
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Tags: []*pb.StringPair{
						{
							Key:   "k1",
							Value: "v1",
						},
					},
				},
				Project:   "project",
				BucketID:  "project/bucket",
				BuilderID: "project/bucket/builder",
				Tags: []string{
					"k1:v1",
				},
			}

			actual := b.ToSimpleBuildProto(ctx)
			So(actual, ShouldResembleProto, &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Tags: []*pb.StringPair{
					{
						Key:   "k1",
						Value: "v1",
					},
				},
			})
		})

		Convey("ExperimentsString", func() {
			b := &Build{}
			check := func(exps []string, enabled string) {
				b.Experiments = exps
				So(b.ExperimentsString(), ShouldEqual, enabled)
			}

			Convey("Returns None", func() {
				check([]string{}, "None")
			})

			Convey("Sorted", func() {
				exps := []string{"+exp4", "-exp3", "+exp1", "-exp10"}
				check(exps, "exp1|exp4")
			})
		})

		Convey("NextBackendSyncTime", func() {
			b := &Build{
				ID:      1,
				Project: "project",
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status:      pb.Status_STARTED,
					CreateTime:  t0pb,
					UpdateTime:  t0pb,
					AncestorIds: []int64{2, 3, 4},
				},
				BackendTarget: "backend",
			}
			b.GenerateNextBackendSyncTime(ctx, 1)
			So(datastore.Put(ctx, b), ShouldBeNil)

			// First save.
			So(datastore.Get(ctx, b), ShouldBeNil)
			ut0 := b.NextBackendSyncTime
			parts := strings.Split(ut0, syncTimeSep)
			So(parts, ShouldHaveLength, 4)
			So(parts[3], ShouldEqual, fmt.Sprint(t0.Add(b.BackendSyncInterval).Truncate(time.Minute).Unix()))
			So(ut0, ShouldEqual, "backend--project--0--1454472600")

			// update soon after, NextBackendSyncTime unchanged.
			b.Proto.UpdateTime = timestamppb.New(t0.Add(time.Second))
			So(datastore.Put(ctx, b), ShouldBeNil)
			So(datastore.Get(ctx, b), ShouldBeNil)
			So(b.NextBackendSyncTime, ShouldEqual, ut0)
			So(b.BackendSyncInterval, ShouldEqual, defaultBuildSyncInterval)

			// update after 30sec, NextBackendSyncTime unchanged.
			b.Proto.UpdateTime = timestamppb.New(t0.Add(40 * time.Second))
			So(datastore.Put(ctx, b), ShouldBeNil)
			So(datastore.Get(ctx, b), ShouldBeNil)
			So(b.NextBackendSyncTime, ShouldBeGreaterThan, ut0)

			// update after 2min, NextBackendSyncTime changed.
			t1 := t0.Add(2 * time.Minute)
			b.Proto.UpdateTime = timestamppb.New(t1)
			So(datastore.Put(ctx, b), ShouldBeNil)
			So(datastore.Get(ctx, b), ShouldBeNil)
			So(b.NextBackendSyncTime, ShouldBeGreaterThan, ut0)
			parts = strings.Split(b.NextBackendSyncTime, syncTimeSep)
			So(parts, ShouldHaveLength, 4)
			So(parts[3], ShouldEqual, fmt.Sprint(t1.Add(b.BackendSyncInterval).Truncate(time.Minute).Unix()))
		})
	})
}
