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

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmem "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/cfg"
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
func MockRequestState(ctx context.Context, state MockedRequestState) context.Context {
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
