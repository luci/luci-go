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

package quota

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gomodule/redigo/redis"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/server/quotabeta/proto"
	"go.chromium.org/luci/server/quotabeta/quotaconfig"
	"go.chromium.org/luci/server/redisconn"
)

func TestQuotaAdmin(t *testing.T) {
	t.Parallel()

	ftt.Run("quotaAdmin", t, func(t *ftt.Test) {
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestRecentTimeLocal)
		now := strconv.FormatInt(tc.Now().Unix(), 10)
		s, err := miniredis.Run()
		assert.Loosely(t, err, should.BeNil)
		defer s.Close()
		ctx = redisconn.UsePool(ctx, &redis.Pool{
			Dial: func() (redis.Conn, error) {
				return redis.Dial("tcp", s.Addr())
			},
		})
		conn, err := redisconn.Get(ctx)
		assert.Loosely(t, err, should.BeNil)
		_, err = conn.Do("HINCRBY", "entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources", 3)
		assert.Loosely(t, err, should.BeNil)
		_, err = conn.Do("HINCRBY", "entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated", tc.Now().Unix())
		assert.Loosely(t, err, should.BeNil)
		m, err := quotaconfig.NewMemory(ctx, []*pb.Policy{
			{
				Name:          "quota",
				Resources:     2,
				Replenishment: 1,
			},
			{
				Name:          "quota/${user}",
				Resources:     5,
				Replenishment: 1,
			},
		})
		assert.Loosely(t, err, should.BeNil)
		ctx = Use(ctx, m)
		srv := &quotaAdmin{}

		t.Run("Get", func(t *ftt.Test) {
			t.Run("nil", func(t *ftt.Test) {
				rsp, err := srv.Get(ctx, nil)
				assert.Loosely(t, err, should.ErrLike("policy is required"))
				assert.Loosely(t, rsp, should.BeNil)
			})

			t.Run("empty", func(t *ftt.Test) {
				req := &pb.GetRequest{}
				rsp, err := srv.Get(ctx, req)
				assert.Loosely(t, err, should.ErrLike("policy is required"))
				assert.Loosely(t, rsp, should.BeNil)
			})

			t.Run("not found", func(t *ftt.Test) {
				req := &pb.GetRequest{
					Policy: "quota/user",
				}

				rsp, err := srv.Get(ctx, req)
				assert.Loosely(t, err, should.ErrLike("not found"))
				assert.Loosely(t, rsp, should.BeNil)
			})

			t.Run("user unspecified", func(t *ftt.Test) {
				req := &pb.GetRequest{
					Policy: "quota/${user}",
				}

				rsp, err := srv.Get(ctx, req)
				assert.Loosely(t, err, should.ErrLike("user not specified"))
				assert.Loosely(t, rsp, should.BeNil)
			})

			t.Run("new", func(t *ftt.Test) {
				req := &pb.GetRequest{
					Policy: "quota",
				}

				rsp, err := srv.Get(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rsp, should.Match(&pb.QuotaEntry{
					Name:      "quota",
					DbName:    "entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					Resources: 2,
				}))

				// Ensure an entry for "quota" was not written to the database.
				assert.Loosely(t, s.Keys(), should.Match([]string{
					"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
				}))
				assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), should.Equal("3"))
				assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), should.Equal(now))
			})

			t.Run("existing", func(t *ftt.Test) {
				req := &pb.GetRequest{
					Policy: "quota/${user}",
					User:   "user@example.com",
				}

				rsp, err := srv.Get(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rsp, should.Match(&pb.QuotaEntry{
					Name:      "quota/user@example.com",
					DbName:    "entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
					Resources: 3,
				}))

				assert.Loosely(t, s.Keys(), should.Match([]string{
					"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
				}))
				assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), should.Equal("3"))
				assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), should.Equal(now))
			})

			t.Run("replenish", func(t *ftt.Test) {
				ctx, _ := testclock.UseTime(ctx, testclock.TestRecentTimeLocal.Add(time.Second))
				req := &pb.GetRequest{
					Policy: "quota/${user}",
					User:   "user@example.com",
				}

				rsp, err := srv.Get(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rsp, should.Match(&pb.QuotaEntry{
					Name:      "quota/user@example.com",
					DbName:    "entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
					Resources: 4,
				}))

				// Ensure replenishment was not written to the database.
				assert.Loosely(t, s.Keys(), should.Match([]string{
					"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
				}))
				assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), should.Equal("3"))
				assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), should.Equal(now))
			})
		})

		t.Run("Set", func(t *ftt.Test) {
			t.Run("nil", func(t *ftt.Test) {
				rsp, err := srv.Set(ctx, nil)
				assert.Loosely(t, err, should.ErrLike("policy is required"))
				assert.Loosely(t, rsp, should.BeNil)
			})

			t.Run("empty", func(t *ftt.Test) {
				req := &pb.SetRequest{}
				rsp, err := srv.Set(ctx, req)
				assert.Loosely(t, err, should.ErrLike("policy is required"))
				assert.Loosely(t, rsp, should.BeNil)
			})

			t.Run("negative", func(t *ftt.Test) {
				req := &pb.SetRequest{
					Policy:    "quota",
					Resources: -1,
				}
				rsp, err := srv.Set(ctx, req)
				assert.Loosely(t, err, should.ErrLike("resources must not be negative"))
				assert.Loosely(t, rsp, should.BeNil)
			})

			t.Run("not found", func(t *ftt.Test) {
				req := &pb.SetRequest{
					Policy: "quota/user",
				}

				rsp, err := srv.Set(ctx, req)
				assert.Loosely(t, err, should.ErrLike("not found"))
				assert.Loosely(t, rsp, should.BeNil)
			})

			t.Run("user unspecified", func(t *ftt.Test) {
				req := &pb.SetRequest{
					Policy: "quota/${user}",
				}

				rsp, err := srv.Set(ctx, req)
				assert.Loosely(t, err, should.ErrLike("user not specified"))
				assert.Loosely(t, rsp, should.BeNil)
			})

			t.Run("new", func(t *ftt.Test) {
				req := &pb.SetRequest{
					Policy:    "quota",
					Resources: 2,
				}

				rsp, err := srv.Set(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rsp, should.Match(&pb.QuotaEntry{
					Name:      "quota",
					DbName:    "entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					Resources: 2,
				}))

				assert.Loosely(t, s.Keys(), should.Match([]string{
					"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
				}))
				assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), should.Equal("2"))
				assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), should.Equal(now))
			})

			t.Run("existing", func(t *ftt.Test) {
				req := &pb.SetRequest{
					Policy:    "quota/${user}",
					User:      "user@example.com",
					Resources: 2,
				}

				rsp, err := srv.Set(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rsp, should.Match(&pb.QuotaEntry{
					Name:      "quota/user@example.com",
					DbName:    "entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
					Resources: 2,
				}))

				assert.Loosely(t, s.Keys(), should.Match([]string{
					"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
				}))
				assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), should.Equal("2"))
				assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), should.Equal(now))
			})

			t.Run("zero", func(t *ftt.Test) {
				req := &pb.SetRequest{
					Policy: "quota/${user}",
					User:   "user@example.com",
				}

				rsp, err := srv.Set(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rsp, should.Match(&pb.QuotaEntry{
					Name:   "quota/user@example.com",
					DbName: "entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
				}))

				assert.Loosely(t, s.Keys(), should.Match([]string{
					"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
				}))
				assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), should.Equal("0"))
				assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), should.Equal(now))
			})

			t.Run("excessive", func(t *ftt.Test) {
				req := &pb.SetRequest{
					Policy:    "quota",
					Resources: 10,
				}

				rsp, err := srv.Set(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rsp, should.Match(&pb.QuotaEntry{
					Name:      "quota",
					DbName:    "entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					Resources: 2,
				}))

				// Ensure resources were capped in the database.
				assert.Loosely(t, s.Keys(), should.Match([]string{
					"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
				}))
				assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), should.Equal("2"))
				assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), should.Equal(now))
			})
		})
	})
}
