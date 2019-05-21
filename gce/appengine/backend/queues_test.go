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
	"math/rand"
	"net/http"
	"testing"
	"time"

	"google.golang.org/api/compute/v1"
	"google.golang.org/genproto/googleapis/type/dayofweek"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/appengine/tq/tqtesting"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"

	"go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/api/projects/v1"
	"go.chromium.org/luci/gce/api/tasks/v1"
	"go.chromium.org/luci/gce/appengine/model"
	rpc "go.chromium.org/luci/gce/appengine/rpc/memory"
	"go.chromium.org/luci/gce/appengine/testing/roundtripper"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestQueues(t *testing.T) {
	t.Parallel()

	Convey("queues", t, func() {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)
		srv := &rpc.Config{}
		rt := &roundtripper.JSONRoundTripper{}
		gce, err := compute.New(&http.Client{Transport: rt})
		So(err, ShouldBeNil)
		c := withCompute(withConfig(withDispatcher(memory.Use(context.Background()), dsp), srv), gce)
		datastore.GetTestable(c).AutoIndex(true)
		datastore.GetTestable(c).Consistent(true)
		tqt := tqtesting.GetTestable(c, dsp)
		tqt.CreateQueues()

		Convey("countVMs", func() {
			Convey("invalid", func() {
				Convey("nil", func() {
					err := countVMs(c, nil)
					So(err, ShouldErrLike, "unexpected payload")
				})

				Convey("empty", func() {
					err := countVMs(c, &tasks.CountVMs{})
					So(err, ShouldErrLike, "ID is required")
				})
			})

			Convey("valid", func() {
				err := countVMs(c, &tasks.CountVMs{
					Id: "id",
				})
				So(err, ShouldBeNil)
			})
		})

		Convey("createVM", func() {
			Convey("invalid", func() {
				Convey("nil", func() {
					err := createVM(c, nil)
					So(err, ShouldErrLike, "unexpected payload")
				})

				Convey("empty", func() {
					err := createVM(c, &tasks.CreateVM{})
					So(err, ShouldErrLike, "is required")
				})

				Convey("ID", func() {
					err := createVM(c, &tasks.CreateVM{
						Config: "config",
					})
					So(err, ShouldErrLike, "ID is required")
				})

				Convey("config", func() {
					err := createVM(c, &tasks.CreateVM{
						Id: "id",
					})
					So(err, ShouldErrLike, "config is required")
				})
			})

			Convey("valid", func() {
				Convey("nil", func() {
					err := createVM(c, &tasks.CreateVM{
						Id:     "id",
						Index:  2,
						Config: "config",
					})
					So(err, ShouldBeNil)
					v := &model.VM{
						ID: "id",
					}
					So(datastore.Get(c, v), ShouldBeNil)
					So(v.Index, ShouldEqual, 2)
					So(v.Config, ShouldEqual, "config")
				})

				Convey("empty", func() {
					err := createVM(c, &tasks.CreateVM{
						Id:         "id",
						Attributes: &config.VM{},
						Index:      2,
						Config:     "config",
					})
					So(err, ShouldBeNil)
					v := &model.VM{
						ID: "id",
					}
					So(datastore.Get(c, v), ShouldBeNil)
					So(v.Index, ShouldEqual, 2)
				})

				Convey("non-empty", func() {
					c := mathrand.Set(c, rand.New(rand.NewSource(1)))
					err := createVM(c, &tasks.CreateVM{
						Id: "id",
						Attributes: &config.VM{
							Disk: []*config.Disk{
								{
									Image: "image",
								},
							},
						},
						Index:  2,
						Config: "config",
						Prefix: "prefix",
					})
					So(err, ShouldBeNil)
					v := &model.VM{
						ID: "id",
					}
					So(datastore.Get(c, v), ShouldBeNil)
					So(v, ShouldResemble, &model.VM{
						ID: "id",
						Attributes: config.VM{
							Disk: []*config.Disk{
								{
									Image: "image",
								},
							},
						},
						AttributesIndexed: []string{
							"disk.image:image",
						},
						Config:   "config",
						Hostname: "prefix-2-fpll",
						Index:    2,
						Prefix:   "prefix",
					})
				})

				Convey("not updated", func() {
					datastore.Put(c, &model.VM{
						ID: "id",
						Attributes: config.VM{
							Zone: "zone",
						},
						Drained: true,
					})
					err := createVM(c, &tasks.CreateVM{
						Id: "id",
						Attributes: &config.VM{
							Project: "project",
						},
						Config: "config",
						Index:  2,
					})
					So(err, ShouldBeNil)
					v := &model.VM{
						ID: "id",
					}
					So(datastore.Get(c, v), ShouldBeNil)
					So(v, ShouldResemble, &model.VM{
						ID: "id",
						Attributes: config.VM{
							Zone: "zone",
						},
						Drained: true,
					})
				})

				Convey("sets zone", func() {
					err := createVM(c, &tasks.CreateVM{
						Id: "id",
						Attributes: &config.VM{
							Disk: []*config.Disk{
								{
									Type: "{{.Zone}}/type",
								},
							},
							MachineType: "{{.Zone}}/type",
							Zone:        "zone",
						},
						Config: "config",
						Index:  2,
					})
					So(err, ShouldBeNil)
					v := &model.VM{
						ID: "id",
					}
					So(datastore.Get(c, v), ShouldBeNil)
					So(v.Attributes, ShouldResemble, config.VM{
						Disk: []*config.Disk{
							{
								Type: "zone/type",
							},
						},
						MachineType: "zone/type",
						Zone:        "zone",
					})
				})
			})
		})

		Convey("drainVM", func() {
			Convey("invalid", func() {
				Convey("config", func() {
					err := drainVM(c, &model.VM{
						ID: "id",
					})
					So(err, ShouldErrLike, "failed to fetch config")
				})
			})

			Convey("valid", func() {
				Convey("config", func() {
					Convey("drained", func() {
						datastore.Put(c, &model.Config{
							ID: "config",
							Config: config.Config{
								Amount: &config.Amount{
									Default: 2,
								},
							},
						})
						v := &model.VM{
							ID:      "id",
							Config:  "config",
							Drained: true,
						}
						So(datastore.Put(c, v), ShouldBeNil)
						So(drainVM(c, v), ShouldBeNil)
						So(v.Drained, ShouldBeTrue)
						So(datastore.Get(c, v), ShouldBeNil)
						So(v.Drained, ShouldBeTrue)
					})

					Convey("deleted", func() {
						v := &model.VM{
							ID:     "id",
							Config: "config",
						}
						So(datastore.Put(c, v), ShouldBeNil)
						So(drainVM(c, v), ShouldBeNil)
						So(v.Drained, ShouldBeTrue)
						So(datastore.Get(c, v), ShouldBeNil)
						So(v.Drained, ShouldBeTrue)
					})

					Convey("amount", func() {
						Convey("unspecified", func() {
							datastore.Put(c, &model.Config{
								ID: "config",
							})
							v := &model.VM{
								ID:     "id",
								Config: "config",
							}
							So(datastore.Put(c, v), ShouldBeNil)
							So(drainVM(c, v), ShouldBeNil)
							So(v.Drained, ShouldBeTrue)
							So(err, ShouldBeNil)
							So(datastore.Get(c, v), ShouldBeNil)
							So(v.Drained, ShouldBeTrue)
						})

						Convey("lesser", func() {
							datastore.Put(c, &model.Config{
								ID: "config",
								Config: config.Config{
									Amount: &config.Amount{
										Default: 1,
									},
								},
							})
							v := &model.VM{
								ID:     "id",
								Config: "config",
								Index:  2,
							}
							So(datastore.Put(c, v), ShouldBeNil)
							So(drainVM(c, v), ShouldBeNil)
							So(v.Drained, ShouldBeTrue)
							So(datastore.Get(c, v), ShouldBeNil)
							So(v.Drained, ShouldBeTrue)
						})

						Convey("equal", func() {
							datastore.Put(c, &model.Config{
								ID: "config",
								Config: config.Config{
									Amount: &config.Amount{
										Default: 2,
									},
								},
							})
							v := &model.VM{
								ID:     "id",
								Config: "config",
								Index:  2,
							}
							So(datastore.Put(c, v), ShouldBeNil)
							So(drainVM(c, v), ShouldBeNil)
							So(v.Drained, ShouldBeTrue)
							So(datastore.Get(c, v), ShouldBeNil)
							So(v.Drained, ShouldBeTrue)
						})

						Convey("greater", func() {
							datastore.Put(c, &model.Config{
								ID: "config",
								Config: config.Config{
									Amount: &config.Amount{
										Default: 3,
									},
								},
							})
							v := &model.VM{
								ID:     "id",
								Config: "config",
								Index:  2,
							}
							So(datastore.Put(c, v), ShouldBeNil)
							So(drainVM(c, v), ShouldBeNil)
							So(v.Drained, ShouldBeFalse)
							So(datastore.Get(c, v), ShouldBeNil)
							So(v.Drained, ShouldBeFalse)
						})

						Convey("schedule", func() {
							datastore.Put(c, &model.Config{
								ID: "config",
								Config: config.Config{
									Amount: &config.Amount{
										Default: 2,
										Change: []*config.Schedule{
											{
												Amount: 3,
												Length: &config.TimePeriod{
													Time: &config.TimePeriod_Duration{
														Duration: "1h",
													},
												},
												Start: &config.TimeOfDay{
													Day:  dayofweek.DayOfWeek_MONDAY,
													Time: "1:00",
												},
											},
										},
									},
								},
							})

							Convey("lesser", func() {
								now := time.Time{}.Add(time.Hour)
								So(now.Weekday(), ShouldEqual, time.Monday)
								So(now.Hour(), ShouldEqual, 1)
								c, _ := testclock.UseTime(c, now)
								v := &model.VM{
									ID:     "id",
									Config: "config",
									Index:  4,
								}
								So(datastore.Put(c, v), ShouldBeNil)
								So(drainVM(c, v), ShouldBeNil)
								So(v.Drained, ShouldBeTrue)
								So(datastore.Get(c, v), ShouldBeNil)
								So(v.Drained, ShouldBeTrue)
							})

							Convey("equal", func() {
								now := time.Time{}
								So(now.Weekday(), ShouldEqual, time.Monday)
								c, _ := testclock.UseTime(c, now)
								v := &model.VM{
									ID:     "id",
									Config: "config",
									Index:  3,
								}
								So(datastore.Put(c, v), ShouldBeNil)
								So(drainVM(c, v), ShouldBeNil)
								So(v.Drained, ShouldBeTrue)
								So(datastore.Get(c, v), ShouldBeNil)
								So(v.Drained, ShouldBeTrue)
							})

							Convey("greater", func() {
								now := time.Time{}.Add(time.Hour)
								So(now.Weekday(), ShouldEqual, time.Monday)
								So(now.Hour(), ShouldEqual, 1)
								c, _ := testclock.UseTime(c, now)
								v := &model.VM{
									ID:     "id",
									Config: "config",
									Index:  2,
								}
								So(datastore.Put(c, v), ShouldBeNil)
								So(drainVM(c, v), ShouldBeNil)
								So(v.Drained, ShouldBeFalse)
								So(datastore.Get(c, v), ShouldBeNil)
								So(v.Drained, ShouldBeFalse)
							})
						})
					})
				})

				Convey("deleted", func() {
					v := &model.VM{
						ID:     "id",
						Config: "config",
					}
					So(drainVM(c, v), ShouldBeNil)
					So(v.Drained, ShouldBeTrue)
					So(datastore.Get(c, v), ShouldEqual, datastore.ErrNoSuchEntity)
				})
			})
		})

		Convey("expandConfig", func() {
			Convey("invalid", func() {
				Convey("nil", func() {
					err := expandConfig(c, nil)
					So(err, ShouldErrLike, "unexpected payload")
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})

				Convey("empty", func() {
					err := expandConfig(c, &tasks.ExpandConfig{})
					So(err, ShouldErrLike, "ID is required")
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})

				Convey("missing", func() {
					err := expandConfig(c, &tasks.ExpandConfig{
						Id: "id",
					})
					So(err, ShouldErrLike, "failed to fetch config")
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})
			})

			Convey("valid", func() {
				Convey("none", func() {
					srv.Ensure(c, &config.EnsureRequest{
						Id: "id",
						Config: &config.Config{
							Attributes: &config.VM{
								Project: "project",
							},
							Prefix: "prefix",
						},
					})
					err := expandConfig(c, &tasks.ExpandConfig{
						Id: "id",
					})
					So(err, ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})

				Convey("default", func() {
					srv.Ensure(c, &config.EnsureRequest{
						Id: "id",
						Config: &config.Config{
							Attributes: &config.VM{
								Project: "project",
							},
							Amount: &config.Amount{
								Default: 3,
							},
							Prefix: "prefix",
						},
					})
					err := expandConfig(c, &tasks.ExpandConfig{
						Id: "id",
					})
					So(err, ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldHaveLength, 3)
				})

				Convey("schedule", func() {
					srv.Ensure(c, &config.EnsureRequest{
						Id: "id",
						Config: &config.Config{
							Attributes: &config.VM{
								Project: "project",
							},
							Amount: &config.Amount{
								Default: 2,
								Change: []*config.Schedule{
									{
										Amount: 5,
										Length: &config.TimePeriod{
											Time: &config.TimePeriod_Duration{
												Duration: "1h",
											},
										},
										Start: &config.TimeOfDay{
											Day:  dayofweek.DayOfWeek_MONDAY,
											Time: "1:00",
										},
									},
								},
							},
							Prefix: "prefix",
						},
					})

					Convey("default", func() {
						now := time.Time{}
						So(now.Weekday(), ShouldEqual, time.Monday)
						c, _ := testclock.UseTime(c, now)
						err := expandConfig(c, &tasks.ExpandConfig{
							Id: "id",
						})
						So(err, ShouldBeNil)
						So(tqt.GetScheduledTasks(), ShouldHaveLength, 2)
					})

					Convey("scheduled", func() {
						now := time.Time{}.Add(time.Hour)
						So(now.Weekday(), ShouldEqual, time.Monday)
						So(now.Hour(), ShouldEqual, 1)
						c, _ := testclock.UseTime(c, now)
						err := expandConfig(c, &tasks.ExpandConfig{
							Id: "id",
						})
						So(err, ShouldBeNil)
						So(tqt.GetScheduledTasks(), ShouldHaveLength, 5)
					})
				})
			})
		})

		Convey("reportQuota", func() {
			Convey("invalid", func() {
				Convey("nil", func() {
					err := reportQuota(c, nil)
					So(err, ShouldErrLike, "unexpected payload")
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})

				Convey("empty", func() {
					err := reportQuota(c, &tasks.ReportQuota{})
					So(err, ShouldErrLike, "ID is required")
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})

				Convey("missing", func() {
					err := reportQuota(c, &tasks.ReportQuota{
						Id: "id",
					})
					So(err, ShouldErrLike, "failed to fetch project")
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})
			})

			Convey("valid", func() {
				rt.Handler = func(req interface{}) (int, interface{}) {
					return http.StatusOK, &compute.RegionList{
						Items: []*compute.Region{
							{
								Name: "ignore",
							},
							{
								Name: "region",
								Quotas: []*compute.Quota{
									{
										Limit:  100.0,
										Metric: "ignore",
										Usage:  0.0,
									},
									{
										Limit:  100.0,
										Metric: "metric",
										Usage:  25.0,
									},
								},
							},
						},
					}
				}
				datastore.Put(c, &model.Project{
					ID: "id",
					Config: projects.Config{
						Metric:  []string{"metric"},
						Project: "project",
						Region:  []string{"region"},
					},
				})
				err := reportQuota(c, &tasks.ReportQuota{
					Id: "id",
				})
				So(err, ShouldBeNil)
			})
		})
	})
}
