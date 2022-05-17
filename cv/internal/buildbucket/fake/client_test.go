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
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.chromium.org/luci/buildbucket/appengine/model"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"

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

func trimmedBuildWithDefaultMask(b *bbpb.Build) *bbpb.Build {
	mask, err := model.NewBuildMask("", nil, nil)
	So(err, ShouldBeNil)
	ret := proto.Clone(b).(*bbpb.Build)
	So(mask.Trim(ret), ShouldBeNil)
	return ret
}
