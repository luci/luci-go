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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

var someTimeAgo = timestamppb.New(time.Date(2022, 1, 1, 1, 1, 1, 0, time.UTC))

func TestDeleteBot(t *testing.T) {
	t.Parallel()

	ftt.Run("deleteBot", t, func(t *ftt.Test) {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)

		swr := &mockSwarmingBotsClient{}

		c := withDispatcher(memory.Use(context.Background()), dsp)
		c = withSwarming(c, func(context.Context, string) swarmingpb.BotsClient { return swr })

		tqt := tqtesting.GetTestable(c, dsp)
		tqt.CreateQueues()

		t.Run("invalid", func(t *ftt.Test) {
			t.Run("nil", func(t *ftt.Test) {
				err := deleteBot(c, nil)
				assert.Loosely(t, err, should.ErrLike("unexpected payload"))
			})

			t.Run("empty", func(t *ftt.Test) {
				err := deleteBot(c, &tasks.DeleteBot{})
				assert.Loosely(t, err, should.ErrLike("ID is required"))
			})

			t.Run("hostname", func(t *ftt.Test) {
				err := deleteBot(c, &tasks.DeleteBot{
					Id: "id",
				})
				assert.Loosely(t, err, should.ErrLike("hostname is required"))
			})
		})

		t.Run("valid", func(t *ftt.Test) {
			t.Run("missing", func(t *ftt.Test) {
				err := deleteBot(c, &tasks.DeleteBot{
					Id:       "id",
					Hostname: "name",
				})
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("error", func(t *ftt.Test) {
				swr.err = status.Errorf(codes.Internal, "boom")
				assert.Loosely(t, datastore.Put(c, &model.VM{
					ID:       "id",
					Created:  1,
					Hostname: "name",
					Lifetime: 1,
					URL:      "url",
				}), should.BeNil)
				err := deleteBot(c, &tasks.DeleteBot{
					Id:       "id",
					Hostname: "name",
				})
				assert.Loosely(t, err, should.ErrLike("failed to delete bot"))
				v := &model.VM{
					ID: "id",
				}
				assert.Loosely(t, datastore.Get(c, v), should.BeNil)
				assert.Loosely(t, v.Created, should.Equal(1))
				assert.Loosely(t, v.Hostname, should.Equal("name"))
				assert.Loosely(t, v.URL, should.Equal("url"))
			})

			t.Run("deleted", func(t *ftt.Test) {
				swr.err = status.Errorf(codes.NotFound, "not found")
				assert.Loosely(t, datastore.Put(c, &model.VM{
					ID:       "id",
					Created:  1,
					Hostname: "name",
					Lifetime: 1,
					URL:      "url",
				}), should.BeNil)
				err := deleteBot(c, &tasks.DeleteBot{
					Id:       "id",
					Hostname: "name",
				})
				assert.Loosely(t, err, should.BeNil)
				v := &model.VM{
					ID: "id",
				}
				assert.Loosely(t, datastore.Get(c, v), should.Equal(datastore.ErrNoSuchEntity))
			})

			t.Run("deletes", func(t *ftt.Test) {
				swr.deleteBotResponse = &swarmingpb.DeleteResponse{}
				assert.Loosely(t, datastore.Put(c, &model.VM{
					ID:       "id",
					Created:  1,
					Hostname: "name",
					Lifetime: 1,
					URL:      "url",
				}), should.BeNil)
				err := deleteBot(c, &tasks.DeleteBot{
					Id:       "id",
					Hostname: "name",
				})
				assert.Loosely(t, err, should.BeNil)
				v := &model.VM{
					ID: "id",
				}
				assert.Loosely(t, datastore.Get(c, v), should.Equal(datastore.ErrNoSuchEntity))
			})
		})
	})
}

func TestDeleteVM(t *testing.T) {
	t.Parallel()

	ftt.Run("deleteVM", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())

		t.Run("deletes", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(c, &model.VM{
				ID:       "id",
				Hostname: "name",
			}), should.BeNil)
			assert.Loosely(t, deleteVM(c, "id", "name"), should.BeNil)
			v := &model.VM{
				ID: "id",
			}
			assert.Loosely(t, datastore.Get(c, v), should.Equal(datastore.ErrNoSuchEntity))
		})

		t.Run("deleted", func(t *ftt.Test) {
			assert.Loosely(t, deleteVM(c, "id", "name"), should.BeNil)
		})

		t.Run("replaced", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(c, &model.VM{
				ID:       "id",
				Hostname: "name-2",
			}), should.BeNil)
			assert.Loosely(t, deleteVM(c, "id", "name-1"), should.BeNil)
			v := &model.VM{
				ID: "id",
			}
			assert.Loosely(t, datastore.Get(c, v), should.BeNil)
			assert.Loosely(t, v.Hostname, should.Equal("name-2"))
		})
	})
}

