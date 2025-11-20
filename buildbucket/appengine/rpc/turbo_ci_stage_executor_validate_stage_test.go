// Copyright 2025 The LUCI Authors.
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

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/turboci/id"
	"google.golang.org/protobuf/types/known/anypb"

	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/rpc/testutil"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"
	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

func TestValidateStage(t *testing.T) {
	t.Parallel()

	ftt.Run("validateStage", t, func(t *ftt.Test) {
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		assert.NoErr(t, config.SetTestSettingsCfg(ctx, &pb.SettingsCfg{}))

		t.Run("with template_build_id", func(t *ftt.Test) {
			req := &pb.ScheduleBuildRequest{
				TemplateBuildId: 1,
			}
			ctx := context.WithValue(ctx, &turboCICallKey, &TurboCICallInfo{ScheduleBuild: req})
			assert.That(t, validateStage(ctx, nil), should.ErrLike("Buildbucket stage with template_build_id is not supported"))
		})

		t.Run("with parent_build_id", func(t *ftt.Test) {
			req := &pb.ScheduleBuildRequest{
				ParentBuildId: 1,
			}
			ctx := context.WithValue(ctx, &turboCICallKey, &TurboCICallInfo{ScheduleBuild: req})
			assert.That(t, validateStage(ctx, nil), should.ErrLike("Buildbucket stage with parent_build_id is not supported"))
		})

		t.Run("invalid schedule build request", func(t *ftt.Test) {
			req := &pb.ScheduleBuildRequest{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				RequestId: "invalid/request",
			}
			ctx := context.WithValue(ctx, &turboCICallKey, &TurboCICallInfo{ScheduleBuild: req})
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:test@example.com",
			})
			assert.That(t, validateStage(ctx, nil), should.ErrLike("request_id cannot contain '/'"))
		})

		t.Run("no permission", func(t *ftt.Test) {
			testutil.PutBucket(ctx, "project", "bucket", nil)
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: identity.Identity("user:another-caller@example.com"),
				FakeDB: authtest.NewFakeDB(
					authtest.MockPermission(identity.Identity("user:caller@example.com"), "project:bucket", bbperms.BuildsAdd),
				),
			})

			req := &pb.ScheduleBuildRequest{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			}
			ctx = context.WithValue(ctx, &turboCICallKey, &TurboCICallInfo{ScheduleBuild: req})
			assert.That(t, validateStage(ctx, nil), should.ErrLike(`requested resource not found or "user:another-caller@example.com" does not have permission to view it`))
		})

		t.Run("valid", func(t *ftt.Test) {
			testutil.PutBucket(ctx, "project", "bucket", &pb.Bucket{
				Swarming: &pb.Swarming{},
			})
			testutil.PutBuilder(ctx, "project", "bucket", "builder", "")
			userID := identity.Identity("user:caller@example.com")
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: userID,
				FakeDB: authtest.NewFakeDB(
					authtest.MockPermission(userID, "project:bucket", bbperms.BuildsAdd),
				),
			})

			req := &pb.ScheduleBuildRequest{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			}
			ctx = context.WithValue(ctx, &turboCICallKey, &TurboCICallInfo{ScheduleBuild: req})
			stg, err := makeStage(req, nil)
			assert.NoErr(t, err)
			assert.NoErr(t, validateStage(ctx, stg))
		})

		t.Run("with_parent", func(t *ftt.Test) {
			pStageAttemptIDStr := "L123456789:Sstage-id:A1"
			pStageAttemptID, err := id.FromString(pStageAttemptIDStr)
			assert.NoErr(t, err)

			pBld := &model.Build{
				ID:             87654321,
				StageAttemptID: pStageAttemptIDStr,
				Proto: &pb.Build{
					Id: 87654321,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "parent-builder",
					},
					Status: pb.Status_STARTED,
				},
			}
			pInfra := &model.BuildInfra{
				Build: datastore.KeyForObj(ctx, pBld),
				Proto: &pb.BuildInfra{},
			}

			req := &pb.ScheduleBuildRequest{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			}
			stg, err := makeStage(req, pStageAttemptID.GetStageAttempt())
			assert.Loosely(t, err, should.BeNil)
			testutil.PutBucket(ctx, "project", "bucket", &pb.Bucket{
				Swarming: &pb.Swarming{},
			})
			testutil.PutBuilder(ctx, "project", "bucket", "builder", "")

			t.Run("parent not found", func(t *ftt.Test) {
				ctx := context.WithValue(ctx, &turboCICallKey, &TurboCICallInfo{ScheduleBuild: req})
				assert.That(t, validateStage(ctx, stg), should.ErrLike(`expect 1 build by stage_attempt_id "L123456789:Sstage-id:A1", but got 0`))
			})

			t.Run("multiple builds with the same stage attempt ID", func(t *ftt.Test) {
				another := &model.Build{
					ID:             98765432,
					StageAttemptID: pStageAttemptIDStr,
					Proto: &pb.Build{
						Id: 98765432,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "parent-builder",
						},
						Status: pb.Status_STARTED,
					},
				}
				assert.NoErr(t, datastore.Put(ctx, pBld, another))
				ctx := context.WithValue(ctx, &turboCICallKey, &TurboCICallInfo{ScheduleBuild: req})
				assert.That(t, validateStage(ctx, stg), should.ErrLike(`expect 1 build by stage_attempt_id "L123456789:Sstage-id:A1", but got 2`))
			})

			t.Run("parent ended", func(t *ftt.Test) {
				pBld.Proto.Status = pb.Status_SUCCESS
				assert.NoErr(t, datastore.Put(ctx, pBld, pInfra))
				ctx := context.WithValue(ctx, &turboCICallKey, &TurboCICallInfo{ScheduleBuild: req})
				assert.That(t, validateStage(ctx, stg), should.ErrLike("has ended, cannot add child to it"))
			})

			t.Run("valid parent", func(t *ftt.Test) {
				pBld.Proto.Status = pb.Status_STARTED
				assert.NoErr(t, datastore.Put(ctx, pBld, pInfra))
				userID := identity.Identity("user:caller@example.com")
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: userID,
					FakeDB: authtest.NewFakeDB(
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildsAdd),
					),
				})
				ctx = context.WithValue(ctx, &turboCICallKey, &TurboCICallInfo{ScheduleBuild: req})
				assert.NoErr(t, validateStage(ctx, stg))
			})
		})
	})
}

func makeStage(req *pb.ScheduleBuildRequest, pStageAttempt *idspb.StageAttempt) (*orchestratorpb.Stage, error) {
	value, err := anypb.New(req)
	if err != nil {
		return nil, err
	}
	stgBldr := orchestratorpb.Stage_builder{
		Args: orchestratorpb.Value_builder{
			Value: value,
		}.Build(),
	}

	if pStageAttempt != nil {
		stgBldr.CreatedBy = orchestratorpb.Actor_builder{
			StageAttempt: pStageAttempt,
		}.Build()
	}

	return stgBldr.Build(), nil
}
