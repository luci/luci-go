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
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgcache"

	listenerpb "go.chromium.org/luci/cv/settings/listener"
)

var (
	cachedListenerCfg = cfgcache.Register(&cfgcache.Entry{
		Path: "listener-settings.cfg",
		Type: (*listenerpb.Settings)(nil),
	})
)

// ImportConfig is called from a cron to import and cache all the configs.
func ImportConfig(ctx context.Context) error {
	_, err := cachedListenerCfg.Update(ctx, nil)
	return err
}

// GetListenerConfig loads cached Listener config.
//
// If meta is not nil, it will be updated with the meta of the loaded config.
func GetListenerConfig(ctx context.Context, meta *config.Meta) (*listenerpb.Settings, error) {
	switch v, err := cachedListenerCfg.Get(ctx, meta); {
	case err != nil:
		return nil, err
	default:
		return v.(*listenerpb.Settings), nil
	}
}

// SetTestListenerConfig is used in tests only.
func SetTestListenerConfig(ctx context.Context, ls *listenerpb.Settings, m *config.Meta) error {
	return cachedListenerCfg.Set(ctx, ls, m)
}

// MakeListenerProjectChecker returns a function that checks if a given project
// is enabled in the Listener config.
func MakeListenerProjectChecker(ls *listenerpb.Settings) (isEnabled func(string) bool, err error) {
	res, err := anchorRegexps(ls.GetDisabledProjectRegexps())
	if err != nil {
		// Must be a bug in the validator.
		return nil, errors.Fmt("invalid disabled_project_regexps: %w", err)
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

// IsProjectEnabledInListener returns true if a given project is enabled
// in the cached Listener config.
func IsProjectEnabledInListener(ctx context.Context, project string) (bool, error) {
	cfg, err := GetListenerConfig(ctx, nil)
	if err != nil {
		return false, err
	}
	chk, err := MakeListenerProjectChecker(cfg)
	if err != nil {
		return false, err
	}
	return chk(project), nil
}
