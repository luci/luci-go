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

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/rpc/testutil"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestGetBuild(t *testing.T) {
	t.Parallel()

	const userID = identity.Identity("user:user@example.com")

	ftt.Run("GetBuild", t, func(t *ftt.Test) {
		srv := &Builds{}
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: userID,
		})

		t.Run("id", func(t *ftt.Test) {
			t.Run("not found", func(t *ftt.Test) {
				req := &pb.GetBuildRequest{
					Id: 1,
				}
				rsp, err := srv.GetBuild(ctx, req)
				assert.Loosely(t, err, should.ErrLike("not found"))
				assert.Loosely(t, rsp, should.BeNil)
			})

			t.Run("with build entity", func(t *ftt.Test) {
				testutil.PutBucket(ctx, "project", "bucket", nil)
				build := &model.Build{
					Proto: &pb.Build{
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
						CancellationMarkdown: "cancelled",
						SummaryMarkdown:      "summary",
					},
				}
				assert.Loosely(t, datastore.Put(ctx, build), should.BeNil)
				key := datastore.KeyForObj(ctx, build)
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

				req := &pb.GetBuildRequest{
					Id: 1,
					Mask: &pb.BuildMask{
						AllFields: true,
					},
				}

				t.Run("permission denied", func(t *ftt.Test) {
					rsp, err := srv.GetBuild(ctx, req)
					assert.Loosely(t, err, should.ErrLike("not found"))
					assert.Loosely(t, rsp, should.BeNil)
				})

				t.Run("permission denied if user only has BuildsList permission", func(t *ftt.Test) {
					ctx = auth.WithState(ctx, &authtest.FakeState{
						Identity: userID,
						FakeDB: authtest.NewFakeDB(
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildersList),
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildsList),
						),
					})
					rsp, err := srv.GetBuild(ctx, req)
					assert.Loosely(t, err, should.ErrLike("not found"))
					assert.Loosely(t, rsp, should.BeNil)
				})

				t.Run("found with BuildsGetLimited permission only", func(t *ftt.Test) {
					ctx = auth.WithState(ctx, &authtest.FakeState{
						Identity: userID,
						FakeDB: authtest.NewFakeDB(
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildersList),
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGetLimited),
						),
					})
					rsp, err := srv.GetBuild(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rsp, should.Resemble(&pb.Build{
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
					}))
				})

				t.Run("found", func(t *ftt.Test) {
					ctx = auth.WithState(ctx, &authtest.FakeState{
						Identity: userID,
						FakeDB: authtest.NewFakeDB(
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
						),
					})
					rsp, err := srv.GetBuild(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rsp, should.Resemble(&pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Input: &pb.Build_Input{
							Properties: &structpb.Struct{
								Fields: map[string]*structpb.Value{
									"input": {
										Kind: &structpb.Value_StringValue{
											StringValue: "input value",
										},
									},
								},
							},
							GerritChanges: []*pb.GerritChange{
								{Host: "h1"},
								{Host: "h2"},
							},
						},
						Output: &pb.Build_Output{
							Properties: &structpb.Struct{
								Fields: map[string]*structpb.Value{
									"output": {
										Kind: &structpb.Value_StringValue{
											StringValue: "output value",
										},
									},
								},
							},
						},
						Infra: &pb.BuildInfra{
							Buildbucket: &pb.BuildInfra_Buildbucket{
								Hostname: "example.com",
							},
							Resultdb: &pb.BuildInfra_ResultDB{
								Hostname:   "rdb.example.com",
								Invocation: "bb-12345",
							},
						},
						Steps: []*pb.Step{
							{Name: "step"},
						},
						CancellationMarkdown: "cancelled",
						SummaryMarkdown:      "summary\ncancelled",
					}))
				})

				t.Run("summary", func(t *ftt.Test) {
					ctx = auth.WithState(ctx, &authtest.FakeState{
						Identity: userID,
						FakeDB: authtest.NewFakeDB(
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
						),
					})
					req.Mask = &pb.BuildMask{
						Fields: &fieldmaskpb.FieldMask{
							Paths: []string{
								"summary_markdown",
							},
						},
					}
					rsp, err := srv.GetBuild(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rsp, should.Resemble(&pb.Build{
						SummaryMarkdown: "summary\ncancelled",
					}))
				})
			})
		})

		t.Run("index", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
				BucketID: "project/bucket",
				Tags:     []string{"build_address:luci.project.bucket/builder/1"},
			}), should.BeNil)

			t.Run("error", func(t *ftt.Test) {
				t.Run("incomplete index", func(t *ftt.Test) {
					assert.Loosely(t, datastore.Put(ctx, &model.TagIndex{
						ID: ":2:build_address:luci.project.bucket/builder/1",
						Entries: []model.TagIndexEntry{
							{
								BuildID: 1,
							},
						},
						Incomplete: true,
					}), should.BeNil)
					req := &pb.GetBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						BuildNumber: 1,
					}
					rsp, err := srv.GetBuild(ctx, req)
					assert.Loosely(t, err, should.ErrLike("unexpected incomplete index"))
					assert.Loosely(t, rsp, should.BeNil)
				})

				t.Run("not found", func(t *ftt.Test) {
					req := &pb.GetBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						BuildNumber: 2,
					}
					rsp, err := srv.GetBuild(ctx, req)
					assert.Loosely(t, err, should.ErrLike("not found"))
					assert.Loosely(t, rsp, should.BeNil)
				})

				t.Run("excessive results", func(t *ftt.Test) {
					assert.Loosely(t, datastore.Put(ctx, &model.TagIndex{
						ID: ":2:build_address:luci.project.bucket/builder/1",
						Entries: []model.TagIndexEntry{
							{
								BuildID:  1,
								BucketID: "proj/bucket",
							},
							{
								BuildID:  2,
								BucketID: "proj/bucket",
							},
						},
					}), should.BeNil)
					req := &pb.GetBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						BuildNumber: 1,
					}
					rsp, err := srv.GetBuild(ctx, req)
					assert.Loosely(t, err, should.ErrLike("unexpected number of results"))
					assert.Loosely(t, rsp, should.BeNil)
				})
			})

			t.Run("ok", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: userID,
					FakeDB: authtest.NewFakeDB(
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
					),
				})
				testutil.PutBucket(ctx, "project", "bucket", nil)
				assert.Loosely(t, datastore.Put(ctx, &model.TagIndex{
					ID: ":2:build_address:luci.project.bucket/builder/1",
					Entries: []model.TagIndexEntry{
						{
							BuildID:  1,
							BucketID: "project/bucket",
						},
					},
				}), should.BeNil)
				req := &pb.GetBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					BuildNumber: 1,
				}
				rsp, err := srv.GetBuild(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rsp, should.Resemble(&pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Input: &pb.Build_Input{},
				}))
			})
		})

		t.Run("led build", func(t *ftt.Test) {
			testutil.PutBucket(ctx, "project", "bucket", nil)
			testutil.PutBucket(ctx, "project", "bucket.shadow", nil)
			build := &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket.shadow",
						Builder: "builder",
					},
					Input: &pb.Build_Input{
						GerritChanges: []*pb.GerritChange{
							{Host: "h1"},
							{Host: "h2"},
						},
					},
					CancellationMarkdown: "cancelled",
					SummaryMarkdown:      "summary",
				},
			}
			assert.Loosely(t, datastore.Put(ctx, build), should.BeNil)
			key := datastore.KeyForObj(ctx, build)
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
					Led: &pb.BuildInfra_Led{
						ShadowedBucket: "bucket",
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

			req := &pb.GetBuildRequest{
				Id: 1,
				Mask: &pb.BuildMask{
					AllFields: true,
				},
			}

			t.Run("permission denied", func(t *ftt.Test) {
				rsp, err := srv.GetBuild(ctx, req)
				assert.Loosely(t, err, should.ErrLike("not found"))
				assert.Loosely(t, rsp, should.BeNil)
			})

			t.Run("found with permission on shadowed bucket", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: userID,
					FakeDB: authtest.NewFakeDB(
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
					),
				})
				rsp, err := srv.GetBuild(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rsp, should.Resemble(&pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket.shadow",
						Builder: "builder",
					},
					Input: &pb.Build_Input{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"input": {
									Kind: &structpb.Value_StringValue{
										StringValue: "input value",
									},
								},
							},
						},
						GerritChanges: []*pb.GerritChange{
							{Host: "h1"},
							{Host: "h2"},
						},
					},
					Output: &pb.Build_Output{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"output": {
									Kind: &structpb.Value_StringValue{
										StringValue: "output value",
									},
								},
							},
						},
					},
					Infra: &pb.BuildInfra{
						Buildbucket: &pb.BuildInfra_Buildbucket{
							Hostname: "example.com",
						},
						Resultdb: &pb.BuildInfra_ResultDB{
							Hostname:   "rdb.example.com",
							Invocation: "bb-12345",
						},
						Led: &pb.BuildInfra_Led{
							ShadowedBucket: "bucket",
						},
					},
					Steps: []*pb.Step{
						{Name: "step"},
					},
					CancellationMarkdown: "cancelled",
					SummaryMarkdown:      "summary\ncancelled",
				}))
			})
		})
	})

	ftt.Run("validateGet", t, func(t *ftt.Test) {
		t.Run("nil", func(t *ftt.Test) {
			err := validateGet(nil)
			assert.Loosely(t, err, should.ErrLike("id or (builder and build_number) is required"))
		})

		t.Run("empty", func(t *ftt.Test) {
			req := &pb.GetBuildRequest{}
			err := validateGet(req)
			assert.Loosely(t, err, should.ErrLike("id or (builder and build_number) is required"))
		})

		t.Run("builder", func(t *ftt.Test) {
			req := &pb.GetBuildRequest{
				Builder: &pb.BuilderID{},
			}
			err := validateGet(req)
			assert.Loosely(t, err, should.ErrLike("id or (builder and build_number) is required"))
		})

		t.Run("build number", func(t *ftt.Test) {
			req := &pb.GetBuildRequest{
				BuildNumber: 1,
			}
			err := validateGet(req)
			assert.Loosely(t, err, should.ErrLike("id or (builder and build_number) is required"))
		})

		t.Run("mutual exclusion", func(t *ftt.Test) {
			t.Run("builder", func(t *ftt.Test) {
				req := &pb.GetBuildRequest{
					Id:      1,
					Builder: &pb.BuilderID{},
				}
				err := validateGet(req)
				assert.Loosely(t, err, should.ErrLike("id is mutually exclusive with (builder and build_number)"))
			})

			t.Run("build number", func(t *ftt.Test) {
				req := &pb.GetBuildRequest{
					Id:          1,
					BuildNumber: 1,
				}
				err := validateGet(req)
				assert.Loosely(t, err, should.ErrLike("id is mutually exclusive with (builder and build_number)"))
			})
		})

		t.Run("builder ID", func(t *ftt.Test) {
			t.Run("project", func(t *ftt.Test) {
				req := &pb.GetBuildRequest{
					Builder:     &pb.BuilderID{},
					BuildNumber: 1,
				}
				err := validateGet(req)
				assert.Loosely(t, err, should.ErrLike("project must match"))
			})

			t.Run("bucket", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req := &pb.GetBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
						},
						BuildNumber: 1,
					}
					err := validateGet(req)
					assert.Loosely(t, err, should.ErrLike("bucket is required"))
				})

				t.Run("v1", func(t *ftt.Test) {
					req := &pb.GetBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "luci.project.bucket",
							Builder: "builder",
						},
						BuildNumber: 1,
					}
					err := validateGet(req)
					assert.Loosely(t, err, should.ErrLike("invalid use of v1 bucket in v2 API"))
				})
			})

			t.Run("builder", func(t *ftt.Test) {
				req := &pb.GetBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
					},
					BuildNumber: 1,
				}
				err := validateGet(req)
				assert.Loosely(t, err, should.ErrLike("builder is required"))
			})
		})
	})
}
