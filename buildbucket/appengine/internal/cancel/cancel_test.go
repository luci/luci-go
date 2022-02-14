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

package cancel

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	// TODO(crbug/1242998): Remove once safe get becomes datastore default.
	_ "go.chromium.org/luci/gae/service/datastore/crbug1242998safeget"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCancelBuild(t *testing.T) {
	Convey("CancelBuild", t, func() {
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, sch := tq.TestingContext(ctx, nil)

		Convey("not found", func() {
			bld, err := Do(ctx, 1)
			So(err, ShouldErrLike, "not found")
			So(bld, ShouldBeNil)
			So(sch.Tasks(), ShouldBeEmpty)
		})

		Convey("found", func() {
			now := testclock.TestRecentTimeLocal
			ctx, _ = testclock.UseTime(ctx, now)
			So(datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
			}), ShouldBeNil)
			bld, err := Do(ctx, 1)
			So(err, ShouldBeNil)
			So(bld.Proto, ShouldResembleProto, &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				UpdateTime: timestamppb.New(now),
				EndTime:    timestamppb.New(now),
				Status:     pb.Status_CANCELED,
			})
			So(sch.Tasks(), ShouldHaveLength, 2)
		})

		Convey("ended", func() {
			So(datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status: pb.Status_SUCCESS,
				},
			}), ShouldBeNil)
			bld, err := Do(ctx, 1)
			So(err, ShouldBeNil)
			So(bld.Proto, ShouldResembleProto, &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status: pb.Status_SUCCESS,
			})
			So(sch.Tasks(), ShouldBeEmpty)
		})

		Convey("task cancellation", func() {
			now := testclock.TestRecentTimeLocal
			ctx, _ = testclock.UseTime(ctx, now)
			So(datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.BuildInfra{
				Build: datastore.MakeKey(ctx, "Build", 1),
				Proto: &pb.BuildInfra{
					Swarming: &pb.BuildInfra_Swarming{
						Hostname: "example.com",
						TaskId:   "id",
					},
				},
			}), ShouldBeNil)
			bld, err := Do(ctx, 1)
			So(err, ShouldBeNil)
			So(bld.Proto, ShouldResembleProto, &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				UpdateTime: timestamppb.New(now),
				EndTime:    timestamppb.New(now),
				Status:     pb.Status_CANCELED,
			})
			So(sch.Tasks(), ShouldHaveLength, 3)
		})

		Convey("resultdb finalization", func() {
			now := testclock.TestRecentTimeLocal
			ctx, _ = testclock.UseTime(ctx, now)
			So(datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.BuildInfra{
				Build: datastore.MakeKey(ctx, "Build", 1),
				Proto: &pb.BuildInfra{
					Resultdb: &pb.BuildInfra_ResultDB{
						Hostname:   "example.com",
						Invocation: "id",
					},
				},
			}), ShouldBeNil)
			bld, err := Do(ctx, 1)
			So(err, ShouldBeNil)
			So(bld.Proto, ShouldResembleProto, &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				UpdateTime: timestamppb.New(now),
				EndTime:    timestamppb.New(now),
				Status:     pb.Status_CANCELED,
			})
			So(sch.Tasks(), ShouldHaveLength, 3)
		})
	})

	Convey("Start", t, func() {
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, sch := tq.TestingContext(ctx, nil)
		now := testclock.TestRecentTimeLocal
		ctx, _ = testclock.UseTime(ctx, now)

		Convey("not found", func() {
			_, err := Start(ctx, 1, "")
			So(err, ShouldErrLike, "not found")
			So(sch.Tasks(), ShouldBeEmpty)
		})

		Convey("found", func() {
			So(datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status: pb.Status_STARTED,
				},
			}), ShouldBeNil)
			bld, err := Start(ctx, 1, "summary")
			So(err, ShouldBeNil)
			So(bld.Proto, ShouldResembleProto, &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				UpdateTime:      timestamppb.New(now),
				Status:          pb.Status_STARTED,
				CancelTime:      timestamppb.New(now),
				CanceledBy:      "buildbucket",
				SummaryMarkdown: "summary",
			})
			So(sch.Tasks(), ShouldHaveLength, 1)
		})

		Convey("ended", func() {
			So(datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status: pb.Status_SUCCESS,
				},
			}), ShouldBeNil)
			bld, err := Start(ctx, 1, "summary")
			So(err, ShouldBeNil)
			So(bld.Proto, ShouldResembleProto, &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status: pb.Status_SUCCESS,
			})
			So(sch.Tasks(), ShouldBeEmpty)
		})

		Convey("in cancel process", func() {
			So(datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status:     pb.Status_STARTED,
					CancelTime: timestamppb.New(now.Add(-time.Minute)),
				},
			}), ShouldBeNil)
			bld, err := Start(ctx, 1, "summary")
			So(err, ShouldBeNil)
			So(bld.Proto, ShouldResembleProto, &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status:     pb.Status_STARTED,
				CancelTime: timestamppb.New(now.Add(-time.Minute)),
			})
			So(sch.Tasks(), ShouldBeEmpty)
		})
	})
}
