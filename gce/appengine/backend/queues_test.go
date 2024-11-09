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

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/api/compute/v1"
	"google.golang.org/genproto/googleapis/type/dayofweek"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/appengine/tq/tqtesting"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
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

func TestQueues(t *testing.T) {
	t.Parallel()

	ftt.Run("queues", t, func(t *ftt.Test) {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)
		rt := &roundtripper.JSONRoundTripper{}
		gce, err := compute.New(&http.Client{Transport: rt})
		assert.Loosely(t, err, should.BeNil)
		c := withCompute(withDispatcher(memory.Use(context.Background()), dsp), ComputeService{Stable: gce})
		datastore.GetTestable(c).AutoIndex(true)
		datastore.GetTestable(c).Consistent(true)
		tqt := tqtesting.GetTestable(c, dsp)
		tqt.CreateQueues()

		t.Run("countVMs", func(t *ftt.Test) {
			t.Run("invalid", func(t *ftt.Test) {
				t.Run("nil", func(t *ftt.Test) {
					err := countVMs(c, nil)
					assert.Loosely(t, err, should.ErrLike("unexpected payload"))
				})

				t.Run("empty", func(t *ftt.Test) {
					err := countVMs(c, &tasks.CountVMs{})
					assert.Loosely(t, err, should.ErrLike("ID is required"))
				})
			})

			t.Run("valid", func(t *ftt.Test) {
				err := countVMs(c, &tasks.CountVMs{
					Id: "id",
				})
				assert.Loosely(t, err, should.BeNil)
			})
		})

		t.Run("createVM", func(t *ftt.Test) {
			t.Run("invalid", func(t *ftt.Test) {
				t.Run("nil", func(t *ftt.Test) {
					err := createVM(c, nil)
					assert.Loosely(t, err, should.ErrLike("unexpected payload"))
				})

				t.Run("empty", func(t *ftt.Test) {
					err := createVM(c, &tasks.CreateVM{})
					assert.Loosely(t, err, should.ErrLike("is required"))
				})

				t.Run("ID", func(t *ftt.Test) {
					err := createVM(c, &tasks.CreateVM{
						Config: "config",
					})
					assert.Loosely(t, err, should.ErrLike("ID is required"))
				})

				t.Run("config", func(t *ftt.Test) {
					err := createVM(c, &tasks.CreateVM{
						Id: "id",
					})
					assert.Loosely(t, err, should.ErrLike("config is required"))
				})
			})

			t.Run("valid", func(t *ftt.Test) {
				t.Run("nil", func(t *ftt.Test) {
					err := createVM(c, &tasks.CreateVM{
						Id:     "id",
						Index:  2,
						Config: "config",
					})
					assert.Loosely(t, err, should.BeNil)
					v := &model.VM{
						ID: "id",
					}
					assert.Loosely(t, datastore.Get(c, v), should.BeNil)
					assert.Loosely(t, v.Index, should.Equal(2))
					assert.Loosely(t, v.Config, should.Equal("config"))
				})

				t.Run("empty", func(t *ftt.Test) {
					err := createVM(c, &tasks.CreateVM{
						Id:         "id",
						Attributes: &config.VM{},
						Index:      2,
						Config:     "config",
					})
					assert.Loosely(t, err, should.BeNil)
					v := &model.VM{
						ID: "id",
					}
					assert.Loosely(t, datastore.Get(c, v), should.BeNil)
					assert.Loosely(t, v.Index, should.Equal(2))
				})

				t.Run("non-empty", func(t *ftt.Test) {
					c = mathrand.Set(c, rand.New(rand.NewSource(1)))
					err := createVM(c, &tasks.CreateVM{
						Id: "id",
						Attributes: &config.VM{
							Disk: []*config.Disk{
								{
									Image: "image",
								},
							},
						},
						ConfigExpandTime: &timestamppb.Timestamp{Seconds: testclock.TestTimeUTC.Unix()},
						Index:            2,
						Config:           "config",
						Prefix:           "prefix",
					})
					assert.Loosely(t, err, should.BeNil)
					v := &model.VM{
						ID: "id",
					}
					assert.Loosely(t, datastore.Get(c, v), should.BeNil)
					assert.Loosely(t, cmp.Diff(v, &model.VM{
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
						Config:         "config",
						ConfigExpanded: testclock.TestTimeUTC.Unix(),
						Hostname:       "prefix-2-fpll",
						Index:          2,
						Prefix:         "prefix",
					}, cmpopts.IgnoreUnexported(*v), protocmp.Transform()), should.BeEmpty)
				})

				t.Run("not updated", func(t *ftt.Test) {
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
					assert.Loosely(t, err, should.BeNil)
					v := &model.VM{
						ID: "id",
					}
					assert.Loosely(t, datastore.Get(c, v), should.BeNil)
					assert.Loosely(t, cmp.Diff(v, &model.VM{
						ID: "id",
						Attributes: config.VM{
							Zone: "zone",
						},
						Drained: true,
					}, cmpopts.IgnoreUnexported(*v), protocmp.Transform()), should.BeEmpty)
				})

				t.Run("sets zone", func(t *ftt.Test) {
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
					assert.Loosely(t, err, should.BeNil)
					v := &model.VM{
						ID: "id",
					}
					assert.Loosely(t, datastore.Get(c, v), should.BeNil)
					assert.Loosely(t, &v.Attributes, should.Resemble(&config.VM{
						Disk: []*config.Disk{
							{
								Type: "zone/type",
							},
						},
						MachineType: "zone/type",
						Zone:        "zone",
					}))
				})
			})
		})

		t.Run("drainVM", func(t *ftt.Test) {
			t.Run("invalid", func(t *ftt.Test) {
				t.Run("config", func(t *ftt.Test) {
					err := drainVM(c, &model.VM{
						ID: "id",
					})
					assert.Loosely(t, err, should.ErrLike("failed to fetch config"))
				})
			})

			t.Run("valid", func(t *ftt.Test) {
				t.Run("config", func(t *ftt.Test) {
					t.Run("drained", func(t *ftt.Test) {
						datastore.Put(c, &model.Config{
							ID: "config",
							Config: &config.Config{
								CurrentAmount: 2,
							},
						})
						v := &model.VM{
							ID:      "id",
							Config:  "config",
							Drained: true,
						}
						assert.Loosely(t, datastore.Put(c, v), should.BeNil)
						assert.Loosely(t, drainVM(c, v), should.BeNil)
						assert.Loosely(t, v.Drained, should.BeTrue)
						assert.Loosely(t, datastore.Get(c, v), should.BeNil)
						assert.Loosely(t, v.Drained, should.BeTrue)
					})

					t.Run("deleted", func(t *ftt.Test) {
						v := &model.VM{
							ID:     "id",
							Config: "config",
						}
						assert.Loosely(t, datastore.Put(c, v), should.BeNil)
						assert.Loosely(t, drainVM(c, v), should.BeNil)
						assert.Loosely(t, v.Drained, should.BeTrue)
						assert.Loosely(t, datastore.Get(c, v), should.BeNil)
						assert.Loosely(t, v.Drained, should.BeTrue)
					})

					t.Run("amount", func(t *ftt.Test) {
						t.Run("unspecified", func(t *ftt.Test) {
							datastore.Put(c, &model.Config{
								ID: "config",
							})
							v := &model.VM{
								ID:     "id",
								Config: "config",
							}
							assert.Loosely(t, datastore.Put(c, v), should.BeNil)
							assert.Loosely(t, drainVM(c, v), should.BeNil)
							assert.Loosely(t, v.Drained, should.BeTrue)
							assert.Loosely(t, err, should.BeNil)
							assert.Loosely(t, datastore.Get(c, v), should.BeNil)
							assert.Loosely(t, v.Drained, should.BeTrue)
						})

						t.Run("lesser", func(t *ftt.Test) {
							datastore.Put(c, &model.Config{
								ID: "config",
								Config: &config.Config{
									CurrentAmount: 1,
								},
							})
							v := &model.VM{
								ID:     "id",
								Config: "config",
								Index:  2,
							}
							assert.Loosely(t, datastore.Put(c, v), should.BeNil)
							assert.Loosely(t, drainVM(c, v), should.BeNil)
							assert.Loosely(t, v.Drained, should.BeTrue)
							assert.Loosely(t, datastore.Get(c, v), should.BeNil)
							assert.Loosely(t, v.Drained, should.BeTrue)
						})

						t.Run("equal", func(t *ftt.Test) {
							datastore.Put(c, &model.Config{
								ID: "config",
								Config: &config.Config{
									CurrentAmount: 2,
								},
							})
							v := &model.VM{
								ID:     "id",
								Config: "config",
								Index:  2,
							}
							assert.Loosely(t, datastore.Put(c, v), should.BeNil)
							assert.Loosely(t, drainVM(c, v), should.BeNil)
							assert.Loosely(t, v.Drained, should.BeTrue)
							assert.Loosely(t, datastore.Get(c, v), should.BeNil)
							assert.Loosely(t, v.Drained, should.BeTrue)
						})

						t.Run("greater", func(t *ftt.Test) {
							datastore.Put(c, &model.Config{
								ID: "config",
								Config: &config.Config{
									CurrentAmount: 3,
								},
							})
							v := &model.VM{
								ID:     "id",
								Config: "config",
								Index:  2,
							}
							assert.Loosely(t, datastore.Put(c, v), should.BeNil)
							assert.Loosely(t, drainVM(c, v), should.BeNil)
							assert.Loosely(t, v.Drained, should.BeFalse)
							assert.Loosely(t, datastore.Get(c, v), should.BeNil)
							assert.Loosely(t, v.Drained, should.BeFalse)
						})
					})

					t.Run("DUTs", func(t *ftt.Test) {
						t.Run("DUT is in config", func(t *ftt.Test) {
							datastore.Put(c, &model.Config{
								ID: "config",
								Config: &config.Config{
									Duts: map[string]*emptypb.Empty{
										"dut1": {},
										"dut2": {},
										"dut3": {},
									},
								},
							})
							v := &model.VM{
								ID:     "id",
								Config: "config",
								DUT:    "dut1",
							}
							assert.Loosely(t, datastore.Put(c, v), should.BeNil)
							assert.Loosely(t, drainVM(c, v), should.BeNil)
							assert.Loosely(t, v.Drained, should.BeFalse)
							assert.Loosely(t, datastore.Get(c, v), should.BeNil)
							assert.Loosely(t, v.Drained, should.BeFalse)
						})

						t.Run("DUT is not in config", func(t *ftt.Test) {
							datastore.Put(c, &model.Config{
								ID: "config",
								Config: &config.Config{
									Duts: map[string]*emptypb.Empty{
										"dut1": {},
										"dut2": {},
										"dut3": {},
									},
								},
							})
							v := &model.VM{
								ID:     "id",
								Config: "config",
								DUT:    "dut4",
							}
							assert.Loosely(t, datastore.Put(c, v), should.BeNil)
							assert.Loosely(t, drainVM(c, v), should.BeNil)
							assert.Loosely(t, v.Drained, should.BeTrue)
							assert.Loosely(t, datastore.Get(c, v), should.BeNil)
							assert.Loosely(t, v.Drained, should.BeTrue)
						})
					})
				})

				t.Run("deleted", func(t *ftt.Test) {
					v := &model.VM{
						ID:     "id",
						Config: "config",
					}
					assert.Loosely(t, drainVM(c, v), should.BeNil)
					assert.Loosely(t, v.Drained, should.BeTrue)
					assert.Loosely(t, datastore.Get(c, v), should.Equal(datastore.ErrNoSuchEntity))
				})
			})
		})

		t.Run("expandConfig", func(t *ftt.Test) {
			t.Run("invalid", func(t *ftt.Test) {
				t.Run("nil", func(t *ftt.Test) {
					err := expandConfig(c, nil)
					assert.Loosely(t, err, should.ErrLike("unexpected payload"))
					assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
				})

				t.Run("empty", func(t *ftt.Test) {
					err := expandConfig(c, &tasks.ExpandConfig{})
					assert.Loosely(t, err, should.ErrLike("ID is required"))
					assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
				})

				t.Run("missing", func(t *ftt.Test) {
					err := expandConfig(c, &tasks.ExpandConfig{
						Id: "id",
					})
					assert.Loosely(t, err, should.ErrLike("failed to fetch config"))
					assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
					cfg := &model.Config{
						ID: "id",
					}
					assert.Loosely(t, datastore.Get(c, cfg), should.Equal(datastore.ErrNoSuchEntity))
				})
			})

			t.Run("valid", func(t *ftt.Test) {
				t.Run("none", func(t *ftt.Test) {
					assert.Loosely(t, datastore.Put(c, &model.Config{
						ID: "id",
						Config: &config.Config{
							Attributes: &config.VM{
								Project: "project",
							},
							Prefix: "prefix",
						},
					}), should.BeNil)
					err := expandConfig(c, &tasks.ExpandConfig{
						Id: "id",
					})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
					cfg := &model.Config{
						ID: "id",
					}
					assert.Loosely(t, datastore.Get(c, cfg), should.BeNil)
					assert.Loosely(t, cfg.Config.CurrentAmount, should.BeZero)
				})

				t.Run("DUTs have priority", func(t *ftt.Test) {
					assert.Loosely(t, datastore.Put(c, &model.Config{
						ID: "id",
						Config: &config.Config{
							Attributes: &config.VM{
								Project: "project",
							},
							Amount: &config.Amount{
								Min: 5,
								Max: 6,
							},
							Prefix: "prefix",
							Duts: map[string]*emptypb.Empty{
								"dut1": {},
								"dut2": {},
								"dut3": {},
							},
						},
					}), should.BeNil)
					err := expandConfig(c, &tasks.ExpandConfig{
						Id: "id",
					})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(3))
					cfg := &model.Config{
						ID: "id",
					}
					assert.Loosely(t, datastore.Get(c, cfg), should.BeNil)
					assert.Loosely(t, cfg.Config.Duts, should.Resemble(map[string]*emptypb.Empty{
						"dut1": {},
						"dut2": {},
						"dut3": {},
					}))
				})

				t.Run("schedule", func(t *ftt.Test) {
					assert.Loosely(t, datastore.Put(c, &model.Config{
						ID: "id",
						Config: &config.Config{
							Attributes: &config.VM{
								Project: "project",
							},
							Amount: &config.Amount{
								Min: 2,
								Max: 2,
								Change: []*config.Schedule{
									{
										Min: 5,
										Max: 5,
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
					}), should.BeNil)

					t.Run("default", func(t *ftt.Test) {
						now := time.Time{}
						assert.Loosely(t, now.Weekday(), should.Equal(time.Monday))
						c, _ = testclock.UseTime(c, now)
						err := expandConfig(c, &tasks.ExpandConfig{
							Id: "id",
						})
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(2))
						cfg := &model.Config{
							ID: "id",
						}
						assert.Loosely(t, datastore.Get(c, cfg), should.BeNil)
						assert.Loosely(t, cfg.Config.CurrentAmount, should.Equal(2))
					})

					t.Run("scheduled", func(t *ftt.Test) {
						now := time.Time{}.Add(time.Hour)
						assert.Loosely(t, now.Weekday(), should.Equal(time.Monday))
						assert.Loosely(t, now.Hour(), should.Equal(1))
						c, _ = testclock.UseTime(c, now)
						err := expandConfig(c, &tasks.ExpandConfig{
							Id: "id",
						})
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(5))
						cfg := &model.Config{
							ID: "id",
						}
						assert.Loosely(t, datastore.Get(c, cfg), should.BeNil)
						assert.Loosely(t, cfg.Config.CurrentAmount, should.Equal(5))
					})
				})
			})
		})

		t.Run("createTasksPerAmount", func(t *ftt.Test) {
			t.Run("invalid", func(t *ftt.Test) {
				t.Run("config.Duts is not empty", func(t *ftt.Test) {
					vms := []*model.VM{}
					m := &model.Config{
						ID: "id",
						Config: &config.Config{
							Attributes: &config.VM{
								Project: "project",
							},
							CurrentAmount: 3,
							Duts: map[string]*emptypb.Empty{
								"dut1": {},
								"dut2": {},
							},
							Prefix: "prefix",
						},
					}
					n := &timestamppb.Timestamp{Seconds: time.Now().Unix()}
					tsks, err := createTasksPerAmount(c, vms, m, n)
					assert.Loosely(t, err, should.ErrLike("config.Duts should be empty"))
					assert.Loosely(t, tsks, should.BeEmpty)
				})
			})

			t.Run("valid", func(t *ftt.Test) {
				t.Run("default", func(t *ftt.Test) {
					vms := []*model.VM{}
					m := &model.Config{
						ID: "id",
						Config: &config.Config{
							Attributes: &config.VM{
								Project: "project",
							},
							CurrentAmount: 3,
							Prefix:        "prefix",
						},
					}
					n := &timestamppb.Timestamp{Seconds: time.Now().Unix()}
					tsks, err := createTasksPerAmount(c, vms, m, n)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(tsks), should.Equal(3))
				})

				t.Run("default - skip existing vms", func(t *ftt.Test) {
					vms := []*model.VM{
						{
							ID:     "prefix-1",
							Config: "id",
							Prefix: "prefix",
						},
					}
					m := &model.Config{
						ID: "id",
						Config: &config.Config{
							Attributes: &config.VM{
								Project: "project",
							},
							CurrentAmount: 3,
							Prefix:        "prefix",
						},
					}
					n := &timestamppb.Timestamp{Seconds: time.Now().Unix()}
					tsks, err := createTasksPerAmount(c, vms, m, n)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, tsks, should.HaveLength(2))
				})

				t.Run("default - skip all vms", func(t *ftt.Test) {
					vms := []*model.VM{
						{
							ID:     "prefix-0",
							Config: "id",
							Prefix: "prefix",
						},
						{
							ID:     "prefix-1",
							Config: "id",
							Prefix: "prefix",
						},
						{
							ID:     "prefix-2",
							Config: "id",
							Prefix: "prefix",
						},
					}
					m := &model.Config{
						ID: "id",
						Config: &config.Config{
							Attributes: &config.VM{
								Project: "project",
							},
							CurrentAmount: 3,
							Prefix:        "prefix",
						},
					}
					n := &timestamppb.Timestamp{Seconds: time.Now().Unix()}
					tsks, err := createTasksPerAmount(c, vms, m, n)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, tsks, should.HaveLength(0))
				})
			})
		})

		t.Run("createTasksPerDUT", func(t *ftt.Test) {
			t.Run("invalid", func(t *ftt.Test) {
				t.Run("config.Duts is nil", func(t *ftt.Test) {
					vms := []*model.VM{}
					m := &model.Config{
						ID: "id",
						Config: &config.Config{
							Attributes: &config.VM{
								Project: "project",
							},
							Prefix: "prefix",
						},
					}
					n := &timestamppb.Timestamp{Seconds: time.Now().Unix()}
					tsks, err := createTasksPerDUT(c, vms, m, n)
					assert.Loosely(t, err, should.ErrLike("config.DUTs cannot be empty"))
					assert.Loosely(t, tsks, should.BeEmpty)
				})
				t.Run("config.Duts is empty", func(t *ftt.Test) {
					vms := []*model.VM{}
					m := &model.Config{
						ID: "id",
						Config: &config.Config{
							Attributes: &config.VM{
								Project: "project",
							},
							Duts:   map[string]*emptypb.Empty{},
							Prefix: "prefix",
						},
					}
					n := &timestamppb.Timestamp{Seconds: time.Now().Unix()}
					tsks, err := createTasksPerDUT(c, vms, m, n)
					assert.Loosely(t, err, should.ErrLike("config.DUTs cannot be empty"))
					assert.Loosely(t, tsks, should.BeEmpty)
				})
			})
		})

		t.Run("valid", func(t *ftt.Test) {
			t.Run("default", func(t *ftt.Test) {
				vms := []*model.VM{}
				m := &model.Config{
					ID: "id",
					Config: &config.Config{
						Attributes: &config.VM{
							Project: "project",
						},
						Duts: map[string]*emptypb.Empty{
							"dut1": {},
							"dut2": {},
							"dut3": {},
						},
						Prefix: "prefix",
					},
				}
				n := &timestamppb.Timestamp{Seconds: time.Now().Unix()}
				tsks, err := createTasksPerDUT(c, vms, m, n)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(tsks), should.Equal(3))
			})

			t.Run("dispatched task contain DUT info", func(t *ftt.Test) {
				vms := []*model.VM{}
				m := &model.Config{
					ID: "id",
					Config: &config.Config{
						Attributes: &config.VM{
							Project: "project",
						},
						Duts: map[string]*emptypb.Empty{
							"dut1": {},
						},
						Prefix: "prefix",
					},
				}
				n := &timestamppb.Timestamp{Seconds: time.Now().Unix()}
				tsks, err := createTasksPerDUT(c, vms, m, n)
				assert.Loosely(t, err, should.BeNil)
				vm := tsks[0].Payload.(*tasks.CreateVM)
				assert.Loosely(t, vm.DUT, should.Equal("dut1"))
			})
		})

		t.Run("reportQuota", func(t *ftt.Test) {
			t.Run("invalid", func(t *ftt.Test) {
				t.Run("nil", func(t *ftt.Test) {
					err := reportQuota(c, nil)
					assert.Loosely(t, err, should.ErrLike("unexpected payload"))
					assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
				})

				t.Run("empty", func(t *ftt.Test) {
					err := reportQuota(c, &tasks.ReportQuota{})
					assert.Loosely(t, err, should.ErrLike("ID is required"))
					assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
				})

				t.Run("missing", func(t *ftt.Test) {
					err := reportQuota(c, &tasks.ReportQuota{
						Id: "id",
					})
					assert.Loosely(t, err, should.ErrLike("failed to fetch project"))
					assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
				})
			})

			t.Run("valid", func(t *ftt.Test) {
				rt.Handler = func(req any) (int, any) {
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
					Config: &projects.Config{
						Metric:  []string{"metric"},
						Project: "project",
						Region:  []string{"region"},
					},
				})
				err := reportQuota(c, &tasks.ReportQuota{
					Id: "id",
				})
				assert.Loosely(t, err, should.BeNil)
			})
		})

		t.Run("abbreviate", func(t *ftt.Test) {
			cases := []struct {
				input string
				limit int
				want  string
			}{
				{
					input: "crossk-chromeos8-row10-rack2-host32-labstation1as",
					limit: 50,
					want:  "c-c8-r10-r2-h32-l1a",
				},
				{
					input: "abc123-def456",
					limit: 50,
					want:  "a123-d456",
				},
				{
					input: "loremipsumdolorsitametconsecteturadipiscingelit",
					limit: 15,
					want:  "l",
				},
				{
					input: "cros---chromium-chromeos8--row29-rack2-host43--",
					limit: 50,
					want:  "c-c-c8-r29-r2-h43-",
				},
				{
					input: "chromeos8-row10-rack12-host13",
					limit: 5,
					want:  "c8-r1",
				},
			}
			for _, c := range cases {
				t.Run(c.input, func(t *ftt.Test) {
					assert.Loosely(t, abbreviate(c.input, c.limit), should.Equal(c.want))
				})
			}
		})
	})
}