func TestManageBot(t *testing.T) {
	t.Parallel()

	ftt.Run("manageBot", t, func(t *ftt.Test) {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)

		swr := &mockSwarmingBotsClient{}

		c := withDispatcher(memory.Use(context.Background()), dsp)
		c = withSwarming(c, func(context.Context, string) swarmingpb.BotsClient { return swr })

		tqt := tqtesting.GetTestable(c, dsp)
		tqt.CreateQueues()

		t.Run("invalid", func(t *ftt.Test) {
			t.Run("nil", func(t *ftt.Test) {
				err := manageBot(c, nil)
				assert.Loosely(t, err, should.ErrLike("unexpected payload"))
			})

			t.Run("empty", func(t *ftt.Test) {
				err := manageBot(c, &tasks.ManageBot{})
				assert.Loosely(t, err, should.ErrLike("ID is required"))
			})
		})

		t.Run("valid", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(c, &model.Config{
				ID: "config",
				Config: &config.Config{
					CurrentAmount: 1,
				},
			}), should.BeNil)

			t.Run("deleted", func(t *ftt.Test) {
				err := manageBot(c, &tasks.ManageBot{
					Id: "id",
				})
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("creating", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(c, &model.VM{
					ID:     "id",
					Config: "config",
				}), should.BeNil)
				err := manageBot(c, &tasks.ManageBot{
					Id: "id",
				})
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("error", func(t *ftt.Test) {
				swr.err = status.Errorf(codes.InvalidArgument, "unexpected error")
				assert.Loosely(t, datastore.Put(c, &model.VM{
					ID:     "id",
					Config: "config",
					URL:    "url",
				}), should.BeNil)
				err := manageBot(c, &tasks.ManageBot{
					Id: "id",
				})
				assert.Loosely(t, err, should.ErrLike("failed to fetch bot"))
			})

			t.Run("missing", func(t *ftt.Test) {
				t.Run("deadline", func(t *ftt.Test) {
					swr.err = status.Errorf(codes.NotFound, "not found")
					assert.Loosely(t, datastore.Put(c, &model.VM{
						ID:       "id",
						Config:   "config",
						Created:  1,
						Hostname: "name",
						Lifetime: 1,
						URL:      "url",
					}), should.BeNil)
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(1))
					assert.Loosely(t, tqt.GetScheduledTasks()[0].Payload, should.HaveType[*tasks.DestroyInstance])
					v := &model.VM{
						ID: "id",
					}
					assert.Loosely(t, datastore.Get(c, v), should.BeNil)
				})

				t.Run("drained & new bots", func(t *ftt.Test) {
					swr.err = status.Errorf(codes.NotFound, "not found")
					assert.Loosely(t, datastore.Put(c, &model.VM{
						ID:       "id",
						Config:   "config",
						Drained:  true,
						Hostname: "name",
						URL:      "url",
						Created:  time.Now().Unix() - 100,
					}), should.BeNil)
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					assert.Loosely(t, err, should.BeNil)
					// For now, won't destroy a instance if it's set to drained or newly created but
					// hasn't connected to swarming yet
					assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(0))
				})

				t.Run("timeout", func(t *ftt.Test) {
					swr.err = status.Errorf(codes.NotFound, "not found")
					assert.Loosely(t, datastore.Put(c, &model.VM{
						ID:       "id",
						Config:   "config",
						Created:  1,
						Hostname: "name",
						Timeout:  1,
						URL:      "url",
					}), should.BeNil)
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(1))
					assert.Loosely(t, tqt.GetScheduledTasks()[0].Payload, should.HaveType[*tasks.DestroyInstance])
				})

				t.Run("wait", func(t *ftt.Test) {
					swr.err = status.Errorf(codes.NotFound, "not found")
					assert.Loosely(t, datastore.Put(c, &model.VM{
						ID:       "id",
						Config:   "config",
						Hostname: "name",
						URL:      "url",
					}), should.BeNil)
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					assert.Loosely(t, err, should.BeNil)
				})
			})

			t.Run("found", func(t *ftt.Test) {
				t.Run("deleted", func(t *ftt.Test) {
					swr.getBotResponse = &swarmingpb.BotInfo{
						BotId:   "id",
						Deleted: true,
					}
					assert.Loosely(t, datastore.Put(c, &model.VM{
						ID:       "id",
						Config:   "config",
						Hostname: "name",
						URL:      "url",
						// Has to be older than time.Now().Unix() - minPendingMinutesForBotConnected * 10
						Created: time.Now().Unix() - 10000,
					}), should.BeNil)
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(1))
					assert.Loosely(t, tqt.GetScheduledTasks()[0].Payload, should.HaveType[*tasks.DestroyInstance])
					v := &model.VM{
						ID: "id",
					}
					assert.Loosely(t, datastore.Get(c, v), should.BeNil)
				})
				t.Run("deleted but newly created", func(t *ftt.Test) {
					swr.getBotResponse = &swarmingpb.BotInfo{
						BotId:   "id",
						Deleted: true,
					}
					assert.Loosely(t, datastore.Put(c, &model.VM{
						ID:       "id",
						Config:   "config",
						Hostname: "name",
						URL:      "url",
						Created:  time.Now().Unix() - 10,
					}), should.BeNil)
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					assert.Loosely(t, err, should.BeNil)
					// Won't destroy the instance if it's a newly created VM
					assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(0))
				})

				t.Run("dead", func(t *ftt.Test) {
					swr.getBotResponse = &swarmingpb.BotInfo{
						BotId:       "id",
						FirstSeenTs: someTimeAgo,
						IsDead:      true,
					}
					assert.Loosely(t, datastore.Put(c, &model.VM{
						ID:       "id",
						Config:   "config",
						Hostname: "name",
						URL:      "url",
						// Has to be older than time.Now().Unix() - minPendingMinutesForBotConnected * 10
						Created: time.Now().Unix() - 10000,
					}), should.BeNil)
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(1))
					assert.Loosely(t, tqt.GetScheduledTasks()[0].Payload, should.HaveType[*tasks.DestroyInstance])
					v := &model.VM{
						ID: "id",
					}
					assert.Loosely(t, datastore.Get(c, v), should.BeNil)
				})

				t.Run("dead but newly created", func(t *ftt.Test) {
					swr.getBotResponse = &swarmingpb.BotInfo{
						BotId:       "id",
						FirstSeenTs: someTimeAgo,
						IsDead:      true,
					}
					assert.Loosely(t, datastore.Put(c, &model.VM{
						ID:       "id",
						Config:   "config",
						Hostname: "name",
						URL:      "url",
						Created:  time.Now().Unix() - 10,
					}), should.BeNil)
					err := manageBot(c, &tasks.ManageBot{
						Id: "id",
					})
					assert.Loosely(t, err, should.BeNil)
					// won't destroy the instance if it's a newly created VM
					assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(0))
				})
			})
		})
	})
}

