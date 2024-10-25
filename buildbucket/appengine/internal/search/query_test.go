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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	bb "go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"
)

const userID = identity.Identity("user:user@example.com")

func TestNewSearchQuery(t *testing.T) {
	t.Parallel()

	ftt.Run("NewQuery", t, func(t *ftt.Test) {
		t.Run("valid input", func(t *ftt.Test) {
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

			assert.Loosely(t, query, should.Resemble(&Query{
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
			}))
		})

		t.Run("empty req", func(t *ftt.Test) {
			assert.Loosely(t, NewQuery(&pb.SearchBuildsRequest{}), should.Resemble(&Query{PageSize: 100}))
		})

		t.Run("empty predict", func(t *ftt.Test) {
			req := &pb.SearchBuildsRequest{
				PageToken: "aa",
				PageSize:  2,
			}
			query := NewQuery(req)

			assert.Loosely(t, query, should.Resemble(&Query{
				PageSize:  2,
				PageToken: "aa",
			}))
		})

		t.Run("empty identity", func(t *ftt.Test) {
			req := &pb.SearchBuildsRequest{
				Predicate: &pb.BuildPredicate{
					CreatedBy: "",
				},
			}
			query := NewQuery(req)

			assert.Loosely(t, query.CreatedBy, should.Equal(identity.Identity("")))
		})

		t.Run("invalid create time", func(t *ftt.Test) {
			req := &pb.SearchBuildsRequest{
				Predicate: &pb.BuildPredicate{
					CreatedBy: string(identity.AnonymousIdentity),
					CreateTime: &pb.TimeRange{
						StartTime: &timestamppb.Timestamp{Seconds: int64(253402300801)},
					},
				},
			}
			assert.Loosely(t, func() { NewQuery(req) }, should.Panic)
		})
	})
}

func TestFixPageSize(t *testing.T) {
	t.Parallel()

	ftt.Run("normal page size", t, func(t *ftt.Test) {
		assert.Loosely(t, fixPageSize(200), should.Equal(200))
	})

	ftt.Run("default page size", t, func(t *ftt.Test) {
		assert.Loosely(t, fixPageSize(0), should.Equal(100))
	})

	ftt.Run("max page size", t, func(t *ftt.Test) {
		assert.Loosely(t, fixPageSize(1500), should.Equal(1000))
	})
}

func TestMustTimestamp(t *testing.T) {
	t.Parallel()
	ftt.Run("normal timestamp", t, func(t *ftt.Test) {
		res := mustTimestamp(&timestamppb.Timestamp{Seconds: 1592701200})
		assert.Loosely(t, res, should.Match(time.Unix(1592701200, 0).UTC()))
	})
	ftt.Run("invalid timestamp", t, func(t *ftt.Test) {
		assert.Loosely(t, func() { mustTimestamp(&timestamppb.Timestamp{Seconds: 253402300801}) }, should.Panic)
	})
	ftt.Run("nil timestamp", t, func(t *ftt.Test) {
		res := mustTimestamp(nil)
		assert.Loosely(t, res.IsZero(), should.BeTrue)
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

	ftt.Run("Fetch", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = memlogger.Use(ctx)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		assert.Loosely(t, datastore.Put(
			ctx,
			&model.Bucket{
				Parent: model.ProjectKey(ctx, "project"),
				ID:     "bucket",
				Proto:  &pb.Bucket{},
			},
			&model.Builder{
				Parent: model.BucketKey(ctx, "project", "bucket"),
				ID:     "builder",
				Config: &pb.BuilderConfig{Name: "builder"},
			},
		), should.BeNil)

		query := NewQuery(&pb.SearchBuildsRequest{
			Predicate: &pb.BuildPredicate{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			},
		})

		t.Run("No permission for requested bucketId", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: userID,
			})
			_, err := query.Fetch(ctx)
			assert.Loosely(t, appstatus.Code(err), should.Match(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("not found"))
		})

		t.Run("With read permission", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: userID,
				FakeDB: authtest.NewFakeDB(
					authtest.MockPermission(userID, "project:bucket", bbperms.BuildersList),
					authtest.MockPermission(userID, "project:bucket", bbperms.BuildsList),
				),
			})

			t.Run("Fetch via TagIndex flow", func(t *ftt.Test) {
				query.Tags = strpair.ParseMap([]string{"buildset:1"})
				actualRsp, err := query.Fetch(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, actualRsp, should.Resemble(&pb.SearchBuildsResponse{}))
			})

			t.Run("Fetch via Build flow", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
				}), should.BeNil)

				query := NewQuery(&pb.SearchBuildsRequest{})
				rsp, err := query.Fetch(ctx)
				assert.Loosely(t, err, should.BeNil)
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
				assert.Loosely(t, rsp, should.Resemble(expectedRsp))
			})

			t.Run("Fallback to fetchOnBuild flow", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, &model.TagIndex{
					ID:         ":10:buildset:1",
					Incomplete: true,
					Entries:    nil,
				}), should.BeNil)
				assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
				}), should.BeNil)

				query.Tags = strpair.ParseMap([]string{"buildset:1"})
				actualRsp, err := query.Fetch(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, actualRsp, should.Resemble(&pb.SearchBuildsResponse{
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
				}))
			})
		})
	})
}

