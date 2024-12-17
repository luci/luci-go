// Copyright 2024 The LUCI Authors.
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

// Package cfgtest allows to mock Swarming configs for tests.
package cfgtest

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmem "go.chromium.org/luci/config/impl/memory"

	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/cfg"
)

const (
	// MockedSwarmingServer is URL of the Swarming server used in tests.
	MockedSwarmingServer = "https://mocked.swarming.example.com"
	// MockedCIPDServer is the CIPD server URL used in tests.
	MockedCIPDServer = "https://mocked.cipd.example.com"
	// MockedBotPackage is bot CIPD package used in tests.
	MockedBotPackage = "mocked/bot/package"
	// MockedAdminGroup is the admin group in the default mocked config.
	MockedAdminGroup = "tests-admin-group"
)

// MockedConfigs is a bundle of configs to use in tests.
type MockedConfigs struct {
	Settings *configpb.SettingsCfg
	Pools    *configpb.PoolsCfg
	Bots     *configpb.BotsCfg
	Scripts  map[string]string
	CIPD     MockedCIPD
}

// NewMockedConfigs returns a default empty MockedConfigs.
func NewMockedConfigs() *MockedConfigs {
	return &MockedConfigs{
		Settings: &configpb.SettingsCfg{
			Auth: &configpb.AuthSettings{
				AdminsGroup: MockedAdminGroup,
			},
		},
		Pools: &configpb.PoolsCfg{
			DefaultExternalServices: &configpb.ExternalServices{
				Cipd: &configpb.ExternalServices_CIPD{
					Server: MockedCIPDServer,
					ClientPackage: &configpb.CipdPackage{
						PackageName: "client/pkg",
						Version:     "latest",
					},
				},
			},
		},
		Bots: &configpb.BotsCfg{
			TrustedDimensions: []string{"pool"},
		},
		Scripts: map[string]string{},
	}
}

// MockPool adds a new pool to the mocked config.
func (cfg *MockedConfigs) MockPool(name, realm string) *configpb.Pool {
	pb := &configpb.Pool{
		Name:  []string{name},
		Realm: realm,
	}
	cfg.Pools.Pool = append(cfg.Pools.Pool, pb)
	return pb
}

// MockBot adds a bot in some pool.
func (cfg *MockedConfigs) MockBot(botID, pool string) *configpb.BotGroup {
	pb := &configpb.BotGroup{
		BotId:      []string{botID},
		Dimensions: []string{"pool:" + pool},
		Auth: []*configpb.BotAuth{
			{
				RequireLuciMachineToken: true,
			},
		},
	}
	cfg.Bots.BotGroup = append(cfg.Bots.BotGroup, pb)
	return pb
}

// MockBotPackage mocks existence of a bot package in the CIPD and configs.
func (cfg *MockedConfigs) MockBotPackage(cipdVersion string, files map[string]string) {
	cfg.CIPD.MockPackage(MockedBotPackage, cipdVersion, files)
	pkg := &configpb.BotDeployment_BotPackage{
		Server:  MockedCIPDServer,
		Pkg:     MockedBotPackage,
		Version: cipdVersion,
	}
	cfg.Settings.BotDeployment = &configpb.BotDeployment{
		Stable:        pkg,
		Canary:        pkg,
		CanaryPercent: 20,
	}
}

// MockedCIPD mocks state in CIPD.
//
// Implements cfg.CIPD.
type MockedCIPD struct {
	pkgs map[string]pkg.Instance
}

// MockPackage puts a mocked package into the CIPD storage.
func (c *MockedCIPD) MockPackage(cipdpkg, version string, files map[string]string) {
	pkgFiles := make([]fs.File, 0, len(files))
	for key, val := range files {
		pkgFiles = append(pkgFiles, fs.NewTestFile(key, val, fs.TestFileOpts{}))
	}
	if c.pkgs == nil {
		c.pkgs = make(map[string]pkg.Instance, 1)
	}
	c.pkgs[fmt.Sprintf("%s:%s", cipdpkg, version)] = fakeInstance{files: pkgFiles}
}

// ResolveVersion resolves a version label into a CIPD instance ID.
func (c *MockedCIPD) ResolveVersion(ctx context.Context, server, cipdpkg, version string) (string, error) {
	if server != MockedCIPDServer {
		return "", status.Errorf(codes.NotFound, "unexpected CIPD server %s", server)
	}
	key := fmt.Sprintf("%s:%s", cipdpkg, version)
	if _, ok := c.pkgs[key]; ok {
		return "iid-" + key, nil
	}
	return "", status.Errorf(codes.NotFound, "no such package: %s %s", cipdpkg, version)
}

// FetchInstance fetches contents of a package given via its instance ID.
func (c *MockedCIPD) FetchInstance(ctx context.Context, server, cipdpkg, iid string) (pkg.Instance, error) {
	if server != MockedCIPDServer {
		return nil, status.Errorf(codes.NotFound, "unexpected CIPD server %s", server)
	}
	if key, ok := strings.CutPrefix(iid, "iid-"); ok {
		if pkg := c.pkgs[key]; pkg != nil {
			return pkg, nil
		}
	}
	return nil, status.Errorf(codes.NotFound, "no such package: %s %s", cipdpkg, iid)
}

// fakeInstance implements subset of pkg.Instance used by MockedCIPD.
type fakeInstance struct {
	pkg.Instance // embedded nil, to panic if any other method is called
	files        []fs.File
}

func (f fakeInstance) Files() []fs.File                              { return f.files }
func (f fakeInstance) Close(ctx context.Context, corrupt bool) error { return nil }

// MockConfigs puts configs into the datastore and loads them into a Provider.
//
// Configs must be valid, panics if they are not.
func MockConfigs(ctx context.Context, configs *MockedConfigs) *cfg.Provider {
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
	if len(configs.Pools.GetPool()) > 0 && configs.Pools.GetDefaultExternalServices() == nil {
		configs.Pools.DefaultExternalServices = &configpb.ExternalServices{
			Cipd: &configpb.ExternalServices_CIPD{
				Server: MockedCIPDServer,
				ClientPackage: &configpb.CipdPackage{
					PackageName: "client/pkg",
					Version:     "latest",
				},
			},
		}
	}
	putPb("pools.cfg", configs.Pools)
	putPb("bots.cfg", configs.Bots)
	for path, body := range configs.Scripts {
		files[path] = body
	}

	// Put new configs into the datastore.
	mockedCfg := cfgclient.Use(ctx, cfgmem.New(map[config.Set]cfgmem.Files{
		"services/${appid}": files,
	}))
	err := cfg.UpdateConfigs(mockedCfg, &cfg.EmbeddedBotSettings{
		ServerURL: MockedSwarmingServer,
	}, &configs.CIPD)
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
