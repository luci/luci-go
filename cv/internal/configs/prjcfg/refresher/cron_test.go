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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmemory "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/quota/quotapb"
	"go.chromium.org/luci/server/tq/tqtesting"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/cvtesting"
)

func TestConfigRefreshCron(t *testing.T) {
	t.Parallel()

	ftt.Run("Config refresh cron works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		pm := mockPM{}
		qm := mockQM{}
		pcr := NewRefresher(ct.TQDispatcher, &pm, &qm, ct.Env)

		t.Run("for a new project", func(t *ftt.Test) {
			ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{
				config.MustProjectSet("chromium"): {ConfigFileName: ""},
			}))
			// Project chromium doesn't exist in datastore.
			err := pcr.SubmitRefreshTasks(ctx)
			assert.NoErr(t, err)
			assert.Loosely(t, ct.TQ.Tasks().Payloads(), should.Resemble([]protoreflect.ProtoMessage{
				&RefreshProjectConfigTask{Project: "chromium"},
			}))
			ct.TQ.Run(ctx, tqtesting.StopAfterTask("refresh-project-config"))
			assert.Loosely(t, pm.updates, should.Resemble([]string{"chromium"}))
			assert.Loosely(t, qm.writes, should.Resemble([]string{"chromium"}))
		})

		t.Run("for an existing project", func(t *ftt.Test) {
			ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{
				config.MustProjectSet("chromium"): {ConfigFileName: ""},
			}))
			assert.Loosely(t, datastore.Put(ctx, &prjcfg.ProjectConfig{
				Project: "chromium",
				Enabled: true,
			}), should.BeNil)
			assert.Loosely(t, pcr.SubmitRefreshTasks(ctx), should.BeNil)
			assert.Loosely(t, ct.TQ.Tasks().Payloads(), should.Resemble([]protoreflect.ProtoMessage{
				&RefreshProjectConfigTask{Project: "chromium"},
			}))
			ct.TQ.Run(ctx, tqtesting.StopAfterTask("refresh-project-config"))
			assert.Loosely(t, pm.updates, should.Resemble([]string{"chromium"}))
			assert.Loosely(t, qm.writes, should.Resemble([]string{"chromium"}))
			pm.updates = nil
			qm.writes = nil

			t.Run("randomly pokes existing projects even if there are no updates", func(t *ftt.Test) {
				// Simulate cron runs every 1 minute and expect PM to be poked at least
				// once per pokePMInterval.
				ctx = mathrand.Set(ctx, rand.New(rand.NewSource(1234)))
				pokeBefore := ct.Clock.Now().Add(pokePMInterval)
				for ct.Clock.Now().Before(pokeBefore) {
					ct.Clock.Add(time.Minute)
					assert.Loosely(t, pcr.SubmitRefreshTasks(ctx), should.BeNil)
					ct.TQ.Run(ctx, tqtesting.StopAfterTask("refresh-project-config"))
					assert.Loosely(t, pm.updates, should.BeEmpty)
					assert.Loosely(t, qm.writes, should.Resemble([]string{"chromium"}))
					if len(pm.pokes) > 0 {
						break
					}
				}
				assert.Loosely(t, pm.pokes, should.Resemble([]string{"chromium"}))
			})
		})

		t.Run("Disable project", func(t *ftt.Test) {
			t.Run("that doesn't have CV config", func(t *ftt.Test) {
				ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{
					config.MustProjectSet("chromium"): {"other.cfg": ""},
				}))
				assert.Loosely(t, datastore.Put(ctx, &prjcfg.ProjectConfig{
					Project: "chromium",
					Enabled: true,
				}), should.BeNil)
				err := pcr.SubmitRefreshTasks(ctx)
				assert.NoErr(t, err)
				assert.Loosely(t, ct.TQ.Tasks().Payloads(), should.Resemble([]protoreflect.ProtoMessage{
					&RefreshProjectConfigTask{Project: "chromium", Disable: true},
				}))
				ct.TQ.Run(ctx, tqtesting.StopAfterTask("refresh-project-config"))
				assert.Loosely(t, pm.updates, should.Resemble([]string{"chromium"}))
				assert.Loosely(t, qm.writes, should.Resemble([]string{"chromium"}))
			})
			t.Run("that doesn't exist in LUCI Config", func(t *ftt.Test) {
				ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{}))
				assert.Loosely(t, datastore.Put(ctx, &prjcfg.ProjectConfig{
					Project: "chromium",
					Enabled: true,
				}), should.BeNil)
				err := pcr.SubmitRefreshTasks(ctx)
				assert.NoErr(t, err)
				assert.Loosely(t, ct.TQ.Tasks().Payloads(), should.Resemble([]protoreflect.ProtoMessage{
					&RefreshProjectConfigTask{Project: "chromium", Disable: true},
				}))
				ct.TQ.Run(ctx, tqtesting.StopAfterTask("refresh-project-config"))
				assert.Loosely(t, pm.updates, should.Resemble([]string{"chromium"}))
				assert.Loosely(t, qm.writes, should.Resemble([]string{"chromium"}))
			})
			t.Run("Skip already disabled Project", func(t *ftt.Test) {
				ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{}))
				assert.Loosely(t, datastore.Put(ctx, &prjcfg.ProjectConfig{
					Project: "foo",
					Enabled: false,
				}), should.BeNil)
				err := pcr.SubmitRefreshTasks(ctx)
				assert.NoErr(t, err)
				assert.Loosely(t, ct.TQ.Tasks(), should.BeEmpty)
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
