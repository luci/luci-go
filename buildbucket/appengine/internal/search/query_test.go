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

package search

import (
	"container/heap"
	"context"
	"sort"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	// TODO(crbug/1242998): Remove once safe get becomes datastore default.
	_ "go.chromium.org/luci/gae/service/datastore/crbug1242998safeget"

	bb "go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestNewSearchQuery(t *testing.T) {
	t.Parallel()

	Convey("NewQuery", t, func() {
		Convey("valid input", func() {
			gerritChanges := make([]*pb.GerritChange, 2)
			gerritChanges[0] = &pb.GerritChange{
				Host:     "a",
				Project:  "b",
				Change:   1,
				Patchset: 1,
			}
			gerritChanges[1] = &pb.GerritChange{
				Host:     "a",
				Project:  "b",
				Change:   2,
				Patchset: 1,
			}
			tags := []*pb.StringPair{
				{Key: "k1", Value: "v1"},
			}
			req := &pb.SearchBuildsRequest{
				Predicate: &pb.BuildPredicate{
					Builder: &pb.BuilderID{
						Project: "infra",
						Bucket:  "ci",
						Builder: "test",
					},
					Status:        pb.Status_ENDED_MASK,
					GerritChanges: gerritChanges,
					CreatedBy:     "abc@test.com",
					Tags:          tags,
					CreateTime: &pb.TimeRange{
						StartTime: &timestamppb.Timestamp{Seconds: 1592701200},
						EndTime:   &timestamppb.Timestamp{Seconds: 1592704800},
					},
					Build: &pb.BuildRange{
						StartBuildId: 200,
						EndBuildId:   100,
					},
					Canary:       pb.Trinary_YES,
					DescendantOf: 2,
				},
			}
			query := NewQuery(req)

			expectedStartTime := time.Unix(1592701200, 0).UTC()
			expectedEndTime := time.Unix(1592704800, 0).UTC()
			expectedTags := strpair.Map{
				"k1":       []string{"v1"},
				"buildset": []string{"patch/gerrit/a/1/1", "patch/gerrit/a/2/1"},
			}
			expectedBuilder := &pb.BuilderID{
				Project: "infra",
				Bucket:  "ci",
				Builder: "test",
			}

			So(query, ShouldResemble, &Query{
				Builder:   expectedBuilder,
				Tags:      expectedTags,
				Status:    pb.Status_ENDED_MASK,
				CreatedBy: identity.Identity("user:abc@test.com"),
				StartTime: expectedStartTime,
				EndTime:   expectedEndTime,
				ExperimentFilters: stringset.NewFromSlice(
					"+"+bb.ExperimentBBCanarySoftware,
					"-"+bb.ExperimentNonProduction,
				),
				BuildIDHigh:  201,
				BuildIDLow:   99,
				DescendantOf: 2,
				PageSize:     100,
				PageToken:    "",
			})
		})

		Convey("empty req", func() {
			So(NewQuery(&pb.SearchBuildsRequest{}), ShouldResemble, &Query{PageSize: 100})
		})

		Convey("empty predict", func() {
			req := &pb.SearchBuildsRequest{
				PageToken: "aa",
				PageSize:  2,
			}
			query := NewQuery(req)

			So(query, ShouldResemble, &Query{
				PageSize:  2,
				PageToken: "aa",
			})
		})

		Convey("empty identity", func() {
			req := &pb.SearchBuildsRequest{
				Predicate: &pb.BuildPredicate{
					CreatedBy: "",
				},
			}
			query := NewQuery(req)

			So(query.CreatedBy, ShouldEqual, identity.Identity(""))
		})

		Convey("invalid create time", func() {
			req := &pb.SearchBuildsRequest{
				Predicate: &pb.BuildPredicate{
					CreatedBy: string(identity.AnonymousIdentity),
					CreateTime: &pb.TimeRange{
						StartTime: &timestamppb.Timestamp{Seconds: int64(253402300801)},
					},
				},
			}
			So(func() { NewQuery(req) }, ShouldPanic)
		})
	})
}

func TestFixPageSize(t *testing.T) {
	t.Parallel()

	Convey("normal page size", t, func() {
		So(fixPageSize(200), ShouldEqual, 200)
	})

	Convey("default page size", t, func() {
		So(fixPageSize(0), ShouldEqual, 100)
	})

	Convey("max page size", t, func() {
		So(fixPageSize(1500), ShouldEqual, 1000)
	})
}

func TestMustTimestamp(t *testing.T) {
	t.Parallel()
	Convey("normal timestamp", t, func() {
		res := mustTimestamp(&timestamppb.Timestamp{Seconds: 1592701200})
		So(res, ShouldEqual, time.Unix(1592701200, 0).UTC())
	})
	Convey("invalid timestamp", t, func() {
		So(func() { mustTimestamp(&timestamppb.Timestamp{Seconds: 253402300801}) }, ShouldPanic)
	})
	Convey("nil timestamp", t, func() {
		res := mustTimestamp(nil)
		So(res.IsZero(), ShouldBeTrue)
	})
}

