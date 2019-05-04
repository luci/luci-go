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

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/appengine/tq/tqtesting"
	"go.chromium.org/luci/common/api/swarming/swarming/v1"

	"go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/api/tasks/v1"
	"go.chromium.org/luci/gce/appengine/model"
	rpc "go.chromium.org/luci/gce/appengine/rpc/memory"
	"go.chromium.org/luci/gce/appengine/testing/roundtripper"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestDeleteBot(t *testing.T) {
	t.Parallel()

	Convey("deleteBot", t, func() {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)
		srv := &rpc.Config{}
		rt := &roundtripper.JSONRoundTripper{}
		swr, err := swarming.New(&http.Client{Transport: rt})
		So(err, ShouldBeNil)
		c := withSwarming(withConfig(withDispatcher(memory.Use(context.Background()), dsp), srv), swr)
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
				rt.Handler = func(req interface{}) (int, interface{}) {
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
				rt.Handler = func(req interface{}) (int, interface{}) {
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
				rt.Handler = func(req interface{}) (int, interface{}) {
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
		srv := &rpc.Config{}
		rt := &roundtripper.JSONRoundTripper{}
		swr, err := swarming.New(&http.Client{Transport: rt})
		So(err, ShouldBeNil)
		c := withSwarming(withConfig(withDispatcher(memory.Use(context.Background()), dsp), srv), swr)
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
				Config: config.Config{
					Amount: &config.Amount{
						Default: 1,
					},
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
				rt.Handler = func(_ interface{}) (int, interface{}) {
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
					rt.Handler = func(_ interface{}) (int, interface{}) {
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
				})

				Convey("drained", func() {
					rt.Handler = func(_ interface{}) (int, interface{}) {
						return http.StatusNotFound, nil
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
					So(tqt.GetScheduledTasks()[0].Payload, ShouldHaveSameTypeAs, &tasks.DestroyInstance{})
				})

				Convey("timeout", func() {
					rt.Handler = func(_ interface{}) (int, interface{}) {
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
					rt.Handler = func(_ interface{}) (int, interface{}) {
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
					rt.Handler = func(_ interface{}) (int, interface{}) {
						return http.StatusOK, &swarming.SwarmingRpcsBotInfo{
							BotId:       "id",
							Deleted:     true,
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
					So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
					So(tqt.GetScheduledTasks()[0].Payload, ShouldHaveSameTypeAs, &tasks.DestroyInstance{})
					v := &model.VM{
						ID: "id",
					}
					So(datastore.Get(c, v), ShouldBeNil)
					So(v.Connected, ShouldNotEqual, 0)
				})

				Convey("dead", func() {
					rt.Handler = func(_ interface{}) (int, interface{}) {
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

				Convey("terminated", func() {
					rt.Handler = func(req interface{}) (int, interface{}) {
						So(req, ShouldHaveSameTypeAs, &map[string]string{})
						So(*(req.(*map[string]string)), ShouldBeEmpty)
						return http.StatusOK, map[string]interface{}{
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
					rt.Handler = func(_ interface{}) (int, interface{}) {
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
					rt.Handler = func(_ interface{}) (int, interface{}) {
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
					rt.Handler = func(_ interface{}) (int, interface{}) {
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
		srv := &rpc.Config{}
		rt := &roundtripper.JSONRoundTripper{}
		swr, err := swarming.New(&http.Client{Transport: rt})
		So(err, ShouldBeNil)
		c := withSwarming(withConfig(withDispatcher(memory.Use(context.Background()), dsp), srv), swr)
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

			Convey("error", func() {
				rt.Handler = func(req interface{}) (int, interface{}) {
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
				datastore.Get(c, v)
				So(v.Hostname, ShouldEqual, "name")
			})

			Convey("terminates", func() {
				rt.Handler = func(req interface{}) (int, interface{}) {
					return http.StatusOK, &swarming.SwarmingRpcsTerminateResponse{}
				}
				datastore.Put(c, &model.VM{
					ID:       "id",
					Hostname: "name",
				})
				err := terminateBot(c, &tasks.TerminateBot{
					Id:       "id",
					Hostname: "name",
				})
				So(err, ShouldBeNil)
			})
		})
	})
}
