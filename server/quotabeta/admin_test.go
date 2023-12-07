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

func TestQuotaAdmin(t *testing.T) {
	t.Parallel()

	Convey("quotaAdmin", t, func() {
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestRecentTimeLocal)
		now := strconv.FormatInt(tc.Now().Unix(), 10)
		s, err := miniredis.Run()
		So(err, ShouldBeNil)
		defer s.Close()
		ctx = redisconn.UsePool(ctx, &redis.Pool{
			Dial: func() (redis.Conn, error) {
				return redis.Dial("tcp", s.Addr())
			},
		})
		conn, err := redisconn.Get(ctx)
		So(err, ShouldBeNil)
		_, err = conn.Do("HINCRBY", "entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources", 3)
		So(err, ShouldBeNil)
		_, err = conn.Do("HINCRBY", "entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated", tc.Now().Unix())
		So(err, ShouldBeNil)
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
		So(err, ShouldBeNil)
		ctx = Use(ctx, m)
		srv := &quotaAdmin{}

		Convey("Get", func() {
			Convey("nil", func() {
				rsp, err := srv.Get(ctx, nil)
				So(err, ShouldErrLike, "policy is required")
				So(rsp, ShouldBeNil)
			})

			Convey("empty", func() {
				req := &pb.GetRequest{}
				rsp, err := srv.Get(ctx, req)
				So(err, ShouldErrLike, "policy is required")
				So(rsp, ShouldBeNil)
			})

			Convey("not found", func() {
				req := &pb.GetRequest{
					Policy: "quota/user",
				}

				rsp, err := srv.Get(ctx, req)
				So(err, ShouldErrLike, "not found")
				So(rsp, ShouldBeNil)
			})

			Convey("user unspecified", func() {
				req := &pb.GetRequest{
					Policy: "quota/${user}",
				}

				rsp, err := srv.Get(ctx, req)
				So(err, ShouldErrLike, "user not specified")
				So(rsp, ShouldBeNil)
			})

			Convey("new", func() {
				req := &pb.GetRequest{
					Policy: "quota",
				}

				rsp, err := srv.Get(ctx, req)
				So(err, ShouldBeNil)
				So(rsp, ShouldResemble, &pb.QuotaEntry{
					Name:      "quota",
					DbName:    "entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					Resources: 2,
				})

				// Ensure an entry for "quota" was not written to the database.
				So(s.Keys(), ShouldResemble, []string{
					"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
				})
				So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), ShouldEqual, "3")
				So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), ShouldEqual, now)
			})

			Convey("existing", func() {
				req := &pb.GetRequest{
					Policy: "quota/${user}",
					User:   "user@example.com",
				}

				rsp, err := srv.Get(ctx, req)
				So(err, ShouldBeNil)
				So(rsp, ShouldResemble, &pb.QuotaEntry{
					Name:      "quota/user@example.com",
					DbName:    "entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
					Resources: 3,
				})

				So(s.Keys(), ShouldResemble, []string{
					"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
				})
				So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), ShouldEqual, "3")
				So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), ShouldEqual, now)
			})

			Convey("replenish", func() {
				ctx, _ := testclock.UseTime(ctx, testclock.TestRecentTimeLocal.Add(time.Second))
				req := &pb.GetRequest{
					Policy: "quota/${user}",
					User:   "user@example.com",
				}

				rsp, err := srv.Get(ctx, req)
				So(err, ShouldBeNil)
				So(rsp, ShouldResemble, &pb.QuotaEntry{
					Name:      "quota/user@example.com",
					DbName:    "entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
					Resources: 4,
				})

				// Ensure replenishment was not written to the database.
				So(s.Keys(), ShouldResemble, []string{
					"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
				})
				So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), ShouldEqual, "3")
				So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), ShouldEqual, now)
			})
		})

		Convey("Set", func() {
			Convey("nil", func() {
				rsp, err := srv.Set(ctx, nil)
				So(err, ShouldErrLike, "policy is required")
				So(rsp, ShouldBeNil)
			})

			Convey("empty", func() {
				req := &pb.SetRequest{}
				rsp, err := srv.Set(ctx, req)
				So(err, ShouldErrLike, "policy is required")
				So(rsp, ShouldBeNil)
			})

			Convey("negative", func() {
				req := &pb.SetRequest{
					Policy:    "quota",
					Resources: -1,
				}
				rsp, err := srv.Set(ctx, req)
				So(err, ShouldErrLike, "resources must not be negative")
				So(rsp, ShouldBeNil)
			})

			Convey("not found", func() {
				req := &pb.SetRequest{
					Policy: "quota/user",
				}

				rsp, err := srv.Set(ctx, req)
				So(err, ShouldErrLike, "not found")
				So(rsp, ShouldBeNil)
			})

			Convey("user unspecified", func() {
				req := &pb.SetRequest{
					Policy: "quota/${user}",
				}

				rsp, err := srv.Set(ctx, req)
				So(err, ShouldErrLike, "user not specified")
				So(rsp, ShouldBeNil)
			})

			Convey("new", func() {
				req := &pb.SetRequest{
					Policy:    "quota",
					Resources: 2,
				}

				rsp, err := srv.Set(ctx, req)
				So(err, ShouldBeNil)
				So(rsp, ShouldResemble, &pb.QuotaEntry{
					Name:      "quota",
					DbName:    "entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					Resources: 2,
				})

				So(s.Keys(), ShouldResemble, []string{
					"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
				})
				So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), ShouldEqual, "2")
				So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), ShouldEqual, now)
			})

			Convey("existing", func() {
				req := &pb.SetRequest{
					Policy:    "quota/${user}",
					User:      "user@example.com",
					Resources: 2,
				}

				rsp, err := srv.Set(ctx, req)
				So(err, ShouldBeNil)
				So(rsp, ShouldResemble, &pb.QuotaEntry{
					Name:      "quota/user@example.com",
					DbName:    "entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
					Resources: 2,
				})

				So(s.Keys(), ShouldResemble, []string{
					"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
				})
				So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), ShouldEqual, "2")
				So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), ShouldEqual, now)
			})

			Convey("zero", func() {
				req := &pb.SetRequest{
					Policy: "quota/${user}",
					User:   "user@example.com",
				}

				rsp, err := srv.Set(ctx, req)
				So(err, ShouldBeNil)
				So(rsp, ShouldResemble, &pb.QuotaEntry{
					Name:   "quota/user@example.com",
					DbName: "entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
				})

				So(s.Keys(), ShouldResemble, []string{
					"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
				})
				So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "resources"), ShouldEqual, "0")
				So(s.HGet("entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d", "updated"), ShouldEqual, now)
			})

			Convey("excessive", func() {
				req := &pb.SetRequest{
					Policy:    "quota",
					Resources: 10,
				}

				rsp, err := srv.Set(ctx, req)
				So(err, ShouldBeNil)
				So(rsp, ShouldResemble, &pb.QuotaEntry{
					Name:      "quota",
					DbName:    "entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					Resources: 2,
				})

				// Ensure resources were capped in the database.
				So(s.Keys(), ShouldResemble, []string{
					"entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7",
					"entry:f20c860d2ea007ea2360c6ebe2d943acc8a531412c18ff3bd47ab1449988aa6d",
				})
				So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "resources"), ShouldEqual, "2")
				So(s.HGet("entry:b878a6801d9a9e68b30ed63430bb5e0bddcd984a37a3ee385abc27ff031c7fe7", "updated"), ShouldEqual, now)
			})
		})
	})
}
