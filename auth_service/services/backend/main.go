// Copyright 2021 The LUCI Authors.
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

// Package main is the main point of entry for the backend module.
//
// It handles task queue tasks and cron jobs.
package main

// Disable linting for imports, because of the dependency on config
// validation globals.
//nolint:all
import (
	"context"

	"go.chromium.org/luci/auth_service/impl"
	"go.chromium.org/luci/auth_service/impl/model"

	// Ensure registration of validation rules.
	// NOTE: this must go before anything that depends on validation globals,
	// e.g. cfgcache.Register in srvcfg files in allowlistcfg/ or oauthcfg/.
	"go.chromium.org/luci/auth_service/internal/configs/validation"

	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/allowlistcfg"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/oauthcfg"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/permissionscfg"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/securitycfg"

	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/module"
)

func main() {
	modules := []module.Module{
		cron.NewModuleFromFlags(),
	}

	impl.Main(modules, func(srv *server.Server) error {
		dryRun := true
		cron.RegisterHandler("update-config", func(ctx context.Context) error {
			historicalComment := "Updated from update-config cron"

			// ip_allowlist.cfg handling.
			if err := allowlistcfg.Update(ctx); err != nil {
				return err
			}
			cfg, err := allowlistcfg.Get(ctx)
			if err != nil {
				return err
			}
			subnets, err := validation.GetSubnets(cfg.IpAllowlists)
			if err != nil {
				return err
			}
			if err := model.UpdateAllowlistEntities(ctx, subnets, dryRun, historicalComment); err != nil {
				return err
			}

			// oauth.cfg handling.
			if err := oauthcfg.Update(ctx); err != nil {
				return err
			}
			oauthcfg, err := oauthcfg.Get(ctx)
			if err != nil {
				return err
			}

			// security.cfg handling.
			if err := securitycfg.Update(ctx); err != nil {
				return err
			}
			securitycfg, err := securitycfg.Get(ctx)
			if err != nil {
				return err
			}

			if err := model.UpdateAuthGlobalConfig(ctx, oauthcfg, securitycfg, dryRun, historicalComment); err != nil {
				return err
			}
			return nil
		})

		cron.RegisterHandler("update-realms", func(ctx context.Context) error {
			// permissions.cfg handling.
			if err := permissionscfg.Update(ctx); err != nil {
				return err
			}
			permsCfg, err := permissionscfg.Get(ctx)
			if err != nil {
				return err
			}
			if err := model.UpdateAuthRealmsGlobals(ctx, permsCfg, dryRun, "Updated from update-realms cron"); err != nil {
				return err
			}
			return nil
		})

		return nil
	})
}
