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

package backend

import (
	"context"
	"fmt"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/buildbucket/bbperms"
	. "go.chromium.org/luci/common/testing/assertions"
	milopb "go.chromium.org/luci/milo/api/service/v1"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"google.golang.org/grpc/codes"
)

func TestBatchCheckPermissions(t *testing.T) {
	t.Parallel()

	Convey(`TestBatchCheckPermissions`, t, func() {
		ctx := context.Background()
		ctx = common.SetUpTestGlobalCache(ctx)
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

		Convey(`e2e`, func() {
			res, err := srv.BatchCheckPermissions(ctx, &milopb.BatchCheckPermissionsRequest{
				Realm:       "testproject:testrealm",
				Permissions: []string{bbperms.BuildsAdd.Name(), bbperms.BuildsList.Name(), bbperms.BuildsCancel.Name()},
			})

			So(err, ShouldBeNil)
			So(res.Results, ShouldResemble, map[string]bool{
				bbperms.BuildsAdd.Name():    true,
				bbperms.BuildsList.Name():   true,
				bbperms.BuildsCancel.Name(): false,
			})
		})

		Convey(`invalid request`, func() {
			res, err := srv.BatchCheckPermissions(ctx, &milopb.BatchCheckPermissionsRequest{
				Permissions: []string{bbperms.BuildsAdd.Name(), bbperms.BuildsList.Name(), bbperms.BuildsCancel.Name()},
			})

			So(err, ShouldHaveAppStatus, codes.InvalidArgument)
			So(err, ShouldErrLike, "realm", "must be specified")
			So(res, ShouldBeNil)
		})

		Convey(`invalid permission`, func() {
			res, err := srv.BatchCheckPermissions(ctx, &milopb.BatchCheckPermissionsRequest{
				Realm:       "testproject:testrealm",
				Permissions: []string{bbperms.BuildsAdd.Name(), "testservice.testsubject.testaction"},
			})

			So(err, ShouldHaveAppStatus, codes.InvalidArgument)
			So(err, ShouldErrLike, "testservice.testsubject.testaction", "permission not registered")
			So(res, ShouldBeNil)
		})
	})
}

func TestValidateBatchCheckPermissionsRequest(t *testing.T) {
	t.Parallel()

	Convey(`TestValidateBatchCheckPermissionsRequest`, t, func() {
		req := &milopb.BatchCheckPermissionsRequest{
			Realm:       "testproject:testrealm",
			Permissions: []string{bbperms.BuildsAdd.Name(), bbperms.BuildsList.Name()},
		}

		Convey(`valid`, func() {
			err := validateBatchCheckPermissionsRequest(req)
			So(err, ShouldBeNil)
		})

		Convey(`no realm`, func() {
			req.Realm = ""
			err := validateBatchCheckPermissionsRequest(req)
			So(err, ShouldErrLike, "realm:", "must be specified")
		})

		Convey(`invalid realm`, func() {
			req.Realm = "testinvalidrealm"
			err := validateBatchCheckPermissionsRequest(req)
			So(err, ShouldErrLike, "realm:", "testinvalidrealm", "should be <project>:<realm>")
		})

		Convey(`too many permissions`, func() {
			perms := strings.Split(strings.Repeat(bbperms.BuildsAdd.Name()+" ", maxPermissions+1), " ")
			// Trim the last one because it's an empty string.
			req.Permissions = perms[0 : maxPermissions+1]
			err := validateBatchCheckPermissionsRequest(req)
			So(err, ShouldErrLike, "permissions:", fmt.Sprintf("at most %d", maxPermissions))
		})
	})
}
