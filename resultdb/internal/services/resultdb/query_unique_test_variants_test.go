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

package resultdb

import (
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateQueryUniqueTestVariantsRequest(t *testing.T) {
	t.Parallel()

	Convey(`ValidateQueryUniqueTestVariantsRequest`, t, func() {
		Convey(`Valid`, func() {
			req := &pb.QueryUniqueTestVariantsRequest{
				Realm:    "testproject:testrealm",
				TestId:   "test-id",
				PageSize: 10,
			}
			So(validateQueryUniqueTestVariantsRequest(req), ShouldBeNil)
		})

		Convey(`Missing realm`, func() {
			req := &pb.QueryUniqueTestVariantsRequest{
				TestId:   "test-id",
				PageSize: 10,
			}
			So(validateQueryUniqueTestVariantsRequest(req), ShouldErrLike, "realm is required")
		})

		Convey(`Invalid realm`, func() {
			req := &pb.QueryUniqueTestVariantsRequest{
				Realm:    "invalid/realm",
				TestId:   "test-id",
				PageSize: 10,
			}
			So(validateQueryUniqueTestVariantsRequest(req), ShouldErrLike, "realm: bad global realm name")
		})

		Convey(`Bad page_size`, func() {
			req := &pb.QueryUniqueTestVariantsRequest{
				Realm:    "testproject:testrealm",
				TestId:   "test-id",
				PageSize: -10,
			}
			So(validateQueryUniqueTestVariantsRequest(req), ShouldErrLike, "page_size, if specified, must be a positive integer")
		})
	})
}

func TestQueryUniqueTestVariants(t *testing.T) {
	Convey(`TestQueryUniqueTestVariants`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		ctx = auth.WithState(
			ctx,
			&authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "testproject:testrealm", Permission: permListTestResults},
				},
			})
		srv := newTestResultDBService()

		Convey(`Authorized`, func() {
			req := &pb.QueryUniqueTestVariantsRequest{
				Realm:  "testproject:testrealm",
				TestId: "test-id",
			}
			_, err := srv.QueryUniqueTestVariants(ctx, req)
			So(err, ShouldBeNil)
		})

		Convey(`Unauthorized`, func() {
			req := &pb.QueryUniqueTestVariantsRequest{
				Realm:  "testproject:secretrealm",
				TestId: "test-id",
			}
			res, err := srv.QueryUniqueTestVariants(ctx, req)
			So(err, ShouldHaveAppStatus, codes.PermissionDenied)
			So(res, ShouldBeNil)
		})
	})
}
