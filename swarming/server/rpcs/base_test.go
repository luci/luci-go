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
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmem "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/cfg"

	. "github.com/smartystreets/goconvey/convey"
)

const (
	// DefaultFakeCaller is used in unit tests by default as the caller identity.
	DefaultFakeCaller identity.Identity = "user:test@example.com"
	// AdminFakeCaller is used in unit tests to make calls as an admin.
	AdminFakeCaller identity.Identity = "user:admin@example.com"
)

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
