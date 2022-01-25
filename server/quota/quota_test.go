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
	"go.chromium.org/luci/server/redisconn"

	pb "go.chromium.org/luci/server/quota/proto"
	"go.chromium.org/luci/server/quota/quotaconfig"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestQuota(t *testing.T) {
	t.Parallel()

	Convey("DebitQuota", t, func() {
		s, err := miniredis.Run()
		So(err, ShouldBeNil)
		defer s.Close()
		ctx := redisconn.UsePool(context.Background(), &redis.Pool{
			Dial: func() (redis.Conn, error) {
				return redis.Dial("tcp", s.Addr())
			},
		})
		ctx = WithConfig(ctx, quotaconfig.NewMemory([]*pb.Policy{
			{
				Name:          "project/quota",
				Resources:     5,
				Replenishment: 1,
			},
			{
				Name:          "project/quota/${user}",
				Resources:     2,
				Replenishment: 1,
			},
		}))
		ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeLocal)
		now := strconv.FormatInt(tc.Now().Unix(), 10)

		Convey("empty database", func() {
			Convey("project not specified", func() {
				up := map[string]int64{
					"fake": 0,
				}

				So(DebitQuota(ctx, up, nil), ShouldErrLike, "project unspecified")
				So(s.Keys(), ShouldBeEmpty)
			})

			Convey("policy not found", func() {
				up := map[string]int64{
					"fake": 0,
				}
				opts := &Options{
					Project: "project",
				}

				So(DebitQuota(ctx, up, opts), ShouldErrLike, "not found")
				So(s.Keys(), ShouldBeEmpty)
			})

			Convey("user not specified", func() {
				up := map[string]int64{
					"quota/${user}": 0,
				}
				opts := &Options{
					Project: "project",
				}

				So(DebitQuota(ctx, up, opts), ShouldErrLike, "user unspecified")
				So(s.Keys(), ShouldBeEmpty)
			})

			Convey("capped", func() {
				up := map[string]int64{
					"quota/${user}": -1,
				}
				opts := &Options{
					Project: "project",
					User:    "user@example.com",
				}

				So(DebitQuota(ctx, up, opts), ShouldBeNil)
				So(s.Keys(), ShouldResemble, []string{
					"entry:50c6ce8e6f680e66574e75fd724f5feb4d7a45ee35c7c5028c62872ee547f085",
				})
				So(s.HGet("entry:50c6ce8e6f680e66574e75fd724f5feb4d7a45ee35c7c5028c62872ee547f085", "resources"), ShouldEqual, "2")
				So(s.HGet("entry:50c6ce8e6f680e66574e75fd724f5feb4d7a45ee35c7c5028c62872ee547f085", "updated"), ShouldEqual, now)
			})

			Convey("zero", func() {
				up := map[string]int64{
					"quota/${user}": 0,
				}
				opts := &Options{
					Project: "project",
					User:    "user@example.com",
				}

				So(DebitQuota(ctx, up, opts), ShouldBeNil)
				So(s.Keys(), ShouldResemble, []string{
					"entry:50c6ce8e6f680e66574e75fd724f5feb4d7a45ee35c7c5028c62872ee547f085",
				})
				So(s.HGet("entry:50c6ce8e6f680e66574e75fd724f5feb4d7a45ee35c7c5028c62872ee547f085", "resources"), ShouldEqual, "2")
				So(s.HGet("entry:50c6ce8e6f680e66574e75fd724f5feb4d7a45ee35c7c5028c62872ee547f085", "updated"), ShouldEqual, now)
			})

			Convey("one", func() {
				up := map[string]int64{
					"quota/${user}": 1,
				}
				opts := &Options{
					Project: "project",
					User:    "user@example.com",
				}

				So(DebitQuota(ctx, up, opts), ShouldBeNil)
				So(s.Keys(), ShouldResemble, []string{
					"entry:50c6ce8e6f680e66574e75fd724f5feb4d7a45ee35c7c5028c62872ee547f085",
				})
				So(s.HGet("entry:50c6ce8e6f680e66574e75fd724f5feb4d7a45ee35c7c5028c62872ee547f085", "resources"), ShouldEqual, "1")
				So(s.HGet("entry:50c6ce8e6f680e66574e75fd724f5feb4d7a45ee35c7c5028c62872ee547f085", "updated"), ShouldEqual, now)
			})

			Convey("all", func() {
				up := map[string]int64{
					"quota/${user}": 2,
				}
				opts := &Options{
					Project: "project",
					User:    "user@example.com",
				}

				So(DebitQuota(ctx, up, opts), ShouldBeNil)
				So(s.Keys(), ShouldResemble, []string{
					"entry:50c6ce8e6f680e66574e75fd724f5feb4d7a45ee35c7c5028c62872ee547f085",
				})
				So(s.HGet("entry:50c6ce8e6f680e66574e75fd724f5feb4d7a45ee35c7c5028c62872ee547f085", "resources"), ShouldEqual, "0")
				So(s.HGet("entry:50c6ce8e6f680e66574e75fd724f5feb4d7a45ee35c7c5028c62872ee547f085", "updated"), ShouldEqual, now)
			})

			Convey("excessive", func() {
				up := map[string]int64{
					"quota/${user}": 3,
				}
				opts := &Options{
					Project: "project",
					User:    "user@example.com",
				}

				So(DebitQuota(ctx, up, opts), ShouldErrLike, "insufficient resources")

				So(s.Keys(), ShouldBeEmpty)
			})

			Convey("multiple", func() {
				up := map[string]int64{
					"quota/${user}": 1,
				}
				opts := &Options{
					Project: "project",
					User:    "user@example.com",
				}

				So(DebitQuota(ctx, up, opts), ShouldBeNil)
				So(DebitQuota(ctx, up, opts), ShouldBeNil)
				So(DebitQuota(ctx, up, opts), ShouldErrLike, "insufficient resources")
				So(s.Keys(), ShouldResemble, []string{
					"entry:50c6ce8e6f680e66574e75fd724f5feb4d7a45ee35c7c5028c62872ee547f085",
				})
				So(s.HGet("entry:50c6ce8e6f680e66574e75fd724f5feb4d7a45ee35c7c5028c62872ee547f085", "resources"), ShouldEqual, "0")
				So(s.HGet("entry:50c6ce8e6f680e66574e75fd724f5feb4d7a45ee35c7c5028c62872ee547f085", "updated"), ShouldEqual, now)
			})

			Convey("atomicity", func() {
				Convey("policy not found", func() {
					up := map[string]int64{
						"quota":         0,
						"quota/${user}": 0,
						"fake":          0,
					}
					opts := &Options{
						Project: "project",
						User:    "user@example.com",
					}

					So(DebitQuota(ctx, up, opts), ShouldErrLike, "not found")
					So(s.Keys(), ShouldBeEmpty)
				})

				Convey("user not specified", func() {
					up := map[string]int64{
						"quota":         0,
						"quota/${user}": 0,
					}
					opts := &Options{
						Project: "project",
					}

					So(DebitQuota(ctx, up, opts), ShouldErrLike, "user unspecified")
					So(s.Keys(), ShouldBeEmpty)
				})

				Convey("one", func() {
					up := map[string]int64{
						"quota":         1,
						"quota/${user}": 1,
					}
					opts := &Options{
						Project: "project",
						User:    "user@example.com",
					}

					So(DebitQuota(ctx, up, opts), ShouldBeNil)
					So(s.Keys(), ShouldResemble, []string{
						"entry:50c6ce8e6f680e66574e75fd724f5feb4d7a45ee35c7c5028c62872ee547f085",
						"entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546",
					})
					So(s.HGet("entry:50c6ce8e6f680e66574e75fd724f5feb4d7a45ee35c7c5028c62872ee547f085", "resources"), ShouldEqual, "1")
					So(s.HGet("entry:50c6ce8e6f680e66574e75fd724f5feb4d7a45ee35c7c5028c62872ee547f085", "updated"), ShouldEqual, now)
					So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "resources"), ShouldEqual, "4")
					So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "updated"), ShouldEqual, now)
				})

				Convey("excessive", func() {
					up := map[string]int64{
						"quota":         1,
						"quota/${user}": 3,
					}
					opts := &Options{
						Project: "project",
						User:    "user@example.com",
					}

					So(DebitQuota(ctx, up, opts), ShouldErrLike, "insufficient resources")
					So(s.Keys(), ShouldBeEmpty)
				})
			})
		})

		Convey("existing database", func() {
			conn, err := redisconn.Get(ctx)
			So(err, ShouldBeNil)
			_, err = conn.Do("HINCRBY", "entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "resources", 2)
			So(err, ShouldBeNil)
			_, err = conn.Do("HINCRBY", "entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "updated", tc.Now().Unix())
			So(err, ShouldBeNil)

			Convey("capped", func() {
				up := map[string]int64{
					"quota": -10,
				}
				opts := &Options{
					Project: "project",
					User:    "user@example.com",
				}

				So(DebitQuota(ctx, up, opts), ShouldBeNil)
				So(s.Keys(), ShouldResemble, []string{
					"entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546",
				})
				So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "resources"), ShouldEqual, "5")
				So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "updated"), ShouldEqual, now)
			})

			Convey("negative", func() {
				up := map[string]int64{
					"quota": -1,
				}
				opts := &Options{
					Project: "project",
					User:    "user@example.com",
				}

				So(DebitQuota(ctx, up, opts), ShouldBeNil)
				So(s.Keys(), ShouldResemble, []string{
					"entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546",
				})
				So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "resources"), ShouldEqual, "3")
				So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "updated"), ShouldEqual, now)
			})

			Convey("one", func() {
				up := map[string]int64{
					"quota": 1,
				}
				opts := &Options{
					Project: "project",
				}

				So(DebitQuota(ctx, up, opts), ShouldBeNil)
				So(s.Keys(), ShouldResemble, []string{
					"entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546",
				})
				So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "resources"), ShouldEqual, "1")
				So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "updated"), ShouldEqual, now)
			})

			Convey("excessive", func() {
				up := map[string]int64{
					"quota": 3,
				}
				opts := &Options{
					Project: "project",
				}

				So(DebitQuota(ctx, up, opts), ShouldErrLike, "insufficient resources")
				So(s.Keys(), ShouldResemble, []string{
					"entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546",
				})
				So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "resources"), ShouldEqual, "2")
				So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "updated"), ShouldEqual, now)
			})

			Convey("atomicity", func() {
				Convey("policy not found", func() {
					up := map[string]int64{
						"quota":         0,
						"quota/${user}": 0,
						"fake":          0,
					}
					opts := &Options{
						Project: "project",
						User:    "user@example.com",
					}

					So(DebitQuota(ctx, up, opts), ShouldErrLike, "not found")
					So(s.Keys(), ShouldResemble, []string{
						"entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546",
					})
					So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "resources"), ShouldEqual, "2")
					So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "updated"), ShouldEqual, now)
				})

				Convey("user not specified", func() {
					up := map[string]int64{
						"quota":         0,
						"quota/${user}": 0,
					}
					opts := &Options{
						Project: "project",
					}

					So(DebitQuota(ctx, up, opts), ShouldErrLike, "user unspecified")
					So(s.Keys(), ShouldResemble, []string{
						"entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546",
					})
					So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "resources"), ShouldEqual, "2")
					So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "updated"), ShouldEqual, now)
				})

				Convey("one", func() {
					up := map[string]int64{
						"quota":         1,
						"quota/${user}": 1,
					}
					opts := &Options{
						Project: "project",
						User:    "user@example.com",
					}

					So(DebitQuota(ctx, up, opts), ShouldBeNil)
					So(s.Keys(), ShouldResemble, []string{
						"entry:50c6ce8e6f680e66574e75fd724f5feb4d7a45ee35c7c5028c62872ee547f085",
						"entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546",
					})
					So(s.HGet("entry:50c6ce8e6f680e66574e75fd724f5feb4d7a45ee35c7c5028c62872ee547f085", "resources"), ShouldEqual, "1")
					So(s.HGet("entry:50c6ce8e6f680e66574e75fd724f5feb4d7a45ee35c7c5028c62872ee547f085", "updated"), ShouldEqual, now)
					So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "resources"), ShouldEqual, "1")
					So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "updated"), ShouldEqual, now)
				})

				Convey("excessive", func() {
					up := map[string]int64{
						"quota":         1,
						"quota/${user}": 3,
					}
					opts := &Options{
						Project: "project",
						User:    "user@example.com",
					}

					So(DebitQuota(ctx, up, opts), ShouldErrLike, "insufficient resources")
					So(s.Keys(), ShouldResemble, []string{
						"entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546",
					})
					So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "resources"), ShouldEqual, "2")
					So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "updated"), ShouldEqual, now)
				})
			})

			Convey("replenishment", func() {
				ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeLocal.Add(time.Second))
				now := strconv.FormatInt(tc.Now().Unix(), 10)

				Convey("time travel", func() {
					ctx, _ := testclock.UseTime(ctx, testclock.TestRecentTimeLocal.Add(-1*time.Second))
					up := map[string]int64{
						"quota": 1,
					}
					opts := &Options{
						Project: "project",
					}

					So(DebitQuota(ctx, up, opts), ShouldErrLike, "last updated in the future")
					So(s.Keys(), ShouldResemble, []string{
						"entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546",
					})
					So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "resources"), ShouldEqual, "2")
					So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "updated"), ShouldEqual, strconv.FormatInt(testclock.TestRecentTimeLocal.Unix(), 10))
				})

				Convey("cap", func() {
					up := map[string]int64{
						"quota": -10,
					}
					opts := &Options{
						Project: "project",
					}

					So(DebitQuota(ctx, up, opts), ShouldBeNil)
					So(s.Keys(), ShouldResemble, []string{
						"entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546",
					})
					So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "resources"), ShouldEqual, "5")
					So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "updated"), ShouldEqual, now)
				})

				Convey("negative", func() {
					up := map[string]int64{
						"quota": -1,
					}
					opts := &Options{
						Project: "project",
					}

					So(DebitQuota(ctx, up, opts), ShouldBeNil)
					So(s.Keys(), ShouldResemble, []string{
						"entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546",
					})
					So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "resources"), ShouldEqual, "4")
					So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "updated"), ShouldEqual, now)
				})

				Convey("zero", func() {
					up := map[string]int64{
						"quota": 0,
					}
					opts := &Options{
						Project: "project",
					}

					So(DebitQuota(ctx, up, opts), ShouldBeNil)
					So(s.Keys(), ShouldResemble, []string{
						"entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546",
					})
					So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "resources"), ShouldEqual, "3")
					So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "updated"), ShouldEqual, now)
				})

				Convey("one", func() {
					up := map[string]int64{
						"quota": 1,
					}
					opts := &Options{
						Project: "project",
					}

					So(DebitQuota(ctx, up, opts), ShouldBeNil)
					So(s.Keys(), ShouldResemble, []string{
						"entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546",
					})
					So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "resources"), ShouldEqual, "2")
					So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "updated"), ShouldEqual, now)
				})

				Convey("all", func() {
					up := map[string]int64{
						"quota": 3,
					}
					opts := &Options{
						Project: "project",
					}

					So(DebitQuota(ctx, up, opts), ShouldBeNil)
					So(s.Keys(), ShouldResemble, []string{
						"entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546",
					})
					So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "resources"), ShouldEqual, "0")
					So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "updated"), ShouldEqual, now)
				})

				Convey("excessive", func() {
					up := map[string]int64{
						"quota": 4,
					}
					opts := &Options{
						Project: "project",
					}

					So(DebitQuota(ctx, up, opts), ShouldErrLike, "insufficient resources")
					So(s.Keys(), ShouldResemble, []string{
						"entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546",
					})
					So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "resources"), ShouldEqual, "2")
					So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "updated"), ShouldEqual, strconv.FormatInt(testclock.TestRecentTimeLocal.Unix(), 10))
				})

				Convey("atomicity", func() {
					Convey("time travel", func() {
						ctx, _ := testclock.UseTime(ctx, testclock.TestRecentTimeLocal.Add(-1*time.Second))
						up := map[string]int64{
							"quota":         1,
							"quota/${user}": 1,
						}
						opts := &Options{
							Project: "project",
							User:    "user@example.com",
						}

						So(DebitQuota(ctx, up, opts), ShouldErrLike, "last updated in the future")
						So(s.Keys(), ShouldResemble, []string{
							"entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546",
						})
						So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "resources"), ShouldEqual, "2")
						So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "updated"), ShouldEqual, strconv.FormatInt(testclock.TestRecentTimeLocal.Unix(), 10))
					})

					Convey("one", func() {
						up := map[string]int64{
							"quota":         1,
							"quota/${user}": 1,
						}
						opts := &Options{
							Project: "project",
							User:    "user@example.com",
						}

						So(DebitQuota(ctx, up, opts), ShouldBeNil)
						So(s.Keys(), ShouldResemble, []string{
							"entry:50c6ce8e6f680e66574e75fd724f5feb4d7a45ee35c7c5028c62872ee547f085",
							"entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546",
						})
						So(s.HGet("entry:50c6ce8e6f680e66574e75fd724f5feb4d7a45ee35c7c5028c62872ee547f085", "resources"), ShouldEqual, "1")
						So(s.HGet("entry:50c6ce8e6f680e66574e75fd724f5feb4d7a45ee35c7c5028c62872ee547f085", "updated"), ShouldEqual, now)
						So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "resources"), ShouldEqual, "2")
						So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "updated"), ShouldEqual, now)
					})

					Convey("excessive", func() {
						up := map[string]int64{
							"quota":         1,
							"quota/${user}": 3,
						}
						opts := &Options{
							Project: "project",
							User:    "user@example.com",
						}

						So(DebitQuota(ctx, up, opts), ShouldErrLike, "insufficient resources")
						So(s.Keys(), ShouldResemble, []string{
							"entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546",
						})
						So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "resources"), ShouldEqual, "2")
						So(s.HGet("entry:9316336a8f8a1e0e34ab8770e856e18b059163b656e2987f8880896d03d72546", "updated"), ShouldEqual, strconv.FormatInt(testclock.TestRecentTimeLocal.Unix(), 10))
					})
				})
			})
		})
	})
}
