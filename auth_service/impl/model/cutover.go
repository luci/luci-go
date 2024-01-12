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

package model

import (
	"os"
)

// Names of enviroment variables which control component functionality
// while transitioning from Auth Service v1 (Python) to
// Auth Service v2 (Go).
//
// Each environment variable should be either "true" or "false".
const (
	DryRunAPIChangesEnvVar    = "DRY_RUN_API_CHANGES"
	DryRunCronConfigEnvVar    = "DRY_RUN_CRON_CONFIG"
	DryRunCronRealmsEnvVar    = "DRY_RUN_CRON_REALMS"
	DryRunTQChangelogEnvVar   = "DRY_RUN_TQ_CHANGELOG"
	DryRunTQReplicationEnvVar = "DRY_RUN_TQ_REPLICATION"
	EnableGroupImportsEnvVar  = "ENABLE_GROUP_IMPORTS"
)

// ParseDryRunEnvVar parses the dry run flag from the given environment
// variable, defaulting to true.
func ParseDryRunEnvVar(envVar string) bool {
	dryRun := true
	if os.Getenv(envVar) == "false" {
		dryRun = false
	}
	return dryRun
}

// ParseEnableEnvVar parses the enable flag from the given environment
// variable, defaulting to false.
func ParseEnableEnvVar(envVar string) bool {
	return os.Getenv(envVar) == "true"
}
