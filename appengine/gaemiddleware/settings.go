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

package gaemiddleware

import (
	"context"
	"fmt"
	"strings"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/metric"
	mc "go.chromium.org/luci/gae/service/memcache"
	"go.chromium.org/luci/server/portal"
	"go.chromium.org/luci/server/settings"
)

// settingsKey is key for global GAE settings (described by gaeSettings struct)
// in the settings store. See go.chromium.org/luci/server/settings.
const settingsKey = "gae"

// gaeSettings contain global Appengine related tweaks. They are stored in app
// settings store (based on the datastore, see appengine/gaesettings module)
// under settingsKey key.
type gaeSettings struct {
	// LoggingLevel is logging level to set the default logger to.
	//
	// Log entries below this level will be completely ignored. They won't even
	// reach GAE logging service. Default is logging.Debug (all logs hit logging
	// service).
	LoggingLevel logging.Level `json:"logging_level"`

	// DisableDSCache is true to disable dscache (the memcache layer on top of
	// the datastore).
	DisableDSCache portal.YesOrNo `json:"disable_dscache"`

	// SimulateMemcacheOutage is true to make every memcache call fail.
	//
	// Useful to verify apps can survive a memcache outage.
	SimulateMemcacheOutage portal.YesOrNo `json:"simulate_memcache_outage"`

	// EncryptionKey is a "sm://<project>/<secret>" path to a AEAD encryption key
	// used to encrypt cookies and other sensitive things.
	EncryptionKey string `json:"encryption_key"`
}

// fetchCachedSettings fetches gaeSettings from the settings store or panics.
//
// Uses in-process global cache to avoid hitting datastore often. The cache
// expiration time is 1 min (see gaesettings.expirationTime), meaning
// the instance will refetch settings once a minute (blocking only one unlucky
// request to do so).
//
// Panics only if there's no cached value (i.e. it is the first call to this
// function in this process ever) and datastore operation fails. It is a good
// idea to implement /_ah/warmup to warm this up.
func fetchCachedSettings(ctx context.Context) gaeSettings {
	s := gaeSettings{}
	switch err := settings.Get(ctx, settingsKey, &s); {
	case err == nil:
		return s
	case err == settings.ErrNoSettings:
		// Defaults.
		return gaeSettings{LoggingLevel: logging.Debug}
	default:
		panic(fmt.Errorf("could not fetch GAE settings - %s", err))
	}
}

// dsCacheDisabled is a metric for reporting the value of DSCacheDisabled in
// gaeSettings.
var dsCacheDisabled = metric.NewBool(
	"appengine/settings/dscache_disabled",
	"Whether or not dscache is disabled in the admin portal.",
	nil,
)

// reportDSCacheDisabled reports the value of DSCacheDisabled in settings to
// tsmon.
func reportDSCacheDisabled(ctx context.Context) {
	dsCacheDisabled.Set(ctx, bool(fetchCachedSettings(ctx).DisableDSCache))
}

////////////////////////////////////////////////////////////////////////////////
// UI for GAE settings.

type settingsPage struct {
	portal.BasePage
}

func (settingsPage) Title(ctx context.Context) (string, error) {
	return "Appengine related settings", nil
}

func (settingsPage) Fields(ctx context.Context) ([]portal.Field, error) {
	return []portal.Field{
		{
			ID:    "LoggingLevel",
			Title: "Minimal logging level",
			Type:  portal.FieldChoice,
			ChoiceVariants: []string{
				"debug",
				"info",
				"warning",
				"error",
			},
			Validator: func(v string) error {
				var l logging.Level
				return l.Set(v)
			},
			Help: `Log entries below this level will be <b>completely</b> ignored.
They won't even reach GAE logging service.`,
		},
		portal.YesOrNoField(portal.Field{
			ID:    "DisableDSCache",
			Title: "Disable datastore cache",
			Help: `Usually caching is a good thing and it can be left enabled. You may
want to disable it if memcache is having issues that prevent entity writes to
succeed. See <a href="https://godoc.org/go.chromium.org/luci/gae/filter/dscache">
dscache documentation</a> for more information. Toggling this on and off has
consequences: <b>memcache is completely flushed</b>. Do not toy with this
setting.`,
		}),
		portal.YesOrNoField(portal.Field{
			ID:    "SimulateMemcacheOutage",
			Title: "Simulate memcache outage",
			Help: `<b>Intended for development only. Do not use in production
applications.</b> When Yes, all memcache calls will fail, as if the memcache
service is unavailable. This is useful to test how application behaves when a
real memcache outage happens.`,
		}),
		{
			ID:    "EncryptionKey",
			Title: "Encryption key URI",
			Type:  portal.FieldText,
			Validator: func(v string) error {
				switch {
				case v == "":
					return nil
				case !strings.HasPrefix(v, "sm://"):
					return fmt.Errorf("expecting an sm://... URI")
				case strings.Count(strings.TrimPrefix(v, "sm://"), "/") != 1:
					return fmt.Errorf("sm://... URI should have form sm://[project]/[secret]")
				}
				return nil
			},
			Help: `An <code>sm://[project]/[secret]</code> URI pointing to an existing
secret in Google Secret Manager to use as an encryption key for encrypting
cookies and other sensitive strings.`,
		},
	}, nil
}

func (settingsPage) ReadSettings(ctx context.Context) (map[string]string, error) {
	s := gaeSettings{}
	err := settings.GetUncached(ctx, settingsKey, &s)
	if err != nil && err != settings.ErrNoSettings {
		return nil, err
	}
	return map[string]string{
		"LoggingLevel":           s.LoggingLevel.String(),
		"DisableDSCache":         s.DisableDSCache.String(),
		"SimulateMemcacheOutage": s.SimulateMemcacheOutage.String(),
		"EncryptionKey":          s.EncryptionKey,
	}, nil
}

func (settingsPage) WriteSettings(ctx context.Context, values map[string]string) error {
	modified := gaeSettings{}
	if err := modified.LoggingLevel.Set(values["LoggingLevel"]); err != nil {
		return err
	}
	if err := modified.DisableDSCache.Set(values["DisableDSCache"]); err != nil {
		return err
	}
	if err := modified.SimulateMemcacheOutage.Set(values["SimulateMemcacheOutage"]); err != nil {
		return err
	}
	modified.EncryptionKey = values["EncryptionKey"]

	// When switching dscache back on, wipe memcache.
	existing := gaeSettings{}
	err := settings.GetUncached(ctx, settingsKey, &existing)
	if err != nil && err != settings.ErrNoSettings {
		return err
	}
	if existing.DisableDSCache && !modified.DisableDSCache {
		logging.Warningf(ctx, "DSCache was reenabled, flushing memcache")
		if err := mc.Flush(ctx); err != nil {
			return fmt.Errorf("failed to flush memcache after reenabling dscache - %s", err)
		}
	}

	return settings.SetIfChanged(ctx, settingsKey, &modified)
}

func init() {
	portal.RegisterPage(settingsKey, settingsPage{})
	tsmon.RegisterGlobalCallback(reportDSCacheDisabled, dsCacheDisabled)
}