func TestTerminateBot(t *testing.T) {
	t.Parallel()

	ftt.Run("terminateBot", t, func(t *ftt.Test) {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)

		swr := &mockSwarmingBotsClient{}

		c := withDispatcher(memory.Use(context.Background()), dsp)
		c = withSwarming(c, func(context.Context, string) swarmingpb.BotsClient { return swr })

		tqt := tqtesting.GetTestable(c, dsp)
		tqt.CreateQueues()

		t.Run("invalid", func(t *ftt.Test) {
			t.Run("nil", func(t *ftt.Test) {
				err := terminateBot(c, nil)
				assert.Loosely(t, err, should.ErrLike("unexpected payload"))
			})

			t.Run("empty", func(t *ftt.Test) {
				err := terminateBot(c, &tasks.TerminateBot{})
				assert.Loosely(t, err, should.ErrLike("ID is required"))
			})

			t.Run("hostname", func(t *ftt.Test) {
				err := terminateBot(c, &tasks.TerminateBot{
					Id: "id",
				})
				assert.Loosely(t, err, should.ErrLike("hostname is required"))
			})
		})

		t.Run("valid", func(t *ftt.Test) {
			t.Run("missing", func(t *ftt.Test) {
				err := terminateBot(c, &tasks.TerminateBot{
					Id:       "id",
					Hostname: "name",
				})
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("replaced", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(c, &model.VM{
					ID:       "id",
					Hostname: "new",
				}), should.BeNil)
				err := terminateBot(c, &tasks.TerminateBot{
					Id:       "id",
					Hostname: "old",
				})
				assert.Loosely(t, err, should.BeNil)
				v := &model.VM{
					ID: "id",
				}
				assert.Loosely(t, datastore.Get(c, v), should.BeNil)
				assert.Loosely(t, v.Hostname, should.Equal("new"))
			})

			t.Run("error", func(t *ftt.Test) {
				swr.err = status.Errorf(codes.Internal, "internal error")
				assert.Loosely(t, datastore.Put(c, &model.VM{
					ID:       "id",
					Hostname: "name",
				}), should.BeNil)
				err := terminateBot(c, &tasks.TerminateBot{
					Id:       "id",
					Hostname: "name",
				})
				assert.Loosely(t, err, should.ErrLike("terminate bot \"name\""))
				v := &model.VM{
					ID: "id",
				}
				assert.Loosely(t, datastore.Get(c, v), should.BeNil)
				assert.Loosely(t, v.Hostname, should.Equal("name"))
			})

			t.Run("terminates", func(t *ftt.Test) {
				c, _ = testclock.UseTime(c, testclock.TestRecentTimeUTC)
				swr.terminateBotResponse = &swarmingpb.TerminateResponse{}
				assert.Loosely(t, datastore.Put(c, &model.VM{
					ID:       "id",
					Hostname: "name",
				}), should.BeNil)
				terminateTask := tasks.TerminateBot{
					Id:       "id",
					Hostname: "name",
				}
				assert.Loosely(t, terminateBot(c, &terminateTask), should.BeNil)
				assert.Loosely(t, swr.calls, should.Equal(1))

				t.Run("wait 1 hour before sending another terminate task", func(t *ftt.Test) {
					c, _ = testclock.UseTime(c, testclock.TestRecentTimeUTC.Add(time.Hour-time.Second))
					assert.Loosely(t, terminateBot(c, &terminateTask), should.BeNil)
					assert.Loosely(t, swr.calls, should.Equal(1))

					c, _ = testclock.UseTime(c, testclock.TestRecentTimeUTC.Add(time.Hour))
					assert.Loosely(t, terminateBot(c, &terminateTask), should.BeNil)
					assert.Loosely(t, swr.calls, should.Equal(2))
				})
			})
		})
	})
}