func TestFetchOnBuild(t *testing.T) {
	t.Parallel()

	ftt.Run("FetchOnBuild", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: userID,
			FakeDB: authtest.NewFakeDB(
				authtest.MockPermission(userID, "project:bucket", bbperms.BuildersList),
				authtest.MockPermission(userID, "project:bucket", bbperms.BuildsList),
			),
		})
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		assert.Loosely(t, datastore.Put(ctx, &model.Bucket{
			ID:     "bucket",
			Parent: model.ProjectKey(ctx, "project"),
			Proto:  &pb.Bucket{},
		}), should.BeNil)
		assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
		}), should.BeNil)
		assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
		}), should.BeNil)

		t.Run("found by builder", func(t *ftt.Test) {
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

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRsp, should.Resemble(expectedRsp))
		})
		t.Run("found by tag", func(t *ftt.Test) {
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

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRsp, should.Resemble(expectedRsp))
		})
		t.Run("found by status", func(t *ftt.Test) {
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

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRsp, should.Resemble(expectedRsp))
		})
		t.Run("found by build range", func(t *ftt.Test) {
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

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRsp, should.Resemble(expectedRsp))
		})
		t.Run("found by create time", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
			}), should.BeNil)
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

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRsp, should.Resemble(expectedRsp))
		})
		t.Run("found by created_by", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
			}), should.BeNil)
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

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRsp, should.Resemble(expectedRsp))
		})
		t.Run("found by ENDED_MASK", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
			}), should.BeNil)

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

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRsp, should.Resemble(expectedRsp))
		})
		t.Run("found by canary", func(t *ftt.Test) {
			req := &pb.SearchBuildsRequest{
				Predicate: &pb.BuildPredicate{
					Canary: pb.Trinary_YES,
				},
			}
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
			}), should.BeNil)
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

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRsp, should.Resemble(expectedRsp))
		})
		t.Run("found only experimental", func(t *ftt.Test) {
			req := &pb.SearchBuildsRequest{
				Predicate: &pb.BuildPredicate{
					Experiments: []string{"+" + bb.ExperimentNonProduction},
				},
			}
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
			}), should.BeNil)
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

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRsp, should.Resemble(expectedRsp))
		})
		t.Run("found non experimental", func(t *ftt.Test) {
			req := &pb.SearchBuildsRequest{
				Predicate: &pb.BuildPredicate{
					Experiments: []string{"-" + bb.ExperimentNonProduction},
				},
			}
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
			}), should.BeNil)
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

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRsp, should.Resemble(expectedRsp))
		})
		t.Run("found by ancestors", func(t *ftt.Test) {
			bIDs := func(rsp *pb.SearchBuildsResponse) []int {
				ids := make([]int, 0, len(rsp.Builds))
				for _, b := range rsp.Builds {
					ids = append(ids, int(b.Id))
				}
				sort.Ints(ids)
				return ids
			}
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
				ID: 1,
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
			}), should.BeNil)
			t.Run("by ancestor_ids", func(t *ftt.Test) {
				req := &pb.SearchBuildsRequest{
					Predicate: &pb.BuildPredicate{
						DescendantOf: 1,
					},
				}
				query := NewQuery(req)
				actualRsp, err := query.fetchOnBuild(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, bIDs(actualRsp), should.Resemble([]int{2, 3, 4}))
			})
			t.Run("by parent_id", func(t *ftt.Test) {
				req := &pb.SearchBuildsRequest{
					Predicate: &pb.BuildPredicate{
						ChildOf: 1,
					},
				}
				query := NewQuery(req)
				actualRsp, err := query.fetchOnBuild(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, bIDs(actualRsp), should.Resemble([]int{2, 3}))
			})
		})
		t.Run("empty request", func(t *ftt.Test) {
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

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRsp, should.Resemble(expectedRsp))
		})
		t.Run("pagination", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
			}), should.BeNil)

			// this build can be fetched from db but not accessible by the user.
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
			}), should.BeNil)

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

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRsp.Builds, should.Resemble(expectedBuilds))
			assert.Loosely(t, actualRsp.NextPageToken, should.Equal("id>200"))

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

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRsp.Builds, should.Resemble(expectedBuilds))
			assert.Loosely(t, actualRsp.NextPageToken, should.BeEmpty)
		})
		t.Run("found by start build id and pagination", func(t *ftt.Test) {
			req := &pb.SearchBuildsRequest{
				Predicate: &pb.BuildPredicate{
					Build: &pb.BuildRange{
						StartBuildId: 199,
					},
				},
				PageToken: "id>199",
			}
			query := NewQuery(req)
			actualRsp, err := query.fetchOnBuild(ctx)
			expectedRsp := &pb.SearchBuildsResponse{}

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRsp, should.Resemble(expectedRsp))
		})
	})
}

