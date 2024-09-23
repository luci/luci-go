// Copyright 2023 The LUCI Authors.
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

package rpcs

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/cfg/cfgtest"
	"go.chromium.org/luci/swarming/server/model"
)

const (
	// DefaultFakeCaller is used in unit tests by default as the caller identity.
	DefaultFakeCaller identity.Identity = "user:test@example.com"
	// AdminFakeCaller is used in unit tests to make calls as an admin.
	AdminFakeCaller identity.Identity = "user:admin@example.com"
)

// TestTime is used in mocked entities in RPC tests.
var TestTime = time.Date(2023, time.January, 1, 2, 3, 4, 0, time.UTC)

// MockedRequestState is all per-RPC state that can be mocked.
type MockedRequestState struct {
	Caller  identity.Identity
	AuthDB  *authtest.FakeDB
	Configs *cfgtest.MockedConfigs
}

// NewMockedRequestState creates a new empty request state that can be mutated
// by calling its various methods before it is passed to MockRequestState.
func NewMockedRequestState() *MockedRequestState {
	return &MockedRequestState{
		Caller: DefaultFakeCaller,
		AuthDB: authtest.NewFakeDB(
			authtest.MockMembership(AdminFakeCaller, cfgtest.MockedAdminGroup),
		),
		Configs: cfgtest.NewMockedConfigs(),
	}
}

// MockPerm mocks a permission the caller will have in some realm.
func (s *MockedRequestState) MockPerm(realm string, perm ...realms.Permission) {
	for _, p := range perm {
		s.AuthDB.AddMocks(authtest.MockPermission(s.Caller, realm, p))
	}
}

// SetCaller returns a copy of the state with a different active caller.
func (s *MockedRequestState) SetCaller(id identity.Identity) *MockedRequestState {
	cpy := *s
	cpy.Caller = id
	return &cpy
}

// MockRequestState prepares a full mock of a per-RPC request state.
//
// Panics if it is invalid.
func MockRequestState(ctx context.Context, state *MockedRequestState) context.Context {
	ctx = auth.WithState(ctx, &authtest.FakeState{
		Identity: state.Caller,
		FakeDB:   state.AuthDB,
	})
	cfg := cfgtest.MockConfigs(ctx, state.Configs).Cached(ctx)
	return context.WithValue(ctx, &requestStateCtxKey, &RequestState{
		Config: cfg,
		ACL:    acls.NewChecker(ctx, cfg),
	})
}

// SetupTestBots mocks a bunch of bots and pools.
func SetupTestBots(ctx context.Context) *MockedRequestState {
	state := NewMockedRequestState()
	state.Configs.Settings.BotDeathTimeoutSecs = 1234

	state.Configs.MockPool("visible-pool1", "project:visible-realm")
	state.Configs.MockPool("visible-pool2", "project:visible-realm")
	state.Configs.MockPool("hidden-pool1", "project:hidden-realm")
	state.Configs.MockPool("hidden-pool2", "project:hidden-realm")

	state.MockPerm("project:visible-realm", acls.PermPoolsListBots)

	type testBot struct {
		id          string
		pool        string
		dims        []string
		quarantined bool
		maintenance bool
		busy        bool
		dead        bool
	}

	testBots := []testBot{}
	addMany := func(num int, pfx testBot) {
		id := pfx.id
		for i := 0; i < num; i++ {
			pfx.id = fmt.Sprintf("%s-%d", id, i)
			pfx.dims = []string{fmt.Sprintf("idx:%d", i), fmt.Sprintf("dup:%d", i)}
			testBots = append(testBots, pfx)
		}
	}

	addMany(3, testBot{
		id:   "visible1",
		pool: "visible-pool1",
	})
	addMany(3, testBot{
		id:   "visible2",
		pool: "visible-pool2",
	})
	addMany(3, testBot{
		id:          "quarantined",
		pool:        "visible-pool1",
		quarantined: true,
	})
	addMany(3, testBot{
		id:          "maintenance",
		pool:        "visible-pool1",
		maintenance: true,
	})
	addMany(3, testBot{
		id:   "busy",
		pool: "visible-pool1",
		busy: true,
	})
	addMany(3, testBot{
		id:   "dead",
		pool: "visible-pool1",
		dead: true,
	})
	addMany(3, testBot{
		id:   "hidden1",
		pool: "hidden-pool1",
	})
	addMany(3, testBot{
		id:   "hidden2",
		pool: "hidden-pool2",
	})

	for _, bot := range testBots {
		pick := func(attr bool, yes model.BotStateEnum, no model.BotStateEnum) model.BotStateEnum {
			if attr {
				return yes
			}
			return no
		}
		state.Configs.MockBot(bot.id, bot.pool) // add it to ACLs
		err := datastore.Put(ctx, &model.BotInfo{
			Key:        model.BotInfoKey(ctx, bot.id),
			Dimensions: append(bot.dims, "pool:"+bot.pool),
			Composite: []model.BotStateEnum{
				pick(bot.maintenance, model.BotStateInMaintenance, model.BotStateNotInMaintenance),
				pick(bot.dead, model.BotStateDead, model.BotStateAlive),
				pick(bot.quarantined, model.BotStateQuarantined, model.BotStateHealthy),
				pick(bot.busy, model.BotStateBusy, model.BotStateIdle),
			},
		})
		if err != nil {
			panic(err)
		}
	}

	return state
}

