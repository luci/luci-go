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

package bbfake

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/buildbucket/appengine/model"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGetBuild(t *testing.T) {
	Convey("GetBuild", t, func() {
		fake := &Fake{}
		ctx := context.Background()
		const (
			bbHost   = "buildbucket.example.com"
			lProject = "testProj"
			buildID  = 12344
		)

		client, err := fake.NewClientFactory().MakeClient(ctx, bbHost, lProject)
		So(err, ShouldBeNil)

		epoch := clock.Now(ctx)
		builderID := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "testBucket",
			Builder: "testBuilder",
		}
		build := NewBuildConstructor().
			WithID(buildID).
			WithHost(bbHost).
			WithBuilderID(builderID).
			WithStatus(bbpb.Status_SUCCESS).
			WithCreateTime(epoch).
			WithStartTime(epoch.Add(1 * time.Minute)).
			WithEndTime(epoch.Add(2 * time.Minute)).
			Construct()
		fake.AddBuild(build)

		Convey("Can get", func() {
			Convey("Without mask", func() {
				expected := proto.Clone(build).(*bbpb.Build)
				res, err := client.GetBuild(ctx, &bbpb.GetBuildRequest{
					Id: buildID,
				})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, trimmedBuildWithDefaultMask(expected))
			})
			Convey("With mask", func() {
				res, err := client.GetBuild(ctx, &bbpb.GetBuildRequest{
					Id: buildID,
					Mask: &bbpb.BuildMask{
						Fields: &fieldmaskpb.FieldMask{
							Paths: []string{"id", "builder"},
						},
					},
				})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &bbpb.Build{
					Id:      buildID,
					Builder: builderID,
				})
			})
		})

		Convey("No ACL", func() {
			client, err := fake.NewClientFactory().MakeClient(ctx, bbHost, "anotherProj")
			So(err, ShouldBeNil)
			res, err := client.GetBuild(ctx, &bbpb.GetBuildRequest{
				Id: buildID,
			})
			So(err, ShouldBeRPCNotFound)
			So(res, ShouldBeNil)
		})
		Convey("Build not exist", func() {
			res, err := client.GetBuild(ctx, &bbpb.GetBuildRequest{
				Id: 777,
			})
			So(err, ShouldBeRPCNotFound)
			So(res, ShouldBeNil)
		})

		Convey("Invalid input", func() {
			Convey("Zero build id", func() {
				res, err := client.GetBuild(ctx, &bbpb.GetBuildRequest{
					Id: 0,
				})
				So(err, ShouldBeRPCInvalidArgument)
				So(res, ShouldBeNil)
			})
			Convey("Builder + build number", func() {
				res, err := client.GetBuild(ctx, &bbpb.GetBuildRequest{
					Id:      buildID,
					Builder: builderID,
				})
				So(err, ShouldHaveRPCCode, codes.Unimplemented)
				So(res, ShouldBeNil)
			})
		})
	})
}

