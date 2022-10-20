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
		)
		client := fake.MustNewClient(ctx, bbHost, lProject)

		builderID := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "testBucket",
			Builder: "testBuilder",
		}

		fake.AddBuilder(bbHost, builderID, nil)
		build, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
			Builder: builderID,
		})
		So(err, ShouldBeNil)

		Convey("Can get", func() {
			Convey("Without mask", func() {
				expected := proto.Clone(build).(*bbpb.Build)
				res, err := client.GetBuild(ctx, &bbpb.GetBuildRequest{
					Id: build.GetId(),
				})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, trimmedBuildWithDefaultMask(expected))
			})
			Convey("With mask", func() {
				res, err := client.GetBuild(ctx, &bbpb.GetBuildRequest{
					Id: build.GetId(),
					Mask: &bbpb.BuildMask{
						Fields: &fieldmaskpb.FieldMask{
							Paths: []string{"id", "builder"},
						},
					},
				})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &bbpb.Build{
					Id:      build.GetId(),
					Builder: builderID,
				})
			})
		})

		Convey("No ACL", func() {
			client := fake.MustNewClient(ctx, bbHost, "anotherProj")
			res, err := client.GetBuild(ctx, &bbpb.GetBuildRequest{
				Id: build.GetId(),
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
					Id:      build.GetId(),
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
		client := fake.MustNewClient(ctx, bbHost, lProject)

		const (
			gHost            = "example-review.googlesource.com"
			gRepo            = "repo/example"
			gChangeNum       = 546
			gPatchset        = 6
			gMinEquiPatchset = 2
		)
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

		fake.AddBuilder(bbHost, builderID, nil)
		build, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
			Builder:       builderID,
			GerritChanges: []*bbpb.GerritChange{gc},
		})
		So(err, ShouldBeNil)

		Convey("Empty Predicate", func() {
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
			build = fake.MutateBuild(ctx, bbHost, build.GetId(), func(b *bbpb.Build) {
				if b.Input == nil {
					b.Input = &bbpb.Build_Input{}
				}
				b.Input.Experimental = true
				b.Input.Experiments = append(b.Input.Experiments, "luci.non_production")
			})

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
			client = fake.MustNewClient(ctx, "another-bb.example.com", lProject)
			res, err := client.SearchBuilds(ctx, &bbpb.SearchBuildsRequest{})
			So(err, ShouldBeNil)
			So(res.GetBuilds(), ShouldBeEmpty)
			So(res.GetNextPageToken(), ShouldBeEmpty)
		})

		Convey("Different project", func() {
			anotherBuilder := &bbpb.BuilderID{
				Project: "anotherProj",
				Bucket:  "someBucket",
				Builder: "someBuilder",
			}
			fake.AddBuilder(bbHost, anotherBuilder, nil)
			_, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
				Builder:       anotherBuilder,
				GerritChanges: []*bbpb.GerritChange{gc},
			})
			So(err, ShouldBeNil)

			res, err := client.SearchBuilds(ctx, &bbpb.SearchBuildsRequest{})
			So(err, ShouldBeNil)
			So(res.GetBuilds(), ShouldHaveLength, 1)
			So(res.GetNextPageToken(), ShouldBeEmpty)
		})

		Convey("Apply customized mask", func() {
			fields, err := fieldmaskpb.New(build, "id")
			So(err, ShouldBeNil)
			res, err := client.SearchBuilds(ctx, &bbpb.SearchBuildsRequest{
				Mask: &bbpb.BuildMask{
					Fields: fields,
				},
			})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &bbpb.SearchBuildsResponse{
				Builds: []*bbpb.Build{{Id: build.GetId()}},
			})
		})

		Convey("Paging", func() {
			const totalBuilds = 50
			// there's already one build in the store.
			for i := 1; i < totalBuilds; i++ {
				_, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
					Builder:       builderID,
					GerritChanges: []*bbpb.GerritChange{gc},
				})
				So(err, ShouldBeNil)
			}

			for _, pageSize := range []int{0, 20} { // 0 means  default size
				buildCnt := 0
				Convey(fmt.Sprintf("With size %d", pageSize), func() {
					actualPageSize := pageSize
					if actualPageSize == 0 {
						actualPageSize = defaultSearchPageSize
					}
					req := &bbpb.SearchBuildsRequest{
						PageSize: int32(pageSize),
					}
					for {
						res, err := client.SearchBuilds(ctx, req)
						So(err, ShouldBeNil)
						So(len(res.GetBuilds()), ShouldBeLessThanOrEqualTo, actualPageSize)
						buildCnt += len(res.GetBuilds())
						if res.NextPageToken == "" {
							break
						}
						req.PageToken = res.NextPageToken
					}
					So(buildCnt, ShouldEqual, totalBuilds)
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
		)
		client := fake.MustNewClient(ctx, bbHost, lProject)

		tc.Add(1 * time.Minute)
		builderID := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "testBucket",
			Builder: "testBuilder",
		}
		fake.AddBuilder(bbHost, builderID, nil)
		build, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
			Builder: builderID,
		})
		So(err, ShouldBeNil)

		Convey("Can cancel", func() {
			Convey("Without mask", func() {
				res, err := client.CancelBuild(ctx, &bbpb.CancelBuildRequest{
					Id:              build.GetId(),
					SummaryMarkdown: "no longer needed",
				})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, trimmedBuildWithDefaultMask(NewBuildConstructor().
					WithID(build.GetId()).
					WithHost(bbHost).
					WithBuilderID(builderID).
					WithStatus(bbpb.Status_CANCELED).
					WithCreateTime(tc.Now()).
					WithStartTime(tc.Now()).
					WithEndTime(tc.Now()).
					WithUpdateTime(tc.Now()).
					WithSummaryMarkdown("no longer needed").
					Construct()))
			})
			Convey("With mask", func() {
				res, err := client.CancelBuild(ctx, &bbpb.CancelBuildRequest{
					Id:              build.GetId(),
					SummaryMarkdown: "no longer needed",
					Mask: &bbpb.BuildMask{
						Fields: &fieldmaskpb.FieldMask{
							Paths: []string{"id", "status"},
						},
					},
				})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &bbpb.Build{
					Id:     build.GetId(),
					Status: bbpb.Status_CANCELED,
				})
			})
		})

		Convey("No ACL", func() {
			client := fake.MustNewClient(ctx, bbHost, "anotherProj")
			res, err := client.CancelBuild(ctx, &bbpb.CancelBuildRequest{
				Id:              build.GetId(),
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
			for _, status := range []bbpb.Status{
				bbpb.Status_SUCCESS,
				bbpb.Status_FAILURE,
				bbpb.Status_INFRA_FAILURE,
				bbpb.Status_CANCELED,
			} {
				Convey(status.String(), func() {
					epoch := tc.Now()
					tc.Add(3 * time.Minute)
					fake.MutateBuild(ctx, bbHost, build.GetId(), func(b *bbpb.Build) {
						b.Status = status
						b.SummaryMarkdown = "ended already"
						b.StartTime = timestamppb.New(epoch.Add(1 * time.Minute))
						b.EndTime = timestamppb.New(epoch.Add(2 * time.Minute))
					})
					tc.Add(2 * time.Minute)
					res, err := client.CancelBuild(ctx, &bbpb.CancelBuildRequest{
						Id:              build.GetId(),
						SummaryMarkdown: "no longer needed",
						Mask: &bbpb.BuildMask{
							Fields: &fieldmaskpb.FieldMask{
								Paths: []string{"id", "status", "summary_markdown", "end_time"},
							},
						},
					})
					So(err, ShouldBeNil)
					So(res, ShouldResembleProto, &bbpb.Build{
						Id:              build.GetId(),
						Status:          status,
						EndTime:         timestamppb.New(epoch.Add(2 * time.Minute)),
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

		client := fake.MustNewClient(ctx, bbHost, lProject)
		Convey("Can schedule", func() {
			Convey("Simple", func() {
				res, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
					Builder: builderNoProp,
				})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, trimmedBuildWithDefaultMask(&bbpb.Build{
					Id:         res.Id,
					Builder:    builderNoProp,
					Status:     bbpb.Status_SCHEDULED,
					CreateTime: timestamppb.New(tc.Now()),
					UpdateTime: timestamppb.New(tc.Now()),
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
				res, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
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
					UpdateTime: timestamppb.New(tc.Now()),
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
				res, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
					Builder:    builderWithProp,
					Properties: props,
				})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, trimmedBuildWithDefaultMask(&bbpb.Build{
					Id:         res.Id,
					Builder:    builderWithProp,
					Status:     bbpb.Status_SCHEDULED,
					CreateTime: timestamppb.New(tc.Now()),
					UpdateTime: timestamppb.New(tc.Now()),
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
				res, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
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
					res, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
						Builder: builder,
					})
					So(err, ShouldBeNil)
					So(res.Id, ShouldBeLessThan, prevBuildID)
					prevBuildID = res.Id
				}
			})
			Convey("Use requestID for deduplication", func() {
				first, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
					RequestId: "foo",
					Builder:   builderNoProp,
				})
				So(err, ShouldBeNil)
				tc.Add(requestDeduplicationWindow / 2)
				dup, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
					RequestId: "foo",
					Builder:   builderNoProp,
				})
				So(err, ShouldBeNil)
				So(dup.Id, ShouldEqual, first.Id)
				// Passes the deduplication window, should generate new build.
				tc.Add(requestDeduplicationWindow)
				newBuild, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
					RequestId: "foo",
					Builder:   builderNoProp,
				})
				So(err, ShouldBeNil)
				So(newBuild.Id, ShouldNotEqual, first.Id)
			})
		})

		Convey("Builder not found", func() {
			res, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
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
		client := fake.MustNewClient(ctx, bbHost, lProject)

		builderFoo := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "testBucket",
			Builder: "foo",
		}
		builderBar := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "testBucket",
			Builder: "bar",
		}
		builderBaz := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "testBucket",
			Builder: "baz",
		}
		fake.AddBuilder(bbHost, builderFoo, nil)
		fake.AddBuilder(bbHost, builderBar, nil)
		fake.AddBuilder(bbHost, builderBaz, nil)

		buildFoo, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
			Builder: builderFoo,
		})
		So(err, ShouldBeNil)
		buildBar, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
			Builder: builderBar,
		})
		So(err, ShouldBeNil)
		tc.Add(1 * time.Minute)
		buildBar = fake.MutateBuild(ctx, bbHost, buildBar.GetId(), func(b *bbpb.Build) {
			b.Status = bbpb.Status_SUCCESS
			b.StartTime = timestamppb.New(tc.Now().Add(-30 * time.Second))
			b.EndTime = timestamppb.New(tc.Now())
		})

		Convey("Batch succeeds", func() {
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
										Paths: []string{"builder", "status"},
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
								Builder: builderBaz,
								Status:  bbpb.Status_SCHEDULED,
							},
						},
					},
				},
			})
		})

		Convey("Batch fails", func() {
			// ScheduleBuild in fake buildbucket will start from math.MaxInt - 1, so
			// there will never be a build in the fake with ID 12345 unless we
			// schedule numerous number of builds.
			const nonExistentBuildID = 12345
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
								Id: nonExistentBuildID,
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