// SetupTestTasks mocks a bunch of tasks and pools.
//
// Additionally returns a map from a fake task name to its request ID.
func SetupTestTasks(ctx context.Context) (*MockedRequestState, map[string]string) {
	state := NewMockedRequestState()

	state.Configs.MockPool("visible-pool1", "project:visible-realm")
	state.Configs.MockPool("visible-pool2", "project:visible-realm")
	state.Configs.MockPool("hidden-pool1", "project:hidden-realm")
	state.Configs.MockPool("hidden-pool2", "project:hidden-realm")

	state.MockPerm("project:visible-realm", acls.PermPoolsCancelTask, acls.PermPoolsListTasks, acls.PermTasksGet)

	// This is used to make sure all task keys are unique, even if they are
	// created at the exact same (mocked) timestamp. In the prod implementation
	// this is a random number.
	var taskCounter int64

	tasks := map[string]string{}

	putTask := func(name string, tags []string, state apipb.TaskState, failure, dedup, bbtask, visible bool, ts time.Duration) {
		reqKey, err := model.TimestampToRequestKey(ctx, TestTime.Add(ts), taskCounter)
		if err != nil {
			panic(err)
		}
		taskCounter++
		var tryNumber datastore.Nullable[int64, datastore.Indexed]
		var dedupedFrom string
		if dedup {
			tryNumber = datastore.NewIndexedNullable[int64](0)
			dupKey, err := model.TimestampToRequestKey(ctx, TestTime.Add(-time.Hour), taskCounter)
			if err != nil {
				panic(err)
			}
			dedupedFrom = model.RequestKeyToTaskID(
				dupKey, // doesn't really exist, but it is not a problem for tests
				model.AsRunResult,
			)
		}
		var realm, pool string
		if visible {
			realm, pool = "project:visible-realm", "visible-pool1"
		} else {
			realm, pool = "project:hidden-realm", "hidden-pool1"
		}
		err = datastore.Put(ctx,
			&model.TaskRequest{
				Key:   reqKey,
				Name:  name,
				Realm: realm,
				TaskSlices: []model.TaskSlice{
					{
						Properties: model.TaskProperties{
							Dimensions: model.TaskDimensions{
								"pool": {pool},
							},
						},
					},
				},
			},
			&model.TaskResultSummary{
				TaskResultCommon: model.TaskResultCommon{
					State:         state,
					BotDimensions: map[string][]string{"payload": {fmt.Sprintf("%s-%s", state, ts)}},
					Started:       datastore.NewIndexedNullable(TestTime.Add(ts)),
					Completed:     datastore.NewIndexedNullable(TestTime.Add(ts)),
					Failure:       failure,
				},
				Key:          model.TaskResultSummaryKey(ctx, reqKey),
				RequestName:  name,
				RequestRealm: realm,
				Tags:         tags,
				TryNumber:    tryNumber,
				DedupedFrom:  dedupedFrom,
			},
			// Note: this is actually ignored for tasks that didn't run at all.
			&model.PerformanceStats{
				Key:             model.PerformanceStatsKey(ctx, reqKey),
				BotOverheadSecs: 123.0,
			},
		)
		if err != nil {
			panic(err)
		}
		if bbtask {
			err = datastore.Put(ctx, &model.BuildTask{
				Key:              model.BuildTaskKey(ctx, reqKey),
				BuildID:          fmt.Sprintf("%d", 1000+taskCounter),
				BuildbucketHost:  "bb-host",
				UpdateID:         100,
				LatestTaskStatus: state,
			})
			if err != nil {
				panic(err)
			}
		}

		tasks[name] = model.RequestKeyToTaskID(reqKey, model.AsRequest)
	}

	putMany := func(pfx string, state apipb.TaskState, failure, dedup, bbtask bool) {
		for i := 0; i < 3; i++ {
			putTask(
				fmt.Sprintf("%s-%d", pfx, i),
				[]string{
					fmt.Sprintf("pfx:%s", pfx),
					fmt.Sprintf("idx:%d", i),
					fmt.Sprintf("dup:%d", i),
				},
				state, failure, dedup, bbtask, i == 0,
				time.Duration(10*i)*time.Minute,
			)
		}
	}

	// 12 groups of tasks, 3 tasks in each group => 36 tasks total.
	//
	// `dedup` and `clienterror` have no BuildTask associated with them.
	putMany("pending", apipb.TaskState_PENDING, false, false, true)
	putMany("running", apipb.TaskState_RUNNING, false, false, true)
	putMany("success", apipb.TaskState_COMPLETED, false, false, true)
	putMany("failure", apipb.TaskState_COMPLETED, true, false, true)
	putMany("dedup", apipb.TaskState_COMPLETED, false, true, false)
	putMany("expired", apipb.TaskState_EXPIRED, false, false, true)
	putMany("timeout", apipb.TaskState_TIMED_OUT, false, false, true)
	putMany("botdead", apipb.TaskState_BOT_DIED, false, false, true)
	putMany("canceled", apipb.TaskState_CANCELED, false, false, true)
	putMany("killed", apipb.TaskState_KILLED, false, false, true)
	putMany("noresource", apipb.TaskState_NO_RESOURCE, false, false, true)
	putMany("clienterror", apipb.TaskState_CLIENT_ERROR, false, false, false)

	// Add a few intentionally missing IDs as well.
	tasks["missing-0"] = "65aba3a3e6b99310"
	tasks["missing-1"] = "75aba3a3e6b99310"
	tasks["missing-2"] = "85aba3a3e6b99310"

	return state, tasks
}

func TestServerInterceptor(t *testing.T) {
	t.Parallel()

	ftt.Run("With config in datastore", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: identity.AnonymousIdentity,
		})
		cfg := cfgtest.MockConfigs(ctx, cfgtest.NewMockedConfigs())

		interceptor := ServerInterceptor(cfg, []string{
			"swarming.v2.Swarming",
		})

		t.Run("Sets up state", func(t *ftt.Test) {
			var state *RequestState
			err := interceptor(ctx, "/swarming.v2.Swarming/GetPermissions", func(ctx context.Context) error {
				state = State(ctx)
				return nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, state, should.NotBeNil)
			assert.Loosely(t, state.Config, should.NotBeNil)
			assert.Loosely(t, state.ACL, should.NotBeNil)
		})

		t.Run("Skips unrelated APIs", func(t *ftt.Test) {
			var called bool
			err := interceptor(ctx, "/another.Service/GetPermissions", func(ctx context.Context) error {
				called = true
				defer func() { assert.Loosely(t, recover(), should.NotBeNilInterface) }()
				State(ctx) // panics
				return nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, called, should.BeTrue)
		})
	})
}
