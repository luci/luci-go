// Copyright 2018 The LUCI Authors.
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

package backend

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/appengine/tq/tqtesting"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/api/projects/v1"
	"go.chromium.org/luci/gce/api/tasks/v1"
	"go.chromium.org/luci/gce/appengine/model"
	"go.chromium.org/luci/gce/appengine/testing/roundtripper"
)

func TestCron(t *testing.T) {
	t.Parallel()

	ftt.Run("cron", t, func(t *ftt.Test) {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)
		rt := &roundtripper.JSONRoundTripper{}
		c := context.Background()
		gce, err := compute.NewService(c, option.WithHTTPClient(&http.Client{Transport: rt}))
		assert.Loosely(t, err, should.BeNil)
		c = withCompute(withDispatcher(memory.Use(c), dsp), ComputeService{Stable: gce})
		datastore.GetTestable(c).Consistent(true)
		tqt := tqtesting.GetTestable(c, dsp)
		tqt.CreateQueues()

		t.Run("countTasks", func(t *ftt.Test) {
			dsp := &tq.Dispatcher{}
			c = withDispatcher(c, dsp)
			q := datastore.NewQuery("TaskCount")

			t.Run("none", func(t *ftt.Test) {
				assert.Loosely(t, countTasks(c), should.BeNil)
				var k []*datastore.Key
				assert.Loosely(t, datastore.GetAll(c, q, &k), should.BeNil)
				assert.Loosely(t, k, should.BeEmpty)
			})

			t.Run("many", func(t *ftt.Test) {
				dsp.RegisterTask(&tasks.CountVMs{}, countVMs, countVMsQueue, nil)
				dsp.RegisterTask(&tasks.ManageBot{}, manageBot, manageBotQueue, nil)
				assert.Loosely(t, countTasks(c), should.BeNil)
				var k []*datastore.Key
				assert.Loosely(t, datastore.GetAll(c, q, &k), should.BeNil)
				assert.Loosely(t, k, should.HaveLength(2))
			})
		})

		t.Run("countVMsAsync", func(t *ftt.Test) {
			t.Run("none", func(t *ftt.Test) {
				assert.Loosely(t, countVMsAsync(c), should.BeNil)
				assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
			})

			t.Run("one", func(t *ftt.Test) {
				datastore.Put(c, &model.Config{
					ID: "id",
				})
				assert.Loosely(t, countVMsAsync(c), should.BeNil)
				assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(1))
			})
		})

		t.Run("createInstancesAsync", func(t *ftt.Test) {
			t.Run("none", func(t *ftt.Test) {
				t.Run("zero", func(t *ftt.Test) {
					assert.Loosely(t, createInstancesAsync(c), should.BeNil)
					assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
				})

				t.Run("exists", func(t *ftt.Test) {
					datastore.Put(c, &model.VM{
						ID:  "id",
						URL: "url",
					})
					assert.Loosely(t, createInstancesAsync(c), should.BeNil)
					assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
				})
			})

			t.Run("one", func(t *ftt.Test) {
				datastore.Put(c, &model.VM{
					ID: "id",
					Attributes: config.VM{
						Zone: "zone",
					},
				})
				assert.Loosely(t, createInstancesAsync(c), should.BeNil)
				assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(1))
			})
		})

		t.Run("expandConfigsAsync", func(t *ftt.Test) {
			t.Run("none", func(t *ftt.Test) {
				assert.Loosely(t, expandConfigsAsync(c), should.BeNil)
				assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
			})

			t.Run("one", func(t *ftt.Test) {
				datastore.Put(c, &model.Config{
					ID: "id",
				})
				assert.Loosely(t, expandConfigsAsync(c), should.BeNil)
				ts := tqt.GetScheduledTasks()
				assert.Loosely(t, ts, should.HaveLength(1))
				cfg, ok := ts[0].Payload.(*tasks.ExpandConfig)
				assert.Loosely(t, ok, should.BeTrue)
				assert.Loosely(t, cfg.GetId(), should.NotBeEmpty)
				assert.Loosely(t, cfg.GetTriggeredUnixTime(), should.BeGreaterThan(1704096000)) // 2024-01-01
			})

			t.Run("many", func(t *ftt.Test) {
				for i := 0; i < 100; i++ {
					datastore.Put(c, &model.Config{
						ID: fmt.Sprintf("id-%d", i),
					})
				}
				assert.Loosely(t, expandConfigsAsync(c), should.BeNil)
				assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(100))
			})
		})

		t.Run("manageBotsAsync", func(t *ftt.Test) {
			t.Run("none", func(t *ftt.Test) {
				t.Run("missing", func(t *ftt.Test) {
					assert.Loosely(t, manageBotsAsync(c), should.BeNil)
					assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
				})

				t.Run("url", func(t *ftt.Test) {
					datastore.Put(c, &model.VM{
						ID: "id",
					})
					assert.Loosely(t, manageBotsAsync(c), should.BeNil)
					assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
				})
			})

			t.Run("one", func(t *ftt.Test) {
				datastore.Put(c, &model.VM{
					ID:  "id",
					URL: "url",
				})
				assert.Loosely(t, manageBotsAsync(c), should.BeNil)
				assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(1))
			})
		})

		t.Run("drainVMsAsync", func(t *ftt.Test) {
			t.Run("none", func(t *ftt.Test) {
				// No VMs will be drained as config is set for 2 VMs
				assert.Loosely(t, datastore.Put(c, &model.VM{
					ID:     "id-0",
					Index:  0,
					Config: "id",
				}), should.BeNil)
				assert.Loosely(t, datastore.Put(c, &model.VM{
					ID:     "id-1",
					Index:  1,
					Config: "id",
				}), should.BeNil)
				assert.Loosely(t, datastore.Put(c, &model.Config{
					ID: "id",
					Config: &config.Config{
						Prefix:        "id",
						CurrentAmount: 2,
					},
				}), should.BeNil)
				t.Run("missing", func(t *ftt.Test) {
					assert.Loosely(t, drainVMsAsync(c), should.BeNil)
					assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
				})
			})

			t.Run("one", func(t *ftt.Test) {
				// 1 VM will be drained as config is set for 1 VM
				assert.Loosely(t, datastore.Put(c, &model.VM{
					ID:     "id-0",
					Index:  0,
					Config: "id",
				}), should.BeNil)
				assert.Loosely(t, datastore.Put(c, &model.VM{
					ID:     "id-1",
					Index:  1,
					Config: "id",
				}), should.BeNil)
				assert.Loosely(t, datastore.Put(c, &model.Config{
					ID: "id",
					Config: &config.Config{
						Prefix:        "id",
						CurrentAmount: 1,
					},
				}), should.BeNil)
				t.Run("missing", func(t *ftt.Test) {
					assert.Loosely(t, drainVMsAsync(c), should.BeNil)
					assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(1))
				})
			})

			t.Run("all", func(t *ftt.Test) {
				// 2 VMs will be drained as config is set for 0 VM
				assert.Loosely(t, datastore.Put(c, &model.VM{
					ID:     "id-0",
					Index:  0,
					Config: "id",
				}), should.BeNil)
				assert.Loosely(t, datastore.Put(c, &model.VM{
					ID:     "id-1",
					Index:  1,
					Config: "id",
				}), should.BeNil)
				assert.Loosely(t, datastore.Put(c, &model.Config{
					ID: "id",
					Config: &config.Config{
						Prefix:        "id",
						CurrentAmount: 0,
					},
				}), should.BeNil)
				t.Run("missing", func(t *ftt.Test) {
					assert.Loosely(t, drainVMsAsync(c), should.BeNil)
					assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(2))
				})
			})
		})

		t.Run("payloadFactory", func(t *ftt.Test) {
			f := payloadFactory(&tasks.CountVMs{})
			p := f("id")
			assert.Loosely(t, p, should.Match(&tasks.CountVMs{
				Id: "id",
			}))
		})

		t.Run("reportQuotasAsync", func(t *ftt.Test) {
			t.Run("none", func(t *ftt.Test) {
				assert.Loosely(t, reportQuotasAsync(c), should.BeNil)
				assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
			})

			t.Run("one", func(t *ftt.Test) {
				datastore.Put(c, &model.Project{
					ID: "id",
				})
				assert.Loosely(t, reportQuotasAsync(c), should.BeNil)
				assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(1))
			})
		})

		t.Run("auditInstances", func(t *ftt.Test) {
			t.Run("none", func(t *ftt.Test) {
				assert.Loosely(t, auditInstances(c), should.BeNil)
				assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
			})

			t.Run("zero", func(t *ftt.Test) {
				count := 0
				rt.Handler = func(req any) (int, any) {
					switch count {
					case 0:
						count += 1
						return http.StatusOK, &compute.ZoneList{
							Items: []*compute.Zone{},
						}
					default:
						count += 1
						return http.StatusInternalServerError, nil
					}
				}
				err := datastore.Put(c, &model.Project{
					ID: "id",
					Config: &projects.Config{
						Project: "gnu-hurd",
					},
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, auditInstances(c), should.BeNil)
				assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(0))
			})

			t.Run("one", func(t *ftt.Test) {
				count := 0
				rt.Handler = func(req any) (int, any) {
					switch count {
					case 0:
						count += 1
						return http.StatusOK, &compute.ZoneList{
							Items: []*compute.Zone{{
								Name: "us-mex-1",
							}, {
								Name: "us-num-1",
							}},
						}
					default:
						count += 1
						return http.StatusInternalServerError, nil
					}
				}
				err := datastore.Put(c, &model.Config{
					ID: "id",
					Config: &config.Config{
						Attributes: &config.VM{
							Project: "gnu-hurd",
						},
					},
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, auditInstances(c), should.BeNil)
				assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(2))
			})

			t.Run("two", func(t *ftt.Test) {
				count := 0
				rt.Handler = func(req any) (int, any) {
					switch count {
					case 0:
						count += 1
						return http.StatusOK, &compute.ZoneList{
							Items: []*compute.Zone{{
								Name: "us-mex-1",
							}, {
								Name: "us-num-1",
							}},
						}
					case 1:
						count += 1
						return http.StatusOK, &compute.ZoneList{
							Items: []*compute.Zone{{
								Name: "us-can-2",
							}},
						}
					default:
						count += 1
						return http.StatusInternalServerError, nil
					}
				}
				err := datastore.Put(c, &model.Config{
					ID: "id",
					Config: &config.Config{
						Attributes: &config.VM{
							Project: "gnu-hurd",
						},
					},
				})
				assert.Loosely(t, err, should.BeNil)
				err = datastore.Put(c, &model.Config{
					ID: "id2",
					Config: &config.Config{
						Attributes: &config.VM{
							Project: "libreboot",
						},
					},
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, auditInstances(c), should.BeNil)
				assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(3))
			})
		})

		t.Run("trigger", func(t *ftt.Test) {
			t.Run("none", func(t *ftt.Test) {
				assert.Loosely(t, trigger(c, &tasks.ManageBot{}, datastore.NewQuery(model.VMKind)), should.BeNil)
				assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
			})

			t.Run("one", func(t *ftt.Test) {
				datastore.Put(c, &model.VM{
					ID: "id",
				})
				assert.Loosely(t, trigger(c, &tasks.ManageBot{}, datastore.NewQuery(model.VMKind)), should.BeNil)
				assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(1))
				assert.Loosely(t, tqt.GetScheduledTasks()[0].Payload, should.Match(&tasks.ManageBot{
					Id: "id",
				}))
			})
		})
	})
}
