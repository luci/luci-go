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

package recorder

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestValidateBatchCreateWorkUnitsPermissions(t *testing.T) {
	t.Parallel()

	ftt.Run(`ValidateBatchCreateWorkUnitsPermissions`, t, func(t *ftt.Test) {
		basePerms := []authtest.RealmPermission{
			{Realm: "project:realm", Permission: permCreateWorkUnit},
			{Realm: "project:realm", Permission: permIncludeWorkUnit},
		}

		authState := &authtest.FakeState{
			Identity:            "user:someone@example.com",
			IdentityPermissions: basePerms,
		}
		ctx := auth.WithState(context.Background(), authState)

		req := &pb.BatchCreateWorkUnitsRequest{
			Requests: []*pb.CreateWorkUnitRequest{
				{
					Parent:     "rootInvocations/u-my-root-id/workUnits/root",
					WorkUnitId: "u-wu1",
					WorkUnit: &pb.WorkUnit{
						Realm: "project:realm",
					},
				},
				{
					Parent:     "rootInvocations/u-my-root-id/workUnits/root",
					WorkUnitId: "u-wu2",
					WorkUnit: &pb.WorkUnit{
						Realm: "project:realm",
					},
				},
			},
			RequestId: "request-id",
		}

		t.Run("valid", func(t *ftt.Test) {
			err := validateBatchCreateWorkUnitsPermissions(ctx, req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("no requests", func(t *ftt.Test) {
			req.Requests = nil
			err := validateBatchCreateWorkUnitsPermissions(ctx, req)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("requests: must have at least one request"))
		})
		t.Run("too many requests", func(t *ftt.Test) {
			req.Requests = make([]*pb.CreateWorkUnitRequest, 501)
			err := validateBatchCreateWorkUnitsPermissions(ctx, req)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("requests: the number of requests in the batch (501) exceeds 500"))
		})

		t.Run("permission denied on one request", func(t *ftt.Test) {
			// We do not need to test all cases, as the verifyCreateWorkUnitPermissions
			// has its own tests.
			req.Requests[1].WorkUnit.Realm = "project:otherrealm"
			err := validateBatchCreateWorkUnitsPermissions(ctx, req)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike(`requests[1]: caller does not have permission "resultdb.workUnits.create" in realm "project:otherrealm"`))
		})
	})
}

func TestValidateBatchCreateWorkUnitsRequest(t *testing.T) {
	t.Parallel()

	ftt.Run(`ValidateBatchCreateWorkUnitsRequest`, t, func(t *ftt.Test) {
		req := &pb.BatchCreateWorkUnitsRequest{
			Requests: []*pb.CreateWorkUnitRequest{
				{
					Parent:     "rootInvocations/u-my-root-id/workUnits/root",
					WorkUnitId: "u-wu1",
					WorkUnit: &pb.WorkUnit{
						Realm: "project:realm",
					},
				},
				{
					Parent:     "rootInvocations/u-my-root-id/workUnits/root",
					WorkUnitId: "u-wu2",
					WorkUnit: &pb.WorkUnit{
						Realm: "project:realm",
					},
				},
			},
			RequestId: "request-id",
		}

		t.Run("valid", func(t *ftt.Test) {
			err := validateBatchCreateWorkUnitsRequest(req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("request_id", func(t *ftt.Test) {
			t.Run("unspecified", func(t *ftt.Test) {
				req.RequestId = ""
				err := validateBatchCreateWorkUnitsRequest(req)
				assert.Loosely(t, err, should.ErrLike("request_id: unspecified"))
			})
			t.Run("invalid", func(t *ftt.Test) {
				req.RequestId = "😃"
				err := validateBatchCreateWorkUnitsRequest(req)
				assert.Loosely(t, err, should.ErrLike("request_id: does not match"))
			})
		})

		t.Run("no requests", func(t *ftt.Test) {
			req.Requests = nil
			err := validateBatchCreateWorkUnitsRequest(req)
			assert.Loosely(t, err, should.ErrLike("requests: must have at least one request"))
		})

		t.Run("too many requests", func(t *ftt.Test) {
			req.Requests = make([]*pb.CreateWorkUnitRequest, 501)
			err := validateBatchCreateWorkUnitsRequest(req)
			assert.Loosely(t, err, should.ErrLike("requests: the number of requests in the batch (501) exceeds 500"))
		})

		t.Run("sub-request", func(t *ftt.Test) {
			t.Run("invalid", func(t *ftt.Test) {
				req.Requests[1].WorkUnitId = "invalid id"
				err := validateBatchCreateWorkUnitsRequest(req)
				assert.Loosely(t, err, should.ErrLike("requests[1]: work_unit_id: does not match"))
			})
			t.Run("request_id", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.Requests[0].RequestId = ""
					err := validateBatchCreateWorkUnitsRequest(req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("inconsistent", func(t *ftt.Test) {
					req.Requests[0].RequestId = "another-id"
					err := validateBatchCreateWorkUnitsRequest(req)
					assert.Loosely(t, err, should.ErrLike("requests[0]: request_id: inconsistent with top-level request_id"))
				})
				t.Run("consistent", func(t *ftt.Test) {
					req.Requests[0].RequestId = req.RequestId
					err := validateBatchCreateWorkUnitsRequest(req)
					assert.Loosely(t, err, should.BeNil)
				})
			})
		})
	})
}