func TestGetScalingType(t *testing.T) {
	t.Parallel()
	changes := []*config.Schedule{
		{
			Min: 2,
			Max: 2,
			Length: &config.TimePeriod{
				Time: &config.TimePeriod_Duration{
					Duration: "2h",
				},
			},
			Start: &config.TimeOfDay{
				Day:  dayofweek.DayOfWeek_SUNDAY,
				Time: "23:00",
			},
		},
	}
	cases := []struct {
		name     string
		input    *config.Config
		expected string
	}{
		{"nil", nil, dynamicScalingType},
		{"empty", &config.Config{}, dynamicScalingType},
		{"empty amound", &config.Config{Amount: nil}, dynamicScalingType},
		{"empty changes", &config.Config{Amount: &config.Amount{Change: nil}}, dynamicScalingType},
		{"fixed ", &config.Config{Amount: &config.Amount{Min: 1, Max: 1, Change: nil}}, fixedScalingType},
		{"autoscaled ", &config.Config{Amount: &config.Amount{Min: 1, Max: 10, Change: nil}}, dynamicScalingType},
		{"time based 1", &config.Config{Amount: &config.Amount{Min: 1, Max: 10, Change: changes}}, dynamicScalingType},
		{"time based 2", &config.Config{Amount: &config.Amount{Change: changes}}, dynamicScalingType},
	}
	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			actual := getScalingType(tt.input)
			if diff := cmp.Diff(tt.expected, actual); diff != "" {
				t.Errorf("TestGetScalingType  %q: unexpected diff (-want +got) %s", tt.name, diff)
			}
		})
	}
}
