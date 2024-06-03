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

	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmem "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/cfg"
	"go.chromium.org/luci/swarming/server/model"

	. "github.com/smartystreets/goconvey/convey"
)

const (
	// DefaultFakeCaller is used in unit tests by default as the caller identity.
	DefaultFakeCaller identity.Identity = "user:test@example.com"
	// AdminFakeCaller is used in unit tests to make calls as an admin.
	AdminFakeCaller identity.Identity = "user:admin@example.com"
)

// TestTime is used in mocked entities in RPC tests.
var TestTime = time.Date(2023, time.January, 1, 2, 3, 4, 0, time.UTC)

// MockedConfig is a bundle of configs to use in tests.
type MockedConfigs struct {
	Settings *configpb.SettingsCfg
	Pools    *configpb.PoolsCfg
	Bots     *configpb.BotsCfg
	Scripts  map[string]string
}

// MockedRequestState is all per-RPC state that can be mocked.
type MockedRequestState struct {
	Caller  identity.Identity
	AuthDB  *authtest.FakeDB
	Configs MockedConfigs
}

// NewMockedRequestState creates a new empty request state that can be mutated
// by calling its various methods before it is passed to MockRequestState.
func NewMockedRequestState() *MockedRequestState {
	return &MockedRequestState{
		Caller: DefaultFakeCaller,
		AuthDB: authtest.NewFakeDB(
			authtest.MockMembership(AdminFakeCaller, "tests-admin-group"),
		),
		Configs: MockedConfigs{
			Settings: &configpb.SettingsCfg{
				Auth: &configpb.AuthSettings{
					AdminsGroup: "tests-admin-group",
				},
			},
			Pools: &configpb.PoolsCfg{},
			Bots: &configpb.BotsCfg{
				TrustedDimensions: []string{"pool"},
			},
			Scripts: map[string]string{},
		},
	}
}

// MockPool adds a new pool to the mocked request state.
func (s *MockedRequestState) MockPool(name, realm string) *configpb.Pool {
	pb := &configpb.Pool{
		Name:  []string{name},
		Realm: realm,
	}
	s.Configs.Pools.Pool = append(s.Configs.Pools.Pool, pb)
	return pb
}

