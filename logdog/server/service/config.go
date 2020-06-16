// Copyright 2020 The LUCI Authors.
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

package service

import (
	"context"

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/appengine/backend/datastore"
	"go.chromium.org/luci/config/impl/filesystem"
	"go.chromium.org/luci/config/server/cfgclient/backend"
	"go.chromium.org/luci/config/server/cfgclient/backend/client"
	"go.chromium.org/luci/config/server/cfgclient/backend/erroring"
	"go.chromium.org/luci/config/server/cfgclient/backend/testconfig"
)

// setupCfgClient prepares server/cfgclient backend in the context.
func setupCfgClient(ctx context.Context, localConfigPath string) (context.Context, error) {
	var b backend.B
	if localConfigPath != "" {
		// If a localConfigPath was specified, use a mock configuration service
		// that loads from a local file.
		fs, err := filesystem.New(localConfigPath)
		if err != nil {
			return nil, err
		}
		b = &client.Backend{
			Provider: &testconfig.Provider{
				Base: fs,
			},
		}
	} else {
		// In production load configs from the datastore cache maintained by the
		// coordinator. Do **not** fallback to LUCI Config client. Configs must be
		// present in the datastore cache.
		b = (&datastore.Config{}).Backend(erroring.New(config.ErrNoConfig))
	}
	return backend.WithBackend(ctx, b), nil
}