func TestInspectSwarming(t *testing.T) {
	t.Parallel()

	ftt.Run("inspectSwarmingAsync", t, func(t *ftt.Test) {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)
		c := withDispatcher(memory.Use(context.Background()), dsp)
		tqt := tqtesting.GetTestable(c, dsp)
		tqt.CreateQueues()
		datastore.GetTestable(c).Consistent(true)

		t.Run("none", func(t *ftt.Test) {
			err := inspectSwarmingAsync(c)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(0))
		})

		t.Run("one", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(c, &model.Config{
				ID: "config-1",
				Config: &config.Config{
					Swarming: "https://gce-swarming.appspot.com",
				},
			}), should.BeNil)
			err := inspectSwarmingAsync(c)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(1))
		})

		t.Run("two", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(c, &model.Config{
				ID: "config-1",
				Config: &config.Config{
					Swarming: "https://gce-swarming.appspot.com",
				},
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(c, &model.Config{
				ID: "config-2",
				Config: &config.Config{
					Swarming: "https://vmleaser-swarming.appspot.com",
				},
			}), should.BeNil)
			err := inspectSwarmingAsync(c)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(2))
		})
	})

	ftt.Run("inspectSwarming", t, func(t *ftt.Test) {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)

		swr := &mockSwarmingBotsClient{}

		c := withDispatcher(memory.Use(context.Background()), dsp)
		c = withSwarming(c, func(context.Context, string) swarmingpb.BotsClient { return swr })

		tqt := tqtesting.GetTestable(c, dsp)
		tqt.CreateQueues()
		datastore.GetTestable(c).Consistent(true)

		t.Run("BadInputs", func(t *ftt.Test) {
			t.Run("nil", func(t *ftt.Test) {
				err := inspectSwarming(c, nil)
				assert.Loosely(t, err, should.NotBeNil)
			})
			t.Run("empty", func(t *ftt.Test) {
				err := inspectSwarming(c, &tasks.InspectSwarming{})
				assert.Loosely(t, err, should.NotBeNil)
			})
		})

		t.Run("Swarming error", func(t *ftt.Test) {
			swr.err = status.Errorf(codes.Internal, "internal server error")
			err := inspectSwarming(c, &tasks.InspectSwarming{
				Swarming: "https://gce-swarming.appspot.com",
			})
			assert.Loosely(t, err, should.NotBeNil)
		})
		t.Run("Ignore non-gce bot", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(c, &model.VM{
				ID:       "vm-1",
				Hostname: "vm-1-abcd",
				Swarming: "https://gce-swarming.appspot.com",
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(c, &model.VM{
				ID:       "vm-2",
				Hostname: "vm-2-abcd",
				Swarming: "https://gce-swarming.appspot.com",
			}), should.BeNil)
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
			assert.Loosely(t, err, should.BeNil)
			// ignoring vm-3-abcd as we didn't see it in datastore
			assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(1))
		})
		t.Run("Delete dead or deleted bots", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(c, &model.VM{
				ID:       "vm-1",
				Hostname: "vm-1-abcd",
				Swarming: "https://gce-swarming.appspot.com",
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(c, &model.VM{
				ID:       "vm-2",
				Hostname: "vm-2-abcd",
				Swarming: "https://gce-swarming.appspot.com",
			}), should.BeNil)
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(2))
		})
		t.Run("HappyPath-1", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(c, &model.VM{
				ID:       "vm-1",
				Hostname: "vm-1-abcd",
				Swarming: "https://gce-swarming.appspot.com",
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(c, &model.VM{
				ID:       "vm-2",
				Hostname: "vm-2-abcd",
				Swarming: "https://gce-swarming.appspot.com",
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(c, &model.VM{
				ID:       "vm-3",
				Hostname: "vm-3-abcd",
				Swarming: "https://vmleaser-swarming.appspot.com",
			}), should.BeNil)
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(1))
		})
		t.Run("HappyPath-2", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(c, &model.VM{
				ID:       "vm-1",
				Hostname: "vm-1-abcd",
				Swarming: "https://gce-swarming.appspot.com",
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(c, &model.VM{
				ID:       "vm-2",
				Hostname: "vm-2-abcd",
				Swarming: "https://gce-swarming.appspot.com",
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(c, &model.VM{
				ID:       "vm-3",
				Hostname: "vm-3-abcd",
				Swarming: "https://vmleaser-swarming.appspot.com",
			}), should.BeNil)
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(1))
		})
		t.Run("HappyPath-3-pagination", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(c, &model.VM{
				ID:       "vm-1",
				Hostname: "vm-1-abcd",
				Swarming: "https://gce-swarming.appspot.com",
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(c, &model.VM{
				ID:       "vm-2",
				Hostname: "vm-2-abcd",
				Swarming: "https://gce-swarming.appspot.com",
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(c, &model.VM{
				ID:       "vm-3",
				Hostname: "vm-3-abcd",
				Swarming: "https://gce-swarming.appspot.com",
			}), should.BeNil)
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
			assert.Loosely(t, err, should.BeNil)
			// One DeleteStaleSwarmingBots tasks and one inspectSwarming task with cursor
			assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(2))
		})

	})
}

