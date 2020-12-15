// Copyright 2020 The LUCI Authors.
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

package configcron

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmemory "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/gae/filter/txndefer"
	gaememory "go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	cvconfig "go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/prjmanager/pmtest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var testNow = testclock.TestTimeLocal.Round(1 * time.Millisecond)

func TestConfigRefreshCron(t *testing.T) {
	t.Parallel()

	Convey("Config refresh cron works", t, func() {
		ctx, tqScheduler := mkTestingCtx()

		Convey("for a new project", func() {
			ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{
				config.ProjectSet("chromium"): {cvconfig.ConfigFileName: ""},
			}))
			// Project chromium doesn't exist in datastore.
			err := SubmitRefreshTasks(ctx, false)
			So(err, ShouldBeNil)
			So(tqScheduler.Tasks().Payloads(), ShouldResembleProto, []*RefreshProjectConfigTask{
				{Project: "chromium"},
			})
			tqScheduler.Run(ctx, tqtesting.StopAfterTask("refresh-project-config"))
			So(pmtest.Projects(tqScheduler.Tasks()), ShouldResemble, []string{"chromium"})
		})

		Convey("for an existing project", func() {
			ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{
				config.ProjectSet("chromium"): {cvconfig.ConfigFileName: ""},
			}))
			So(datastore.Put(ctx, &cvconfig.ProjectConfig{
				Project: "chromium",
				Enabled: true,
			}), ShouldBeNil)
			err := SubmitRefreshTasks(ctx, false)
			So(err, ShouldBeNil)
			So(tqScheduler.Tasks().Payloads(), ShouldResembleProto, []*RefreshProjectConfigTask{
				{Project: "chromium"},
			})
			tqScheduler.Run(ctx, tqtesting.StopAfterTask("refresh-project-config"))
			So(pmtest.Projects(tqScheduler.Tasks()), ShouldResemble, []string{"chromium"})
		})

		Convey("Disable project", func() {
			Convey("that doesn't have CV config", func() {
				ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{
					config.ProjectSet("chromium"): {"other.cfg": ""},
				}))
				So(datastore.Put(ctx, &cvconfig.ProjectConfig{
					Project: "chromium",
					Enabled: true,
				}), ShouldBeNil)
				err := SubmitRefreshTasks(ctx, false)
				So(err, ShouldBeNil)
				So(tqScheduler.Tasks().Payloads(), ShouldResembleProto, []*RefreshProjectConfigTask{
					{Project: "chromium", Disable: true},
				})
				tqScheduler.Run(ctx, tqtesting.StopAfterTask("refresh-project-config"))
				So(pmtest.Projects(tqScheduler.Tasks()), ShouldResemble, []string{"chromium"})
			})
			Convey("that doesn't exist in LUCI Config", func() {
				ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{}))
				So(datastore.Put(ctx, &cvconfig.ProjectConfig{
					Project: "chromium",
					Enabled: true,
				}), ShouldBeNil)
				err := SubmitRefreshTasks(ctx, false)
				So(err, ShouldBeNil)
				So(tqScheduler.Tasks().Payloads(), ShouldResembleProto, []*RefreshProjectConfigTask{
					{Project: "chromium", Disable: true},
				})
				tqScheduler.Run(ctx, tqtesting.StopAfterTask("refresh-project-config"))
				So(pmtest.Projects(tqScheduler.Tasks()), ShouldResemble, []string{"chromium"})
			})

			Convey("Skip already disabled Project", func() {
				ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{}))
				So(datastore.Put(ctx, &cvconfig.ProjectConfig{
					Project: "foo",
					Enabled: false,
				}), ShouldBeNil)
				err := SubmitRefreshTasks(ctx, false)
				So(err, ShouldBeNil)
				So(tqScheduler.Tasks(), ShouldBeEmpty)
			})
		})
	})
}

func mkTestingCtx() (context.Context, *tqtesting.Scheduler) {
	ctx, tclock := testclock.UseTime(context.Background(), testNow)
	tclock.SetTimerCallback(func(dur time.Duration, _ clock.Timer) {
		// Move fake time forward whenever someone's waiting for it.
		tclock.Add(dur)
	})

	ctx = txndefer.FilterRDS(gaememory.Use(ctx))
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)

	ctx, scheduler := tq.TestingContext(ctx, nil)
	return ctx, scheduler
}

func toProtoText(msg proto.Message) string {
	bs, err := prototext.Marshal(msg)
	So(err, ShouldBeNil)
	return string(bs)
}
