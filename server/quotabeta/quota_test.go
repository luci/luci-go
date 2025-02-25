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

func TestQuota(t *testing.T) {
	t.Parallel()

	ftt.Run("getInterface", t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run("panic", func(t *ftt.Test) {
			shouldPanic := func() {
				getInterface(ctx)
			}
			assert.Loosely(t, shouldPanic, should.PanicLike("quotaconfig.Interface implementation not found"))
		})

		t.Run("ok", func(t *ftt.Test) {
			m, err := quotaconfig.NewMemory(ctx, nil)
			assert.Loosely(t, err, should.BeNil)
			ctx = Use(ctx, m)
			assert.Loosely(t, getInterface(ctx), should.NotBeNil)
		})
	})

	ftt.Run("UpdateQuota", t, func(t *ftt.Test) {
		s, err := miniredis.Run()
		assert.Loosely(t, err, should.BeNil)
		defer s.Close()
		ctx := redisconn.UsePool(context.Background(), &redis.Pool{
			Dial: func() (redis.Conn, error) {
				return redis.Dial("tcp", s.Addr())
			},
		})
		m, err := quotaconfig.NewMemory(ctx, []*pb.Policy{
			{
				Name:          "quota",
				Resources:     5,
				Replenishment: 1,
			},
			{
				Name:      "quota/${user}",
				Resources: 2,
			},
		})
		assert.Loosely(t, err, should.BeNil)
		ctx, tc := testclock.UseTime(Use(ctx, m), testclock.TestRecentTimeLocal)
		now := strconv.FormatInt(tc.Now().Unix(), 10)

		t.Run("empty database", func(t *ftt.Test) {
			t.Run("policy not found", func(t *ftt.Test) {
				up := map[string]int64{
					"fake": 0,
				}

				assert.Loosely(t, UpdateQuota(ctx, up, nil), should.ErrLike("not found"))
				assert.Loosely(t, s.Keys(), should.BeEmpty)
			})

			t.Run("user not specified", func(t *ftt.Test) {
				up := map[string]int64{
					"quota/${user}": 0,
				}

				assert.Loosely(t, UpdateQuota(ctx, up, nil), should.ErrLike("user unspecified"))
				assert.Loosely(t, s.Keys(), should.BeEmpty)
			})

			t.Run("capped", func(t *ftt.Test) {
				up := map[string]int64{
					"quota/${user}": 1,
				}
				opts := &Options{
					User: "user@example.com",
				}

				assert.Loosely(t, UpdateQuota(ctx, up, opts), should.BeNil)
				assert.Loosely(t, s.Keys(), should.Match([]string{
					"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
				}))
				assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), should.Equal("2"))
				assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), should.Equal(now))
			})

			t.Run("zero", func(t *ftt.Test) {
				up := map[string]int64{
					"quota/${user}": 0,
				}
				opts := &Options{
					User: "user@example.com",
				}

				assert.Loosely(t, UpdateQuota(ctx, up, opts), should.BeNil)
				assert.Loosely(t, s.Keys(), should.Match([]string{
					"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
				}))
				assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), should.Equal("2"))
				assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), should.Equal(now))
			})

			t.Run("debit one", func(t *ftt.Test) {
				up := map[string]int64{
					"quota/${user}": -1,
				}
				opts := &Options{
					User: "user@example.com",
				}

				assert.Loosely(t, UpdateQuota(ctx, up, opts), should.BeNil)
				assert.Loosely(t, s.Keys(), should.Match([]string{
					"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
				}))
				assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), should.Equal("1"))
				assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), should.Equal(now))
			})

			t.Run("debit all", func(t *ftt.Test) {
				up := map[string]int64{
					"quota/${user}": -2,
				}
				opts := &Options{
					User: "user@example.com",
				}

				assert.Loosely(t, UpdateQuota(ctx, up, opts), should.BeNil)
				assert.Loosely(t, s.Keys(), should.Match([]string{
					"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
				}))
				assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), should.Equal("0"))
				assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), should.Equal(now))
			})

			t.Run("debit excessive", func(t *ftt.Test) {
				up := map[string]int64{
					"quota/${user}": -3,
				}
				opts := &Options{
					User: "user@example.com",
				}

				assert.Loosely(t, UpdateQuota(ctx, up, opts), should.Equal(ErrInsufficientQuota))

				assert.Loosely(t, s.Keys(), should.BeEmpty)
			})

			t.Run("debit multiple", func(t *ftt.Test) {
				up := map[string]int64{
					"quota/${user}": -1,
				}
				opts := &Options{
					User: "user@example.com",
				}

				assert.Loosely(t, UpdateQuota(ctx, up, opts), should.BeNil)
				assert.Loosely(t, UpdateQuota(ctx, up, opts), should.BeNil)
				assert.Loosely(t, UpdateQuota(ctx, up, opts), should.Equal(ErrInsufficientQuota))
				assert.Loosely(t, s.Keys(), should.Match([]string{
					"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
				}))
				assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), should.Equal("0"))
				assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), should.Equal(now))
			})

			t.Run("atomicity", func(t *ftt.Test) {
				t.Run("policy not found", func(t *ftt.Test) {
					up := map[string]int64{
						"quota":         0,
						"quota/${user}": 0,
						"fake":          0,
					}
					opts := &Options{
						User: "user@example.com",
					}

					assert.Loosely(t, UpdateQuota(ctx, up, opts), should.ErrLike("not found"))
					assert.Loosely(t, s.Keys(), should.BeEmpty)
				})

				t.Run("user not specified", func(t *ftt.Test) {
					up := map[string]int64{
						"quota":         0,
						"quota/${user}": 0,
					}
					opts := &Options{}

					assert.Loosely(t, UpdateQuota(ctx, up, opts), should.ErrLike("user unspecified"))
					assert.Loosely(t, s.Keys(), should.BeEmpty)
				})

				t.Run("debit one", func(t *ftt.Test) {
					up := map[string]int64{
						"quota":         -1,
						"quota/${user}": -1,
					}
					opts := &Options{
						User: "user@example.com",
					}

					assert.Loosely(t, UpdateQuota(ctx, up, opts), should.BeNil)
					assert.Loosely(t, s.Keys(), should.Match([]string{
						"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
						"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
					}))
					assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), should.Equal("4"))
					assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), should.Equal(now))
					assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), should.Equal("1"))
					assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), should.Equal(now))
				})

				t.Run("debit excessive", func(t *ftt.Test) {
					up := map[string]int64{
						"quota":         -1,
						"quota/${user}": -3,
					}
					opts := &Options{
						User: "user@example.com",
					}

					assert.Loosely(t, UpdateQuota(ctx, up, opts), should.Equal(ErrInsufficientQuota))
					assert.Loosely(t, s.Keys(), should.BeEmpty)
				})
			})

			t.Run("idempotence", func(t *ftt.Test) {
				t.Run("deduplication", func(t *ftt.Test) {
					up := map[string]int64{
						"quota/${user}": -1,
					}
					opts := &Options{
						RequestID: "request-id",
						User:      "user@example.com",
					}

					assert.Loosely(t, UpdateQuota(ctx, up, opts), should.BeNil)
					assert.Loosely(t, s.Keys(), should.Match([]string{
						"deduplicationKeys",
						"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
					}))
					assert.Loosely(t, s.HGet("deduplicationKeys", "request-id"), should.Equal(strconv.FormatInt(testclock.TestRecentTimeLocal.Add(time.Hour).Unix(), 10)))
					assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), should.Equal("1"))
					assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), should.Equal(now))

					// Ensure update succeeds without modifying the database again.
					ctx, _ := testclock.UseTime(ctx, testclock.TestRecentTimeLocal.Add(30*time.Minute))
					assert.Loosely(t, UpdateQuota(ctx, up, opts), should.BeNil)
					assert.Loosely(t, s.Keys(), should.Match([]string{
						"deduplicationKeys",
						"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
					}))
					assert.Loosely(t, s.HGet("deduplicationKeys", "request-id"), should.Equal(strconv.FormatInt(testclock.TestRecentTimeLocal.Add(time.Hour).Unix(), 10)))
					assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), should.Equal("1"))
					assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), should.Equal(now))
				})

				t.Run("expired", func(t *ftt.Test) {
					up := map[string]int64{
						"quota/${user}": -1,
					}
					opts := &Options{
						RequestID: "request-id",
						User:      "user@example.com",
					}

					assert.Loosely(t, UpdateQuota(ctx, up, opts), should.BeNil)
					assert.Loosely(t, s.Keys(), should.Match([]string{
						"deduplicationKeys",
						"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
					}))
					assert.Loosely(t, s.HGet("deduplicationKeys", "request-id"), should.Equal(strconv.FormatInt(testclock.TestRecentTimeLocal.Add(time.Hour).Unix(), 10)))
					assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), should.Equal("1"))
					assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), should.Equal(now))

					// Ensure update modifies the database after the deduplication deadline.
					ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeLocal.Add(2*time.Hour))
					assert.Loosely(t, UpdateQuota(ctx, up, opts), should.BeNil)
					assert.Loosely(t, s.Keys(), should.Match([]string{
						"deduplicationKeys",
						"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
					}))
					assert.Loosely(t, s.HGet("deduplicationKeys", "request-id"), should.Equal(strconv.FormatInt(tc.Now().Add(time.Hour).Unix(), 10)))
					assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), should.Equal("0"))
					assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), should.Equal(strconv.FormatInt(tc.Now().Unix(), 10)))
				})

				t.Run("error", func(t *ftt.Test) {
					up := map[string]int64{
						"quota/${user}": -10,
					}
					opts := &Options{
						RequestID: "request-id",
						User:      "user@example.com",
					}

					assert.Loosely(t, UpdateQuota(ctx, up, opts), should.ErrLike("insufficient quota"))
					assert.Loosely(t, s.Keys(), should.Match([]string{
						"deduplicationKeys",
					}))
					assert.Loosely(t, s.HGet("deduplicationKeys", "request-id"), should.Equal("0"))

					up = map[string]int64{
						"quota/${user}": -1,
					}

					assert.Loosely(t, UpdateQuota(ctx, up, opts), should.BeNil)
					assert.Loosely(t, s.Keys(), should.Match([]string{
						"deduplicationKeys",
						"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
					}))
					assert.Loosely(t, s.HGet("deduplicationKeys", "request-id"), should.Equal(strconv.FormatInt(testclock.TestRecentTimeLocal.Add(time.Hour).Unix(), 10)))
					assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), should.Equal("1"))
					assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), should.Equal(now))
				})
			})
		})

		t.Run("existing database", func(t *ftt.Test) {
			conn, err := redisconn.Get(ctx)
			assert.Loosely(t, err, should.BeNil)
			_, err = conn.Do("HINCRBY", "entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources", 2)
			assert.Loosely(t, err, should.BeNil)
			_, err = conn.Do("HINCRBY", "entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated", tc.Now().Unix())
			assert.Loosely(t, err, should.BeNil)

			t.Run("capped", func(t *ftt.Test) {
				up := map[string]int64{
					"quota": 10,
				}
				opts := &Options{
					User: "user@example.com",
				}

				assert.Loosely(t, UpdateQuota(ctx, up, opts), should.BeNil)
				assert.Loosely(t, s.Keys(), should.Match([]string{
					"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
				}))
				assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), should.Equal("5"))
				assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), should.Equal(now))
			})

			t.Run("credit one", func(t *ftt.Test) {
				up := map[string]int64{
					"quota": 1,
				}
				opts := &Options{
					User: "user@example.com",
				}

				assert.Loosely(t, UpdateQuota(ctx, up, opts), should.BeNil)
				assert.Loosely(t, s.Keys(), should.Match([]string{
					"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
				}))
				assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), should.Equal("3"))
				assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), should.Equal(now))
			})

			t.Run("debit one", func(t *ftt.Test) {
				up := map[string]int64{
					"quota": -1,
				}
				opts := &Options{}

				assert.Loosely(t, UpdateQuota(ctx, up, opts), should.BeNil)
				assert.Loosely(t, s.Keys(), should.Match([]string{
					"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
				}))
				assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), should.Equal("1"))
				assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), should.Equal(now))
			})

			t.Run("debit excessive", func(t *ftt.Test) {
				up := map[string]int64{
					"quota": -3,
				}
				opts := &Options{}

				assert.Loosely(t, UpdateQuota(ctx, up, opts), should.Equal(ErrInsufficientQuota))
				assert.Loosely(t, s.Keys(), should.Match([]string{
					"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
				}))
				assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), should.Equal("2"))
				assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), should.Equal(now))
			})

			t.Run("atomicity", func(t *ftt.Test) {
				t.Run("policy not found", func(t *ftt.Test) {
					up := map[string]int64{
						"quota":         0,
						"quota/${user}": 0,
						"fake":          0,
					}
					opts := &Options{
						User: "user@example.com",
					}

					assert.Loosely(t, UpdateQuota(ctx, up, opts), should.ErrLike("not found"))
					assert.Loosely(t, s.Keys(), should.Match([]string{
						"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					}))
					assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), should.Equal("2"))
					assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), should.Equal(now))
				})

				t.Run("user not specified", func(t *ftt.Test) {
					up := map[string]int64{
						"quota":         0,
						"quota/${user}": 0,
					}
					opts := &Options{}

					assert.Loosely(t, UpdateQuota(ctx, up, opts), should.ErrLike("user unspecified"))
					assert.Loosely(t, s.Keys(), should.Match([]string{
						"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					}))
					assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), should.Equal("2"))
					assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), should.Equal(now))
				})

				t.Run("debit one", func(t *ftt.Test) {
					up := map[string]int64{
						"quota":         -1,
						"quota/${user}": -1,
					}
					opts := &Options{
						User: "user@example.com",
					}

					assert.Loosely(t, UpdateQuota(ctx, up, opts), should.BeNil)
					assert.Loosely(t, s.Keys(), should.Match([]string{
						"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
						"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
					}))
					assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), should.Equal("1"))
					assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), should.Equal(now))
					assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), should.Equal("1"))
					assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), should.Equal(now))
				})

				t.Run("debit excessive", func(t *ftt.Test) {
					up := map[string]int64{
						"quota":         -1,
						"quota/${user}": -3,
					}
					opts := &Options{
						User: "user@example.com",
					}

					assert.Loosely(t, UpdateQuota(ctx, up, opts), should.Equal(ErrInsufficientQuota))
					assert.Loosely(t, s.Keys(), should.Match([]string{
						"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					}))
					assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), should.Equal("2"))
					assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), should.Equal(now))
				})
			})

			t.Run("replenishment", func(t *ftt.Test) {
				ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeLocal.Add(time.Second))
				now := strconv.FormatInt(tc.Now().Unix(), 10)

				t.Run("future update", func(t *ftt.Test) {
					ctx, _ := testclock.UseTime(ctx, testclock.TestRecentTimeLocal.Add(-1*time.Second))
					up := map[string]int64{
						"quota": 1,
					}

					assert.Loosely(t, UpdateQuota(ctx, up, nil), should.ErrLike("last updated in the future"))
					assert.Loosely(t, s.Keys(), should.Match([]string{
						"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					}))
					assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), should.Equal("2"))
					assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), should.Equal(strconv.FormatInt(testclock.TestRecentTimeLocal.Unix(), 10)))
				})

				t.Run("distant past update", func(t *ftt.Test) {
					ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeLocal.Add(60*time.Second))
					now := strconv.FormatInt(tc.Now().Unix(), 10)
					up := map[string]int64{
						"quota": -1,
					}

					assert.Loosely(t, UpdateQuota(ctx, up, nil), should.BeNil)
					assert.Loosely(t, s.Keys(), should.Match([]string{
						"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					}))
					assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), should.Equal("4"))
					assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), should.Equal(now))
				})

				t.Run("cap", func(t *ftt.Test) {
					up := map[string]int64{
						"quota": 10,
					}

					assert.Loosely(t, UpdateQuota(ctx, up, nil), should.BeNil)
					assert.Loosely(t, s.Keys(), should.Match([]string{
						"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					}))
					assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), should.Equal("5"))
					assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), should.Equal(now))
				})

				t.Run("credit one", func(t *ftt.Test) {
					up := map[string]int64{
						"quota": 1,
					}

					assert.Loosely(t, UpdateQuota(ctx, up, nil), should.BeNil)
					assert.Loosely(t, s.Keys(), should.Match([]string{
						"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					}))
					assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), should.Equal("4"))
					assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), should.Equal(now))
				})

				t.Run("zero", func(t *ftt.Test) {
					up := map[string]int64{
						"quota": 0,
					}

					assert.Loosely(t, UpdateQuota(ctx, up, nil), should.BeNil)
					assert.Loosely(t, s.Keys(), should.Match([]string{
						"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					}))
					assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), should.Equal("3"))
					assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), should.Equal(now))
				})

				t.Run("debit one", func(t *ftt.Test) {
					up := map[string]int64{
						"quota": -1,
					}

					assert.Loosely(t, UpdateQuota(ctx, up, nil), should.BeNil)
					assert.Loosely(t, s.Keys(), should.Match([]string{
						"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					}))
					assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), should.Equal("2"))
					assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), should.Equal(now))
				})

				t.Run("debit all", func(t *ftt.Test) {
					up := map[string]int64{
						"quota": -3,
					}

					assert.Loosely(t, UpdateQuota(ctx, up, nil), should.BeNil)
					assert.Loosely(t, s.Keys(), should.Match([]string{
						"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					}))
					assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), should.Equal("0"))
					assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), should.Equal(now))
				})

				t.Run("debit excessive", func(t *ftt.Test) {
					up := map[string]int64{
						"quota": -4,
					}

					assert.Loosely(t, UpdateQuota(ctx, up, nil), should.Equal(ErrInsufficientQuota))
					assert.Loosely(t, s.Keys(), should.Match([]string{
						"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					}))
					assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), should.Equal("2"))
					assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), should.Equal(strconv.FormatInt(testclock.TestRecentTimeLocal.Unix(), 10)))
				})

				t.Run("atomicity", func(t *ftt.Test) {
					t.Run("future update", func(t *ftt.Test) {
						ctx, _ := testclock.UseTime(ctx, testclock.TestRecentTimeLocal.Add(-1*time.Second))
						up := map[string]int64{
							"quota":         -1,
							"quota/${user}": -1,
						}
						opts := &Options{
							User: "user@example.com",
						}

						assert.Loosely(t, UpdateQuota(ctx, up, opts), should.ErrLike("last updated in the future"))
						assert.Loosely(t, s.Keys(), should.Match([]string{
							"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
						}))
						assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), should.Equal("2"))
						assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), should.Equal(strconv.FormatInt(testclock.TestRecentTimeLocal.Unix(), 10)))
					})

					t.Run("debit one", func(t *ftt.Test) {
						up := map[string]int64{
							"quota":         -1,
							"quota/${user}": -1,
						}
						opts := &Options{
							User: "user@example.com",
						}

						assert.Loosely(t, UpdateQuota(ctx, up, opts), should.BeNil)
						assert.Loosely(t, s.Keys(), should.Match([]string{
							"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
							"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
						}))
						assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), should.Equal("2"))
						assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), should.Equal(now))
						assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), should.Equal("1"))
						assert.Loosely(t, s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), should.Equal(now))
					})

					t.Run("debit excessive", func(t *ftt.Test) {
						up := map[string]int64{
							"quota":         -1,
							"quota/${user}": -3,
						}
						opts := &Options{
							User: "user@example.com",
						}

						assert.Loosely(t, UpdateQuota(ctx, up, opts), should.Equal(ErrInsufficientQuota))
						assert.Loosely(t, s.Keys(), should.Match([]string{
							"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
						}))
						assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), should.Equal("2"))
						assert.Loosely(t, s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), should.Equal(strconv.FormatInt(testclock.TestRecentTimeLocal.Unix(), 10)))
					})
				})
			})
		})
	})
}
