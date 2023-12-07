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

	pb "go.chromium.org/luci/server/quotabeta/proto"
	"go.chromium.org/luci/server/quotabeta/quotaconfig"
	"go.chromium.org/luci/server/redisconn"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestQuota(t *testing.T) {
	t.Parallel()

	Convey("getInterface", t, func() {
		ctx := context.Background()

		Convey("panic", func() {
			shouldPanic := func() {
				getInterface(ctx)
			}
			So(shouldPanic, ShouldPanicLike, "quotaconfig.Interface implementation not found")
		})

		Convey("ok", func() {
			m, err := quotaconfig.NewMemory(ctx, nil)
			So(err, ShouldBeNil)
			ctx = Use(ctx, m)
			So(getInterface(ctx), ShouldNotBeNil)
		})
	})

	Convey("UpdateQuota", t, func() {
		s, err := miniredis.Run()
		So(err, ShouldBeNil)
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
		So(err, ShouldBeNil)
		ctx, tc := testclock.UseTime(Use(ctx, m), testclock.TestRecentTimeLocal)
		now := strconv.FormatInt(tc.Now().Unix(), 10)

		Convey("empty database", func() {
			Convey("policy not found", func() {
				up := map[string]int64{
					"fake": 0,
				}

				So(UpdateQuota(ctx, up, nil), ShouldErrLike, "not found")
				So(s.Keys(), ShouldBeEmpty)
			})

			Convey("user not specified", func() {
				up := map[string]int64{
					"quota/${user}": 0,
				}

				So(UpdateQuota(ctx, up, nil), ShouldErrLike, "user unspecified")
				So(s.Keys(), ShouldBeEmpty)
			})

			Convey("capped", func() {
				up := map[string]int64{
					"quota/${user}": 1,
				}
				opts := &Options{
					User: "user@example.com",
				}

				So(UpdateQuota(ctx, up, opts), ShouldBeNil)
				So(s.Keys(), ShouldResemble, []string{
					"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
				})
				So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), ShouldEqual, "2")
				So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), ShouldEqual, now)
			})

			Convey("zero", func() {
				up := map[string]int64{
					"quota/${user}": 0,
				}
				opts := &Options{
					User: "user@example.com",
				}

				So(UpdateQuota(ctx, up, opts), ShouldBeNil)
				So(s.Keys(), ShouldResemble, []string{
					"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
				})
				So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), ShouldEqual, "2")
				So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), ShouldEqual, now)
			})

			Convey("debit one", func() {
				up := map[string]int64{
					"quota/${user}": -1,
				}
				opts := &Options{
					User: "user@example.com",
				}

				So(UpdateQuota(ctx, up, opts), ShouldBeNil)
				So(s.Keys(), ShouldResemble, []string{
					"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
				})
				So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), ShouldEqual, "1")
				So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), ShouldEqual, now)
			})

			Convey("debit all", func() {
				up := map[string]int64{
					"quota/${user}": -2,
				}
				opts := &Options{
					User: "user@example.com",
				}

				So(UpdateQuota(ctx, up, opts), ShouldBeNil)
				So(s.Keys(), ShouldResemble, []string{
					"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
				})
				So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), ShouldEqual, "0")
				So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), ShouldEqual, now)
			})

			Convey("debit excessive", func() {
				up := map[string]int64{
					"quota/${user}": -3,
				}
				opts := &Options{
					User: "user@example.com",
				}

				So(UpdateQuota(ctx, up, opts), ShouldEqual, ErrInsufficientQuota)

				So(s.Keys(), ShouldBeEmpty)
			})

			Convey("debit multiple", func() {
				up := map[string]int64{
					"quota/${user}": -1,
				}
				opts := &Options{
					User: "user@example.com",
				}

				So(UpdateQuota(ctx, up, opts), ShouldBeNil)
				So(UpdateQuota(ctx, up, opts), ShouldBeNil)
				So(UpdateQuota(ctx, up, opts), ShouldEqual, ErrInsufficientQuota)
				So(s.Keys(), ShouldResemble, []string{
					"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
				})
				So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), ShouldEqual, "0")
				So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), ShouldEqual, now)
			})

			Convey("atomicity", func() {
				Convey("policy not found", func() {
					up := map[string]int64{
						"quota":         0,
						"quota/${user}": 0,
						"fake":          0,
					}
					opts := &Options{
						User: "user@example.com",
					}

					So(UpdateQuota(ctx, up, opts), ShouldErrLike, "not found")
					So(s.Keys(), ShouldBeEmpty)
				})

				Convey("user not specified", func() {
					up := map[string]int64{
						"quota":         0,
						"quota/${user}": 0,
					}
					opts := &Options{}

					So(UpdateQuota(ctx, up, opts), ShouldErrLike, "user unspecified")
					So(s.Keys(), ShouldBeEmpty)
				})

				Convey("debit one", func() {
					up := map[string]int64{
						"quota":         -1,
						"quota/${user}": -1,
					}
					opts := &Options{
						User: "user@example.com",
					}

					So(UpdateQuota(ctx, up, opts), ShouldBeNil)
					So(s.Keys(), ShouldResemble, []string{
						"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
						"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
					})
					So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), ShouldEqual, "4")
					So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), ShouldEqual, now)
					So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), ShouldEqual, "1")
					So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), ShouldEqual, now)
				})

				Convey("debit excessive", func() {
					up := map[string]int64{
						"quota":         -1,
						"quota/${user}": -3,
					}
					opts := &Options{
						User: "user@example.com",
					}

					So(UpdateQuota(ctx, up, opts), ShouldEqual, ErrInsufficientQuota)
					So(s.Keys(), ShouldBeEmpty)
				})
			})

			Convey("idempotence", func() {
				Convey("deduplication", func() {
					up := map[string]int64{
						"quota/${user}": -1,
					}
					opts := &Options{
						RequestID: "request-id",
						User:      "user@example.com",
					}

					So(UpdateQuota(ctx, up, opts), ShouldBeNil)
					So(s.Keys(), ShouldResemble, []string{
						"deduplicationKeys",
						"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
					})
					So(s.HGet("deduplicationKeys", "request-id"), ShouldEqual, strconv.FormatInt(testclock.TestRecentTimeLocal.Add(time.Hour).Unix(), 10))
					So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), ShouldEqual, "1")
					So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), ShouldEqual, now)

					// Ensure update succeeds without modifying the database again.
					ctx, _ := testclock.UseTime(ctx, testclock.TestRecentTimeLocal.Add(30*time.Minute))
					So(UpdateQuota(ctx, up, opts), ShouldBeNil)
					So(s.Keys(), ShouldResemble, []string{
						"deduplicationKeys",
						"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
					})
					So(s.HGet("deduplicationKeys", "request-id"), ShouldEqual, strconv.FormatInt(testclock.TestRecentTimeLocal.Add(time.Hour).Unix(), 10))
					So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), ShouldEqual, "1")
					So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), ShouldEqual, now)
				})

				Convey("expired", func() {
					up := map[string]int64{
						"quota/${user}": -1,
					}
					opts := &Options{
						RequestID: "request-id",
						User:      "user@example.com",
					}

					So(UpdateQuota(ctx, up, opts), ShouldBeNil)
					So(s.Keys(), ShouldResemble, []string{
						"deduplicationKeys",
						"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
					})
					So(s.HGet("deduplicationKeys", "request-id"), ShouldEqual, strconv.FormatInt(testclock.TestRecentTimeLocal.Add(time.Hour).Unix(), 10))
					So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), ShouldEqual, "1")
					So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), ShouldEqual, now)

					// Ensure update modifies the database after the deduplication deadline.
					ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeLocal.Add(2*time.Hour))
					So(UpdateQuota(ctx, up, opts), ShouldBeNil)
					So(s.Keys(), ShouldResemble, []string{
						"deduplicationKeys",
						"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
					})
					So(s.HGet("deduplicationKeys", "request-id"), ShouldEqual, strconv.FormatInt(tc.Now().Add(time.Hour).Unix(), 10))
					So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), ShouldEqual, "0")
					So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), ShouldEqual, strconv.FormatInt(tc.Now().Unix(), 10))
				})

				Convey("error", func() {
					up := map[string]int64{
						"quota/${user}": -10,
					}
					opts := &Options{
						RequestID: "request-id",
						User:      "user@example.com",
					}

					So(UpdateQuota(ctx, up, opts), ShouldErrLike, "insufficient quota")
					So(s.Keys(), ShouldResemble, []string{
						"deduplicationKeys",
					})
					So(s.HGet("deduplicationKeys", "request-id"), ShouldEqual, "0")

					up = map[string]int64{
						"quota/${user}": -1,
					}

					So(UpdateQuota(ctx, up, opts), ShouldBeNil)
					So(s.Keys(), ShouldResemble, []string{
						"deduplicationKeys",
						"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
					})
					So(s.HGet("deduplicationKeys", "request-id"), ShouldEqual, strconv.FormatInt(testclock.TestRecentTimeLocal.Add(time.Hour).Unix(), 10))
					So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), ShouldEqual, "1")
					So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), ShouldEqual, now)
				})
			})
		})

		Convey("existing database", func() {
			conn, err := redisconn.Get(ctx)
			So(err, ShouldBeNil)
			_, err = conn.Do("HINCRBY", "entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources", 2)
			So(err, ShouldBeNil)
			_, err = conn.Do("HINCRBY", "entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated", tc.Now().Unix())
			So(err, ShouldBeNil)

			Convey("capped", func() {
				up := map[string]int64{
					"quota": 10,
				}
				opts := &Options{
					User: "user@example.com",
				}

				So(UpdateQuota(ctx, up, opts), ShouldBeNil)
				So(s.Keys(), ShouldResemble, []string{
					"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
				})
				So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), ShouldEqual, "5")
				So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), ShouldEqual, now)
			})

			Convey("credit one", func() {
				up := map[string]int64{
					"quota": 1,
				}
				opts := &Options{
					User: "user@example.com",
				}

				So(UpdateQuota(ctx, up, opts), ShouldBeNil)
				So(s.Keys(), ShouldResemble, []string{
					"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
				})
				So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), ShouldEqual, "3")
				So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), ShouldEqual, now)
			})

			Convey("debit one", func() {
				up := map[string]int64{
					"quota": -1,
				}
				opts := &Options{}

				So(UpdateQuota(ctx, up, opts), ShouldBeNil)
				So(s.Keys(), ShouldResemble, []string{
					"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
				})
				So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), ShouldEqual, "1")
				So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), ShouldEqual, now)
			})

			Convey("debit excessive", func() {
				up := map[string]int64{
					"quota": -3,
				}
				opts := &Options{}

				So(UpdateQuota(ctx, up, opts), ShouldEqual, ErrInsufficientQuota)
				So(s.Keys(), ShouldResemble, []string{
					"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
				})
				So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), ShouldEqual, "2")
				So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), ShouldEqual, now)
			})

			Convey("atomicity", func() {
				Convey("policy not found", func() {
					up := map[string]int64{
						"quota":         0,
						"quota/${user}": 0,
						"fake":          0,
					}
					opts := &Options{
						User: "user@example.com",
					}

					So(UpdateQuota(ctx, up, opts), ShouldErrLike, "not found")
					So(s.Keys(), ShouldResemble, []string{
						"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					})
					So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), ShouldEqual, "2")
					So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), ShouldEqual, now)
				})

				Convey("user not specified", func() {
					up := map[string]int64{
						"quota":         0,
						"quota/${user}": 0,
					}
					opts := &Options{}

					So(UpdateQuota(ctx, up, opts), ShouldErrLike, "user unspecified")
					So(s.Keys(), ShouldResemble, []string{
						"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					})
					So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), ShouldEqual, "2")
					So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), ShouldEqual, now)
				})

				Convey("debit one", func() {
					up := map[string]int64{
						"quota":         -1,
						"quota/${user}": -1,
					}
					opts := &Options{
						User: "user@example.com",
					}

					So(UpdateQuota(ctx, up, opts), ShouldBeNil)
					So(s.Keys(), ShouldResemble, []string{
						"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
						"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
					})
					So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), ShouldEqual, "1")
					So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), ShouldEqual, now)
					So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), ShouldEqual, "1")
					So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), ShouldEqual, now)
				})

				Convey("debit excessive", func() {
					up := map[string]int64{
						"quota":         -1,
						"quota/${user}": -3,
					}
					opts := &Options{
						User: "user@example.com",
					}

					So(UpdateQuota(ctx, up, opts), ShouldEqual, ErrInsufficientQuota)
					So(s.Keys(), ShouldResemble, []string{
						"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					})
					So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), ShouldEqual, "2")
					So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), ShouldEqual, now)
				})
			})

			Convey("replenishment", func() {
				ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeLocal.Add(time.Second))
				now := strconv.FormatInt(tc.Now().Unix(), 10)

				Convey("future update", func() {
					ctx, _ := testclock.UseTime(ctx, testclock.TestRecentTimeLocal.Add(-1*time.Second))
					up := map[string]int64{
						"quota": 1,
					}

					So(UpdateQuota(ctx, up, nil), ShouldErrLike, "last updated in the future")
					So(s.Keys(), ShouldResemble, []string{
						"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					})
					So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), ShouldEqual, "2")
					So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), ShouldEqual, strconv.FormatInt(testclock.TestRecentTimeLocal.Unix(), 10))
				})

				Convey("distant past update", func() {
					ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeLocal.Add(60*time.Second))
					now := strconv.FormatInt(tc.Now().Unix(), 10)
					up := map[string]int64{
						"quota": -1,
					}

					So(UpdateQuota(ctx, up, nil), ShouldBeNil)
					So(s.Keys(), ShouldResemble, []string{
						"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					})
					So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), ShouldEqual, "4")
					So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), ShouldEqual, now)
				})

				Convey("cap", func() {
					up := map[string]int64{
						"quota": 10,
					}

					So(UpdateQuota(ctx, up, nil), ShouldBeNil)
					So(s.Keys(), ShouldResemble, []string{
						"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					})
					So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), ShouldEqual, "5")
					So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), ShouldEqual, now)
				})

				Convey("credit one", func() {
					up := map[string]int64{
						"quota": 1,
					}

					So(UpdateQuota(ctx, up, nil), ShouldBeNil)
					So(s.Keys(), ShouldResemble, []string{
						"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					})
					So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), ShouldEqual, "4")
					So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), ShouldEqual, now)
				})

				Convey("zero", func() {
					up := map[string]int64{
						"quota": 0,
					}

					So(UpdateQuota(ctx, up, nil), ShouldBeNil)
					So(s.Keys(), ShouldResemble, []string{
						"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					})
					So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), ShouldEqual, "3")
					So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), ShouldEqual, now)
				})

				Convey("debit one", func() {
					up := map[string]int64{
						"quota": -1,
					}

					So(UpdateQuota(ctx, up, nil), ShouldBeNil)
					So(s.Keys(), ShouldResemble, []string{
						"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					})
					So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), ShouldEqual, "2")
					So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), ShouldEqual, now)
				})

				Convey("debit all", func() {
					up := map[string]int64{
						"quota": -3,
					}

					So(UpdateQuota(ctx, up, nil), ShouldBeNil)
					So(s.Keys(), ShouldResemble, []string{
						"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					})
					So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), ShouldEqual, "0")
					So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), ShouldEqual, now)
				})

				Convey("debit excessive", func() {
					up := map[string]int64{
						"quota": -4,
					}

					So(UpdateQuota(ctx, up, nil), ShouldEqual, ErrInsufficientQuota)
					So(s.Keys(), ShouldResemble, []string{
						"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					})
					So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), ShouldEqual, "2")
					So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), ShouldEqual, strconv.FormatInt(testclock.TestRecentTimeLocal.Unix(), 10))
				})

				Convey("atomicity", func() {
					Convey("future update", func() {
						ctx, _ := testclock.UseTime(ctx, testclock.TestRecentTimeLocal.Add(-1*time.Second))
						up := map[string]int64{
							"quota":         -1,
							"quota/${user}": -1,
						}
						opts := &Options{
							User: "user@example.com",
						}

						So(UpdateQuota(ctx, up, opts), ShouldErrLike, "last updated in the future")
						So(s.Keys(), ShouldResemble, []string{
							"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
						})
						So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), ShouldEqual, "2")
						So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), ShouldEqual, strconv.FormatInt(testclock.TestRecentTimeLocal.Unix(), 10))
					})

					Convey("debit one", func() {
						up := map[string]int64{
							"quota":         -1,
							"quota/${user}": -1,
						}
						opts := &Options{
							User: "user@example.com",
						}

						So(UpdateQuota(ctx, up, opts), ShouldBeNil)
						So(s.Keys(), ShouldResemble, []string{
							"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
							"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
						})
						So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), ShouldEqual, "2")
						So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), ShouldEqual, now)
						So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), ShouldEqual, "1")
						So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), ShouldEqual, now)
					})

					Convey("debit excessive", func() {
						up := map[string]int64{
							"quota":         -1,
							"quota/${user}": -3,
						}
						opts := &Options{
							User: "user@example.com",
						}

						So(UpdateQuota(ctx, up, opts), ShouldEqual, ErrInsufficientQuota)
						So(s.Keys(), ShouldResemble, []string{
							"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
						})
						So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), ShouldEqual, "2")
						So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), ShouldEqual, strconv.FormatInt(testclock.TestRecentTimeLocal.Unix(), 10))
					})
				})
			})
		})
	})
}