func TestSearchBuild(t *testing.T) {
	Convey("Search", t, func() {
		fake := &Fake{}
		ctx := context.Background()
		const (
			bbHost   = "buildbucket.example.com"
			lProject = "testProj"
		)

		client, err := fake.NewClientFactory().MakeClient(ctx, bbHost, lProject)
		So(err, ShouldBeNil)

		const (
			buildID          = 12344
			gHost            = "example-review.googlesource.com"
			gRepo            = "repo/example"
			gChangeNum       = 546
			gPatchset        = 6
			gMinEquiPatchset = 2
		)
		epoch := clock.Now(ctx)
		builderID := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "testBucket",
			Builder: "testBuilder",
		}
		gc := &bbpb.GerritChange{
			Host:     gHost,
			Project:  gRepo,
			Change:   gChangeNum,
			Patchset: gPatchset,
		}
		build := NewBuildConstructor().
			WithID(buildID).
			WithHost(bbHost).
			WithBuilderID(builderID).
			WithStatus(bbpb.Status_SUCCESS).
			WithCreateTime(epoch).
			WithStartTime(epoch.Add(1 * time.Minute)).
			WithEndTime(epoch.Add(2 * time.Minute)).
			AppendGerritChanges(gc).
			Construct()

		Convey("Empty Predicate", func() {
			fake.AddBuild(build)
			res, err := client.SearchBuilds(ctx, &bbpb.SearchBuildsRequest{})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &bbpb.SearchBuildsResponse{
				Builds: []*bbpb.Build{trimmedBuildWithDefaultMask(build)},
			})
		})

		Convey("Error on not allowed predicate", func() {
			_, err := client.SearchBuilds(ctx, &bbpb.SearchBuildsRequest{
				Predicate: &bbpb.BuildPredicate{
					CreatedBy: "foo",
				},
			})
			So(err, ShouldBeRPCInvalidArgument)
		})

		Convey("Can apply Gerrit Change predicate", func() {
			fake.AddBuild(build)
			Convey("Match", func() {
				res, err := client.SearchBuilds(ctx, &bbpb.SearchBuildsRequest{
					Predicate: &bbpb.BuildPredicate{
						GerritChanges: []*bbpb.GerritChange{gc},
					},
				})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &bbpb.SearchBuildsResponse{
					Builds: []*bbpb.Build{trimmedBuildWithDefaultMask(build)},
				})
			})
			Convey("Mismatch change", func() {
				gc := proto.Clone(gc).(*bbpb.GerritChange)
				gc.Change += 2
				res, err := client.SearchBuilds(ctx, &bbpb.SearchBuildsRequest{
					Predicate: &bbpb.BuildPredicate{
						GerritChanges: []*bbpb.GerritChange{gc},
					},
				})
				So(err, ShouldBeNil)
				So(res.GetBuilds(), ShouldBeEmpty)
				So(res.GetNextPageToken(), ShouldBeEmpty)
			})
			Convey("Extra change", func() {
				anotherGC := proto.Clone(gc).(*bbpb.GerritChange)
				anotherGC.Change += 2
				res, err := client.SearchBuilds(ctx, &bbpb.SearchBuildsRequest{
					Predicate: &bbpb.BuildPredicate{
						GerritChanges: []*bbpb.GerritChange{gc, anotherGC},
					},
				})
				So(err, ShouldBeNil)
				So(res.GetBuilds(), ShouldBeEmpty)
				So(res.GetNextPageToken(), ShouldBeEmpty)
			})
		})

		Convey("Can apply experimental predicate", func() {
			build := NewConstructorFromBuild(build).
				WithExperimental(true).
				Construct()
			fake.AddBuild(build)
			Convey("Include experimental", func() {
				res, err := client.SearchBuilds(ctx, &bbpb.SearchBuildsRequest{
					Predicate: &bbpb.BuildPredicate{
						IncludeExperimental: true,
					},
				})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &bbpb.SearchBuildsResponse{
					Builds: []*bbpb.Build{trimmedBuildWithDefaultMask(build)},
				})
			})
			Convey("Not include experimental", func() {
				res, err := client.SearchBuilds(ctx, &bbpb.SearchBuildsRequest{
					Predicate: &bbpb.BuildPredicate{},
				})
				So(err, ShouldBeNil)
				So(res.GetBuilds(), ShouldBeEmpty)
				So(res.GetNextPageToken(), ShouldBeEmpty)
			})
		})

		Convey("Different host", func() {
			build := NewConstructorFromBuild(build).
				WithHost("another-bb.example.com").
				Construct()
			fake.AddBuild(build)
			res, err := client.SearchBuilds(ctx, &bbpb.SearchBuildsRequest{})
			So(err, ShouldBeNil)
			So(res.GetBuilds(), ShouldBeEmpty)
			So(res.GetNextPageToken(), ShouldBeEmpty)
		})

		Convey("Different project", func() {
			build := NewConstructorFromBuild(build).
				WithBuilderID(&bbpb.BuilderID{
					Project: "anotherProj",
					Bucket:  "someBucket",
					Builder: "someBuilder",
				}).
				Construct()
			fake.AddBuild(build)
			res, err := client.SearchBuilds(ctx, &bbpb.SearchBuildsRequest{})
			So(err, ShouldBeNil)
			So(res.GetBuilds(), ShouldBeEmpty)
			So(res.GetNextPageToken(), ShouldBeEmpty)
		})

		Convey("Apply customized mask", func() {
			fake.AddBuild(build)
			fields, err := fieldmaskpb.New(build, "id")
			So(err, ShouldBeNil)
			res, err := client.SearchBuilds(ctx, &bbpb.SearchBuildsRequest{
				Mask: &bbpb.BuildMask{
					Fields: fields,
				},
			})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &bbpb.SearchBuildsResponse{
				Builds: []*bbpb.Build{{Id: build.Id}},
			})
		})

		Convey("Paging", func() {
			for i := 1; i <= 50; i++ {
				build := NewConstructorFromBuild(build).WithID(int64(i)).Construct()
				fake.AddBuild(build)
			}

			for _, pageSize := range []int{0, 20} { // 0 means  default size
				Convey(fmt.Sprintf("With size %d", pageSize), func() {
					actualPageSize := pageSize
					if actualPageSize == 0 {
						actualPageSize = defaultSearchPageSize
					}
					req := &bbpb.SearchBuildsRequest{
						PageSize: int32(pageSize),
					}
					expectedID := int64(1)
					for {
						res, err := client.SearchBuilds(ctx, req)
						So(err, ShouldBeNil)
						So(len(res.GetBuilds()), ShouldBeLessThanOrEqualTo, actualPageSize)
						for _, b := range res.GetBuilds() {
							So(b.Id, ShouldEqual, expectedID)
							expectedID++
						}
						if res.NextPageToken == "" {
							break
						}
						req.PageToken = res.NextPageToken
					}
				})
			}
		})
	})
}

