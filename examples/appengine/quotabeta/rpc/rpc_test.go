// Copyright 2019 The LUCI Authors.
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
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gomodule/redigo/redis"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	quota "go.chromium.org/luci/server/quotabeta"
	pb "go.chromium.org/luci/server/quotabeta/proto"
	"go.chromium.org/luci/server/quotabeta/quotaconfig"
	"go.chromium.org/luci/server/redisconn"
)

func TestDemo(t *testing.T) {
	t.Parallel()

	ftt.Run("Demo", t, func(t *ftt.Test) {
		// Fix clock to control quota replenishment over time.
		ctx, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeLocal)

		// Fix user for per-user calls.
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:caller@example.com",
		})

		// Set up in-memory Redis instance.
		s, err := miniredis.Run()
		assert.Loosely(t, err, should.BeNil)
		defer s.Close()
		ctx = redisconn.UsePool(ctx, &redis.Pool{
			Dial: func() (redis.Conn, error) {
				return redis.Dial("tcp", s.Addr())
			},
		})

		// Set up an in-memory quotaconfig.Interface with policies expected by the
		// Demo service.
		m, err := quotaconfig.NewMemory(ctx, []*pb.Policy{
			{
				Name:          "global-rate-limit",
				Resources:     60,
				Replenishment: 1,
			},
			{
				Name:          "per-user-rate-limit/${user}",
				Resources:     60,
				Replenishment: 1,
			},
		})
		assert.Loosely(t, err, should.BeNil)
		ctx = quota.Use(ctx, m)
		srv := &Demo{}

		t.Run("GlobalRateLimit", func(t *ftt.Test) {
			t.Run("error", func(t *ftt.Test) {
				_, err := srv.GlobalRateLimit(quota.Use(context.Background(), m), nil)
				assert.Loosely(t, err, should.ErrLike("connection pool is not configured"))
			})

			t.Run("ok", func(t *ftt.Test) {
				_, err := srv.GlobalRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("rate limit", func(t *ftt.Test) {
				// Ensure quota is exhausted.
				_, err := srv.GlobalRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.GlobalRateLimit(ctx, nil)
				assert.Loosely(t, err, should.ErrLike("global rate limit exceeded"))
			})

			t.Run("replenish", func(t *ftt.Test) {
				// Ensure quota is exhausted.
				_, err := srv.GlobalRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.GlobalRateLimit(ctx, nil)
				assert.Loosely(t, err, should.ErrLike("global rate limit exceeded"))

				// Ensure quota is replenished.
				ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeLocal.Add(time.Minute))
				_, err = srv.GlobalRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.GlobalRateLimit(ctx, nil)
				assert.Loosely(t, err, should.ErrLike("global rate limit exceeded"))
			})

			t.Run("replenish excess", func(t *ftt.Test) {
				// Ensure quota is exhausted.
				_, err := srv.GlobalRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.GlobalRateLimit(ctx, nil)
				assert.Loosely(t, err, should.ErrLike("global rate limit exceeded"))

				// Ensure quota is replenished only up to the cap.
				ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeLocal.Add(time.Hour))
				_, err = srv.GlobalRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.GlobalRateLimit(ctx, nil)
				assert.Loosely(t, err, should.ErrLike("global rate limit exceeded"))
			})

			t.Run("per-user", func(t *ftt.Test) {
				// Ensure quota is exhausted.
				_, err := srv.GlobalRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.GlobalRateLimit(ctx, nil)
				assert.Loosely(t, err, should.ErrLike("global rate limit exceeded"))

				// Ensure user is irrelevant.
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:other@example.com",
				})
				_, err = srv.GlobalRateLimit(ctx, nil)
				assert.Loosely(t, err, should.ErrLike("global rate limit exceeded"))
			})
		})

		t.Run("GlobalQuotaReset", func(t *ftt.Test) {
			t.Run("error", func(t *ftt.Test) {
				_, err := srv.GlobalQuotaReset(quota.Use(context.Background(), m), nil)
				assert.Loosely(t, err, should.ErrLike("connection pool is not configured"))
			})

			t.Run("ok", func(t *ftt.Test) {
				// Ensure quota is exhausted.
				_, err := srv.GlobalRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.GlobalRateLimit(ctx, nil)
				assert.Loosely(t, err, should.ErrLike("global rate limit exceeded"))

				// Ensure quota is reset.
				_, err = srv.GlobalQuotaReset(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.GlobalRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("reset cap", func(t *ftt.Test) {
				// Ensure quota is exhausted.
				_, err := srv.GlobalRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.GlobalRateLimit(ctx, nil)
				assert.Loosely(t, err, should.ErrLike("global rate limit exceeded"))

				// Reset multiple times.
				_, err = srv.GlobalQuotaReset(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.GlobalQuotaReset(ctx, nil)
				assert.Loosely(t, err, should.BeNil)

				// Ensure only enough quota for one call.
				_, err = srv.GlobalRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.GlobalRateLimit(ctx, nil)
				assert.Loosely(t, err, should.ErrLike("global rate limit exceeded"))
			})
		})

		t.Run("PerUserRateLimit", func(t *ftt.Test) {
			t.Run("error", func(t *ftt.Test) {
				_, err := srv.PerUserRateLimit(quota.Use(context.Background(), m), nil)
				assert.Loosely(t, err, should.ErrLike("connection pool is not configured"))
			})

			t.Run("ok", func(t *ftt.Test) {
				_, err := srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("rate limit", func(t *ftt.Test) {
				// Ensure quota is exhausted.
				_, err := srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.ErrLike("per-user rate limit exceeded"))
			})

			t.Run("replenish", func(t *ftt.Test) {
				// Ensure quota is exhausted.
				_, err := srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.ErrLike("per-user rate limit exceeded"))

				// Ensure quota is replenished.
				ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeLocal.Add(time.Minute))
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.ErrLike("per-user rate limit exceeded"))
			})

			t.Run("per-user", func(t *ftt.Test) {
				// Ensure quota is exhausted.
				_, err := srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.ErrLike("per-user rate limit exceeded"))

				// Ensure another user's quota isn't impacted.
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:other@example.com",
				})
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.ErrLike("per-user rate limit exceeded"))
			})

			t.Run("per-user replenishment", func(t *ftt.Test) {
				// Timeline for two users, "caller" and "other".
				// t:     caller's quota is reduced to 0, other's quota is 60.
				// t+30s: caller's quota is replenished to 30, other's quota is reduced to 0.
				// t+60s: caller's quota is replenished to 60, other's quota is replenished to 30.

				// Ensure quota is exhausted.
				_, err := srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.ErrLike("per-user rate limit exceeded"))

				// Replenish enough quota for one call.
				ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeLocal.Add(30*time.Second))

				// Ensure another user has quota for two calls.
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:other@example.com",
				})
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.ErrLike("per-user rate limit exceeded"))

				// Replenish enough quota for one more call.
				ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeLocal.Add(60*time.Second))

				// Ensure original user has quota for two calls.
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:caller@example.com",
				})
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.ErrLike("per-user rate limit exceeded"))

				// Ensure secondary user has quota for one call.
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:other@example.com",
				})
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.ErrLike("per-user rate limit exceeded"))
			})
		})

		t.Run("PerUserQuotaReset", func(t *ftt.Test) {
			t.Run("error", func(t *ftt.Test) {
				_, err := srv.PerUserQuotaReset(quota.Use(context.Background(), m), nil)
				assert.Loosely(t, err, should.ErrLike("connection pool is not configured"))
			})

			t.Run("ok", func(t *ftt.Test) {
				// Ensure quota is exhausted.
				_, err := srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.ErrLike("per-user rate limit exceeded"))

				// Ensure quota is reset.
				_, err = srv.PerUserQuotaReset(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("reset cap", func(t *ftt.Test) {
				// Ensure quota is exhausted.
				_, err := srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.ErrLike("per-user rate limit exceeded"))

				// Reset multiple times.
				_, err = srv.PerUserQuotaReset(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.PerUserQuotaReset(ctx, nil)
				assert.Loosely(t, err, should.BeNil)

				// Ensure only enough quota for two calls.
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.ErrLike("per-user rate limit exceeded"))
			})

			t.Run("per-user", func(t *ftt.Test) {
				// Ensure quota is exhausted.
				_, err := srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.BeNil)
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.ErrLike("per-user rate limit exceeded"))

				// Replenish another user's quota.
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:other@example.com",
				})
				_, err = srv.PerUserQuotaReset(ctx, nil)
				assert.Loosely(t, err, should.BeNil)

				// Ensure original user's quota is still exhausted.
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:caller@example.com",
				})
				_, err = srv.PerUserRateLimit(ctx, nil)
				assert.Loosely(t, err, should.ErrLike("per-user rate limit exceeded"))
			})
		})
	})
}
