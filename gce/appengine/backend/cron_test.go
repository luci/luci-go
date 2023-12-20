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
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/api/projects/v1"
	"go.chromium.org/luci/gce/api/tasks/v1"
	"go.chromium.org/luci/gce/appengine/model"
	"go.chromium.org/luci/gce/appengine/testing/roundtripper"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCron(t *testing.T) {
	t.Parallel()

	Convey("cron", t, func() {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)
		rt := &roundtripper.JSONRoundTripper{}
		c := context.Background()
		gce, err := compute.NewService(c, option.WithHTTPClient(&http.Client{Transport: rt}))
		So(err, ShouldBeNil)
		c = withCompute(withDispatcher(memory.Use(c), dsp), ComputeService{Stable: gce})
		datastore.GetTestable(c).Consistent(true)
		tqt := tqtesting.GetTestable(c, dsp)
		tqt.CreateQueues()

		Convey("countTasks", func() {
			dsp := &tq.Dispatcher{}
			c = withDispatcher(c, dsp)
			q := datastore.NewQuery("TaskCount")

			Convey("none", func() {
				So(countTasks(c), ShouldBeNil)
				var k []*datastore.Key
				So(datastore.GetAll(c, q, &k), ShouldBeNil)
				So(k, ShouldBeEmpty)
			})

			Convey("many", func() {
				dsp.RegisterTask(&tasks.CountVMs{}, countVMs, countVMsQueue, nil)
				dsp.RegisterTask(&tasks.ManageBot{}, manageBot, manageBotQueue, nil)
				So(countTasks(c), ShouldBeNil)
				var k []*datastore.Key
				So(datastore.GetAll(c, q, &k), ShouldBeNil)
				So(k, ShouldHaveLength, 2)
			})
		})

		Convey("countVMsAsync", func() {
			Convey("none", func() {
				So(countVMsAsync(c), ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldBeEmpty)
			})

			Convey("one", func() {
				datastore.Put(c, &model.Config{
					ID: "id",
				})
				So(countVMsAsync(c), ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
			})
		})

		Convey("createInstancesAsync", func() {
			Convey("none", func() {
				Convey("zero", func() {
					So(createInstancesAsync(c), ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})

				Convey("exists", func() {
					datastore.Put(c, &model.VM{
						ID:  "id",
						URL: "url",
					})
					So(createInstancesAsync(c), ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})
			})

			Convey("one", func() {
				datastore.Put(c, &model.VM{
					ID: "id",
					Attributes: config.VM{
						Zone: "zone",
					},
				})
				So(createInstancesAsync(c), ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
			})
		})

		Convey("expandConfigsAsync", func() {
			Convey("none", func() {
				So(expandConfigsAsync(c), ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldBeEmpty)
			})

			Convey("one", func() {
				datastore.Put(c, &model.Config{
					ID: "id",
				})
				So(expandConfigsAsync(c), ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
			})

			Convey("many", func() {
				for i := 0; i < 100; i++ {
					datastore.Put(c, &model.Config{
						ID: fmt.Sprintf("id-%d", i),
					})
				}
				So(expandConfigsAsync(c), ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldHaveLength, 100)
			})
		})

		Convey("manageBotsAsync", func() {
			Convey("none", func() {
				Convey("missing", func() {
					So(manageBotsAsync(c), ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})

				Convey("url", func() {
					datastore.Put(c, &model.VM{
						ID: "id",
					})
					So(manageBotsAsync(c), ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})
			})

			Convey("one", func() {
				datastore.Put(c, &model.VM{
					ID:  "id",
					URL: "url",
				})
				So(manageBotsAsync(c), ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
			})
		})

		Convey("drainVMsAsync", func() {
			Convey("none", func() {
				// No VMs will be drained as config is set for 2 VMs
				So(datastore.Put(c, &model.VM{
					ID:     "id-0",
					Index:  0,
					Config: "id",
				}), ShouldBeNil)
				So(datastore.Put(c, &model.VM{
					ID:     "id-1",
					Index:  1,
					Config: "id",
				}), ShouldBeNil)
				So(datastore.Put(c, &model.Config{
					ID: "id",
					Config: &config.Config{
						Prefix:        "id",
						CurrentAmount: 2,
					},
				}), ShouldBeNil)
				Convey("missing", func() {
					So(drainVMsAsync(c), ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})
			})

			Convey("one", func() {
				// 1 VM will be drained as config is set for 1 VM
				So(datastore.Put(c, &model.VM{
					ID:     "id-0",
					Index:  0,
					Config: "id",
				}), ShouldBeNil)
				So(datastore.Put(c, &model.VM{
					ID:     "id-1",
					Index:  1,
					Config: "id",
				}), ShouldBeNil)
				So(datastore.Put(c, &model.Config{
					ID: "id",
					Config: &config.Config{
						Prefix:        "id",
						CurrentAmount: 1,
					},
				}), ShouldBeNil)
				Convey("missing", func() {
					So(drainVMsAsync(c), ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
				})
			})

			Convey("all", func() {
				// 2 VMs will be drained as config is set for 0 VM
				So(datastore.Put(c, &model.VM{
					ID:     "id-0",
					Index:  0,
					Config: "id",
				}), ShouldBeNil)
				So(datastore.Put(c, &model.VM{
					ID:     "id-1",
					Index:  1,
					Config: "id",
				}), ShouldBeNil)
				So(datastore.Put(c, &model.Config{
					ID: "id",
					Config: &config.Config{
						Prefix:        "id",
						CurrentAmount: 0,
					},
				}), ShouldBeNil)
				Convey("missing", func() {
					So(drainVMsAsync(c), ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldHaveLength, 2)
				})
			})
		})

		Convey("payloadFactory", func() {
			f := payloadFactory(&tasks.CountVMs{})
			p := f("id")
			So(p, ShouldResemble, &tasks.CountVMs{
				Id: "id",
			})
		})

		Convey("reportQuotasAsync", func() {
			Convey("none", func() {
				So(reportQuotasAsync(c), ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldBeEmpty)
			})

			Convey("one", func() {
				datastore.Put(c, &model.Project{
					ID: "id",
				})
				So(reportQuotasAsync(c), ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
			})
		})

		Convey("auditInstances", func() {
			Convey("none", func() {
				So(auditInstances(c), ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldBeEmpty)
			})

			Convey("zero", func() {
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
				So(err, ShouldBeNil)
				So(auditInstances(c), ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldHaveLength, 0)
			})

			Convey("one", func() {
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
				err := datastore.Put(c, &model.Project{
					ID: "id",
					Config: &projects.Config{
						Project: "gnu-hurd",
					},
				})
				So(err, ShouldBeNil)
				So(auditInstances(c), ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldHaveLength, 2)
			})

			Convey("two", func() {
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
				err := datastore.Put(c, &model.Project{
					ID: "id",
					Config: &projects.Config{
						Project: "gnu-hurd",
					},
				})
				So(err, ShouldBeNil)
				err = datastore.Put(c, &model.Project{
					ID: "id2",
					Config: &projects.Config{
						Project: "libreboot",
					},
				})
				So(err, ShouldBeNil)
				So(auditInstances(c), ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldHaveLength, 3)
			})
		})

		Convey("trigger", func() {
			Convey("none", func() {
				So(trigger(c, &tasks.ManageBot{}, datastore.NewQuery(model.VMKind)), ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldBeEmpty)
			})

			Convey("one", func() {
				datastore.Put(c, &model.VM{
					ID: "id",
				})
				So(trigger(c, &tasks.ManageBot{}, datastore.NewQuery(model.VMKind)), ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
				So(tqt.GetScheduledTasks()[0].Payload, ShouldResembleProto, &tasks.ManageBot{
					Id: "id",
				})
			})
		})
	})
}
