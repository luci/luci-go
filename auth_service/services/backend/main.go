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

import (
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/module"

	"go.chromium.org/luci/auth_service/impl"
	"go.chromium.org/luci/auth_service/impl/model"

	// Ensure registration of validation rules.
	_ "go.chromium.org/luci/auth_service/internal/configs/validation"
)

func main() {
	modules := []module.Module{
		cron.NewModuleFromFlags(),
	}

	// Parse flags from environment variables.
	dryRunCronConfig := model.ParseDryRunEnvVar(model.DryRunCronConfigEnvVar)
	dryRunCronRealms := model.ParseDryRunEnvVar(model.DryRunCronRealmsEnvVar)
	dryRunCronStaleAuth := model.ParseDryRunEnvVar(model.DryRunCronStaleAuthEnvVar)

	impl.Main(modules,
		func(srv *server.Server) error {
			// Register task queue handlers.
			model.RegisterChangeHandler()
			model.RegisterReplicationHandler()

			// Register cron task handlers.
			cron.RegisterHandler("refresh-replicated-authdb",
				model.ReplicatedAuthDBRefresher)
			cron.RegisterHandler("revoke-stale-authorization",
				model.StaleAuthorizationCronHandler(dryRunCronStaleAuth))
			cron.RegisterHandler("update-config",
				model.ServiceConfigCronHandler(dryRunCronConfig))
			cron.RegisterHandler("update-realms",
				model.RealmsConfigCronHandler(dryRunCronRealms))

			return nil
		})
}