func TestCancelBuild(t *testing.T) {
	Convey("CancelBuild", t, func() {
		fake := &Fake{}
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		const (
			bbHost   = "buildbucket.example.com"
			lProject = "testProj"
			buildID  = 12344
		)

		client, err := fake.NewClientFactory().MakeClient(ctx, bbHost, lProject)
		So(err, ShouldBeNil)

		epoch := tc.Now()
		tc.Add(1 * time.Minute)
		builderID := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "testBucket",
			Builder: "testBuilder",
		}
		build := NewBuildConstructor().
			WithID(buildID).
			WithHost(bbHost).
			WithBuilderID(builderID).
			WithStatus(bbpb.Status_SCHEDULED).
			WithCreateTime(epoch).
			Construct()
		fake.AddBuild(build)

		Convey("Can cancel", func() {
			Convey("Without mask", func() {
				res, err := client.CancelBuild(ctx, &bbpb.CancelBuildRequest{
					Id:              buildID,
					SummaryMarkdown: "no longer needed",
				})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, trimmedBuildWithDefaultMask(NewBuildConstructor().
					WithID(buildID).
					WithHost(bbHost).
					WithBuilderID(builderID).
					WithStatus(bbpb.Status_CANCELED).
					WithCreateTime(epoch).
					WithStartTime(tc.Now()).
					WithEndTime(tc.Now()).
					WithSummaryMarkdown("no longer needed").
					Construct()))
			})
			Convey("With mask", func() {
				res, err := client.CancelBuild(ctx, &bbpb.CancelBuildRequest{
					Id:              buildID,
					SummaryMarkdown: "no longer needed",
					Mask: &bbpb.BuildMask{
						Fields: &fieldmaskpb.FieldMask{
							Paths: []string{"id", "status"},
						},
					},
				})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &bbpb.Build{
					Id:     buildID,
					Status: bbpb.Status_CANCELED,
				})
			})
		})

		Convey("No ACL", func() {
			client, err := fake.NewClientFactory().MakeClient(ctx, bbHost, "anotherProj")
			So(err, ShouldBeNil)
			res, err := client.CancelBuild(ctx, &bbpb.CancelBuildRequest{
				Id:              buildID,
				SummaryMarkdown: "no longer needed",
			})
			So(err, ShouldBeRPCNotFound)
			So(res, ShouldBeNil)
		})

		Convey("Build not exist", func() {
			res, err := client.CancelBuild(ctx, &bbpb.CancelBuildRequest{
				Id: 777,
			})
			So(err, ShouldBeRPCNotFound)
			So(res, ShouldBeNil)
		})

		Convey("Noop for build end with", func() {
			for i, status := range []bbpb.Status{
				bbpb.Status_SUCCESS,
				bbpb.Status_FAILURE,
				bbpb.Status_INFRA_FAILURE,
				bbpb.Status_CANCELED,
			} {
				Convey(status.String(), func() {
					endAt := tc.Now()
					id := buildID + int64(i)
					build = NewConstructorFromBuild(build).
						WithID(id).
						WithStatus(status).
						WithSummaryMarkdown("ended already").
						WithStartTime(endAt.Add(-10 * time.Second)).
						WithEndTime(endAt).
						Construct()
					fake.AddBuild(build)
					tc.Add(2 * time.Minute)
					res, err := client.CancelBuild(ctx, &bbpb.CancelBuildRequest{
						Id:              id,
						SummaryMarkdown: "no longer needed",
						Mask: &bbpb.BuildMask{
							Fields: &fieldmaskpb.FieldMask{
								Paths: []string{"id", "status", "summary_markdown", "end_time"},
							},
						},
					})
					So(err, ShouldBeNil)
					So(res, ShouldResembleProto, &bbpb.Build{
						Id:              id,
						Status:          status,
						EndTime:         timestamppb.New(endAt),
						SummaryMarkdown: "ended already",
					})

				})
			}
		})
	})
}

