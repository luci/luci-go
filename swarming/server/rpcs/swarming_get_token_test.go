// Copyright 2024 The LUCI Authors.
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

package rpcs

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/caching"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/model"
)

func TestGetToken(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())
	ctx = caching.WithEmptyProcessCache(ctx)
	ctx, _ = testclock.UseTime(ctx, TestTime)

	err := datastore.Put(ctx, &model.LegacyBootstrapSecret{
		Key:    model.LegacyBootstrapSecretKey(ctx),
		Values: [][]byte{{0, 1, 2, 3, 4}},
	})
	assert.Loosely(t, err, should.BeNil)

	srv := SwarmingServer{}

	ftt.Run("Authorized", t, func(t *ftt.Test) {
		state := NewMockedRequestState().SetCaller(AdminFakeCaller)
		resp, err := srv.GetToken(MockRequestState(ctx, state), nil)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, resp, should.Match(&apipb.BootstrapToken{
			BootstrapToken: "AXsiX2kiOiIxNjcyNTM4NTg0MDAwIiwiZm9yIjoidXNlcjphZG1pbkBleGFtcGxlLmNvbSJ9i3LA39OM5O41TJCkK4587wYA-_GUASEHqnIlCjZ7QGA",
		}))
	})

	ftt.Run("Not authorized", t, func(t *ftt.Test) {
		state := NewMockedRequestState()
		_, err := srv.GetDetails(MockRequestState(ctx, state), nil)
		assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
	})
}
