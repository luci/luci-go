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
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/rpc/testutil"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestCancelBuild(t *testing.T) {
	t.Parallel()

	const userID = identity.Identity("user:user@example.com")

	ftt.Run("validateCancel", t, func(t *ftt.Test) {
		t.Run("request", func(t *ftt.Test) {
			t.Run("nil", func(t *ftt.Test) {
				err := validateCancel(nil)
				assert.Loosely(t, err, should.ErrLike("id is required"))
			})

			t.Run("empty", func(t *ftt.Test) {
				req := &pb.CancelBuildRequest{}
				err := validateCancel(req)
				assert.Loosely(t, err, should.ErrLike("id is required"))
			})

			t.Run("id", func(t *ftt.Test) {
				req := &pb.CancelBuildRequest{
					Id: 1,
				}
				err := validateCancel(req)
				assert.Loosely(t, err, should.ErrLike("summary_markdown is required"))
			})
		})
	})

	ftt.Run("CancelBuild", t, func(t *ftt.Test) {
		srv := &Builds{}
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, sch := tq.TestingContext(ctx, nil)

		t.Run("id", func(t *ftt.Test) {
			t.Run("not found", func(t *ftt.Test) {
				req := &pb.CancelBuildRequest{
					Id:              1,
					SummaryMarkdown: "summary",
				}
				rsp, err := srv.CancelBuild(ctx, req)
				assert.Loosely(t, err, should.ErrLike("not found"))
				assert.Loosely(t, rsp, should.BeNil)
				assert.Loosely(t, sch.Tasks(), should.BeEmpty)
			})

			t.Run("permission denied", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: userID,
					FakeDB: authtest.NewFakeDB(
						// Read only permission: not enough to cancel.
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
					),
				})
				testutil.PutBucket(ctx, "project", "bucket 1", nil)
				assert.Loosely(t, datastore.Put(ctx, &model.Build{
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					},
				}), should.BeNil)
				req := &pb.CancelBuildRequest{
					Id:              1,
					SummaryMarkdown: "summary",
				}
				rsp, err := srv.CancelBuild(ctx, req)
				assert.Loosely(t, err, should.ErrLike("does not have permission"))
				assert.Loosely(t, rsp, should.BeNil)
				assert.Loosely(t, sch.Tasks(), should.BeEmpty)
			})

			t.Run("found", func(t *ftt.Test) {
				now := testclock.TestRecentTimeLocal
				ctx, _ = testclock.UseTime(ctx, now)
				testutil.PutBucket(ctx, "project", "bucket", nil)
				build := &model.Build{
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Status: pb.Status_STARTED,
						Input: &pb.Build_Input{
							GerritChanges: []*pb.GerritChange{
								{Host: "h1"},
								{Host: "h2"},
							},
						},
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

				t.Run("found with BuildsList permission only", func(t *ftt.Test) {
					ctx = auth.WithState(ctx, &authtest.FakeState{
						Identity: userID,
						FakeDB: authtest.NewFakeDB(
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildsList),
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildsCancel),
						),
					})
					req := &pb.CancelBuildRequest{
						Id:              1,
						SummaryMarkdown: "summary",
						Mask: &pb.BuildMask{
							AllFields: true,
						},
					}
					rsp, err := srv.CancelBuild(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rsp, should.Match(&pb.Build{
						Id:     1,
						Status: pb.Status_STARTED,
					}))
					assert.Loosely(t, sch.Tasks(), should.HaveLength(1))
				})

				t.Run("found with BuildsGetLimited permission only", func(t *ftt.Test) {
					ctx = auth.WithState(ctx, &authtest.FakeState{
						Identity: userID,
						FakeDB: authtest.NewFakeDB(
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGetLimited),
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildsCancel),
						),
					})
					req := &pb.CancelBuildRequest{
						Id:              1,
						SummaryMarkdown: "summary",
						Mask: &pb.BuildMask{
							AllFields: true,
						},
					}
					rsp, err := srv.CancelBuild(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rsp, should.Match(&pb.Build{
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
						UpdateTime: timestamppb.New(now),
						CancelTime: timestamppb.New(now),
						Status:     pb.Status_STARTED,
					}))
					assert.Loosely(t, sch.Tasks(), should.HaveLength(1))
				})

				t.Run("found with BuildsGet permission", func(t *ftt.Test) {
					ctx = auth.WithState(ctx, &authtest.FakeState{
						Identity: userID,
						FakeDB: authtest.NewFakeDB(
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildsCancel),
						),
					})
					req := &pb.CancelBuildRequest{
						Id:              1,
						SummaryMarkdown: "summary",
						Mask: &pb.BuildMask{
							Fields: &fieldmaskpb.FieldMask{
								Paths: []string{
									"id",
									"builder",
									"update_time",
									"cancel_time",
									"status",
									"cancellation_markdown",
								},
							},
						},
					}
					rsp, err := srv.CancelBuild(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rsp, should.Match(&pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						UpdateTime:           timestamppb.New(now),
						CancelTime:           timestamppb.New(now),
						Status:               pb.Status_STARTED,
						CancellationMarkdown: "summary",
					}))
					assert.Loosely(t, sch.Tasks(), should.HaveLength(1))
				})
			})
		})
	})
}
