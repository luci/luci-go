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
	"fmt"
	"time"

	"go.chromium.org/luci/auth_service/impl"
	"go.chromium.org/luci/auth_service/impl/model"

	// Ensure registration of validation rules.
	// NOTE: this must go before anything that depends on validation globals,
	// e.g. cfgcache.Register in srvcfg files in allowlistcfg/ or oauthcfg/.
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/allowlistcfg"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/oauthcfg"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/permissionscfg"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/securitycfg"
	"go.chromium.org/luci/auth_service/internal/configs/validation"

	"go.chromium.org/luci/auth_service/internal/permissions"
	"go.chromium.org/luci/auth_service/internal/realmsinternals"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/module"
)

func main() {
	modules := []module.Module{
		cron.NewModuleFromFlags(),
	}

	// Parse flags from environment variables.
	dryRunCronConfig := model.ParseDryRunEnvVar(model.DryRunCronConfigEnvVar)
	dryRunCronRealms := model.ParseDryRunEnvVar(model.DryRunCronRealmsEnvVar)

	impl.Main(modules, func(srv *server.Server) error {
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
			if err := model.UpdateAllowlistEntities(ctx, subnets, dryRunCronConfig, historicalComment); err != nil {
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

			if err := model.UpdateAuthGlobalConfig(ctx, oauthcfg, securitycfg, dryRunCronConfig, historicalComment); err != nil {
				return err
			}
			return nil
		})

		cron.RegisterHandler("update-realms", func(ctx context.Context) error {
			historicalComment := "Updated from update-realms cron"

			// permissions.cfg handling.
			if err := permissionscfg.Update(ctx); err != nil {
				return err
			}
			permsCfg, permsMeta, err := permissionscfg.GetWithMetadata(ctx)
			if err != nil {
				return err
			}
			if err := model.UpdateAuthRealmsGlobals(ctx, permsCfg, dryRunCronRealms, historicalComment); err != nil {
				return err
			}

			// Make the PermissionsDB for realms expansion.
			permsDB := permissions.NewPermissionsDB(permsCfg, permsMeta)

			// realms.cfg handling.
			latestRealms, storedRealms, err := realmsinternals.GetConfigs(ctx)
			if err != nil {
				logging.Errorf(ctx, "aborting realms update - failed to fetch latest for all configs: %v", err)
				return err
			}
			jobs, err := realmsinternals.CheckConfigChanges(ctx, permsDB, latestRealms, storedRealms, dryRunCronRealms, historicalComment)
			if err != nil {
				return err
			}
			if !executeJobs(ctx, jobs, 2*time.Second) {
				return fmt.Errorf("not all jobs succeeded when refreshing realms")
			}

			return nil
		})

		// TODO: Remove comparison code once we have fully rolled out Auth
		// Service v2 (b/321019030).
		cron.RegisterHandler("auth-service-v2-validation", func(ctx context.Context) error {
			logging.Infof(ctx, "starting comparison of V2 entities for validation")
			return model.CompareV2Entities(ctx)
		})

		return nil
	})
}

// executeJobs executes the callbacks, sleeping the set amount of time
// between each. Note: all callbacks will be run, even if a previous job
// returned an error.
//
// Returns whether any job returned an error.
func executeJobs(ctx context.Context, jobs []func() error, sleepTime time.Duration) bool {
	success := true
	for i, job := range jobs {
		if i > 0 {
			time.Sleep(sleepTime)
		}
		if err := job(); err != nil {
			logging.Errorf(ctx, "job %d out of %d failed: %s", i+1, len(jobs), err)
			success = false
		}
	}
	return success
}