// MockBot adds a bot in some pool.
func (s *MockedRequestState) MockBot(botID, pool string) *configpb.BotGroup {
	pb := &configpb.BotGroup{
		BotId:      []string{botID},
		Dimensions: []string{"pool:" + pool},
		Auth: []*configpb.BotAuth{
			{
				RequireLuciMachineToken: true,
			},
		},
	}
	s.Configs.Bots.BotGroup = append(s.Configs.Bots.BotGroup, pb)
	return pb
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

// MockConfig puts configs in datastore and loads then into cfg.Provider.
//
// Configs must be valid, panics if they are not.
func MockConfigs(ctx context.Context, configs MockedConfigs) *cfg.Provider {
	files := make(cfgmem.Files)

	putPb := func(path string, msg proto.Message) {
		if msg != nil {
			blob, err := prototext.Marshal(msg)
			if err != nil {
				panic(err)
			}
			files[path] = string(blob)
		}
	}

	putPb("settings.cfg", configs.Settings)
	putPb("pools.cfg", configs.Pools)
	putPb("bots.cfg", configs.Bots)
	for path, body := range configs.Scripts {
		files[path] = body
	}

	// Put new configs into the datastore.
	err := cfg.UpdateConfigs(cfgclient.Use(ctx, cfgmem.New(map[config.Set]cfgmem.Files{
		"services/${appid}": files,
	})))
	if err != nil {
		panic(err)
	}

	// Load them back in a queriable form.
	p, err := cfg.NewProvider(ctx)
	if err != nil {
		panic(err)
	}
	return p
}

// MockRequestState prepares a full mock of a per-RPC request state.
//
// Panics if it is invalid.
func MockRequestState(ctx context.Context, state *MockedRequestState) context.Context {
	ctx = auth.WithState(ctx, &authtest.FakeState{
		Identity: state.Caller,
		FakeDB:   state.AuthDB,
	})
	cfg := MockConfigs(ctx, state.Configs).Config(ctx)
	return context.WithValue(ctx, &requestStateCtxKey, &RequestState{
		Config: cfg,
		ACL:    acls.NewChecker(ctx, cfg),
	})
}

// SetupTestBots mocks a bunch of bots and pools.
func SetupTestBots(ctx context.Context) *MockedRequestState {
	state := NewMockedRequestState()
	state.Configs.Settings.BotDeathTimeoutSecs = 1234

	state.MockPool("visible-pool1", "project:visible-realm")
	state.MockPool("visible-pool2", "project:visible-realm")
	state.MockPool("hidden-pool1", "project:hidden-realm")
	state.MockPool("hidden-pool2", "project:hidden-realm")

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
		state.MockBot(bot.id, bot.pool) // add it to ACLs
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
func SetupTestTasks(ctx context.Context) *MockedRequestState {
	state := NewMockedRequestState()

	state.MockPool("visible-pool1", "project:visible-realm")
	state.MockPool("visible-pool2", "project:visible-realm")
	state.MockPool("hidden-pool1", "project:hidden-realm")
	state.MockPool("hidden-pool2", "project:hidden-realm")

	state.MockPerm("project:visible-realm", acls.PermPoolsListTasks, acls.PermTasksGet)

	// This is used to make sure all task keys are unique, even if they are
	// created at the exact same (mocked) timestamp. In the prod implementation
	// this is a random number.
	var taskCounter int64

	putTask := func(name string, tags []string, state apipb.TaskState, failure, dedup, visible bool, ts time.Duration) {
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
	}

	putMany := func(pfx string, state apipb.TaskState, failure, dedup bool) {
		for i := 0; i < 3; i++ {
			putTask(
				fmt.Sprintf("%s-%d", pfx, i),
				[]string{
					fmt.Sprintf("pfx:%s", pfx),
					fmt.Sprintf("idx:%d", i),
					fmt.Sprintf("dup:%d", i),
				},
				state, failure, dedup, i == 0,
				time.Duration(10*i)*time.Minute,
			)
		}
	}

	// 12 groups of tasks, 3 tasks in each group => 36 tasks total.
	putMany("pending", apipb.TaskState_PENDING, false, false)
	putMany("running", apipb.TaskState_RUNNING, false, false)
	putMany("success", apipb.TaskState_COMPLETED, false, false)
	putMany("failure", apipb.TaskState_COMPLETED, true, false)
	putMany("dedup", apipb.TaskState_COMPLETED, false, true)
	putMany("expired", apipb.TaskState_EXPIRED, false, false)
	putMany("timeout", apipb.TaskState_TIMED_OUT, false, false)
	putMany("botdead", apipb.TaskState_BOT_DIED, false, false)
	putMany("canceled", apipb.TaskState_CANCELED, false, false)
	putMany("killed", apipb.TaskState_KILLED, false, false)
	putMany("noresource", apipb.TaskState_NO_RESOURCE, false, false)
	putMany("clienterror", apipb.TaskState_CLIENT_ERROR, false, false)

	return state
}

func TestServerInterceptor(t *testing.T) {
	t.Parallel()

	Convey("With config in datastore", t, func() {
		ctx := memory.Use(context.Background())
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: identity.AnonymousIdentity,
		})
		cfg := MockConfigs(ctx, MockedConfigs{})

		interceptor := ServerInterceptor(cfg, []*grpc.ServiceDesc{
			&apipb.Swarming_ServiceDesc,
		})

		Convey("Sets up state", func() {
			var state *RequestState
			err := interceptor(ctx, "/swarming.v2.Swarming/GetPermissions", func(ctx context.Context) error {
				state = State(ctx)
				return nil
			})
			So(err, ShouldBeNil)
			So(state, ShouldNotBeNil)
			So(state.Config, ShouldNotBeNil)
			So(state.ACL, ShouldNotBeNil)
		})

		Convey("Skips unrelated APIs", func() {
			var called bool
			err := interceptor(ctx, "/another.Service/GetPermissions", func(ctx context.Context) error {
				called = true
				defer func() { So(recover(), ShouldNotBeNil) }()
				State(ctx) // panics
				return nil
			})
			So(err, ShouldBeNil)
			So(called, ShouldBeTrue)
		})
	})
}