func TestIndexedTags(t *testing.T) {
	t.Parallel()

	ftt.Run("tags", t, func(t *ftt.Test) {
		tags := strpair.Map{
			"a":        []string{"b"},
			"buildset": []string{"b1"},
		}
		result := IndexedTags(tags)
		assert.Loosely(t, result, should.Resemble([]string{"buildset:b1"}))
	})

	ftt.Run("duplicate tags", t, func(t *ftt.Test) {
		tags := strpair.Map{
			"buildset":      []string{"b1", "b1"},
			"build_address": []string{"address"},
		}
		result := IndexedTags(tags)
		assert.Loosely(t, result, should.Resemble([]string{"build_address:address", "buildset:b1"}))
	})

	ftt.Run("empty tags", t, func(t *ftt.Test) {
		tags := strpair.Map{}
		result := IndexedTags(tags)
		assert.Loosely(t, result, should.Resemble([]string{}))
	})
}

func TestUpdateTagIndex(t *testing.T) {
	t.Parallel()

	ftt.Run("UpdateTagIndex", t, func(t *ftt.Test) {
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
		assert.Loosely(t, UpdateTagIndex(ctx, builds), should.BeEmpty)

		idx, err := model.SearchTagIndex(ctx, "a", "b")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, idx, should.BeNil)

		idx, err = model.SearchTagIndex(ctx, "buildset", "b1")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, idx, should.Resemble([]*model.TagIndexEntry{
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
		}))

		idx, err = model.SearchTagIndex(ctx, "build_address", "address")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, idx, should.Resemble([]*model.TagIndexEntry{
			{
				BuildID:     int64(2),
				BucketID:    "project/bucket",
				CreatedTime: datastore.RoundTime(testclock.TestRecentTimeUTC),
			},
		}))
	})
}

