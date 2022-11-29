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

// Package srvcfg provides service-wide configs.
package srvcfg

import (
	"context"
	"regexp"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgcache"

	migrationpb "go.chromium.org/luci/cv/api/migration"
	listenerpb "go.chromium.org/luci/cv/settings/listener"
)

var (
	cachedMigrationCfg = cfgcache.Register(&cfgcache.Entry{
		Path:      "migration-settings.cfg",
		ConfigSet: "services/commit-queue",
		Type:      (*migrationpb.Settings)(nil),
	})
	cachedListenerCfg = cfgcache.Register(&cfgcache.Entry{
		Path: "listener-settings.cfg",
		Type: (*listenerpb.Settings)(nil),
	})
)

// ImportConfig is called from a cron to import and cache all the configs.
func ImportConfig(ctx context.Context) error {
	return parallel.FanOutIn(func(workCh chan<- func() error) {
		workCh <- func() error {
			_, err := cachedMigrationCfg.Update(ctx, nil)
			return err
		}
		workCh <- func() error {
			_, err := cachedListenerCfg.Update(ctx, nil)
			return err
		}
	})
}

// GetMigrationConfig loads typically cached migration config.
func GetMigrationConfig(ctx context.Context) (*migrationpb.Settings, error) {
	switch v, err := cachedMigrationCfg.Get(ctx, &config.Meta{}); {
	case err != nil:
		return nil, err
	default:
		return v.(*migrationpb.Settings), nil
	}
}

// SetTestMigrationConfig is used in tests only.
func SetTestMigrationConfig(ctx context.Context, m *migrationpb.Settings) error {
	return cachedMigrationCfg.Set(ctx, m, &config.Meta{})
}

// GetListenerConfig loads cached Listener config.
func GetListenerConfig(ctx context.Context) (*listenerpb.Settings, error) {
	switch v, err := cachedListenerCfg.Get(ctx, &config.Meta{}); {
	case err != nil:
		return nil, err
	default:
		return v.(*listenerpb.Settings), nil
	}
}

// SetTestListenerConfig is used in tests only.
func SetTestListenerConfig(ctx context.Context, ls *listenerpb.Settings) error {
	return cachedListenerCfg.Set(ctx, ls, &config.Meta{})
}

// MakeListenerProjectChecker returns a function that checks if a given project is
// enabled in the listener settings.
func MakeListenerProjectChecker(ls *listenerpb.Settings) (isEnabled func(string) bool, err error) {
	res, err := anchorRegexps(ls.GetDisabledProjectRegexps())
	if err != nil {
		// Must be a bug in the validator.
		return nil, errors.Annotate(err, "invalid disabled_project_regexps").Err()
	}
	return func(prj string) bool {
		for _, re := range res {
			if re.Match([]byte(prj)) {
				return false
			}
		}
		return true
	}, nil
}

// anchorRegexps converts partial match regexs to full matches.
//
// Returns compiled regexps of them.
func anchorRegexps(partials []string) ([]*regexp.Regexp, error) {
	var me errors.MultiError
	var res []*regexp.Regexp
	for _, p := range partials {
		re, err := regexp.Compile("^" + p + "$")
		me.MaybeAdd(err)
		res = append(res, re)
	}
	if me.AsError() == nil {
		return res, nil
	}
	return nil, me
}
