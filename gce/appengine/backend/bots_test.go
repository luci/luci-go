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

			Convey("missing", func() {
				err := deleteBot(c, &tasks.DeleteBot{
					Id:       "id",
					Hostname: "name",
				})
				So(err, ShouldErrLike, "failed to fetch VM")
			})
		})

		Convey("valid", func() {
			Convey("error", func() {
				rt.Handler = func(req interface{}) (int, interface{}) {
					return http.StatusInternalServerError, nil
				}
				datastore.Put(c, &model.VM{
					ID:       "id",
					Created:  1,
					Hostname: "name",
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
				datastore.Get(c, v)
				So(v.Created, ShouldEqual, 0)
				So(v.Hostname, ShouldBeEmpty)
				So(v.URL, ShouldBeEmpty)
			})

			Convey("deletes", func() {
				rt.Handler = func(req interface{}) (int, interface{}) {
					return http.StatusOK, &swarming.SwarmingRpcsDeletedResponse{}
				}
				datastore.Put(c, &model.VM{
					ID:       "id",
					Created:  1,
					Hostname: "name",
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
				datastore.Get(c, v)
				So(v.Created, ShouldEqual, 0)
				So(v.Hostname, ShouldBeEmpty)
				So(v.URL, ShouldBeEmpty)
			})
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

			Convey("missing", func() {
				err := manageBot(c, &tasks.ManageBot{
					Id: "id",
				})
				So(err, ShouldErrLike, "failed to fetch VM")
			})
		})

		Convey("valid", func() {
			Convey("error", func() {
				rt.Handler = func(_ interface{}) (int, interface{}) {
					return http.StatusConflict, nil
				}
				datastore.Put(c, &model.VM{
					ID:  "id",
					URL: "url",
				})
				err := manageBot(c, &tasks.ManageBot{
					Id: "id",
				})
				So(err, ShouldErrLike, "failed to fetch bot")
			})

			Convey("missing", func() {
				rt.Handler = func(_ interface{}) (int, interface{}) {
					return http.StatusNotFound, nil
				}
				datastore.Put(c, &model.VM{
					ID:  "id",
					URL: "url",
				})
				err := manageBot(c, &tasks.ManageBot{
					Id: "id",
				})
				So(err, ShouldBeNil)
			})

			Convey("found", func() {
				Convey("dead", func() {
					rt.Handler = func(_ interface{}) (int, interface{}) {
						return http.StatusOK, &swarming.SwarmingRpcsBotInfo{
							BotId:  "id",
							IsDead: true,
						}
					}
					datastore.Put(c, &model.VM{
						ID:  "id",
						URL: "url",
					})
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					So(err, ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
					So(tqt.GetScheduledTasks()[0].Payload, ShouldHaveSameTypeAs, &tasks.DestroyInstance{})
				})

				Convey("deleted", func() {
					rt.Handler = func(_ interface{}) (int, interface{}) {
						return http.StatusOK, &swarming.SwarmingRpcsBotInfo{
							BotId:   "id",
							Deleted: true,
						}
					}
					datastore.Put(c, &model.VM{
						ID:  "id",
						URL: "url",
					})
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					So(err, ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
					So(tqt.GetScheduledTasks()[0].Payload, ShouldHaveSameTypeAs, &tasks.DestroyInstance{})
				})

				Convey("deadline", func() {
					rt.Handler = func(_ interface{}) (int, interface{}) {
						return http.StatusOK, &swarming.SwarmingRpcsBotInfo{
							BotId: "id",
						}
					}
					datastore.Put(c, &model.VM{
						ID:       "id",
						Created:  1,
						Lifetime: 1,
						URL:      "url",
					})
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					So(err, ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
					So(tqt.GetScheduledTasks()[0].Payload, ShouldHaveSameTypeAs, &tasks.TerminateBot{})
				})

				Convey("drained", func() {
					rt.Handler = func(_ interface{}) (int, interface{}) {
						return http.StatusOK, &swarming.SwarmingRpcsBotInfo{
							BotId: "id",
						}
					}
					datastore.Put(c, &model.VM{
						ID:       "id",
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
				})

				Convey("alive", func() {
					rt.Handler = func(_ interface{}) (int, interface{}) {
						return http.StatusOK, &swarming.SwarmingRpcsBotInfo{
							BotId: "id",
						}
					}
					datastore.Put(c, &model.VM{
						ID:  "id",
						URL: "url",
					})
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					So(err, ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
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

			Convey("missing", func() {
				err := terminateBot(c, &tasks.TerminateBot{
					Id:       "id",
					Hostname: "name",
				})
				So(err, ShouldErrLike, "failed to fetch VM")
			})
		})

		Convey("valid", func() {
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
