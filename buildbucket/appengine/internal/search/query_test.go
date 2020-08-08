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
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/grpc/codes"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

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
					CreatedBy:     string(identity.AnonymousIdentity),
					Tags:          tags,
					CreateTime: &pb.TimeRange{
						StartTime: &timestamp.Timestamp{Seconds: 1592701200},
						EndTime:   &timestamp.Timestamp{Seconds: 1592704800},
					},
					Build: &pb.BuildRange{
						StartBuildId: 200,
						EndBuildId:   100,
					},
					Canary: pb.Trinary_YES,
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
				Builder:             expectedBuilder,
				Tags:                expectedTags,
				Status:              pb.Status_ENDED_MASK,
				CreatedBy:           identity.AnonymousIdentity,
				StartTime:           expectedStartTime,
				EndTime:             expectedEndTime,
				IncludeExperimental: false,
				BuildIdHigh:         201,
				BuildIdLow:          99,
				Canary:              proto.Bool(true),
				PageSize:            100,
				StartCursor:         "",
			})
		})

		Convey("empty req", func() {
			So(NewQuery(&pb.SearchBuildsRequest{}), ShouldResemble, &Query{})
		})

		Convey("empty predict", func() {
			req := &pb.SearchBuildsRequest{
				PageToken: "aa",
				PageSize:  2,
			}
			query := NewQuery(req)

			So(query, ShouldResemble, &Query{
				PageSize:    2,
				StartCursor: "aa",
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
						StartTime: &timestamp.Timestamp{Seconds: int64(253402300801)},
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
		res := mustTimestamp(&timestamp.Timestamp{Seconds: 1592701200})
		So(res, ShouldEqual, time.Unix(1592701200, 0).UTC())
	})
	Convey("invalid timestamp", t, func() {
		So(func() { mustTimestamp(&timestamp.Timestamp{Seconds: 253402300801}) }, ShouldPanic)
	})
	Convey("nil timestamp", t, func() {
		res := mustTimestamp(nil)
		So(res.IsZero(), ShouldBeTrue)
	})
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
					Config: pb.Builder{Name: "builder"},
				},
			), ShouldBeNil)

			_, err := query.Fetch(ctx)
			So(err, ShouldHaveAppStatus, codes.NotFound, "not found")
		})

		// TODO(crbug/1090540): Add more tests after searchBuilds func completed.
		Convey("Fetch via TagIndex flow", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:user",
			})
			So(datastore.Put(
				ctx,
				&model.Bucket{
					Parent: model.ProjectKey(ctx, "project"),
					ID:     "bucket",
					Proto: pb.Bucket{
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
					Config: pb.Builder{Name: "builder"},
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
				Proto: pb.Bucket{
					Acls: []*pb.Acl{
						{
							Identity: "user:user",
							Role:     pb.Acl_READER,
						},
					},
				},
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.Build{
				Proto: pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
				BuilderID: "project/bucket/builder",
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
	})
}

func TestFetchOnBuild(t *testing.T) {
	t.Parallel()

	Convey("FetchOnBuild", t, func() {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		So(datastore.Put(ctx, &model.Build{
			ID: 100,
			Proto: pb.Build{
				Id: 100,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder1",
				},
				Status: pb.Status_SUCCESS,
			},
			Status:    pb.Status_SUCCESS,
			Project:   "project",
			BuilderID: "project/bucket/builder1",
			Tags:      []string{"k1:v1", "k2:v2"},
		}), ShouldBeNil)
		So(datastore.Put(ctx, &model.Build{
			ID: 200,
			Proto: pb.Build{
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
			BuilderID: "project/bucket/builder2",
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
				Proto: pb.Build{
					Id: 8764414515958775808,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
			}), ShouldBeNil)
			req := &pb.SearchBuildsRequest{
				Predicate: &pb.BuildPredicate{
					CreateTime: &pb.TimeRange{
						StartTime: &timestamp.Timestamp{Seconds: 1592701200},
						EndTime:   &timestamp.Timestamp{Seconds: 1700000000},
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
							Builder: "builder",
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
				Proto: pb.Build{
					Id: 1111,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					CreatedBy: "project:infra",
				},
				CreatedBy: "project:infra",
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
						Id: 1111,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						CreatedBy: "project:infra",
					},
				},
			}

			So(err, ShouldBeNil)
			So(actualRsp, ShouldResembleProto, expectedRsp)
		})
		Convey("pagination", func() {
			req := &pb.SearchBuildsRequest{
				Predicate: &pb.BuildPredicate{
					Builder: &pb.BuilderID{
						Project: "project",
					},
				},
				PageSize: 1,
			}
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
			}

			So(err, ShouldBeNil)
			So(actualRsp.Builds, ShouldResembleProto, expectedBuilds)
			So(actualRsp.NextPageToken, ShouldNotBeEmpty)
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
			Proto: pb.Bucket{
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
			Proto: pb.Build{
				Id: 100,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder1",
				},
				Status: pb.Status_SUCCESS,
			},
			Status:    pb.Status_SUCCESS,
			Project:   "project",
			BucketID:  "project/bucket",
			BuilderID: "project/bucket/builder1",
			Tags:      []string{"buildset:commit/git/abcd"},
		}), ShouldBeNil)
		So(datastore.Put(ctx, &model.Build{
			ID: 200,
			Proto: pb.Build{
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
