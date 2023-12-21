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

package backend

import (
	"context"
	"net/http"
	"testing"
	"time"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/appengine/tq/tqtesting"
	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"google.golang.org/api/option"

	"go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/api/tasks/v1"
	"go.chromium.org/luci/gce/appengine/model"
	"go.chromium.org/luci/gce/appengine/testing/roundtripper"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestDeleteBot(t *testing.T) {
	t.Parallel()

	Convey("deleteBot", t, func() {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)
		rt := &roundtripper.JSONRoundTripper{}
		swr, err := swarming.New(&http.Client{Transport: rt})
		So(err, ShouldBeNil)
		c := withSwarming(withDispatcher(memory.Use(context.Background()), dsp), swr)
		tqt := tqtesting.GetTestable(c, dsp)
		tqt.CreateQueues()

		Convey("invalid", func() {
			Convey("nil", func() {
				err := deleteBot(c, nil)
				So(err, ShouldErrLike, "unexpected payload")
			})

			Convey("empty", func() {
				err := deleteBot(c, &tasks.DeleteBot{})
				So(err, ShouldErrLike, "ID is required")
			})

			Convey("hostname", func() {
				err := deleteBot(c, &tasks.DeleteBot{
					Id: "id",
				})
				So(err, ShouldErrLike, "hostname is required")
			})
		})

		Convey("valid", func() {
			Convey("missing", func() {
				err := deleteBot(c, &tasks.DeleteBot{
					Id:       "id",
					Hostname: "name",
				})
				So(err, ShouldBeNil)
			})

			Convey("error", func() {
				rt.Handler = func(_ any) (int, any) {
					return http.StatusInternalServerError, nil
				}
				datastore.Put(c, &model.VM{
					ID:       "id",
					Created:  1,
					Hostname: "name",
					Lifetime: 1,
					URL:      "url",
				})
				err := deleteBot(c, &tasks.DeleteBot{
					Id:       "id",
					Hostname: "name",
				})
				So(err, ShouldErrLike, "failed to delete bot")
				v := &model.VM{
					ID: "id",
				}
				datastore.Get(c, v)
				So(v.Created, ShouldEqual, 1)
				So(v.Hostname, ShouldEqual, "name")
				So(v.URL, ShouldEqual, "url")
			})

			Convey("deleted", func() {
				rt.Handler = func(_ any) (int, any) {
					return http.StatusNotFound, nil
				}
				datastore.Put(c, &model.VM{
					ID:       "id",
					Created:  1,
					Hostname: "name",
					Lifetime: 1,
					URL:      "url",
				})
				err := deleteBot(c, &tasks.DeleteBot{
					Id:       "id",
					Hostname: "name",
				})
				So(err, ShouldBeNil)
				v := &model.VM{
					ID: "id",
				}
				So(datastore.Get(c, v), ShouldEqual, datastore.ErrNoSuchEntity)
			})

			Convey("deletes", func() {
				rt.Handler = func(_ any) (int, any) {
					return http.StatusOK, &swarming.SwarmingRpcsDeletedResponse{}
				}
				datastore.Put(c, &model.VM{
					ID:       "id",
					Created:  1,
					Hostname: "name",
					Lifetime: 1,
					URL:      "url",
				})
				err := deleteBot(c, &tasks.DeleteBot{
					Id:       "id",
					Hostname: "name",
				})
				So(err, ShouldBeNil)
				v := &model.VM{
					ID: "id",
				}
				So(datastore.Get(c, v), ShouldEqual, datastore.ErrNoSuchEntity)
			})
		})
	})
}

