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

package cfg

import (
	"context"
)

// Config is an immutable queryable representation of Swarming server configs.
//
// It is a snapshot of configs at some particular revision. Use an instance of
// Provider to get it.
type Config struct {
	// Revision is the config repo commit the config was loaded from.
	Revision string
	// ViewURL is an URL to the config repo commit.
	ViewURL string

	// TODO: add the actual config
}

// UpdateConfigs fetches the most recent server configs from LUCI Config and
// stores them in the local datastore if they appear to be valid.
//
// Called from a cron job once a minute.
func UpdateConfigs(ctx context.Context) error {
	// TODO: Fetch bots.cfg, revalidate via validateBotsCfg.
	// TODO: Fetch pools.cfg, revalidate via validatePoolsCfg.
	// TODO: Fetch settings.cfg, revalidate via validateSettingsCfg.
	return nil
}

// fetchFromDatastore fetches the config from the datastore.
//
// If there's no config in the datastore, returns some default empty config.
//
// If `cur` is not nil its (immutable) parts may be used to construct the
// new Config in case they didn't change.
func fetchFromDatastore(ctx context.Context, cur *Config) (*Config, error) {
	// TODO: Implement.
	return &Config{}, nil
}
