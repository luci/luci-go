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
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	bb "go.chromium.org/luci/buildbucket"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestBuild(t *testing.T) {
	// Non-parallel due to manipulation of GlobalBuildUpdateTimeClock
	// t.Parallel()

	Convey("Build", t, func() {
		ctx := memory.Use(context.Background())
		ctx, tclock := testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		defer OverrideGlobalBuildUpdateTimeClock(tclock)()

		t0pb := timestamppb.New(tclock.Now())

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
					Status: pb.Status_SUCCESS,
				},
				CreateTime: testclock.TestRecentTimeUTC,
			}), ShouldBeNil)

			b := &Build{
				ID: 1,
			}
			So(datastore.Get(ctx, b), ShouldBeNil)
			p := proto.Clone(b.Proto).(*pb.Build)
			b.Proto = &pb.Build{}
			So(b, ShouldResemble, &Build{
				ID:                1,
				Proto:             &pb.Build{},
				BucketID:          "project/bucket",
				BuilderID:         "project/bucket/builder",
				Canary:            false,
				CreateTime:        datastore.RoundTime(testclock.TestRecentTimeUTC),
				StatusChangedTime: datastore.RoundTime(testclock.TestRecentTimeUTC),
				Experimental:      false,
				Incomplete:        false,
				Status:            pb.Status_SUCCESS,
				Project:           "project",
				LegacyProperties: LegacyProperties{
					Result: Success,
					Status: Completed,
				},
			})
			So(p, ShouldResembleProto, &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status:     pb.Status_SUCCESS,
				UpdateTime: t0pb,
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
						Status: pb.Status_INFRA_FAILURE,
					},
					CreateTime: testclock.TestRecentTimeUTC,
				}), ShouldBeNil)

				b := &Build{
					ID: 1,
				}
				So(datastore.Get(ctx, b), ShouldBeNil)
				p := proto.Clone(b.Proto).(*pb.Build)
				b.Proto = &pb.Build{}
				So(b, ShouldResemble, &Build{
					ID:                1,
					Proto:             &pb.Build{},
					BucketID:          "project/bucket",
					BuilderID:         "project/bucket/builder",
					Canary:            false,
					CreateTime:        datastore.RoundTime(testclock.TestRecentTimeUTC),
					StatusChangedTime: datastore.RoundTime(testclock.TestRecentTimeUTC),
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
					},
					CreateTime: testclock.TestRecentTimeUTC,
				}), ShouldBeNil)

				b := &Build{
					ID: 1,
				}
				So(datastore.Get(ctx, b), ShouldBeNil)
				p := proto.Clone(b.Proto).(*pb.Build)
				b.Proto = &pb.Build{}
				So(b, ShouldResemble, &Build{
					ID:                1,
					Proto:             &pb.Build{},
					BucketID:          "project/bucket",
					BuilderID:         "project/bucket/builder",
					Canary:            false,
					CreateTime:        datastore.RoundTime(testclock.TestRecentTimeUTC),
					StatusChangedTime: datastore.RoundTime(testclock.TestRecentTimeUTC),
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
						Status: pb.Status_CANCELED,
					},
					CreateTime: testclock.TestRecentTimeUTC,
				}), ShouldBeNil)

				b := &Build{
					ID: 1,
				}
				So(datastore.Get(ctx, b), ShouldBeNil)
				p := proto.Clone(b.Proto).(*pb.Build)
				b.Proto = &pb.Build{}
				So(b, ShouldResemble, &Build{
					ID:                1,
					Proto:             &pb.Build{},
					BucketID:          "project/bucket",
					BuilderID:         "project/bucket/builder",
					Canary:            false,
					CreateTime:        datastore.RoundTime(testclock.TestRecentTimeUTC),
					StatusChangedTime: datastore.RoundTime(testclock.TestRecentTimeUTC),
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
					UpdateTime: t0pb,
				})
			})
		})

		Convey("Realm", func() {
			Convey("implicitly disabled", func() {
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
				So(b.Realm(), ShouldEqual, "")
			})

			Convey("explicitly disabled", func() {
				b := &Build{
					ID: 1,
					Experiments: []string{
						"-" + bb.ExperimentUseRealms,
					},
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					},
				}
				So(b.Realm(), ShouldEqual, "")
			})

			Convey("enabled", func() {
				b := &Build{
					ID: 1,
					Experiments: []string{
						"+enabled",
						"+" + bb.ExperimentUseRealms,
						"-disabled",
					},
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
				Proto: DSStruct{
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

			Convey("mask", func() {
				Convey("include", func() {
					m := HardcodedBuildMask("id")
					p, err := b.ToProto(ctx, m)
					So(err, ShouldBeNil)
					So(p.Id, ShouldEqual, 1)
				})

				Convey("exclude", func() {
					m := HardcodedBuildMask("builder")
					p, err := b.ToProto(ctx, m)
					So(err, ShouldBeNil)
					So(p.Id, ShouldEqual, 0)
				})
			})

			Convey("tags", func() {
				p, err := b.ToProto(ctx, m)
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
				p, err := b.ToProto(ctx, m)
				So(err, ShouldBeNil)
				So(p.Infra, ShouldResembleProto, &pb.BuildInfra{
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "example.com",
					},
				})
				So(b.Proto.Infra, ShouldBeNil)
			})

			Convey("input properties", func() {
				p, err := b.ToProto(ctx, m)
				So(err, ShouldBeNil)
				So(p.Input.Properties, ShouldResembleProtoJSON, `{"input": "input value"}`)
				So(b.Proto.Input, ShouldBeNil)
			})

			Convey("output properties", func() {
				So(datastore.Put(ctx, &BuildOutputProperties{
					Build: key,
					Proto: DSStruct{
						Struct: structpb.Struct{
							Fields: map[string]*structpb.Value{
								"output": {
									Kind: &structpb.Value_StringValue{
										StringValue: "output value",
									},
								},
							},
						},
					},
				}), ShouldBeNil)
				p, err := b.ToProto(ctx, m)
				So(err, ShouldBeNil)
				So(p.Output.Properties, ShouldResembleProtoJSON, `{"output": "output value"}`)
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
				p, err := b.ToProto(ctx, m)
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
	})
}
