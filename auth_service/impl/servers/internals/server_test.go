// Copyright 2021 The LUCI Authors.
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

package internals

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/xsrf"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"

	"go.chromium.org/luci/auth_service/api/internalspb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestInternalsServer(t *testing.T) {
	t.Parallel()

	Convey("With server", t, func() {
		ctx := secrets.Use(context.Background(), &testsecrets.Store{})
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:someone@example.com",
		})

		srv := Server{}

		Convey("RefreshXSRFToken: OK", func() {
			goodTok, err := xsrf.Token(ctx)
			So(err, ShouldBeNil)

			resp, err := srv.RefreshXSRFToken(ctx, &internalspb.RefreshXSRFTokenRequest{
				XsrfToken: goodTok,
			})
			So(err, ShouldBeNil)

			So(xsrf.Check(ctx, resp.XsrfToken), ShouldBeNil)
		})

		Convey("RefreshXSRFToken: bad token", func() {
			_, err := srv.RefreshXSRFToken(ctx, &internalspb.RefreshXSRFTokenRequest{
				XsrfToken: "bad token",
			})
			So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
		})
	})
}
