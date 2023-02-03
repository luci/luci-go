// Copyright 2019 The LUCI Authors.
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

package monitoring

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgcache"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"

	api "go.chromium.org/luci/cipd/api/config/v1"
)

var cachedCfg = cfgcache.Register(&cfgcache.Entry{
	Path: "monitoring.cfg",
	Type: (*api.ClientMonitoringWhitelist)(nil),
})

// ImportConfig is called from a cron to import monitoring.cfg into datastore.
func ImportConfig(ctx context.Context) error {
	_, err := cachedCfg.Update(ctx, nil)
	if errors.Unwrap(err) == config.ErrNoConfig {
		logging.Warningf(ctx, "No monitoring.cfg config file")
		return nil
	}
	return err
}

// monitoringConfig returns the *api.ClientMonitoringConfig which applies to the
// current auth.State, or nil if there isn't one.
func monitoringConfig(ctx context.Context) (*api.ClientMonitoringConfig, error) {
	cfg, err := cachedCfg.Get(ctx, nil)
	if err != nil {
		if errors.Contains(err, datastore.ErrNoSuchEntity) {
			return nil, nil
		}
		return nil, errors.Annotate(err, "failed to fetch client monitoring config").Tag(transient.Tag).Err()
	}
	for _, e := range cfg.(*api.ClientMonitoringWhitelist).ClientMonitoringConfig {
		switch ok, err := auth.IsAllowedIP(ctx, e.IpWhitelist); {
		case err != nil:
			return nil, err
		case ok:
			return e, nil
		}
	}
	return nil, nil
}
