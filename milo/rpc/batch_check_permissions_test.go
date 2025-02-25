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

package rpc

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/buildbucket/bbperms"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/milo/internal/testutils"
	milopb "go.chromium.org/luci/milo/proto/v1"
)

func TestBatchCheckPermissions(t *testing.T) {
	t.Parallel()

	ftt.Run(`TestBatchCheckPermissions`, t, func(t *ftt.Test) {
		ctx := context.Background()
		ctx = testutils.SetUpTestGlobalCache(ctx)
		ctx = auth.WithState(
			ctx,
			&authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "testproject:testrealm", Permission: bbperms.BuildsList},
					{Realm: "testproject:testrealm", Permission: bbperms.BuildsAdd},
					{Realm: "testproject:testrealm2", Permission: bbperms.BuildsCancel},
				},
			})

		srv := &MiloInternalService{}

		t.Run(`e2e`, func(t *ftt.Test) {
			res, err := srv.BatchCheckPermissions(ctx, &milopb.BatchCheckPermissionsRequest{
				Realm:       "testproject:testrealm",
				Permissions: []string{bbperms.BuildsAdd.Name(), bbperms.BuildsList.Name(), bbperms.BuildsCancel.Name()},
			})

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Results, should.Match(map[string]bool{
				bbperms.BuildsAdd.Name():    true,
				bbperms.BuildsList.Name():   true,
				bbperms.BuildsCancel.Name(): false,
			}))
		})

		t.Run(`invalid request`, func(t *ftt.Test) {
			res, err := srv.BatchCheckPermissions(ctx, &milopb.BatchCheckPermissionsRequest{
				Permissions: []string{bbperms.BuildsAdd.Name(), bbperms.BuildsList.Name(), bbperms.BuildsCancel.Name()},
			})

			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("must be specified"))
			assert.Loosely(t, res, should.BeNil)
		})

		t.Run(`invalid permission`, func(t *ftt.Test) {
			res, err := srv.BatchCheckPermissions(ctx, &milopb.BatchCheckPermissionsRequest{
				Realm:       "testproject:testrealm",
				Permissions: []string{bbperms.BuildsAdd.Name(), "testservice.testsubject.testaction"},
			})

			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("permission not registered"))
			assert.Loosely(t, res, should.BeNil)
		})
	})
}

func TestValidateBatchCheckPermissionsRequest(t *testing.T) {
	t.Parallel()

	ftt.Run(`TestValidateBatchCheckPermissionsRequest`, t, func(t *ftt.Test) {
		req := &milopb.BatchCheckPermissionsRequest{
			Realm:       "testproject:testrealm",
			Permissions: []string{bbperms.BuildsAdd.Name(), bbperms.BuildsList.Name()},
		}

		t.Run(`valid`, func(t *ftt.Test) {
			err := validateBatchCheckPermissionsRequest(req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`no realm`, func(t *ftt.Test) {
			req.Realm = ""
			err := validateBatchCheckPermissionsRequest(req)
			assert.Loosely(t, err, should.ErrLike("must be specified"))
		})

		t.Run(`invalid realm`, func(t *ftt.Test) {
			req.Realm = "testinvalidrealm"
			err := validateBatchCheckPermissionsRequest(req)
			assert.Loosely(t, err, should.ErrLike("should be <project>:<realm>"))
		})

		t.Run(`too many permissions`, func(t *ftt.Test) {
			perms := strings.Split(strings.Repeat(bbperms.BuildsAdd.Name()+" ", maxPermissions+1), " ")
			// Trim the last one because it's an empty string.
			req.Permissions = perms[0 : maxPermissions+1]
			err := validateBatchCheckPermissionsRequest(req)
			assert.Loosely(t, err, should.ErrLike(fmt.Sprintf("at most %d", maxPermissions)))
		})
	})
}
