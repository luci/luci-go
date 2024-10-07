// Copyright 2016 The LUCI Authors.
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

package gaeconfig

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"google.golang.org/grpc/credentials"

	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/impl/erroring"
	"go.chromium.org/luci/config/vars"
)

// devCfgDir is a name of the directory with config files when running in
// local dev appserver model. See Use for details.
const devCfgDir = "devcfg"

// defaultLazyConfigClient is the lazyConfigClient that will be shared by
// all requests.
var defaultLazyConfigClient = newLazyConfigClient(func(ctx context.Context) config.Interface {
	return newClientFromSettings(ctx, mustFetchCachedSettings(ctx))
})

// Use installs the default luci-config client.
//
// The client is configured to use luci-config URL specified in the settings,
// using GAE app service account for authentication.
//
// If running in prod, and the settings don't specify luci-config URL, produces
// an implementation that returns a "not configured" error from all methods.
//
// If running on devserver, and the settings don't specify luci-config URL,
// returns a filesystem-based implementation that reads configs from a directory
// (or a symlink) named 'devcfg' located in the GAE module directory (where
// app.yaml is) or its immediate parent directory.
//
// If such directory can not be located, produces an implementation of that
// returns errors from all methods.
//
// Panics if it can't load the settings (should not happen since they are in
// the local memory cache usually).
func Use(ctx context.Context) context.Context {
	return cfgclient.Use(ctx, defaultLazyConfigClient)
}

// devServerConfigsDir finds a directory with configs to use on the dev server.
func devServerConfigsDir() (string, error) {
	pwd := os.Getenv("PWD") // os.Getwd works funny with symlinks, use PWD
	candidates := []string{
		filepath.Join(pwd, devCfgDir),
		filepath.Join(filepath.Dir(pwd), devCfgDir),
	}
	for _, dir := range candidates {
		if _, err := os.Stat(dir); err == nil {
			return dir, nil
		}
	}
	return "", fmt.Errorf("luci-config: could not find local configs in any of %s", candidates)
}

// newClientFromSettings instantiates a LUCI Config client based on settings.
func newClientFromSettings(c context.Context, s *Settings) config.Interface {
	var configsDir string
	if s.ConfigServiceHost == "" && info.IsDevAppServer(c) {
		var err error
		if configsDir, err = devServerConfigsDir(); err != nil {
			return erroring.New(err)
		}
	}

	var creds credentials.PerRPCCredentials
	if s.ConfigServiceHost != "" {
		var err error
		creds, err = auth.GetPerRPCCredentials(c,
			auth.AsSelf,
			auth.WithIDTokenAudience("https://"+s.ConfigServiceHost),
		)
		if err != nil {
			return erroring.New(fmt.Errorf("failed to get credentials to access %s: %s", s.ConfigServiceHost, err))
		}
	}

	client, err := cfgclient.New(c, cfgclient.Options{
		Vars:              &vars.Vars,
		ServiceHost:       s.ConfigServiceHost,
		ConfigsDir:        configsDir,
		PerRPCCredentials: creds,
		UserAgent:         info.AppID(c),
	})
	if err != nil {
		return erroring.New(err)
	}
	return client
}
