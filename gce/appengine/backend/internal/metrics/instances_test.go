// Copyright 2019 The LUCI Authors.
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

package metrics

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestInstances(t *testing.T) {
	t.Parallel()

	ftt.Run("Instances", t, func(t *ftt.Test) {
		c, _ := tsmon.WithDummyInMemory(memory.Use(context.Background()))
		datastore.GetTestable(c).AutoIndex(true)
		datastore.GetTestable(c).Consistent(true)
		s := tsmon.Store(c)

		t.Run("InstanceCount", func(t *ftt.Test) {
			ic := &InstanceCount{
				ScalingType: "dynamic",
			}
			assert.Loosely(t, ic.Configured, should.BeEmpty)
			assert.Loosely(t, ic.Created, should.BeEmpty)
			assert.Loosely(t, ic.Connected, should.BeEmpty)

			t.Run("Configured", func(t *ftt.Test) {
				ic.AddConfigured(1, "project-1")
				ic.AddConfigured(1, "project-2")
				ic.AddConfigured(1, "project-1")
				assert.Loosely(t, ic.Configured, should.HaveLength(2))
				assert.Loosely(t, ic.Configured, should.Contain(configuredCount{
					Count:   2,
					Project: "project-1",
				}))
				assert.Loosely(t, ic.Configured, should.Contain(configuredCount{
					Count:   1,
					Project: "project-2",
				}))
			})

			t.Run("Created", func(t *ftt.Test) {
				ic.AddCreated(1, "project", "zone-1")
				ic.AddCreated(1, "project", "zone-2")
				ic.AddCreated(1, "project", "zone-1")
				assert.Loosely(t, ic.Created, should.HaveLength(2))
				assert.Loosely(t, ic.Created, should.Contain(createdCount{
					Count:   2,
					Project: "project",
					Zone:    "zone-1",
				}))
				assert.Loosely(t, ic.Created, should.Contain(createdCount{
					Count:   1,
					Project: "project",
					Zone:    "zone-2",
				}))
			})

			t.Run("Connected", func(t *ftt.Test) {
				ic.AddConnected(1, "project", "server-1", "zone")
				ic.AddConnected(1, "project", "server-2", "zone")
				ic.AddConnected(1, "project", "server-1", "zone")
				assert.Loosely(t, ic.Connected, should.HaveLength(2))
				assert.Loosely(t, ic.Connected, should.Contain(connectedCount{
					Count:   2,
					Project: "project",
					Server:  "server-1",
					Zone:    "zone",
				}))
				assert.Loosely(t, ic.Connected, should.Contain(connectedCount{
					Count:   1,
					Project: "project",
					Server:  "server-2",
					Zone:    "zone",
				}))
			})

			t.Run("Update", func(t *ftt.Test) {
				ic.AddConfigured(1, "project")
				ic.AddCreated(1, "project", "zone")
				ic.AddConnected(1, "project", "server", "zone")
				assert.Loosely(t, ic.Update(c, "prefix", "resource_group", "dynamic"), should.BeNil)
				ic = &InstanceCount{
					ID: "prefix",
				}
				assert.Loosely(t, datastore.Get(c, ic), should.BeNil)
				assert.Loosely(t, ic.Configured, should.HaveLength(1))
				assert.Loosely(t, ic.Created, should.HaveLength(1))
				assert.Loosely(t, ic.Connected, should.HaveLength(1))
				assert.Loosely(t, ic.Prefix, should.Equal(ic.ID))
				assert.Loosely(t, ic.ResourceGroup, should.Equal("resource_group"))
				assert.Loosely(t, ic.ScalingType, should.Equal("dynamic"))
			})
		})

		t.Run("updateInstances", func(t *ftt.Test) {
			confFields := []any{"prefix", "project", "resource_group", "dynamic"}
			creaFields1 := []any{"prefix", "project", "dynamic", "zone-1"}
			creaFields2 := []any{"prefix", "project", "dynamic", "zone-2"}
			connFields := []any{"prefix", "project", "dynamic", "resource_group", "server", "zone"}

			ic := &InstanceCount{
				ID:            "prefix",
				ResourceGroup: "resource_group",
				ScalingType:   "dynamic",
				Configured: []configuredCount{
					{
						Count:   3,
						Project: "project",
					},
				},
				Created: []createdCount{
					{
						Count:   2,
						Project: "project",
						Zone:    "zone-1",
					},
					{
						Count:   1,
						Project: "project",
						Zone:    "zone-2",
					},
				},
				Connected: []connectedCount{
					{
						Count:   1,
						Project: "project",
						Server:  "server",
						Zone:    "zone",
					},
				},
				Prefix: "prefix",
			}

			ic.Computed = time.Time{}
			assert.Loosely(t, datastore.Put(c, ic), should.BeNil)
			updateInstances(c)
			assert.Loosely(t, s.Get(c, configuredInstances, time.Time{}, confFields), should.BeNil)
			assert.Loosely(t, s.Get(c, createdInstances, time.Time{}, creaFields1), should.BeNil)
			assert.Loosely(t, s.Get(c, createdInstances, time.Time{}, creaFields2), should.BeNil)
			assert.Loosely(t, s.Get(c, connectedInstances, time.Time{}, connFields), should.BeNil)
			assert.Loosely(t, datastore.Get(c, &InstanceCount{
				ID: ic.ID,
			}), should.Equal(datastore.ErrNoSuchEntity))

			ic.Computed = time.Now().UTC()
			assert.Loosely(t, datastore.Put(c, ic), should.BeNil)
			updateInstances(c)
			assert.Loosely(t, s.Get(c, configuredInstances, time.Time{}, confFields).(int64), should.Equal(3))
			assert.Loosely(t, s.Get(c, createdInstances, time.Time{}, creaFields1).(int64), should.Equal(2))
			assert.Loosely(t, s.Get(c, createdInstances, time.Time{}, creaFields2).(int64), should.Equal(1))
			assert.Loosely(t, s.Get(c, connectedInstances, time.Time{}, connFields).(int64), should.Equal(1))
			assert.Loosely(t, datastore.Get(c, &InstanceCount{
				ID: ic.ID,
			}), should.BeNil)
		})
	})
}