func TestDeleteVM(t *testing.T) {
	t.Parallel()

	Convey("deleteVM", t, func() {
		c := memory.Use(context.Background())

		Convey("deletes", func() {
			datastore.Put(c, &model.VM{
				ID:       "id",
				Hostname: "name",
			})
			So(deleteVM(c, "id", "name"), ShouldBeNil)
			v := &model.VM{
				ID: "id",
			}
			So(datastore.Get(c, v), ShouldEqual, datastore.ErrNoSuchEntity)
		})

		Convey("deleted", func() {
			So(deleteVM(c, "id", "name"), ShouldBeNil)
		})

		Convey("replaced", func() {
			datastore.Put(c, &model.VM{
				ID:       "id",
				Hostname: "name-2",
			})
			So(deleteVM(c, "id", "name-1"), ShouldBeNil)
			v := &model.VM{
				ID: "id",
			}
			So(datastore.Get(c, v), ShouldBeNil)
			So(v.Hostname, ShouldEqual, "name-2")
		})
	})
}

func TestManageBot(t *testing.T) {
	t.Parallel()

	Convey("manageBot", t, func() {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)
		rt := &roundtripper.JSONRoundTripper{}
		swr, err := swarming.New(&http.Client{Transport: rt})
		So(err, ShouldBeNil)
		c := withSwarming(withDispatcher(memory.Use(context.Background()), dsp), swr)
		tqt := tqtesting.GetTestable(c, dsp)
		tqt.CreateQueues()

		Convey("invalid", func() {
			Convey("nil", func() {
				err := manageBot(c, nil)
				So(err, ShouldErrLike, "unexpected payload")
			})

			Convey("empty", func() {
				err := manageBot(c, &tasks.ManageBot{})
				So(err, ShouldErrLike, "ID is required")
			})
		})

		Convey("valid", func() {
			datastore.Put(c, &model.Config{
				ID: "config",
				Config: &config.Config{
					CurrentAmount: 1,
				},
			})

			Convey("deleted", func() {
				err := manageBot(c, &tasks.ManageBot{
					Id: "id",
				})
				So(err, ShouldBeNil)
			})

			Convey("creating", func() {
				datastore.Put(c, &model.VM{
					ID:     "id",
					Config: "config",
				})
				err := manageBot(c, &tasks.ManageBot{
					Id: "id",
				})
				So(err, ShouldBeNil)
			})

			Convey("error", func() {
				rt.Handler = func(_ any) (int, any) {
					return http.StatusConflict, nil
				}
				datastore.Put(c, &model.VM{
					ID:     "id",
					Config: "config",
					URL:    "url",
				})
				err := manageBot(c, &tasks.ManageBot{
					Id: "id",
				})
				So(err, ShouldErrLike, "failed to fetch bot")
			})

			Convey("missing", func() {
				Convey("deadline", func() {
					rt.Handler = func(_ any) (int, any) {
						return http.StatusNotFound, nil
					}
					datastore.Put(c, &model.VM{
						ID:       "id",
						Config:   "config",
						Created:  1,
						Hostname: "name",
						Lifetime: 1,
						URL:      "url",
					})
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					So(err, ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
					So(tqt.GetScheduledTasks()[0].Payload, ShouldHaveSameTypeAs, &tasks.DestroyInstance{})
					v := &model.VM{
						ID: "id",
					}
					So(datastore.Get(c, v), ShouldBeNil)
				})

				Convey("drained & new bots", func() {
					rt.Handler = func(_ any) (int, any) {
						return http.StatusNotFound, nil
					}
					datastore.Put(c, &model.VM{
						ID:       "id",
						Config:   "config",
						Drained:  true,
						Hostname: "name",
						URL:      "url",
						Created:  time.Now().Unix() - 100,
					})
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					So(err, ShouldBeNil)
					// For now, won't destroy a instance if it's set to drained or newly created but
					// hasn't connected to swarming yet
					So(tqt.GetScheduledTasks(), ShouldHaveLength, 0)
				})

				Convey("timeout", func() {
					rt.Handler = func(_ any) (int, any) {
						return http.StatusNotFound, nil
					}
					datastore.Put(c, &model.VM{
						ID:       "id",
						Config:   "config",
						Created:  1,
						Hostname: "name",
						Timeout:  1,
						URL:      "url",
					})
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					So(err, ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
					So(tqt.GetScheduledTasks()[0].Payload, ShouldHaveSameTypeAs, &tasks.DestroyInstance{})
				})

				Convey("wait", func() {
					rt.Handler = func(_ any) (int, any) {
						return http.StatusNotFound, nil
					}
					datastore.Put(c, &model.VM{
						ID:       "id",
						Config:   "config",
						Hostname: "name",
						URL:      "url",
					})
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					So(err, ShouldBeNil)
				})
			})

			Convey("found", func() {
				Convey("deleted", func() {
					rt.Handler = func(_ any) (int, any) {
						return http.StatusOK, &swarming.SwarmingRpcsBotInfo{
							BotId:   "id",
							Deleted: true,
						}
					}
					datastore.Put(c, &model.VM{
						ID:       "id",
						Config:   "config",
						Hostname: "name",
						URL:      "url",
						// Has to be older than time.Now().Unix() - minPendingMinutesForBotConnected * 10
						Created: time.Now().Unix() - 10000,
					})
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					So(err, ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
					So(tqt.GetScheduledTasks()[0].Payload, ShouldHaveSameTypeAs, &tasks.DestroyInstance{})
					v := &model.VM{
						ID: "id",
					}
					So(datastore.Get(c, v), ShouldBeNil)
				})

				Convey("deleted but newly created", func() {
					rt.Handler = func(_ any) (int, any) {
						return http.StatusOK, &swarming.SwarmingRpcsBotInfo{
							BotId:   "id",
							Deleted: true,
						}
					}
					datastore.Put(c, &model.VM{
						ID:       "id",
						Config:   "config",
						Hostname: "name",
						URL:      "url",
						Created:  time.Now().Unix() - 10,
					})
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					So(err, ShouldBeNil)
					// Won't destroy the instance if it's a newly created VM
					So(tqt.GetScheduledTasks(), ShouldHaveLength, 0)
				})

				Convey("dead", func() {
					rt.Handler = func(_ any) (int, any) {
						return http.StatusOK, &swarming.SwarmingRpcsBotInfo{
							BotId:       "id",
							FirstSeenTs: "2019-03-13T00:12:29.882948",
							IsDead:      true,
						}
					}
					datastore.Put(c, &model.VM{
						ID:       "id",
						Config:   "config",
						Hostname: "name",
						URL:      "url",
						// Has to be older than time.Now().Unix() - minPendingMinutesForBotConnected * 10
						Created: time.Now().Unix() - 10000,
					})
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					So(err, ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
					So(tqt.GetScheduledTasks()[0].Payload, ShouldHaveSameTypeAs, &tasks.DestroyInstance{})
					v := &model.VM{
						ID: "id",
					}
					So(datastore.Get(c, v), ShouldBeNil)
				})

				Convey("dead but newly created", func() {
					rt.Handler = func(_ any) (int, any) {
						return http.StatusOK, &swarming.SwarmingRpcsBotInfo{
							BotId:       "id",
							FirstSeenTs: "2019-03-13T00:12:29.882948",
							IsDead:      true,
						}
					}
					datastore.Put(c, &model.VM{
						ID:       "id",
						Config:   "config",
						Hostname: "name",
						URL:      "url",
						Created:  time.Now().Unix() - 10,
					})
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					So(err, ShouldBeNil)
					// won't destroy the instance if it's a newly created VM
					So(tqt.GetScheduledTasks(), ShouldHaveLength, 0)
				})

				Convey("terminated", func() {
					rt.Handler = func(_ any) (int, any) {
						return http.StatusOK, map[string]any{
							"bot_id":        "id",
							"first_seen_ts": "2019-03-13T00:12:29.882948",
							"items": []*swarming.SwarmingRpcsBotEvent{
								{
									EventType: "bot_terminate",
								},
							},
						}
					}
					datastore.Put(c, &model.VM{
						ID:       "id",
						Config:   "config",
						Hostname: "name",
						URL:      "url",
					})
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					So(err, ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
					So(tqt.GetScheduledTasks()[0].Payload, ShouldHaveSameTypeAs, &tasks.DestroyInstance{})
					v := &model.VM{
						ID: "id",
					}
					So(datastore.Get(c, v), ShouldBeNil)
					So(v.Connected, ShouldNotEqual, 0)
				})

				Convey("deadline", func() {
					rt.Handler = func(_ any) (int, any) {
						return http.StatusOK, &swarming.SwarmingRpcsBotInfo{
							BotId:       "id",
							FirstSeenTs: "2019-03-13T00:12:29.882948",
						}
					}
					datastore.Put(c, &model.VM{
						ID:       "id",
						Config:   "config",
						Created:  1,
						Lifetime: 1,
						Hostname: "name",
						URL:      "url",
					})
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					So(err, ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
					So(tqt.GetScheduledTasks()[0].Payload, ShouldHaveSameTypeAs, &tasks.TerminateBot{})
					v := &model.VM{
						ID: "id",
					}
					So(datastore.Get(c, v), ShouldBeNil)
					So(v.Connected, ShouldNotEqual, 0)
				})

				Convey("drained", func() {
					rt.Handler = func(_ any) (int, any) {
						return http.StatusOK, &swarming.SwarmingRpcsBotInfo{
							BotId:       "id",
							FirstSeenTs: "2019-03-13T00:12:29.882948",
						}
					}
					datastore.Put(c, &model.VM{
						ID:       "id",
						Config:   "config",
						Drained:  true,
						Hostname: "name",
						URL:      "url",
					})
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					So(err, ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
					So(tqt.GetScheduledTasks()[0].Payload, ShouldHaveSameTypeAs, &tasks.TerminateBot{})
					v := &model.VM{
						ID: "id",
					}
					So(datastore.Get(c, v), ShouldBeNil)
					So(v.Connected, ShouldNotEqual, 0)
				})

				Convey("alive", func() {
					rt.Handler = func(_ any) (int, any) {
						return http.StatusOK, &swarming.SwarmingRpcsBotInfo{
							BotId:       "id",
							FirstSeenTs: "2019-03-13T00:12:29.882948",
						}
					}
					datastore.Put(c, &model.VM{
						ID:       "id",
						Config:   "config",
						Hostname: "name",
						URL:      "url",
					})
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					So(err, ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
					v := &model.VM{
						ID: "id",
					}
					So(datastore.Get(c, v), ShouldBeNil)
					So(v.Connected, ShouldNotEqual, 0)
				})
			})
		})
	})
}

func TestTerminateBot(t *testing.T) {
	t.Parallel()

	Convey("terminateBot", t, func() {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)
		rt := &roundtripper.JSONRoundTripper{}
		swr, err := swarming.New(&http.Client{Transport: rt})
		So(err, ShouldBeNil)
		c := withSwarming(withDispatcher(memory.Use(context.Background()), dsp), swr)
		tqt := tqtesting.GetTestable(c, dsp)
		tqt.CreateQueues()

		Convey("invalid", func() {
			Convey("nil", func() {
				err := terminateBot(c, nil)
				So(err, ShouldErrLike, "unexpected payload")
			})

			Convey("empty", func() {
				err := terminateBot(c, &tasks.TerminateBot{})
				So(err, ShouldErrLike, "ID is required")
			})

			Convey("hostname", func() {
				err := terminateBot(c, &tasks.TerminateBot{
					Id: "id",
				})
				So(err, ShouldErrLike, "hostname is required")
			})
		})

		Convey("valid", func() {
			Convey("missing", func() {
				err := terminateBot(c, &tasks.TerminateBot{
					Id:       "id",
					Hostname: "name",
				})
				So(err, ShouldBeNil)
			})

			Convey("replaced", func() {
				datastore.Put(c, &model.VM{
					ID:       "id",
					Hostname: "new",
				})
				err := terminateBot(c, &tasks.TerminateBot{
					Id:       "id",
					Hostname: "old",
				})
				So(err, ShouldBeNil)
				v := &model.VM{
					ID: "id",
				}
				So(datastore.Get(c, v), ShouldBeNil)
				So(v.Hostname, ShouldEqual, "new")
			})

			Convey("error", func() {
				rt.Handler = func(_ any) (int, any) {
					return http.StatusInternalServerError, nil
				}
				datastore.Put(c, &model.VM{
					ID:       "id",
					Hostname: "name",
				})
				err := terminateBot(c, &tasks.TerminateBot{
					Id:       "id",
					Hostname: "name",
				})
				So(err, ShouldErrLike, "failed to terminate bot")
				v := &model.VM{
					ID: "id",
				}
				So(datastore.Get(c, v), ShouldBeNil)
				So(v.Hostname, ShouldEqual, "name")
			})

			Convey("terminates", func() {
				c, _ = testclock.UseTime(c, testclock.TestRecentTimeUTC)
				rpcsToSwarming := 0
				rt.Handler = func(_ any) (int, any) {
					rpcsToSwarming++
					return http.StatusOK, &swarming.SwarmingRpcsTerminateResponse{}
				}
				datastore.Put(c, &model.VM{
					ID:       "id",
					Hostname: "name",
				})
				terminateTask := tasks.TerminateBot{
					Id:       "id",
					Hostname: "name",
				}
				So(terminateBot(c, &terminateTask), ShouldBeNil)
				So(rpcsToSwarming, ShouldEqual, 1)

				Convey("wait 1 hour before sending another terminate task", func() {
					c, _ = testclock.UseTime(c, testclock.TestRecentTimeUTC.Add(time.Hour-time.Second))
					So(terminateBot(c, &terminateTask), ShouldBeNil)
					So(rpcsToSwarming, ShouldEqual, 1)

					c, _ = testclock.UseTime(c, testclock.TestRecentTimeUTC.Add(time.Hour))
					So(terminateBot(c, &terminateTask), ShouldBeNil)
					So(err, ShouldBeNil)
					So(rpcsToSwarming, ShouldEqual, 2)
				})
			})
		})
	})
}

func TestInspectSwarming(t *testing.T) {
	t.Parallel()

	Convey("inspectSwarmingAsync", t, func() {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)
		c := withDispatcher(memory.Use(context.Background()), dsp)
		tqt := tqtesting.GetTestable(c, dsp)
		tqt.CreateQueues()
		datastore.GetTestable(c).Consistent(true)

		Convey("none", func() {
			err := inspectSwarmingAsync(c)
			So(err, ShouldBeNil)
			So(tqt.GetScheduledTasks(), ShouldHaveLength, 0)
		})

		Convey("one", func() {
			So(datastore.Put(c, &model.Config{
				ID: "config-1",
				Config: &config.Config{
					Swarming: "https://gce-swarming.appspot.com",
				},
			}), ShouldBeNil)
			err := inspectSwarmingAsync(c)
			So(err, ShouldBeNil)
			So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
		})

		Convey("two", func() {
			So(datastore.Put(c, &model.Config{
				ID: "config-1",
				Config: &config.Config{
					Swarming: "https://gce-swarming.appspot.com",
				},
			}), ShouldBeNil)
			So(datastore.Put(c, &model.Config{
				ID: "config-2",
				Config: &config.Config{
					Swarming: "https://vmleaser-swarming.appspot.com",
				},
			}), ShouldBeNil)
			err := inspectSwarmingAsync(c)
			So(err, ShouldBeNil)
			So(tqt.GetScheduledTasks(), ShouldHaveLength, 2)
		})
	})

	Convey("inspectSwarming", t, func() {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)
		rt := &roundtripper.JSONRoundTripper{}
		c := withDispatcher(memory.Use(context.Background()), dsp)
		swr, err := swarming.NewService(c, option.WithHTTPClient(&http.Client{Transport: rt}))
		So(err, ShouldBeNil)
		c = withSwarming(c, swr)
		tqt := tqtesting.GetTestable(c, dsp)
		tqt.CreateQueues()
		datastore.GetTestable(c).Consistent(true)
		Convey("BadInputs", func() {
			Convey("nil", func() {
				err := inspectSwarming(c, nil)
				So(err, ShouldNotBeNil)
			})
			Convey("empty", func() {
				err := inspectSwarming(c, &tasks.InspectSwarming{})
				So(err, ShouldNotBeNil)
			})
		})

		Convey("Swarming error", func() {
			So(datastore.Put(c, &model.VM{
				ID:       "vm-1",
				Hostname: "vm-1-abcd",
				Swarming: "https://gce-swarming.appspot.com",
			}), ShouldBeNil)
			rt.Handler = func(_ any) (int, any) {
				return http.StatusInternalServerError, nil
			}
			err := inspectSwarming(c, &tasks.InspectSwarming{
				Swarming: "https://gce-swarming.appspot.com",
			})
			So(err, ShouldNotBeNil)
		})
		Convey("HappyPath-1", func() {
			So(datastore.Put(c, &model.VM{
				ID:       "vm-1",
				Hostname: "vm-1-abcd",
				Swarming: "https://gce-swarming.appspot.com",
			}), ShouldBeNil)
			So(datastore.Put(c, &model.VM{
				ID:       "vm-2",
				Hostname: "vm-2-abcd",
				Swarming: "https://gce-swarming.appspot.com",
			}), ShouldBeNil)
			So(datastore.Put(c, &model.VM{
				ID:       "vm-3",
				Hostname: "vm-3-abcd",
				Swarming: "https://vmleaser-swarming.appspot.com",
			}), ShouldBeNil)
			rt.Handler = func(_ any) (int, any) {
				return http.StatusOK, &swarming.SwarmingRpcsBotList{
					Cursor: "",
					Items: []*swarming.SwarmingRpcsBotInfo{
						&swarming.SwarmingRpcsBotInfo{
							BotId:       "vm-1-abcd",
							FirstSeenTs: "2023-01-02T15:04:05",
						},
						&swarming.SwarmingRpcsBotInfo{
							BotId:       "vm-2-abcd",
							FirstSeenTs: "2023-01-02T15:04:05",
						},
					},
				}
			}
			err := inspectSwarming(c, &tasks.InspectSwarming{
				Swarming: "https://gce-swarming.appspot.com",
			})
			So(err, ShouldBeNil)
			So(tqt.GetScheduledTasks(), ShouldHaveLength, 2)
		})
		Convey("HappyPath-2", func() {
			So(datastore.Put(c, &model.VM{
				ID:       "vm-1",
				Hostname: "vm-1-abcd",
				Swarming: "https://gce-swarming.appspot.com",
			}), ShouldBeNil)
			So(datastore.Put(c, &model.VM{
				ID:       "vm-2",
				Hostname: "vm-2-abcd",
				Swarming: "https://gce-swarming.appspot.com",
			}), ShouldBeNil)
			So(datastore.Put(c, &model.VM{
				ID:       "vm-3",
				Hostname: "vm-3-abcd",
				Swarming: "https://vmleaser-swarming.appspot.com",
			}), ShouldBeNil)
			rt.Handler = func(_ any) (int, any) {
				return http.StatusOK, &swarming.SwarmingRpcsBotList{
					Cursor: "",
					Items: []*swarming.SwarmingRpcsBotInfo{
						&swarming.SwarmingRpcsBotInfo{
							BotId:       "vm-3-abcd",
							FirstSeenTs: "2023-01-02T15:04:05",
						},
					},
				}
			}
			err := inspectSwarming(c, &tasks.InspectSwarming{
				Swarming: "https://vmleaser-swarming.appspot.com",
			})
			So(err, ShouldBeNil)
			So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
		})
		Convey("HappyPath-3-pagination", func() {
			So(datastore.Put(c, &model.VM{
				ID:       "vm-1",
				Hostname: "vm-1-abcd",
				Swarming: "https://gce-swarming.appspot.com",
			}), ShouldBeNil)
			So(datastore.Put(c, &model.VM{
				ID:       "vm-2",
				Hostname: "vm-2-abcd",
				Swarming: "https://gce-swarming.appspot.com",
			}), ShouldBeNil)
			So(datastore.Put(c, &model.VM{
				ID:       "vm-3",
				Hostname: "vm-3-abcd",
				Swarming: "https://gce-swarming.appspot.com",
			}), ShouldBeNil)
			call := 0
			rt.Handler = func(_ any) (int, any) {
				// Returns 2 bots and a cursor for first call
				if call == 0 {
					call += 1
					return http.StatusOK, &swarming.SwarmingRpcsBotList{
						Cursor: "cursor",
						Items: []*swarming.SwarmingRpcsBotInfo{
							&swarming.SwarmingRpcsBotInfo{
								BotId:       "vm-1-abcd",
								FirstSeenTs: "2023-01-02T15:04:05",
							},
							&swarming.SwarmingRpcsBotInfo{
								BotId:       "vm-2-abcd",
								FirstSeenTs: "2023-01-02T15:04:05",
							},
						},
					}
				} else {
					// Returns last bot and no cursor for subsequent calls
					return http.StatusOK, &swarming.SwarmingRpcsBotList{
						Cursor: "",
						Items: []*swarming.SwarmingRpcsBotInfo{
							&swarming.SwarmingRpcsBotInfo{
								BotId:       "vm-3-abcd",
								FirstSeenTs: "2023-01-02T15:04:05",
							},
						},
					}
				}
			}
			err := inspectSwarming(c, &tasks.InspectSwarming{
				Swarming: "https://gce-swarming.appspot.com",
			})
			So(err, ShouldBeNil)
			So(tqt.GetScheduledTasks(), ShouldHaveLength, 3)
		})

	})
}

func TestDeleteStaleSwarmingBot(t *testing.T) {
	t.Parallel()

	Convey("deleteStaleSwarmingBot", t, func() {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)
		rt := &roundtripper.JSONRoundTripper{}
		c := withDispatcher(memory.Use(context.Background()), dsp)
		swr, err := swarming.NewService(c, option.WithHTTPClient(&http.Client{Transport: rt}))
		So(err, ShouldBeNil)
		c = withSwarming(c, swr)
		tqt := tqtesting.GetTestable(c, dsp)
		tqt.CreateQueues()
		datastore.GetTestable(c).Consistent(true)

		Convey("BadInputs", func() {
			Convey("nil", func() {
				err := deleteStaleSwarmingBot(c, nil)
				So(err, ShouldNotBeNil)
			})
			Convey("empty", func() {
				err := deleteStaleSwarmingBot(c, &tasks.DeleteStaleSwarmingBot{})
				So(err, ShouldNotBeNil)
			})
			Convey("missing timestamp", func() {
				err := deleteStaleSwarmingBot(c, &tasks.DeleteStaleSwarmingBot{
					Id: "id-1",
				})
				So(err, ShouldNotBeNil)
			})
		})
		Convey("VM issues", func() {
			Convey("Missing VM", func() {
				// Don't err if the VM is missing, prob deleted already
				err := deleteStaleSwarmingBot(c, &tasks.DeleteStaleSwarmingBot{
					Id:          "id-1",
					FirstSeenTs: "onceUponATime",
				})
				So(err, ShouldBeNil)
			})
			Convey("Missing URL in VM", func() {
				So(datastore.Put(c, &model.VM{
					ID:       "vm-3",
					Hostname: "vm-3-abcd",
					Swarming: "https://gce-swarming.appspot.com",
				}), ShouldBeNil)
				// Don't err if the URL in VM is missing
				err := deleteStaleSwarmingBot(c, &tasks.DeleteStaleSwarmingBot{
					Id:          "id-1",
					FirstSeenTs: "onceUponATime",
				})
				So(err, ShouldBeNil)
			})
		})
		Convey("Swarming Issues", func() {
			Convey("Failed to fetch", func() {
				So(datastore.Put(c, &model.VM{
					ID:       "vm-3",
					Hostname: "vm-3-abcd",
					URL:      "https://www.googleapis.com/compute/v1/projects/vmleaser/zones/us-numba1-c/instances/vm-3-abcd",
					Swarming: "https://gce-swarming.appspot.com",
				}), ShouldBeNil)
				So(err, ShouldBeNil)
				rt.Handler = func(_ any) (int, any) {
					return http.StatusNotFound, nil
				}
				err := deleteStaleSwarmingBot(c, &tasks.DeleteStaleSwarmingBot{
					Id:          "id-1",
					FirstSeenTs: "onceUponATime",
				})
				So(err, ShouldBeNil)
			})
		})
		Convey("Happy paths", func() {
			Convey("Bot terminated", func() {
				So(datastore.Put(c, &model.VM{
					ID:       "vm-3",
					Hostname: "vm-3-abcd",
					URL:      "https://www.googleapis.com/compute/v1/projects/vmleaser/zones/us-numba1-c/instances/vm-3-abcd",
					Swarming: "https://gce-swarming.appspot.com",
				}), ShouldBeNil)
				So(err, ShouldBeNil)
				rt.Handler = func(_ any) (int, any) {
					return http.StatusOK, map[string]any{
						"bot_id":        "vm-3-abcd",
						"first_seen_ts": "2019-03-13T00:12:29.882948",
						"items": []*swarming.SwarmingRpcsBotEvent{
							{
								EventType: "bot_terminate",
							},
						},
					}
				}
				err := deleteStaleSwarmingBot(c, &tasks.DeleteStaleSwarmingBot{
					Id:          "vm-3",
					FirstSeenTs: "2019-03-13T00:12:29.882948",
				})
				So(err, ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
			})
			Convey("Bot retirement", func() {
				So(datastore.Put(c, &model.VM{
					ID:       "vm-3",
					Hostname: "vm-3-abcd",
					URL:      "https://www.googleapis.com/compute/v1/projects/vmleaser/zones/us-numba1-c/instances/vm-3-abcd",
					Swarming: "https://gce-swarming.appspot.com",
					Lifetime: 99,
					Created:  time.Now().Unix() - 100,
				}), ShouldBeNil)
				So(err, ShouldBeNil)
				rt.Handler = func(_ any) (int, any) {
					return http.StatusOK, map[string]any{
						"bot_id":        "vm-3-abcd",
						"first_seen_ts": "2019-03-13T00:12:29.882948",
						"items":         []*swarming.SwarmingRpcsBotEvent{},
					}
				}
				err := deleteStaleSwarmingBot(c, &tasks.DeleteStaleSwarmingBot{
					Id:          "vm-3",
					FirstSeenTs: "2019-03-13T00:12:29.882948",
				})
				So(err, ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
			})
			Convey("Bot drained", func() {
				So(datastore.Put(c, &model.VM{
					ID:       "vm-3",
					Hostname: "vm-3-abcd",
					URL:      "https://www.googleapis.com/compute/v1/projects/vmleaser/zones/us-numba1-c/instances/vm-3-abcd",
					Swarming: "https://gce-swarming.appspot.com",
					Lifetime: 100000000,
					Created:  time.Now().Unix(),
					Drained:  true,
				}), ShouldBeNil)
				So(err, ShouldBeNil)
				rt.Handler = func(_ any) (int, any) {
					return http.StatusOK, map[string]any{
						"bot_id":        "vm-3-abcd",
						"first_seen_ts": "2019-03-13T00:12:29.882948",
						"items":         []*swarming.SwarmingRpcsBotEvent{},
					}
				}
				err := deleteStaleSwarmingBot(c, &tasks.DeleteStaleSwarmingBot{
					Id:          "vm-3",
					FirstSeenTs: "2019-03-13T00:12:29.882948",
				})
				So(err, ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
			})
		})

	})
}
