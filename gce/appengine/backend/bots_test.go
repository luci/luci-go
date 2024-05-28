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
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/appengine/tq/tqtesting"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"

	"go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/api/tasks/v1"
	"go.chromium.org/luci/gce/appengine/model"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var someTimeAgo = timestamppb.New(time.Date(2022, 1, 1, 1, 1, 1, 0, time.UTC))

func TestDeleteBot(t *testing.T) {
	t.Parallel()

	Convey("deleteBot", t, func() {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)

		swr := &mockSwarmingBotsClient{}

		c := withDispatcher(memory.Use(context.Background()), dsp)
		c = withSwarming(c, func(context.Context, string) swarmingpb.BotsClient { return swr })

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
				swr.err = status.Errorf(codes.Internal, "boom")
				So(datastore.Put(c, &model.VM{
					ID:       "id",
					Created:  1,
					Hostname: "name",
					Lifetime: 1,
					URL:      "url",
				}), ShouldBeNil)
				err := deleteBot(c, &tasks.DeleteBot{
					Id:       "id",
					Hostname: "name",
				})
				So(err, ShouldErrLike, "failed to delete bot")
				v := &model.VM{
					ID: "id",
				}
				So(datastore.Get(c, v), ShouldBeNil)
				So(v.Created, ShouldEqual, 1)
				So(v.Hostname, ShouldEqual, "name")
				So(v.URL, ShouldEqual, "url")
			})

			Convey("deleted", func() {
				swr.err = status.Errorf(codes.NotFound, "not found")
				So(datastore.Put(c, &model.VM{
					ID:       "id",
					Created:  1,
					Hostname: "name",
					Lifetime: 1,
					URL:      "url",
				}), ShouldBeNil)
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
				swr.deleteBotResponse = &swarmingpb.DeleteResponse{}
				So(datastore.Put(c, &model.VM{
					ID:       "id",
					Created:  1,
					Hostname: "name",
					Lifetime: 1,
					URL:      "url",
				}), ShouldBeNil)
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
			So(datastore.Put(c, &model.VM{
				ID:       "id",
				Hostname: "name",
			}), ShouldBeNil)
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
			So(datastore.Put(c, &model.VM{
				ID:       "id",
				Hostname: "name-2",
			}), ShouldBeNil)
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

		swr := &mockSwarmingBotsClient{}

		c := withDispatcher(memory.Use(context.Background()), dsp)
		c = withSwarming(c, func(context.Context, string) swarmingpb.BotsClient { return swr })

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
			So(datastore.Put(c, &model.Config{
				ID: "config",
				Config: &config.Config{
					CurrentAmount: 1,
				},
			}), ShouldBeNil)

			Convey("deleted", func() {
				err := manageBot(c, &tasks.ManageBot{
					Id: "id",
				})
				So(err, ShouldBeNil)
			})

			Convey("creating", func() {
				So(datastore.Put(c, &model.VM{
					ID:     "id",
					Config: "config",
				}), ShouldBeNil)
				err := manageBot(c, &tasks.ManageBot{
					Id: "id",
				})
				So(err, ShouldBeNil)
			})

			Convey("error", func() {
				swr.err = status.Errorf(codes.InvalidArgument, "unexpected error")
				So(datastore.Put(c, &model.VM{
					ID:     "id",
					Config: "config",
					URL:    "url",
				}), ShouldBeNil)
				err := manageBot(c, &tasks.ManageBot{
					Id: "id",
				})
				So(err, ShouldErrLike, "failed to fetch bot")
			})

			Convey("missing", func() {
				Convey("deadline", func() {
					swr.err = status.Errorf(codes.NotFound, "not found")
					So(datastore.Put(c, &model.VM{
						ID:       "id",
						Config:   "config",
						Created:  1,
						Hostname: "name",
						Lifetime: 1,
						URL:      "url",
					}), ShouldBeNil)
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
					swr.err = status.Errorf(codes.NotFound, "not found")
					So(datastore.Put(c, &model.VM{
						ID:       "id",
						Config:   "config",
						Drained:  true,
						Hostname: "name",
						URL:      "url",
						Created:  time.Now().Unix() - 100,
					}), ShouldBeNil)
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					So(err, ShouldBeNil)
					// For now, won't destroy a instance if it's set to drained or newly created but
					// hasn't connected to swarming yet
					So(tqt.GetScheduledTasks(), ShouldHaveLength, 0)
				})

				Convey("timeout", func() {
					swr.err = status.Errorf(codes.NotFound, "not found")
					So(datastore.Put(c, &model.VM{
						ID:       "id",
						Config:   "config",
						Created:  1,
						Hostname: "name",
						Timeout:  1,
						URL:      "url",
					}), ShouldBeNil)
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					So(err, ShouldBeNil)
					So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
					So(tqt.GetScheduledTasks()[0].Payload, ShouldHaveSameTypeAs, &tasks.DestroyInstance{})
				})

				Convey("wait", func() {
					swr.err = status.Errorf(codes.NotFound, "not found")
					So(datastore.Put(c, &model.VM{
						ID:       "id",
						Config:   "config",
						Hostname: "name",
						URL:      "url",
					}), ShouldBeNil)
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					So(err, ShouldBeNil)
				})
			})

			Convey("found", func() {
				Convey("deleted", func() {
					swr.getBotResponse = &swarmingpb.BotInfo{
						BotId:   "id",
						Deleted: true,
					}
					So(datastore.Put(c, &model.VM{
						ID:       "id",
						Config:   "config",
						Hostname: "name",
						URL:      "url",
						// Has to be older than time.Now().Unix() - minPendingMinutesForBotConnected * 10
						Created: time.Now().Unix() - 10000,
					}), ShouldBeNil)
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
					swr.getBotResponse = &swarmingpb.BotInfo{
						BotId:   "id",
						Deleted: true,
					}
					So(datastore.Put(c, &model.VM{
						ID:       "id",
						Config:   "config",
						Hostname: "name",
						URL:      "url",
						Created:  time.Now().Unix() - 10,
					}), ShouldBeNil)
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					So(err, ShouldBeNil)
					// Won't destroy the instance if it's a newly created VM
					So(tqt.GetScheduledTasks(), ShouldHaveLength, 0)
				})

				Convey("dead", func() {
					swr.getBotResponse = &swarmingpb.BotInfo{
						BotId:       "id",
						FirstSeenTs: someTimeAgo,
						IsDead:      true,
					}
					So(datastore.Put(c, &model.VM{
						ID:       "id",
						Config:   "config",
						Hostname: "name",
						URL:      "url",
						// Has to be older than time.Now().Unix() - minPendingMinutesForBotConnected * 10
						Created: time.Now().Unix() - 10000,
					}), ShouldBeNil)
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
					swr.getBotResponse = &swarmingpb.BotInfo{
						BotId:       "id",
						FirstSeenTs: someTimeAgo,
						IsDead:      true,
					}
					So(datastore.Put(c, &model.VM{
						ID:       "id",
						Config:   "config",
						Hostname: "name",
						URL:      "url",
						Created:  time.Now().Unix() - 10,
					}), ShouldBeNil)
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					So(err, ShouldBeNil)
					// won't destroy the instance if it's a newly created VM
					So(tqt.GetScheduledTasks(), ShouldHaveLength, 0)
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

		swr := &mockSwarmingBotsClient{}

		c := withDispatcher(memory.Use(context.Background()), dsp)
		c = withSwarming(c, func(context.Context, string) swarmingpb.BotsClient { return swr })

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
				So(datastore.Put(c, &model.VM{
					ID:       "id",
					Hostname: "new",
				}), ShouldBeNil)
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
				swr.err = status.Errorf(codes.Internal, "internal error")
				So(datastore.Put(c, &model.VM{
					ID:       "id",
					Hostname: "name",
				}), ShouldBeNil)
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
				swr.terminateBotResponse = &swarmingpb.TerminateResponse{}
				So(datastore.Put(c, &model.VM{
					ID:       "id",
					Hostname: "name",
				}), ShouldBeNil)
				terminateTask := tasks.TerminateBot{
					Id:       "id",
					Hostname: "name",
				}
				So(terminateBot(c, &terminateTask), ShouldBeNil)
				So(swr.calls, ShouldEqual, 1)

				Convey("wait 1 hour before sending another terminate task", func() {
					c, _ = testclock.UseTime(c, testclock.TestRecentTimeUTC.Add(time.Hour-time.Second))
					So(terminateBot(c, &terminateTask), ShouldBeNil)
					So(swr.calls, ShouldEqual, 1)

					c, _ = testclock.UseTime(c, testclock.TestRecentTimeUTC.Add(time.Hour))
					So(terminateBot(c, &terminateTask), ShouldBeNil)
					So(swr.calls, ShouldEqual, 2)
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

		swr := &mockSwarmingBotsClient{}

		c := withDispatcher(memory.Use(context.Background()), dsp)
		c = withSwarming(c, func(context.Context, string) swarmingpb.BotsClient { return swr })

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
			swr.err = status.Errorf(codes.Internal, "internal server error")
			err := inspectSwarming(c, &tasks.InspectSwarming{
				Swarming: "https://gce-swarming.appspot.com",
			})
			So(err, ShouldNotBeNil)
		})
		Convey("Ignore non-gce bot", func() {
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
			swr.listBotsResponse = &swarmingpb.BotInfoListResponse{
				Cursor: "",
				Items: []*swarmingpb.BotInfo{
					{
						BotId:       "vm-1-abcd",
						FirstSeenTs: someTimeAgo,
					},
					{
						BotId:       "vm-2-abcd",
						FirstSeenTs: someTimeAgo,
					},
					// We don't have a record for this bot in datastore
					{
						BotId:       "vm-3-abcd",
						FirstSeenTs: someTimeAgo,
					},
				},
			}
			err := inspectSwarming(c, &tasks.InspectSwarming{
				Swarming: "https://gce-swarming.appspot.com",
			})
			So(err, ShouldBeNil)
			// ignoring vm-3-abcd as we didn't see it in datastore
			So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
		})
		Convey("Delete dead or deleted bots", func() {
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
			swr.listBotsResponse = &swarmingpb.BotInfoListResponse{
				Cursor: "",
				Items: []*swarmingpb.BotInfo{
					{
						BotId:       "vm-1-abcd",
						FirstSeenTs: someTimeAgo,
						IsDead:      true,
					},
					{
						BotId:       "vm-2-abcd",
						FirstSeenTs: someTimeAgo,
						Deleted:     true,
					},
				},
			}
			err := inspectSwarming(c, &tasks.InspectSwarming{
				Swarming: "https://gce-swarming.appspot.com",
			})
			So(err, ShouldBeNil)
			So(tqt.GetScheduledTasks(), ShouldHaveLength, 2)
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
			swr.listBotsResponse = &swarmingpb.BotInfoListResponse{
				Cursor: "",
				Items: []*swarmingpb.BotInfo{
					{
						BotId:       "vm-1-abcd",
						FirstSeenTs: someTimeAgo,
					},
					{
						BotId:       "vm-2-abcd",
						FirstSeenTs: someTimeAgo,
					},
				},
			}
			err := inspectSwarming(c, &tasks.InspectSwarming{
				Swarming: "https://gce-swarming.appspot.com",
			})
			So(err, ShouldBeNil)
			So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
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
			swr.listBotsResponse = &swarmingpb.BotInfoListResponse{
				Cursor: "",
				Items: []*swarmingpb.BotInfo{
					{
						BotId:       "vm-3-abcd",
						FirstSeenTs: someTimeAgo,
					},
				},
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
			swr.listBotsResponse = &swarmingpb.BotInfoListResponse{
				Cursor: "cursor",
				Items: []*swarmingpb.BotInfo{
					{
						BotId:       "vm-1-abcd",
						FirstSeenTs: someTimeAgo,
					},
					{
						BotId:       "vm-2-abcd",
						FirstSeenTs: someTimeAgo,
					},
				},
			}
			err := inspectSwarming(c, &tasks.InspectSwarming{
				Swarming: "https://gce-swarming.appspot.com",
			})
			So(err, ShouldBeNil)
			// One DeleteStaleSwarmingBots tasks and one inspectSwarming task with cursor
			So(tqt.GetScheduledTasks(), ShouldHaveLength, 2)
		})

	})
}

func TestDeleteStaleSwarmingBot(t *testing.T) {
	t.Parallel()

	Convey("deleteStaleSwarmingBot", t, func() {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)

		swr := &mockSwarmingBotsClient{}

		c := withDispatcher(memory.Use(context.Background()), dsp)
		c = withSwarming(c, func(context.Context, string) swarmingpb.BotsClient { return swr })

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
				swr.err = status.Errorf(codes.NotFound, "not found")
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
				swr.getBotResponse = &swarmingpb.BotInfo{
					BotId:       "vm-3-abcd",
					FirstSeenTs: someTimeAgo,
				}
				swr.listBotEventsResponse = &swarmingpb.BotEventsResponse{
					Items: []*swarmingpb.BotEventResponse{
						{EventType: "bot_terminate"},
					},
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
				swr.getBotResponse = &swarmingpb.BotInfo{
					BotId:       "vm-3-abcd",
					FirstSeenTs: someTimeAgo,
				}
				swr.listBotEventsResponse = &swarmingpb.BotEventsResponse{
					Items: []*swarmingpb.BotEventResponse{},
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
				swr.getBotResponse = &swarmingpb.BotInfo{
					BotId:       "vm-3-abcd",
					FirstSeenTs: someTimeAgo,
				}
				swr.listBotEventsResponse = &swarmingpb.BotEventsResponse{
					Items: []*swarmingpb.BotEventResponse{},
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

type mockSwarmingBotsClient struct {
	err   error
	calls int

	deleteBotResponse     *swarmingpb.DeleteResponse
	getBotResponse        *swarmingpb.BotInfo
	listBotEventsResponse *swarmingpb.BotEventsResponse
	terminateBotResponse  *swarmingpb.TerminateResponse
	listBotsResponse      *swarmingpb.BotInfoListResponse

	swarmingpb.BotsClient // "implements" remaining RPCs by nil panicking
}

func handleCall[R any](mc *mockSwarmingBotsClient, method string, resp *R) (*R, error) {
	mc.calls++
	if mc.err != nil {
		return nil, mc.err
	}
	if resp == nil {
		panic(fmt.Sprintf("unexpected call to %s", method))
	}
	return resp, nil
}

func (mc *mockSwarmingBotsClient) GetBot(context.Context, *swarmingpb.BotRequest, ...grpc.CallOption) (*swarmingpb.BotInfo, error) {
	return handleCall(mc, "GetBot", mc.getBotResponse)
}

func (mc *mockSwarmingBotsClient) TerminateBot(context.Context, *swarmingpb.TerminateRequest, ...grpc.CallOption) (*swarmingpb.TerminateResponse, error) {
	return handleCall(mc, "TerminateBot", mc.terminateBotResponse)
}

func (mc *mockSwarmingBotsClient) DeleteBot(context.Context, *swarmingpb.BotRequest, ...grpc.CallOption) (*swarmingpb.DeleteResponse, error) {
	return handleCall(mc, "DeleteBot", mc.deleteBotResponse)
}

func (mc *mockSwarmingBotsClient) ListBotEvents(context.Context, *swarmingpb.BotEventsRequest, ...grpc.CallOption) (*swarmingpb.BotEventsResponse, error) {
	return handleCall(mc, "ListBotEvents", mc.listBotEventsResponse)
}

func (mc *mockSwarmingBotsClient) ListBots(context.Context, *swarmingpb.BotsRequest, ...grpc.CallOption) (*swarmingpb.BotInfoListResponse, error) {
	return handleCall(mc, "ListBots", mc.listBotsResponse)
}