func experiments(canary, experimental bool) (ret []string) {
	if canary {
		ret = append(ret, "+"+bb.ExperimentBBCanarySoftware)
	} else {
		ret = append(ret, "-"+bb.ExperimentBBCanarySoftware)
	}

	if experimental {
		ret = append(ret, "+"+bb.ExperimentNonProduction)
	}
	return
}

func TestMainFetchFlow(t *testing.T) {
	t.Parallel()

	Convey("Fetch", t, func() {
		ctx := memory.Use(context.Background())
		ctx = memlogger.Use(ctx)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		query := NewQuery(&pb.SearchBuildsRequest{
			Predicate: &pb.BuildPredicate{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			},
		})
		Convey("No permission for requested bucketId", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:user",
			})
			So(datastore.Put(
				ctx,
				&model.Bucket{
					Parent: model.ProjectKey(ctx, "project"),
					ID:     "bucket",
				},
				&model.Builder{
					Parent: model.BucketKey(ctx, "project", "bucket"),
					ID:     "builder",
					Config: &pb.BuilderConfig{Name: "builder"},
				},
			), ShouldBeNil)

			_, err := query.Fetch(ctx)
			So(err, ShouldHaveAppStatus, codes.NotFound, "not found")
		})
		Convey("Fetch via TagIndex flow", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:user",
			})
			So(datastore.Put(
				ctx,
				&model.Bucket{
					Parent: model.ProjectKey(ctx, "project"),
					ID:     "bucket",
					Proto: &pb.Bucket{
						Acls: []*pb.Acl{
							{
								Identity: "user:user",
								Role:     pb.Acl_READER,
							},
						},
					},
				},
				&model.Builder{
					Parent: model.BucketKey(ctx, "project", "bucket"),
					ID:     "builder",
					Config: &pb.BuilderConfig{Name: "builder"},
				},
			), ShouldBeNil)

			query.Tags = strpair.ParseMap([]string{"buildset:1"})
			actualRsp, err := query.Fetch(ctx)
			So(err, ShouldBeNil)
			So(actualRsp, ShouldResembleProto, &pb.SearchBuildsResponse{})
		})
		Convey("Fetch via Build flow", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: identity.Identity("user:user"),
			})
			So(datastore.Put(ctx, &model.Bucket{
				ID:     "bucket",
				Parent: model.ProjectKey(ctx, "project"),
				Proto: &pb.Bucket{
					Acls: []*pb.Acl{
						{
							Identity: "user:user",
							Role:     pb.Acl_READER,
						},
					},
				},
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
				BucketID:    "project/bucket",
				BuilderID:   "project/bucket/builder",
				Experiments: experiments(false, false),
			}), ShouldBeNil)

			query := NewQuery(&pb.SearchBuildsRequest{})
			rsp, err := query.Fetch(ctx)
			So(err, ShouldBeNil)
			expectedRsp := &pb.SearchBuildsResponse{
				Builds: []*pb.Build{
					{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					},
				},
			}
			So(rsp, ShouldResembleProto, expectedRsp)
		})
		Convey("Fallback to fetchOnBuild flow", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:user",
			})
			So(datastore.Put(
				ctx,
				&model.Bucket{
					Parent: model.ProjectKey(ctx, "project"),
					ID:     "bucket",
					Proto: &pb.Bucket{
						Acls: []*pb.Acl{
							{
								Identity: "user:user",
								Role:     pb.Acl_READER,
							},
						},
					},
				},
				&model.Builder{
					Parent: model.BucketKey(ctx, "project", "bucket"),
					ID:     "builder",
					Config: &pb.BuilderConfig{Name: "builder"},
				},
			), ShouldBeNil)
			So(datastore.Put(ctx, &model.TagIndex{
				ID:         ":10:buildset:1",
				Incomplete: true,
				Entries:    nil,
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
				BucketID:    "project/bucket",
				BuilderID:   "project/bucket/builder",
				Tags:        []string{"buildset:1"},
				Experiments: experiments(false, false),
			}), ShouldBeNil)

			query.Tags = strpair.ParseMap([]string{"buildset:1"})
			actualRsp, err := query.Fetch(ctx)
			So(err, ShouldBeNil)
			So(actualRsp, ShouldResembleProto, &pb.SearchBuildsResponse{
				Builds: []*pb.Build{
					{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Tags: []*pb.StringPair{
							{
								Key:   "buildset",
								Value: "1",
							},
						},
					},
				},
			})
		})
	})
}