func TestScheduleBuild(t *testing.T) {
	Convey("ScheduleBuild", t, func() {
		fake := &Fake{}
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		const (
			bbHost   = "buildbucket.example.com"
			lProject = "testProj"
		)
		builderNoProp := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "testBucket",
			Builder: "builderNoProp",
		}
		builderWithProp := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "testBucket",
			Builder: "builderWithProp",
		}
		fake.AddBuilder(bbHost, builderNoProp, nil)
		fake.AddBuilder(bbHost, builderWithProp,
			map[string]interface{}{"foo": "bar"},
		)

		c, err := fake.NewClientFactory().MakeClient(ctx, bbHost, lProject)
		So(err, ShouldBeNil)
		client := c.(*Client)

		Convey("Can schedule", func() {
			Convey("Simple", func() {
				res, err := client.scheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
					Builder: builderNoProp,
				})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, trimmedBuildWithDefaultMask(&bbpb.Build{
					Id:         res.Id,
					Builder:    builderNoProp,
					Status:     bbpb.Status_SCHEDULED,
					CreateTime: timestamppb.New(tc.Now()),
					Input:      &bbpb.Build_Input{},
					Infra: &bbpb.BuildInfra{
						Buildbucket: &bbpb.BuildInfra_Buildbucket{
							Hostname: bbHost,
						},
					},
				}))
			})
			Convey("With additional args", func() {
				props, err := structpb.NewStruct(map[string]interface{}{
					"coolKey": "coolVal",
				})
				So(err, ShouldBeNil)
				gc := &bbpb.GerritChange{
					Host:     "example-review.com",
					Project:  "testProj",
					Change:   1,
					Patchset: 2,
				}
				res, err := client.scheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
					Builder:       builderNoProp,
					Properties:    props,
					GerritChanges: []*bbpb.GerritChange{gc},
					Tags: []*bbpb.StringPair{
						{Key: "tagKey", Value: "tagValue"},
					},
				})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, trimmedBuildWithDefaultMask(&bbpb.Build{
					Id:         res.Id,
					Builder:    builderNoProp,
					Status:     bbpb.Status_SCHEDULED,
					CreateTime: timestamppb.New(tc.Now()),
					Input: &bbpb.Build_Input{
						Properties:    props,
						GerritChanges: []*bbpb.GerritChange{gc},
					},
					Infra: &bbpb.BuildInfra{
						Buildbucket: &bbpb.BuildInfra_Buildbucket{
							RequestedProperties: props,
							Hostname:            bbHost,
						},
					},
					Tags: []*bbpb.StringPair{
						{Key: "tagKey", Value: "tagValue"},
					},
				}))
			})
			Convey("Override builder properties", func() {
				props, err := structpb.NewStruct(map[string]interface{}{
					"foo":  "not_bar",
					"cool": "awesome",
				})
				So(err, ShouldBeNil)
				res, err := client.scheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
					Builder:    builderWithProp,
					Properties: props,
				})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, trimmedBuildWithDefaultMask(&bbpb.Build{
					Id:         res.Id,
					Builder:    builderWithProp,
					Status:     bbpb.Status_SCHEDULED,
					CreateTime: timestamppb.New(tc.Now()),
					Input: &bbpb.Build_Input{
						Properties: props,
					},
					Infra: &bbpb.BuildInfra{
						Buildbucket: &bbpb.BuildInfra_Buildbucket{
							RequestedProperties: props,
							Hostname:            bbHost,
						},
					},
				}))
			})
			Convey("With mask", func() {
				res, err := client.scheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
					Builder: builderNoProp,
					Mask: &bbpb.BuildMask{
						Fields: &fieldmaskpb.FieldMask{
							Paths: []string{"id", "status"},
						},
					},
				})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &bbpb.Build{
					Id:     res.Id,
					Status: bbpb.Status_SCHEDULED,
				})
			})
			Convey("Decreasing build ID", func() {
				const N = 10
				var prevBuildID int64 = math.MaxInt64
				for i := 0; i < N; i++ {
					builder := builderNoProp
					if i%2 == 1 {
						builder = builderWithProp
					}
					res, err := client.scheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
						Builder: builder,
					})
					So(err, ShouldBeNil)
					So(res.Id, ShouldBeLessThan, prevBuildID)
					prevBuildID = res.Id
				}
			})
		})

		Convey("Builder not found", func() {
			res, err := client.scheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
				Builder: &bbpb.BuilderID{
					Project: lProject,
					Bucket:  "testBucket",
					Builder: "non-existent-builder",
				},
			})
			So(err, ShouldBeRPCNotFound)
			So(res, ShouldBeNil)
		})
	})
}

