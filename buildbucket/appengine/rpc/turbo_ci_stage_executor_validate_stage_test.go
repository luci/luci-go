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
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/buildbucket/appengine/rpc/testutil"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestValidateStage(t *testing.T) {
	t.Parallel()

	ftt.Run("validateStage", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		t.Run("no call info", func(t *ftt.Test) {
			assert.That(t, validateStage(ctx), should.ErrLike("missing call info"))
		})

		t.Run("with template_build_id", func(t *ftt.Test) {
			req := &pb.ScheduleBuildRequest{
				TemplateBuildId: 1,
			}
			ctx := context.WithValue(ctx, &turboCICallKey, &TurboCICallInfo{ScheduleBuild: req})
			assert.That(t, validateStage(ctx), should.ErrLike("Buildbucket stage with template_build_id is not supported"))
		})

		t.Run("with parent_build_id", func(t *ftt.Test) {
			req := &pb.ScheduleBuildRequest{
				ParentBuildId: 1,
			}
			ctx := context.WithValue(ctx, &turboCICallKey, &TurboCICallInfo{ScheduleBuild: req})
			assert.That(t, validateStage(ctx), should.ErrLike("Buildbucket stage with parent_build_id is not supported"))
		})

		t.Run("with shadow_input", func(t *ftt.Test) {
			req := &pb.ScheduleBuildRequest{
				ShadowInput: &pb.ScheduleBuildRequest_ShadowInput{},
			}
			ctx := context.WithValue(ctx, &turboCICallKey, &TurboCICallInfo{ScheduleBuild: req})
			assert.That(t, validateStage(ctx), should.ErrLike("Buildbucket stage with shadow_input is not supported"))
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
			assert.That(t, validateStage(ctx), should.ErrLike("request_id cannot contain '/'"))
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

			assert.That(t, validateStage(ctx), should.ErrLike(`requested resource not found or "user:another-caller@example.com" does not have permission to view it`))
		})

		t.Run("valid", func(t *ftt.Test) {
			testutil.PutBucket(ctx, "project", "bucket", nil)
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

			assert.NoErr(t, validateStage(ctx))
		})
	})
}
