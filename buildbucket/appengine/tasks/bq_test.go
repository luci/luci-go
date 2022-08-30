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

package tasks

import (
	"context"
	"testing"

	"cloud.google.com/go/bigquery"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/internal/clients"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestBQ(t *testing.T) {
	t.Parallel()

	Convey("ExportBuild", t, func() {
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		now := testclock.TestRecentTimeLocal
		ctx, _ = testclock.UseTime(ctx, now)
		fakeBq := &clients.FakeBqClient{}
		ctx = clients.WithBqClient(ctx, fakeBq)
		b := &model.Build{
			ID: 123,
			Proto: &pb.Build{
				Id: 123,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status:     pb.Status_CANCELED,
			},
		}
		bk := datastore.KeyForObj(ctx, b)
		bs := &model.BuildSteps{ID: 1, Build: bk}
		So(bs.FromProto([]*pb.Step{
			{
				Name: "step",
				SummaryMarkdown: "summary",
				Logs: []*pb.Log{{
					Name: "log1",
					Url: "url",
					ViewUrl: "view_url",
				},
				},
			},
		}), ShouldBeNil)
		bi := &model.BuildInfra{
			ID: 1,
			Build: bk,
			Proto: &pb.BuildInfra{
				Buildbucket: &pb.BuildInfra_Buildbucket{
					Hostname: "hostname",
				},
			},
		}
		So(datastore.Put(ctx, b, bi, bs), ShouldBeNil)

		Convey("build not found", func() {
			err := ExportBuild(ctx, 111)
			So(tq.Fatal.In(err), ShouldBeTrue)
			So(err, ShouldErrLike, "build 111 not found when exporting into BQ")
		})

		Convey("bad row", func() {
			ctx1 := context.WithValue(ctx, &clients.FakeBqErrCtxKey, bigquery.PutMultiError{bigquery.RowInsertionError{}})
			err := ExportBuild(ctx1, 123)
			So(err, ShouldErrLike, "bad row for build 123")
			So(tq.Fatal.In(err), ShouldBeTrue)
		})

		Convey("transient BQ err", func() {
			ctx1 := context.WithValue(ctx, &clients.FakeBqErrCtxKey, errors.New("transient"))
			err := ExportBuild(ctx1, 123)
			So(err, ShouldErrLike, "transient error when inserting BQ for build 123")
			So(transient.Tag.In(err), ShouldBeTrue)
		})

		Convey("success", func() {
			So(ExportBuild(ctx, 123), ShouldBeNil)
			rows := fakeBq.GetRows("raw", "completed_builds")
			So(len(rows), ShouldEqual, 1)
			So(rows[0].InsertID, ShouldEqual, "123")
			p, _ := rows[0].Message.(*pb.Build)
			So(p, ShouldResembleProto, &pb.Build{
				Id: 123,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status:     pb.Status_CANCELED,
				Steps:  []*pb.Step{{
					Name: "step",
					Logs: []*pb.Log{{Name: "log1"}},
				}},
				Infra: &pb.BuildInfra{Buildbucket: &pb.BuildInfra_Buildbucket{}},
				Input: &pb.Build_Input{},
				Output: &pb.Build_Output{},
			})
		})
	})
}