func TestFetchOnBuild(t *testing.T) {
	t.Parallel()

	Convey("FetchOnBuild", t, func() {
		ctx := memory.Use(context.Background())
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: identity.Identity("user:user"),
		})
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		So(datastore.Put(ctx, &model.Bucket{
			ID:     "bucket",
			Parent: model.ProjectKey(ctx, "project"),
			Proto: &pb.Bucket{
				Acls: []*pb.Acl{
					{
						Identity: "user:user",
						Role:     pb.Acl_READER,
					},
				},
			},
		}), ShouldBeNil)
		So(datastore.Put(ctx, &model.Build{
			ID: 100,
			Proto: &pb.Build{
				Id: 100,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder1",
				},
				Status: pb.Status_SUCCESS,
			},
			Status:      pb.Status_SUCCESS,
			Project:     "project",
			BucketID:    "project/bucket",
			BuilderID:   "project/bucket/builder1",
			Tags:        []string{"k1:v1", "k2:v2"},
			Experiments: experiments(false, false),
		}), ShouldBeNil)
		So(datastore.Put(ctx, &model.Build{
			ID: 200,
			Proto: &pb.Build{
				Id: 200,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder2",
				},
				Status: pb.Status_CANCELED,
			},
			Status:      pb.Status_CANCELED,
			Project:     "project",
			BucketID:    "project/bucket",
			BuilderID:   "project/bucket/builder2",
			Experiments: experiments(false, false),
		}), ShouldBeNil)

		Convey("found by builder", func() {
			req := &pb.SearchBuildsRequest{
				Predicate: &pb.BuildPredicate{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder1",
					},
				},
			}
			query := NewQuery(req)
			actualRsp, err := query.fetchOnBuild(ctx)
			expectedRsp := &pb.SearchBuildsResponse{
				Builds: []*pb.Build{
					{
						Id: 100,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder1",
						},
						Tags: []*pb.StringPair{
							{Key: "k1", Value: "v1"},
							{Key: "k2", Value: "v2"},
						},
						Status: pb.Status_SUCCESS,
					},
				},
			}

			So(err, ShouldBeNil)
			So(actualRsp, ShouldResembleProto, expectedRsp)
		})
		Convey("found by tag", func() {
			req := &pb.SearchBuildsRequest{
				Predicate: &pb.BuildPredicate{
					Status: pb.Status_SUCCESS,
					Tags: []*pb.StringPair{
						{Key: "k1", Value: "v1"},
						{Key: "k2", Value: "v2"},
					},
				},
			}
			query := NewQuery(req)
			actualRsp, err := query.fetchOnBuild(ctx)
			expectedRsp := &pb.SearchBuildsResponse{
				Builds: []*pb.Build{
					{
						Id: 100,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder1",
						},
						Tags: []*pb.StringPair{
							{Key: "k1", Value: "v1"},
							{Key: "k2", Value: "v2"},
						},
						Status: pb.Status_SUCCESS,
					},
				},
			}

			So(err, ShouldBeNil)
			So(actualRsp, ShouldResembleProto, expectedRsp)
		})
		Convey("found by status", func() {
			req := &pb.SearchBuildsRequest{
				Predicate: &pb.BuildPredicate{
					Status: pb.Status_SUCCESS,
				},
			}
			query := NewQuery(req)
			actualRsp, err := query.fetchOnBuild(ctx)
			expectedRsp := &pb.SearchBuildsResponse{
				Builds: []*pb.Build{
					{
						Id: 100,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder1",
						},
						Tags: []*pb.StringPair{
							{Key: "k1", Value: "v1"},
							{Key: "k2", Value: "v2"},
						},
						Status: pb.Status_SUCCESS,
					},
				},
			}

			So(err, ShouldBeNil)
			So(actualRsp, ShouldResembleProto, expectedRsp)
		})
		Convey("found by build range", func() {
			req := &pb.SearchBuildsRequest{
				Predicate: &pb.BuildPredicate{
					Build: &pb.BuildRange{
						StartBuildId: 199,
						EndBuildId:   99,
					},
				},
			}
			query := NewQuery(req)
			actualRsp, err := query.fetchOnBuild(ctx)
			expectedRsp := &pb.SearchBuildsResponse{
				Builds: []*pb.Build{
					{
						Id: 100,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder1",
						},
						Tags: []*pb.StringPair{
							{Key: "k1", Value: "v1"},
							{Key: "k2", Value: "v2"},
						},
						Status: pb.Status_SUCCESS,
					},
				},
			}

			So(err, ShouldBeNil)
			So(actualRsp, ShouldResembleProto, expectedRsp)
		})
		Convey("found by create time", func() {
			So(datastore.Put(ctx, &model.Build{
				ID: 8764414515958775808,
				Proto: &pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder5808",
					},
					Id: 8764414515958775808,
				},
				BucketID:    "project/bucket",
				Experiments: experiments(false, false),
			}), ShouldBeNil)
			req := &pb.SearchBuildsRequest{
				Predicate: &pb.BuildPredicate{
					CreateTime: &pb.TimeRange{
						StartTime: &timestamppb.Timestamp{Seconds: 1592701200},
						EndTime:   &timestamppb.Timestamp{Seconds: 1700000000},
					},
				},
			}
			query := NewQuery(req)
			actualRsp, err := query.fetchOnBuild(ctx)
			expectedRsp := &pb.SearchBuildsResponse{
				Builds: []*pb.Build{
					{
						Id: 8764414515958775808,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder5808",
						},
					},
				},
			}

			So(err, ShouldBeNil)
			So(actualRsp, ShouldResembleProto, expectedRsp)
		})
		Convey("found by created_by", func() {
			So(datastore.Put(ctx, &model.Build{
				ID: 1111,
				Proto: &pb.Build{
					Id:        1111,
					CreatedBy: "project:infra",
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder1111",
					},
				},
				CreatedBy:   "project:infra",
				BucketID:    "project/bucket",
				Experiments: experiments(false, false),
			}), ShouldBeNil)
			req := &pb.SearchBuildsRequest{
				Predicate: &pb.BuildPredicate{
					CreatedBy: "project:infra",
				},
			}
			query := NewQuery(req)
			actualRsp, err := query.fetchOnBuild(ctx)
			expectedRsp := &pb.SearchBuildsResponse{
				Builds: []*pb.Build{
					{
						Id:        1111,
						CreatedBy: "project:infra",
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder1111",
						},
					},
				},
			}

			So(err, ShouldBeNil)
			So(actualRsp, ShouldResembleProto, expectedRsp)
		})
		Convey("found by ENDED_MASK", func() {
			So(datastore.Put(ctx, &model.Build{
				ID: 300,
				Proto: &pb.Build{
					Id: 300,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder3",
					},
					Status: pb.Status_STARTED,
				},
				Project:     "project",
				BucketID:    "project/bucket",
				BuilderID:   "project/bucket/builder3",
				Status:      pb.Status_STARTED,
				Experiments: experiments(false, false),
			}), ShouldBeNil)

			req := &pb.SearchBuildsRequest{
				Predicate: &pb.BuildPredicate{
					Status: pb.Status_ENDED_MASK,
				},
			}
			query := NewQuery(req)
			actualRsp, err := query.fetchOnBuild(ctx)
			expectedRsp := &pb.SearchBuildsResponse{
				Builds: []*pb.Build{
					{
						Id: 100,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder1",
						},
						Tags: []*pb.StringPair{
							{Key: "k1", Value: "v1"},
							{Key: "k2", Value: "v2"},
						},
						Status: pb.Status_SUCCESS,
					},
					{
						Id: 200,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder2",
						},
						Status: pb.Status_CANCELED,
					},
				},
			}

			So(err, ShouldBeNil)
			So(actualRsp, ShouldResembleProto, expectedRsp)
		})
		Convey("found by canary", func() {
			req := &pb.SearchBuildsRequest{
				Predicate: &pb.BuildPredicate{
					Canary: pb.Trinary_YES,
				},
			}
			So(datastore.Put(ctx, &model.Build{
				ID: 321,
				Proto: &pb.Build{
					Id: 321,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder321",
					},
					Canary: true,
				},
				Project:     "project",
				BucketID:    "project/bucket",
				BuilderID:   "project/bucket/builder321",
				Experiments: experiments(true, false),
			}), ShouldBeNil)
			query := NewQuery(req)
			actualRsp, err := query.fetchOnBuild(ctx)
			expectedRsp := &pb.SearchBuildsResponse{
				Builds: []*pb.Build{
					{
						Id: 321,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder321",
						},
						Canary: true,
					},
				},
			}

			So(err, ShouldBeNil)
			So(actualRsp, ShouldResembleProto, expectedRsp)
		})
		Convey("found only experimental", func() {
			req := &pb.SearchBuildsRequest{
				Predicate: &pb.BuildPredicate{
					Experiments: []string{"+" + bb.ExperimentNonProduction},
				},
			}
			So(datastore.Put(ctx, &model.Build{
				ID: 321,
				Proto: &pb.Build{
					Id: 321,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder321",
					},
					Input: &pb.Build_Input{Experimental: true},
				},
				Project:     "project",
				BucketID:    "project/bucket",
				BuilderID:   "project/bucket/builder321",
				Experiments: experiments(false, true),
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.Build{
				ID: 123,
				Proto: &pb.Build{
					Id: 123,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder321",
					},
				},
				Project:     "project",
				BucketID:    "project/bucket",
				BuilderID:   "project/bucket/builder123",
				Experiments: experiments(false, false),
			}), ShouldBeNil)
			query := NewQuery(req)
			actualRsp, err := query.fetchOnBuild(ctx)
			expectedRsp := &pb.SearchBuildsResponse{
				Builds: []*pb.Build{
					{
						Id: 321,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder321",
						},
						Input: &pb.Build_Input{Experimental: true},
					},
				},
			}

			So(err, ShouldBeNil)
			So(actualRsp, ShouldResembleProto, expectedRsp)
		})
		Convey("found non experimental", func() {
			req := &pb.SearchBuildsRequest{
				Predicate: &pb.BuildPredicate{
					Experiments: []string{"-" + bb.ExperimentNonProduction},
				},
			}
			So(datastore.Put(ctx, &model.Build{
				ID: 321,
				Proto: &pb.Build{
					Id: 321,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder321",
					},
					Input: &pb.Build_Input{Experimental: true},
				},
				Project:     "project",
				BucketID:    "project/bucket",
				BuilderID:   "project/bucket/builder321",
				Experiments: experiments(false, true),
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.Build{
				ID: 123,
				Proto: &pb.Build{
					Id: 123,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder123",
					},
				},
				Project:     "project",
				BucketID:    "project/bucket",
				BuilderID:   "project/bucket/builder123",
				Experiments: experiments(false, false),
			}), ShouldBeNil)
			query := NewQuery(req)
			actualRsp, err := query.fetchOnBuild(ctx)
			expectedRsp := &pb.SearchBuildsResponse{
				Builds: []*pb.Build{
					{
						Id: 100,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder1",
						},
						Tags: []*pb.StringPair{
							{Key: "k1", Value: "v1"},
							{Key: "k2", Value: "v2"},
						},
						Status: pb.Status_SUCCESS,
					},
					{
						Id: 123,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder123",
						},
					},
					{
						Id: 200,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder2",
						},
						Status: pb.Status_CANCELED,
					},
				},
			}

			So(err, ShouldBeNil)
			So(actualRsp, ShouldResembleProto, expectedRsp)
		})
		Convey("found by ancestors", func() {
			bIDs := func(rsp *pb.SearchBuildsResponse) []int {
				ids := make([]int, 0, len(rsp.Builds))
				for _, b := range rsp.Builds {
					ids = append(ids, int(b.Id))
				}
				sort.Ints(ids)
				return ids
			}
			So(datastore.Put(ctx, &model.Build{
				ID: 1,
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.Build{
				ID: 2,
				Proto: &pb.Build{
					Id: 2,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					AncestorIds: []int64{1},
				},
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.Build{
				ID: 3,
				Proto: &pb.Build{
					Id: 3,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					AncestorIds: []int64{1},
				},
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.Build{
				ID: 4,
				Proto: &pb.Build{
					Id: 4,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					AncestorIds: []int64{1, 2},
				},
			}), ShouldBeNil)
			Convey("by ancestor_ids", func() {
				req := &pb.SearchBuildsRequest{
					Predicate: &pb.BuildPredicate{
						DescendantOf: 1,
					},
				}
				query := NewQuery(req)
				actualRsp, err := query.fetchOnBuild(ctx)
				So(err, ShouldBeNil)
				So(bIDs(actualRsp), ShouldResemble, []int{2, 3, 4})
			})
			Convey("by parent_id", func() {
				req := &pb.SearchBuildsRequest{
					Predicate: &pb.BuildPredicate{
						ChildOf: 1,
					},
				}
				query := NewQuery(req)
				actualRsp, err := query.fetchOnBuild(ctx)
				So(err, ShouldBeNil)
				So(bIDs(actualRsp), ShouldResemble, []int{2, 3})
			})
		})
		Convey("empty request", func() {
			req := &pb.SearchBuildsRequest{
				Predicate: &pb.BuildPredicate{},
			}
			query := NewQuery(req)
			actualRsp, err := query.fetchOnBuild(ctx)
			expectedRsp := &pb.SearchBuildsResponse{
				Builds: []*pb.Build{
					{
						Id: 100,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder1",
						},
						Tags: []*pb.StringPair{
							{Key: "k1", Value: "v1"},
							{Key: "k2", Value: "v2"},
						},
						Status: pb.Status_SUCCESS,
					},
					{
						Id: 200,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder2",
						},
						Status: pb.Status_CANCELED,
					},
				},
			}

			So(err, ShouldBeNil)
			So(actualRsp, ShouldResembleProto, expectedRsp)
		})
		Convey("pagination", func() {
			So(datastore.Put(ctx, &model.Build{
				ID: 300,
				Proto: &pb.Build{
					Id: 300,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder3",
					},
				},
				Project:     "project",
				BucketID:    "project/bucket",
				BuilderID:   "project/bucket/builder3",
				Experiments: experiments(false, false),
			}), ShouldBeNil)

			// this build can be fetched from db but not accessible by the user.
			So(datastore.Put(ctx, &model.Build{
				ID: 400,
				Proto: &pb.Build{
					Id: 400,
					Builder: &pb.BuilderID{
						Project: "project_no_access",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
				Project:     "project_no_access",
				BucketID:    "project_no_access/bucket",
				BuilderID:   "project_no_access/bucket/builder",
				Experiments: experiments(false, false),
			}), ShouldBeNil)

			req := &pb.SearchBuildsRequest{
				PageSize: 2,
			}

			// fetch 1st page.
			query := NewQuery(req)
			actualRsp, err := query.fetchOnBuild(ctx)
			expectedBuilds := []*pb.Build{
				{
					Id: 100,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder1",
					},
					Tags: []*pb.StringPair{
						{Key: "k1", Value: "v1"},
						{Key: "k2", Value: "v2"},
					},
					Status: pb.Status_SUCCESS,
				},
				{
					Id: 200,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder2",
					},
					Status: pb.Status_CANCELED,
				},
			}

			So(err, ShouldBeNil)
			So(actualRsp.Builds, ShouldResembleProto, expectedBuilds)
			So(actualRsp.NextPageToken, ShouldEqual, "id>200")

			// fetch the following page (response should have a build with the ID - 400).
			req.PageToken = actualRsp.NextPageToken
			query = NewQuery(req)
			actualRsp, err = query.fetchOnBuild(ctx)
			expectedBuilds = []*pb.Build{
				{
					Id: 300,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder3",
					},
				},
			}

			So(err, ShouldBeNil)
			So(actualRsp.Builds, ShouldResembleProto, expectedBuilds)
			So(actualRsp.NextPageToken, ShouldBeEmpty)
		})
	})
}

