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

package rpcquota

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gomodule/redigo/redis"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	quota "go.chromium.org/luci/server/quotabeta"
	quotapb "go.chromium.org/luci/server/quotabeta/proto"
	"go.chromium.org/luci/server/quotabeta/quotaconfig"
	"go.chromium.org/luci/server/redisconn"

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

// testQuotaContext returns a context with the given policies.
func testQuotaContext(ctx context.Context, t testing.TB, policies []*quotapb.Policy) context.Context {
	// Create a miniredis instance for the luci-quota library to use.
	// Arguably it would be better to stub out the quota implementation
	// entirely, but that's not cleanly supported by luci-quota.
	s, err := miniredis.Run()
	assert.Loosely(t, err, should.BeNil)
	t.Cleanup(s.Close)
	ctx = redisconn.UsePool(ctx, &redis.Pool{
		Dial: func() (redis.Conn, error) {
			return redis.Dial("tcp", s.Addr())
		},
	})
	// Use a simple quota config that grants a generous 1k RPCs (per 10
	// minutes) to every 'user', which should be plenty for unit tests.
	quotacfg, err := quotaconfig.NewMemory(ctx, policies)
	assert.Loosely(t, err, should.BeNil)
	return quota.Use(ctx, quotacfg)
}

func TestUpdateUserQuota(t *testing.T) {
	ftt.Run(`SpecificUser`, t, func(t *ftt.Test) {
		ctx := testQuotaContext(testutil.TestingContext(), t, []*quotapb.Policy{
			{
				Name:          "someresource/user/aaa/example/com",
				Resources:     1000,
				Replenishment: 20,
			},
		})
		ctx = auth.WithState(ctx, &authtest.FakeState{Identity: "user:aaa@example.com"})
		t.Run(`Enough quota`, func(t *ftt.Test) {
			err := UpdateUserQuota(ctx, "someresource/", 1, "svc", "meth")
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run(`Insufficient quota`, func(t *ftt.Test) {
			err := UpdateUserQuota(ctx, "someresource/", 9999, "svc", "meth")
			assert.Loosely(t, err, convey.Adapt(ShouldHaveAppStatus)(codes.ResourceExhausted))
		})
		t.Run(`No matching policy`, func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{Identity: "user:bbb@example.com"})
			err := UpdateUserQuota(ctx, "someresource/", 9999, "svc", "meth")
			assert.Loosely(t, err, convey.Adapt(ShouldHaveAppStatus)(codes.ResourceExhausted))
		})
		t.Run(`Track only`, func(t *ftt.Test) {
			ctx := context.WithValue(ctx, &quotaTrackOnlyKey, true)
			err := UpdateUserQuota(ctx, "someresource/", 9999, "svc", "meth")
			assert.Loosely(t, err, should.BeNil)
		})
	})
	ftt.Run(`WildcardFallback`, t, func(t *ftt.Test) {
		ctx := testQuotaContext(testutil.TestingContext(), t, []*quotapb.Policy{
			{
				Name:          "someresource/${user}",
				Resources:     1000,
				Replenishment: 20,
			},
			{
				Name:          "someresource/user/aaa/example/com",
				Resources:     1000,
				Replenishment: 20,
			},
		})
		ctx = auth.WithState(ctx, &authtest.FakeState{Identity: "user:bbb@example.com"})
		t.Run(`Enough quota`, func(t *ftt.Test) {
			err := UpdateUserQuota(ctx, "someresource/", 1, "svc", "meth")
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run(`Insufficient quota`, func(t *ftt.Test) {
			err := UpdateUserQuota(ctx, "someresource/", 9999, "svc", "meth")
			assert.Loosely(t, err, convey.Adapt(ShouldHaveAppStatus)(codes.ResourceExhausted))
		})
	})

}
