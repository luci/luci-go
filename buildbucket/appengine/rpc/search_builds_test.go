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
	"testing"

	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	bb "go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/rpc/testutil"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestValidateSearchBuilds(t *testing.T) {
	t.Parallel()

	ftt.Run("validateChange", t, func(t *ftt.Test) {
		t.Run("nil", func(t *ftt.Test) {
			err := validateChange(nil)
			assert.Loosely(t, err, should.ErrLike("host is required"))
		})

		t.Run("empty", func(t *ftt.Test) {
			ch := &pb.GerritChange{}
			err := validateChange(ch)
			assert.Loosely(t, err, should.ErrLike("host is required"))
		})

		t.Run("change", func(t *ftt.Test) {
			ch := &pb.GerritChange{
				Host: "host",
			}
			err := validateChange(ch)
			assert.Loosely(t, err, should.ErrLike("change is required"))
		})

		t.Run("patchset", func(t *ftt.Test) {
			ch := &pb.GerritChange{
				Host:   "host",
				Change: 1,
			}
			err := validateChange(ch)
			assert.Loosely(t, err, should.ErrLike("patchset is required"))
		})

		t.Run("valid", func(t *ftt.Test) {
			ch := &pb.GerritChange{
				Host:     "host",
				Change:   1,
				Patchset: 1,
			}
			err := validateChange(ch)
			assert.Loosely(t, err, should.BeNil)
		})
	})

	ftt.Run("validatePredicate", t, func(t *ftt.Test) {
		t.Run("nil", func(t *ftt.Test) {
			err := validatePredicate(nil)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("empty", func(t *ftt.Test) {
			pr := &pb.BuildPredicate{}
			err := validatePredicate(pr)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("mutual exclusion", func(t *ftt.Test) {
			pr := &pb.BuildPredicate{
				Build:      &pb.BuildRange{},
				CreateTime: &pb.TimeRange{},
			}
			err := validatePredicate(pr)
			assert.Loosely(t, err, should.ErrLike("build is mutually exclusive with create_time"))
		})

		t.Run("builder id", func(t *ftt.Test) {
			t.Run("no project", func(t *ftt.Test) {
				pr := &pb.BuildPredicate{
					Builder: &pb.BuilderID{Bucket: "bucket"},
				}
				err := validatePredicate(pr)
				assert.Loosely(t, err, should.ErrLike(`builder: project must match "^[a-z0-9\\-_]+$"`))
			})
			t.Run("only project and builder", func(t *ftt.Test) {
				pr := &pb.BuildPredicate{
					Builder: &pb.BuilderID{
						Project: "project",
						Builder: "builder",
					},
				}
				err := validatePredicate(pr)
				assert.Loosely(t, err, should.ErrLike("builder: bucket is required"))
			})
		})

		t.Run("experiments", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				pr := &pb.BuildPredicate{
					Experiments: []string{""},
				}
				err := validatePredicate(pr)
				assert.Loosely(t, err, should.ErrLike(`too short (expected [+-]$experiment_name)`))
			})
			t.Run("bang", func(t *ftt.Test) {
				pr := &pb.BuildPredicate{
					Experiments: []string{"!something"},
				}
				err := validatePredicate(pr)
				assert.Loosely(t, err, should.ErrLike(`first character must be + or -`))
			})
			t.Run("canary conflict", func(t *ftt.Test) {
				pr := &pb.BuildPredicate{
					Experiments: []string{"+" + bb.ExperimentBBCanarySoftware},
					Canary:      pb.Trinary_YES,
				}
				err := validatePredicate(pr)
				assert.Loosely(t, err, should.ErrLike(
					`cannot specify "luci.buildbucket.canary_software" and canary in the same predicate`))
			})
			t.Run("duplicate (bad)", func(t *ftt.Test) {
				pr := &pb.BuildPredicate{
					Experiments: []string{
						"+" + bb.ExperimentBBCanarySoftware,
						"-" + bb.ExperimentBBCanarySoftware,
					},
				}
				err := validatePredicate(pr)
				assert.Loosely(t, err, should.ErrLike(
					`"luci.buildbucket.canary_software" has both inclusive and exclusive filter`))
			})

			t.Run("ok", func(t *ftt.Test) {
				pr := &pb.BuildPredicate{
					Experiments: []string{
						"+" + bb.ExperimentBBCanarySoftware,
						"+" + bb.ExperimentNonProduction,
					},
				}
				assert.Loosely(t, validatePredicate(pr), should.BeNil)
			})
			t.Run("duplicate (ok)", func(t *ftt.Test) {
				pr := &pb.BuildPredicate{
					Experiments: []string{
						"+" + bb.ExperimentBBCanarySoftware,
						"+" + bb.ExperimentBBCanarySoftware,
					},
				}
				assert.Loosely(t, validatePredicate(pr), should.BeNil)
			})
		})

		t.Run("descendant_of and child_of mutual exclusion", func(t *ftt.Test) {
			pr := &pb.BuildPredicate{
				DescendantOf: 1,
				ChildOf:      1,
			}
			err := validatePredicate(pr)
			assert.Loosely(t, err, should.ErrLike("descendant_of is mutually exclusive with child_of"))
		})
	})

	ftt.Run("validatePageToken", t, func(t *ftt.Test) {
		t.Run("empty token", func(t *ftt.Test) {
			err := validatePageToken("")
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("invalid page token", func(t *ftt.Test) {
			err := validatePageToken("abc")
			assert.Loosely(t, err, should.ErrLike("invalid page_token"))
		})

		t.Run("valid page token", func(t *ftt.Test) {
			err := validatePageToken("id>123")
			assert.Loosely(t, err, should.BeNil)
		})
	})

	ftt.Run("validateSearch", t, func(t *ftt.Test) {
		t.Run("nil", func(t *ftt.Test) {
			err := validateSearch(nil)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("empty", func(t *ftt.Test) {
			req := &pb.SearchBuildsRequest{}
			err := validateSearch(req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("page size", func(t *ftt.Test) {
			t.Run("negative", func(t *ftt.Test) {
				req := &pb.SearchBuildsRequest{
					PageSize: -1,
				}
				err := validateSearch(req)
				assert.Loosely(t, err, should.ErrLike("page_size cannot be negative"))
			})

			t.Run("zero", func(t *ftt.Test) {
				req := &pb.SearchBuildsRequest{
					PageSize: 0,
				}
				err := validateSearch(req)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("positive", func(t *ftt.Test) {
				req := &pb.SearchBuildsRequest{
					PageSize: 1,
				}
				err := validateSearch(req)
				assert.Loosely(t, err, should.BeNil)
			})
		})
	})
}

func TestSearchBuilds(t *testing.T) {
	t.Parallel()

	const userID = identity.Identity("user:user@example.com")

	ftt.Run("search builds", t, func(t *ftt.Test) {
		srv := &Builds{}
		ctx := memory.Use(context.Background())
		ctx = memlogger.Use(ctx)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: userID,
			FakeDB: authtest.NewFakeDB(
				authtest.MockPermission(userID, "project:bucket", bbperms.BuildersList),
				authtest.MockPermission(userID, "project:bucket", bbperms.BuildsList),
				authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
			),
		})
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		testutil.PutBucket(ctx, "project", "bucket", nil)
		assert.Loosely(t, datastore.Put(ctx, &model.Build{
			Proto: &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				SummaryMarkdown: "foo summary",
				Input: &pb.Build_Input{
					GerritChanges: []*pb.GerritChange{
						{Host: "h1"},
						{Host: "h2"},
					},
				},
			},
			BucketID:  "project/bucket",
			BuilderID: "project/bucket/builder",
			Tags:      []string{"k1:v1", "k2:v2"},
		}), should.BeNil)
		assert.Loosely(t, datastore.Put(ctx, &model.Build{
			Proto: &pb.Build{
				Id: 2,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder2",
				},
			},
			BucketID:  "project/bucket",
			BuilderID: "project/bucket/builder2",
		}), should.BeNil)
		t.Run("query search on Builds", func(t *ftt.Test) {
			req := &pb.SearchBuildsRequest{
				Predicate: &pb.BuildPredicate{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Tags: []*pb.StringPair{
						{Key: "k1", Value: "v1"},
						{Key: "k2", Value: "v2"},
					},
				},
			}
			rsp, err := srv.SearchBuilds(ctx, req)
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
						Input: &pb.Build_Input{
							GerritChanges: []*pb.GerritChange{
								{Host: "h1"},
								{Host: "h2"},
							},
						},
					},
				},
			}
			assert.Loosely(t, rsp, should.Resemble(expectedRsp))
		})

		t.Run("search builds with field masks", func(t *ftt.Test) {
			b := &model.Build{
				ID: 1,
			}
			key := datastore.KeyForObj(ctx, b)
			assert.Loosely(t, datastore.Put(ctx, &model.BuildInfra{
				Build: key,
				Proto: &pb.BuildInfra{
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "example.com",
					},
				},
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, &model.BuildInputProperties{
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
			}), should.BeNil)

			req := &pb.SearchBuildsRequest{
				Fields: &field_mask.FieldMask{
					Paths: []string{"builds.*.id", "builds.*.input", "builds.*.infra"},
				},
			}
			rsp, err := srv.SearchBuilds(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			expectedRsp := &pb.SearchBuildsResponse{
				Builds: []*pb.Build{
					{
						Id: 1,
						Input: &pb.Build_Input{
							GerritChanges: []*pb.GerritChange{
								{Host: "h1"},
								{Host: "h2"},
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
						},
						Infra: &pb.BuildInfra{
							Buildbucket: &pb.BuildInfra_Buildbucket{
								Hostname: "example.com",
							},
						},
					},
					{
						Id:    2,
						Input: &pb.Build_Input{},
					},
				},
			}
			assert.Loosely(t, rsp, should.Resemble(expectedRsp))
		})

		t.Run("search builds with limited access", func(t *ftt.Test) {
			key := datastore.KeyForObj(ctx, &model.Build{ID: 1})
			s, err := proto.Marshal(&pb.Build{
				Steps: []*pb.Step{
					{
						Name: "step",
					},
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, &model.BuildSteps{
				Build:    key,
				Bytes:    s,
				IsZipped: false,
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, &model.BuildInfra{
				Build: key,
				Proto: &pb.BuildInfra{
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "example.com",
					},
					Resultdb: &pb.BuildInfra_ResultDB{
						Hostname:   "rdb.example.com",
						Invocation: "bb-12345",
					},
				},
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, &model.BuildInputProperties{
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
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, &model.BuildOutputProperties{
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
			}), should.BeNil)

			req := &pb.SearchBuildsRequest{
				Mask: &pb.BuildMask{
					AllFields: true,
				},
			}

			t.Run("BuildsGetLimited only", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: userID,
					FakeDB: authtest.NewFakeDB(
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildersList),
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGetLimited),
					),
				})

				rsp, err := srv.SearchBuilds(ctx, req)
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
							Input: &pb.Build_Input{
								GerritChanges: []*pb.GerritChange{
									{Host: "h1"},
									{Host: "h2"},
								},
							},
							Infra: &pb.BuildInfra{
								Resultdb: &pb.BuildInfra_ResultDB{
									Hostname:   "rdb.example.com",
									Invocation: "bb-12345",
								},
							},
						},
						{
							Id: 2,
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket",
								Builder: "builder2",
							},
							Input: &pb.Build_Input{},
						},
					},
				}
				assert.Loosely(t, rsp, should.Resemble(expectedRsp))
			})

			t.Run("BuildsList only", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: userID,
					FakeDB: authtest.NewFakeDB(
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildersList),
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildsList),
					),
				})

				rsp, err := srv.SearchBuilds(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				expectedRsp := &pb.SearchBuildsResponse{
					Builds: []*pb.Build{
						{
							Id: 1,
						},
						{
							Id: 2,
						},
					},
				}
				assert.Loosely(t, rsp, should.Resemble(expectedRsp))
			})
		})
	})
}