func TestIndexedTags(t *testing.T) {
	t.Parallel()

	Convey("tags", t, func() {
		tags := strpair.Map{
			"a":        []string{"b"},
			"buildset": []string{"b1"},
		}
		result := IndexedTags(tags)
		So(result, ShouldResemble, []string{"buildset:b1"})
	})

	Convey("duplicate tags", t, func() {
		tags := strpair.Map{
			"buildset":      []string{"b1", "b1"},
			"build_address": []string{"address"},
		}
		result := IndexedTags(tags)
		So(result, ShouldResemble, []string{"build_address:address", "buildset:b1"})
	})

	Convey("empty tags", t, func() {
		tags := strpair.Map{}
		result := IndexedTags(tags)
		So(result, ShouldResemble, []string{})
	})
}

func TestUpdateTagIndex(t *testing.T) {
	t.Parallel()

	Convey("UpdateTagIndex", t, func() {
		ctx, _ := testclock.UseTime(memory.Use(context.Background()), testclock.TestRecentTimeUTC)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		builds := []*model.Build{
			{
				ID: 1,
				Proto: &pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
				},
				Tags: []string{
					"a:b",
					"buildset:b1",
				},
				Experiments: experiments(false, false),
			},
			{
				ID: 2,
				Proto: &pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
				},
				Tags: []string{
					"a:b",
					"build_address:address",
					"buildset:b1",
				},
				Experiments: experiments(false, false),
			},
		}
		So(UpdateTagIndex(ctx, builds), ShouldBeNil)

		idx, err := model.SearchTagIndex(ctx, "a", "b")
		So(err, ShouldBeNil)
		So(idx, ShouldBeNil)

		idx, err = model.SearchTagIndex(ctx, "buildset", "b1")
		So(err, ShouldBeNil)
		So(idx, ShouldResemble, []*model.TagIndexEntry{
			{
				BuildID:     int64(1),
				BucketID:    "project/bucket",
				CreatedTime: datastore.RoundTime(testclock.TestRecentTimeUTC),
			},
			{
				BuildID:     int64(2),
				BucketID:    "project/bucket",
				CreatedTime: datastore.RoundTime(testclock.TestRecentTimeUTC),
			},
		})

		idx, err = model.SearchTagIndex(ctx, "build_address", "address")
		So(err, ShouldBeNil)
		So(idx, ShouldResemble, []*model.TagIndexEntry{
			{
				BuildID:     int64(2),
				BucketID:    "project/bucket",
				CreatedTime: datastore.RoundTime(testclock.TestRecentTimeUTC),
			},
		})
	})
}