func TestBatch(t *testing.T) {
	Convey("Batch", t, func() {
		fake := &Fake{}
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		const (
			bbHost   = "buildbucket.example.com"
			lProject = "testProj"
		)

		client, err := fake.NewClientFactory().MakeClient(ctx, bbHost, lProject)
		So(err, ShouldBeNil)

		epoch := tc.Now()
		tc.Add(1 * time.Minute)
		buildFoo := NewBuildConstructor().
			WithID(986).
			WithHost(bbHost).
			WithBuilderID(&bbpb.BuilderID{
				Project: lProject,
				Bucket:  "testBucket",
				Builder: "builderFoo",
			}).
			WithStatus(bbpb.Status_SCHEDULED).
			WithCreateTime(epoch).
			Construct()
		buildBar := NewBuildConstructor().
			WithID(867).
			WithHost(bbHost).
			WithBuilderID(&bbpb.BuilderID{
				Project: lProject,
				Bucket:  "testBucket",
				Builder: "builderFoo",
			}).
			WithStatus(bbpb.Status_SUCCESS).
			WithCreateTime(epoch).
			WithStartTime(epoch.Add(30 * time.Second)).
			WithEndTime(epoch.Add(1 * time.Minute)).
			Construct()
		builderBaz := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "testBucket",
			Builder: "baz",
		}
		fake.AddBuilder(bbHost, builderBaz, nil)

		Convey("Batch succeeds", func() {
			fake.AddBuild(buildFoo)
			fake.AddBuild(buildBar)
			res, err := client.Batch(ctx, &bbpb.BatchRequest{
				Requests: []*bbpb.BatchRequest_Request{
					{
						Request: &bbpb.BatchRequest_Request_CancelBuild{
							CancelBuild: &bbpb.CancelBuildRequest{
								Id:              buildFoo.GetId(),
								SummaryMarkdown: "no longer needed",
								Mask: &bbpb.BuildMask{
									Fields: &fieldmaskpb.FieldMask{
										Paths: []string{"id", "status", "summary_markdown"},
									},
								},
							},
						},
					},
					{
						Request: &bbpb.BatchRequest_Request_GetBuild{
							GetBuild: &bbpb.GetBuildRequest{
								Id: buildBar.GetId(),
							},
						},
					},
					{
						Request: &bbpb.BatchRequest_Request_ScheduleBuild{
							ScheduleBuild: &bbpb.ScheduleBuildRequest{
								Builder: builderBaz,
								Mask: &bbpb.BuildMask{
									Fields: &fieldmaskpb.FieldMask{
										Paths: []string{"id", "builder", "status"},
									},
								},
							},
						},
					},
				},
			})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &bbpb.BatchResponse{
				Responses: []*bbpb.BatchResponse_Response{
					{
						Response: &bbpb.BatchResponse_Response_CancelBuild{
							CancelBuild: &bbpb.Build{
								Id:              buildFoo.Id,
								Status:          bbpb.Status_CANCELED,
								SummaryMarkdown: "no longer needed",
							},
						},
					},
					{
						Response: &bbpb.BatchResponse_Response_GetBuild{
							GetBuild: trimmedBuildWithDefaultMask(buildBar),
						},
					},
					{
						Response: &bbpb.BatchResponse_Response_ScheduleBuild{
							ScheduleBuild: &bbpb.Build{
								Id:      math.MaxInt64 - 1,
								Builder: builderBaz,
								Status:  bbpb.Status_SCHEDULED,
							},
						},
					},
				},
			})
		})

		Convey("Batch fails", func() {
			fake.AddBuild(buildFoo)
			// buildBar does not exist
			res, err := client.Batch(ctx, &bbpb.BatchRequest{
				Requests: []*bbpb.BatchRequest_Request{
					{
						Request: &bbpb.BatchRequest_Request_CancelBuild{
							CancelBuild: &bbpb.CancelBuildRequest{
								Id:              buildFoo.GetId(),
								SummaryMarkdown: "no longer needed",
								Mask: &bbpb.BuildMask{
									Fields: &fieldmaskpb.FieldMask{
										Paths: []string{"id", "status", "summary_markdown"},
									},
								},
							},
						},
					},
					{
						Request: &bbpb.BatchRequest_Request_GetBuild{
							GetBuild: &bbpb.GetBuildRequest{
								Id: buildBar.GetId(),
							},
						},
					},
				},
			})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &bbpb.BatchResponse{
				Responses: []*bbpb.BatchResponse_Response{
					{
						Response: &bbpb.BatchResponse_Response_CancelBuild{
							CancelBuild: &bbpb.Build{
								Id:              buildFoo.Id,
								Status:          bbpb.Status_CANCELED,
								SummaryMarkdown: "no longer needed",
							},
						},
					},
					{
						Response: &bbpb.BatchResponse_Response_Error{
							Error: status.Newf(codes.NotFound, "requested resource not found or \"project:%s\" does not have permission to view it", lProject).Proto(),
						},
					},
				},
			})
		})
	})
}

func trimmedBuildWithDefaultMask(b *bbpb.Build) *bbpb.Build {
	mask, err := model.NewBuildMask("", nil, nil)
	So(err, ShouldBeNil)
	ret := proto.Clone(b).(*bbpb.Build)
	So(mask.Trim(ret), ShouldBeNil)
	return ret
}