func TestFetchOnTagIndex(t *testing.T) {
	t.Parallel()

	ftt.Run("FetchOnTagIndex", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: userID,
			FakeDB: authtest.NewFakeDB(
				authtest.MockPermission(userID, "project:bucket", bbperms.BuildersList),
				authtest.MockPermission(userID, "project:bucket", bbperms.BuildsList),
			),
		})
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		assert.Loosely(t, datastore.Put(ctx, &model.Bucket{
			ID:     "bucket",
			Parent: model.ProjectKey(ctx, "project"),
			Proto:  &pb.Bucket{},
		}), should.BeNil)
		assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
		}), should.BeNil)
		assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
		}), should.BeNil)
		assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
		}), should.BeNil)
		assert.Loosely(t, datastore.Put(ctx, &model.TagIndex{
			ID: ":2:buildset:commit/git/abcd",
			Entries: []model.TagIndexEntry{
				{
					BuildID:  100,
					BucketID: "project/bucket",
				},
			},
		}), should.BeNil)
		assert.Loosely(t, datastore.Put(ctx, &model.TagIndex{
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
		}), should.BeNil)
		req := &pb.SearchBuildsRequest{
			Predicate: &pb.BuildPredicate{
				Tags: []*pb.StringPair{
					{Key: "buildset", Value: "commit/git/abcd"},
				},
			},
		}
		t.Run("filter only by an indexed tag", func(t *ftt.Test) {
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

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRsp, should.Resemble(expectedRsp))
		})

		t.Run("filter by status", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRsp, should.Resemble(expectedRsp))
		})
		t.Run("filter by ENDED_MASK", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, &model.TagIndex{
				ID: ":4:buildset:commit/git/abcd",
				Entries: []model.TagIndexEntry{
					{
						BuildID:  999,
						BucketID: "project/bucket",
					},
				},
			}), should.BeNil)
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRsp, should.Resemble(expectedRsp))
		})
		t.Run("filter by an indexed tag and a normal tag", func(t *ftt.Test) {
			req.Predicate.Tags = append(req.Predicate.Tags, &pb.StringPair{Key: "k1", Value: "v1"})
			query := NewQuery(req)
			actualRsp, err := query.fetchOnTagIndex(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRsp, should.Resemble(&pb.SearchBuildsResponse{}))
		})
		t.Run("filter by build range", func(t *ftt.Test) {
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

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRsp, should.Resemble(expectedRsp))
		})
		t.Run("filter by created_by", func(t *ftt.Test) {
			req.Predicate.CreatedBy = "project:infra"
			query := NewQuery(req)
			actualRsp, err := query.fetchOnTagIndex(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRsp, should.Resemble(&pb.SearchBuildsResponse{}))
		})
		t.Run("filter by canary", func(t *ftt.Test) {
			req.Predicate.Canary = pb.Trinary_YES
			query := NewQuery(req)
			actualRsp, err := query.fetchOnTagIndex(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRsp, should.Resemble(&pb.SearchBuildsResponse{}))
		})
		t.Run("filter by IncludeExperimental", func(t *ftt.Test) {
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

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRsp, should.Resemble(expectedRsp))
		})
		t.Run("pagination", func(t *ftt.Test) {
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

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRsp, should.Resemble(expectedRsp))
		})
		t.Run("No permission on requested buckets", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:none",
			})
			query := NewQuery(req)
			actualRsp, err := query.fetchOnTagIndex(ctx)

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRsp, should.Resemble(&pb.SearchBuildsResponse{}))
		})
	})
}

func TestMinHeap(t *testing.T) {
	t.Parallel()

	ftt.Run("minHeap", t, func(t *ftt.Test) {
		h := &minHeap{{BuildID: 2}, {BuildID: 1}, {BuildID: 5}}

		heap.Init(h)
		heap.Push(h, &model.TagIndexEntry{BuildID: 3})
		var res []int64
		for h.Len() > 0 {
			res = append(res, heap.Pop(h).(*model.TagIndexEntry).BuildID)
		}
		assert.Loosely(t, res, should.Resemble([]int64{1, 2, 3, 5}))
	})

}