func TestFetchOnTagIndex(t *testing.T) {
	t.Parallel()

	Convey("FetchOnTagIndex", t, func() {
		ctx := memory.Use(context.Background())
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:user",
		})
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		So(datastore.Put(ctx, &model.Bucket{
			ID:     "bucket",
			Parent: model.ProjectKey(ctx, "project"),
			Proto: &pb.Bucket{
				Acls: []*pb.Acl{
					{
						Identity: "user:user",
						Role:     pb.Acl_READER,
					},
				},
			},
		}), ShouldBeNil)
		So(datastore.Put(ctx, &model.Build{
			ID: 100,
			Proto: &pb.Build{
				Id: 100,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder1",
				},
				Status: pb.Status_SUCCESS,
			},
			Status:      pb.Status_SUCCESS,
			Project:     "project",
			BucketID:    "project/bucket",
			BuilderID:   "project/bucket/builder1",
			Tags:        []string{"buildset:commit/git/abcd"},
			Experiments: experiments(false, false),
		}), ShouldBeNil)
		So(datastore.Put(ctx, &model.Build{
			ID: 200,
			Proto: &pb.Build{
				Id: 200,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder2",
				},
				Status: pb.Status_CANCELED,
			},
			Status:    pb.Status_CANCELED,
			Project:   "project",
			BucketID:  "project/bucket",
			BuilderID: "project/bucket/builder2",
			Tags:      []string{"buildset:commit/git/abcd"},
			// legacy; no Experiments, assumed to be prod
		}), ShouldBeNil)
		So(datastore.Put(ctx, &model.Build{
			ID: 300,
			Proto: &pb.Build{
				Id: 300,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder3",
				},
				Status: pb.Status_CANCELED,
				Input:  &pb.Build_Input{Experimental: true},
			},
			Status:      pb.Status_CANCELED,
			Project:     "project",
			BucketID:    "project/bucket",
			BuilderID:   "project/bucket/builder3",
			Tags:        []string{"buildset:commit/git/abcd"},
			Experiments: experiments(false, true),
		}), ShouldBeNil)
		So(datastore.Put(ctx, &model.TagIndex{
			ID: ":2:buildset:commit/git/abcd",
			Entries: []model.TagIndexEntry{
				{
					BuildID:  100,
					BucketID: "project/bucket",
				},
			},
		}), ShouldBeNil)
		So(datastore.Put(ctx, &model.TagIndex{
			ID: ":3:buildset:commit/git/abcd",
			Entries: []model.TagIndexEntry{
				{
					BuildID:  200,
					BucketID: "project/bucket",
				},
				{
					BuildID:  300,
					BucketID: "project/bucket",
				},
			},
		}), ShouldBeNil)
		req := &pb.SearchBuildsRequest{
			Predicate: &pb.BuildPredicate{
				Tags: []*pb.StringPair{
					{Key: "buildset", Value: "commit/git/abcd"},
				},
			},
		}
		Convey("filter only by an indexed tag", func() {
			query := NewQuery(req)
			actualRsp, err := query.fetchOnTagIndex(ctx)
			expectedRsp := &pb.SearchBuildsResponse{
				Builds: []*pb.Build{
					{
						Id: 100,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder1",
						},
						Tags: []*pb.StringPair{
							{Key: "buildset", Value: "commit/git/abcd"},
						},
						Status: pb.Status_SUCCESS,
					},
					{
						Id: 200,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder2",
						},
						Tags: []*pb.StringPair{
							{Key: "buildset", Value: "commit/git/abcd"},
						},
						Status: pb.Status_CANCELED,
					},
				},
			}

			So(err, ShouldBeNil)
			So(actualRsp, ShouldResembleProto, expectedRsp)
		})

		Convey("filter by status", func() {
			req.Predicate.Status = pb.Status_CANCELED
			query := NewQuery(req)
			actualRsp, err := query.fetchOnTagIndex(ctx)
			expectedRsp := &pb.SearchBuildsResponse{
				Builds: []*pb.Build{
					{
						Id: 200,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder2",
						},
						Tags: []*pb.StringPair{
							{Key: "buildset", Value: "commit/git/abcd"},
						},
						Status: pb.Status_CANCELED,
					},
				},
			}
			So(err, ShouldBeNil)
			So(actualRsp, ShouldResembleProto, expectedRsp)
		})
		Convey("filter by ENDED_MASK", func() {
			So(datastore.Put(ctx, &model.Build{
				ID: 999,
				Proto: &pb.Build{
					Id:     999,
					Status: pb.Status_STARTED,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder999",
					},
				},
				Project:     "project",
				BucketID:    "project/bucket",
				BuilderID:   "project/bucket/builder999",
				Status:      pb.Status_STARTED,
				Tags:        []string{"buildset:commit/git/abcd"},
				Experiments: experiments(false, false),
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.TagIndex{
				ID: ":4:buildset:commit/git/abcd",
				Entries: []model.TagIndexEntry{
					{
						BuildID:  999,
						BucketID: "project/bucket",
					},
				},
			}), ShouldBeNil)
			req.Predicate.Status = pb.Status_ENDED_MASK
			query := NewQuery(req)
			actualRsp, err := query.fetchOnTagIndex(ctx)
			expectedRsp := &pb.SearchBuildsResponse{
				Builds: []*pb.Build{
					{
						Id: 100,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder1",
						},
						Tags: []*pb.StringPair{
							{Key: "buildset", Value: "commit/git/abcd"},
						},
						Status: pb.Status_SUCCESS,
					},
					{
						Id: 200,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder2",
						},
						Tags: []*pb.StringPair{
							{Key: "buildset", Value: "commit/git/abcd"},
						},
						Status: pb.Status_CANCELED,
					},
				},
			}
			So(err, ShouldBeNil)
			So(actualRsp, ShouldResembleProto, expectedRsp)
		})
		Convey("filter by an indexed tag and a normal tag", func() {
			req.Predicate.Tags = append(req.Predicate.Tags, &pb.StringPair{Key: "k1", Value: "v1"})
			query := NewQuery(req)
			actualRsp, err := query.fetchOnTagIndex(ctx)
			So(err, ShouldBeNil)
			So(actualRsp, ShouldResembleProto, &pb.SearchBuildsResponse{})
		})
		Convey("filter by build range", func() {
			req.Predicate.Build = &pb.BuildRange{
				StartBuildId: 199,
				EndBuildId:   99,
			}
			query := NewQuery(req)
			actualRsp, err := query.fetchOnTagIndex(ctx)
			expectedRsp := &pb.SearchBuildsResponse{
				Builds: []*pb.Build{
					{
						Id: 100,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder1",
						},
						Tags: []*pb.StringPair{
							{Key: "buildset", Value: "commit/git/abcd"},
						},
						Status: pb.Status_SUCCESS,
					},
				},
			}

			So(err, ShouldBeNil)
			So(actualRsp, ShouldResembleProto, expectedRsp)
		})
		Convey("filter by created_by", func() {
			req.Predicate.CreatedBy = "project:infra"
			query := NewQuery(req)
			actualRsp, err := query.fetchOnTagIndex(ctx)
			So(err, ShouldBeNil)
			So(actualRsp, ShouldResembleProto, &pb.SearchBuildsResponse{})
		})
		Convey("filter by canary", func() {
			req.Predicate.Canary = pb.Trinary_YES
			query := NewQuery(req)
			actualRsp, err := query.fetchOnTagIndex(ctx)
			So(err, ShouldBeNil)
			So(actualRsp, ShouldResembleProto, &pb.SearchBuildsResponse{})
		})
		Convey("filter by IncludeExperimental", func() {
			req.Predicate.IncludeExperimental = true
			query := NewQuery(req)
			actualRsp, err := query.fetchOnTagIndex(ctx)
			expectedRsp := &pb.SearchBuildsResponse{
				Builds: []*pb.Build{
					{
						Id: 100,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder1",
						},
						Tags: []*pb.StringPair{
							{Key: "buildset", Value: "commit/git/abcd"},
						},
						Status: pb.Status_SUCCESS,
					},
					{
						Id: 200,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder2",
						},
						Tags: []*pb.StringPair{
							{Key: "buildset", Value: "commit/git/abcd"},
						},
						Status: pb.Status_CANCELED,
					},
					{
						Id: 300,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder3",
						},
						Tags: []*pb.StringPair{
							{Key: "buildset", Value: "commit/git/abcd"},
						},
						Input:  &pb.Build_Input{Experimental: true},
						Status: pb.Status_CANCELED,
					},
				},
			}

			So(err, ShouldBeNil)
			So(actualRsp, ShouldResembleProto, expectedRsp)
		})
		Convey("pagination", func() {
			req.PageSize = 1
			query := NewQuery(req)
			actualRsp, err := query.fetchOnTagIndex(ctx)
			expectedRsp := &pb.SearchBuildsResponse{
				Builds: []*pb.Build{
					{
						Id: 100,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder1",
						},
						Tags: []*pb.StringPair{
							{Key: "buildset", Value: "commit/git/abcd"},
						},
						Status: pb.Status_SUCCESS,
					},
				},
				NextPageToken: "id>100",
			}

			So(err, ShouldBeNil)
			So(actualRsp, ShouldResembleProto, expectedRsp)
		})
		Convey("No permission on requested buckets", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:none",
			})
			query := NewQuery(req)
			actualRsp, err := query.fetchOnTagIndex(ctx)

			So(err, ShouldBeNil)
			So(actualRsp, ShouldResembleProto, &pb.SearchBuildsResponse{})
		})
	})
}

func TestMinHeap(t *testing.T) {
	t.Parallel()

	Convey("minHeap", t, func() {
		h := &minHeap{{BuildID: 2}, {BuildID: 1}, {BuildID: 5}}

		heap.Init(h)
		heap.Push(h, &model.TagIndexEntry{BuildID: 3})
		var res []int64
		for h.Len() > 0 {
			res = append(res, heap.Pop(h).(*model.TagIndexEntry).BuildID)
		}
		So(res, ShouldResemble, []int64{1, 2, 3, 5})
	})

}
