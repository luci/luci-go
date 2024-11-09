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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/xsrf"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"

	"go.chromium.org/luci/auth_service/api/internalspb"
)

func TestInternalsServer(t *testing.T) {
	t.Parallel()

	ftt.Run("With server", t, func(t *ftt.Test) {
		ctx := secrets.Use(context.Background(), &testsecrets.Store{})
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:someone@example.com",
		})

		srv := Server{}

		t.Run("RefreshXSRFToken: OK", func(t *ftt.Test) {
			goodTok, err := xsrf.Token(ctx)
			assert.Loosely(t, err, should.BeNil)

			resp, err := srv.RefreshXSRFToken(ctx, &internalspb.RefreshXSRFTokenRequest{
				XsrfToken: goodTok,
			})
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, xsrf.Check(ctx, resp.XsrfToken), should.BeNil)
		})

		t.Run("RefreshXSRFToken: bad token", func(t *ftt.Test) {
			_, err := srv.RefreshXSRFToken(ctx, &internalspb.RefreshXSRFTokenRequest{
				XsrfToken: "bad token",
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
		})
	})
}
