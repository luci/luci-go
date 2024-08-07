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

package refresher

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmemory "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/quota/quotapb"
	"go.chromium.org/luci/server/tq/tqtesting"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/cvtesting"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestConfigRefreshCron(t *testing.T) {
	t.Parallel()

	Convey("Config refresh cron works", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		pm := mockPM{}
		qm := mockQM{}
		pcr := NewRefresher(ct.TQDispatcher, &pm, &qm, ct.Env)

		Convey("for a new project", func() {
			ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{
				config.MustProjectSet("chromium"): {ConfigFileName: ""},
			}))
			// Project chromium doesn't exist in datastore.
			err := pcr.SubmitRefreshTasks(ctx)
			So(err, ShouldBeNil)
			So(ct.TQ.Tasks().Payloads(), ShouldResembleProto, []protoreflect.ProtoMessage{
				&RefreshProjectConfigTask{Project: "chromium"},
			})
			ct.TQ.Run(ctx, tqtesting.StopAfterTask("refresh-project-config"))
			So(pm.updates, ShouldResemble, []string{"chromium"})
			So(qm.writes, ShouldResemble, []string{"chromium"})
		})

		Convey("for an existing project", func() {
			ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{
				config.MustProjectSet("chromium"): {ConfigFileName: ""},
			}))
			So(datastore.Put(ctx, &prjcfg.ProjectConfig{
				Project: "chromium",
				Enabled: true,
			}), ShouldBeNil)
			So(pcr.SubmitRefreshTasks(ctx), ShouldBeNil)
			So(ct.TQ.Tasks().Payloads(), ShouldResembleProto, []protoreflect.ProtoMessage{
				&RefreshProjectConfigTask{Project: "chromium"},
			})
			ct.TQ.Run(ctx, tqtesting.StopAfterTask("refresh-project-config"))
			So(pm.updates, ShouldResemble, []string{"chromium"})
			So(qm.writes, ShouldResemble, []string{"chromium"})
			pm.updates = nil
			qm.writes = nil

			Convey("randomly pokes existing projects even if there are no updates", func() {
				// Simulate cron runs every 1 minute and expect PM to be poked at least
				// once per pokePMInterval.
				ctx = mathrand.Set(ctx, rand.New(rand.NewSource(1234)))
				pokeBefore := ct.Clock.Now().Add(pokePMInterval)
				for ct.Clock.Now().Before(pokeBefore) {
					ct.Clock.Add(time.Minute)
					So(pcr.SubmitRefreshTasks(ctx), ShouldBeNil)
					ct.TQ.Run(ctx, tqtesting.StopAfterTask("refresh-project-config"))
					So(pm.updates, ShouldBeEmpty)
					So(qm.writes, ShouldResemble, []string{"chromium"})
					if len(pm.pokes) > 0 {
						break
					}
				}
				So(pm.pokes, ShouldResemble, []string{"chromium"})
			})
		})

		Convey("Disable project", func() {
			Convey("that doesn't have CV config", func() {
				ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{
					config.MustProjectSet("chromium"): {"other.cfg": ""},
				}))
				So(datastore.Put(ctx, &prjcfg.ProjectConfig{
					Project: "chromium",
					Enabled: true,
				}), ShouldBeNil)
				err := pcr.SubmitRefreshTasks(ctx)
				So(err, ShouldBeNil)
				So(ct.TQ.Tasks().Payloads(), ShouldResembleProto, []protoreflect.ProtoMessage{
					&RefreshProjectConfigTask{Project: "chromium", Disable: true},
				})
				ct.TQ.Run(ctx, tqtesting.StopAfterTask("refresh-project-config"))
				So(pm.updates, ShouldResemble, []string{"chromium"})
				So(qm.writes, ShouldResemble, []string{"chromium"})
			})
			Convey("that doesn't exist in LUCI Config", func() {
				ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{}))
				So(datastore.Put(ctx, &prjcfg.ProjectConfig{
					Project: "chromium",
					Enabled: true,
				}), ShouldBeNil)
				err := pcr.SubmitRefreshTasks(ctx)
				So(err, ShouldBeNil)
				So(ct.TQ.Tasks().Payloads(), ShouldResembleProto, []protoreflect.ProtoMessage{
					&RefreshProjectConfigTask{Project: "chromium", Disable: true},
				})
				ct.TQ.Run(ctx, tqtesting.StopAfterTask("refresh-project-config"))
				So(pm.updates, ShouldResemble, []string{"chromium"})
				So(qm.writes, ShouldResemble, []string{"chromium"})
			})
			Convey("Skip already disabled Project", func() {
				ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{}))
				So(datastore.Put(ctx, &prjcfg.ProjectConfig{
					Project: "foo",
					Enabled: false,
				}), ShouldBeNil)
				err := pcr.SubmitRefreshTasks(ctx)
				So(err, ShouldBeNil)
				So(ct.TQ.Tasks(), ShouldBeEmpty)
			})
		})
	})
}

type mockPM struct {
	pokes   []string
	updates []string
}

func (m *mockPM) Poke(ctx context.Context, luciProject string) error {
	m.pokes = append(m.pokes, luciProject)
	return nil
}

func (m *mockPM) UpdateConfig(ctx context.Context, luciProject string) error {
	m.updates = append(m.updates, luciProject)
	return nil
}

type mockQM struct {
	writes []string
}

func (qm *mockQM) WritePolicy(ctx context.Context, project string) (*quotapb.PolicyConfigID, error) {
	qm.writes = append(qm.writes, project)
	return nil, nil
}