func TestDeleteStaleSwarmingBot(t *testing.T) {
	t.Parallel()

	ftt.Run("deleteStaleSwarmingBot", t, func(t *ftt.Test) {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)

		swr := &mockSwarmingBotsClient{}

		c := withDispatcher(memory.Use(context.Background()), dsp)
		c = withSwarming(c, func(context.Context, string) swarmingpb.BotsClient { return swr })

		tqt := tqtesting.GetTestable(c, dsp)
		tqt.CreateQueues()
		datastore.GetTestable(c).Consistent(true)

		t.Run("BadInputs", func(t *ftt.Test) {
			t.Run("nil", func(t *ftt.Test) {
				err := deleteStaleSwarmingBot(c, nil)
				assert.Loosely(t, err, should.NotBeNil)
			})
			t.Run("empty", func(t *ftt.Test) {
				err := deleteStaleSwarmingBot(c, &tasks.DeleteStaleSwarmingBot{})
				assert.Loosely(t, err, should.NotBeNil)
			})
			t.Run("missing timestamp", func(t *ftt.Test) {
				err := deleteStaleSwarmingBot(c, &tasks.DeleteStaleSwarmingBot{
					Id: "id-1",
				})
				assert.Loosely(t, err, should.NotBeNil)
			})
		})
		t.Run("VM issues", func(t *ftt.Test) {
			t.Run("Missing VM", func(t *ftt.Test) {
				// Don't err if the VM is missing, prob deleted already
				err := deleteStaleSwarmingBot(c, &tasks.DeleteStaleSwarmingBot{
					Id:          "id-1",
					FirstSeenTs: "onceUponATime",
				})
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("Missing URL in VM", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(c, &model.VM{
					ID:       "vm-3",
					Hostname: "vm-3-abcd",
					Swarming: "https://gce-swarming.appspot.com",
				}), should.BeNil)
				// Don't err if the URL in VM is missing
				err := deleteStaleSwarmingBot(c, &tasks.DeleteStaleSwarmingBot{
					Id:          "id-1",
					FirstSeenTs: "onceUponATime",
				})
				assert.Loosely(t, err, should.BeNil)
			})
		})
		t.Run("Swarming Issues", func(t *ftt.Test) {
			t.Run("Failed to fetch", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(c, &model.VM{
					ID:       "vm-3",
					Hostname: "vm-3-abcd",
					URL:      "https://www.googleapis.com/compute/v1/projects/vmleaser/zones/us-numba1-c/instances/vm-3-abcd",
					Swarming: "https://gce-swarming.appspot.com",
				}), should.BeNil)
				swr.err = status.Errorf(codes.NotFound, "not found")
				err := deleteStaleSwarmingBot(c, &tasks.DeleteStaleSwarmingBot{
					Id:          "id-1",
					FirstSeenTs: "onceUponATime",
				})
				assert.Loosely(t, err, should.BeNil)
			})
		})
		t.Run("Happy paths", func(t *ftt.Test) {
			t.Run("Bot terminated", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(c, &model.VM{
					ID:       "vm-3",
					Hostname: "vm-3-abcd",
					URL:      "https://www.googleapis.com/compute/v1/projects/vmleaser/zones/us-numba1-c/instances/vm-3-abcd",
					Swarming: "https://gce-swarming.appspot.com",
				}), should.BeNil)
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
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(1))
			})
			t.Run("Bot retirement", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(c, &model.VM{
					ID:       "vm-3",
					Hostname: "vm-3-abcd",
					URL:      "https://www.googleapis.com/compute/v1/projects/vmleaser/zones/us-numba1-c/instances/vm-3-abcd",
					Swarming: "https://gce-swarming.appspot.com",
					Lifetime: 99,
					Created:  time.Now().Unix() - 100,
				}), should.BeNil)
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
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(1))
			})
			t.Run("Bot drained", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(c, &model.VM{
					ID:       "vm-3",
					Hostname: "vm-3-abcd",
					URL:      "https://www.googleapis.com/compute/v1/projects/vmleaser/zones/us-numba1-c/instances/vm-3-abcd",
					Swarming: "https://gce-swarming.appspot.com",
					Lifetime: 100000000,
					Created:  time.Now().Unix(),
					Drained:  true,
				}), should.BeNil)
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
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(1))
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
