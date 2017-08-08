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

package tsmon

import (
	"errors"
	"fmt"
	"html/template"
	"strconv"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/common/tsmon/monitor"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/settings"
)

// settingsKey is key for tsmon settings (described by tsmonSettings struct)
// in the settings store. See go.chromium.org/luci/server/settings.
const settingsKey = "tsmon"

// tsmonSettings contain global tsmon settings for the GAE app.
//
// They are stored in app settings store (based on the datastore, see
// appengine/gaesettings module) under settingsKey key.
type tsmonSettings struct {
	// Enabled is false to completely shutoff the monitoring.
	//
	// Default is false.
	Enabled settings.YesOrNo `json:"enabled"`

	// ProdXAccount is a service account to use to send metrics to ProdX endpoint.
	//
	// If not set, metrics will be logged to local GAE log. Default is "".
	ProdXAccount string `json:"prodx_account"`

	// FlushIntervalSec defines how often to flush metrics to the pubsub topic.
	//
	// Default is 60 sec.
	FlushIntervalSec int `json:"flush_interval_sec"`

	// ReportRuntimeStats is true to enable reporting of Go RT stats on flush.
	//
	// Default is false.
	ReportRuntimeStats settings.YesOrNo `json:"report_runtime_stats"`
}

// Prefilled portion of settings.
var defaultSettings = tsmonSettings{
	FlushIntervalSec: 60,
}

// fetchCachedSettings fetches tsmonSettings from the settings store or panics.
//
// Uses in-process global cache to avoid hitting datastore often. The cache
// expiration time is 1 min (see gaesettings.expirationTime), meaning
// the instance will refetch settings once a minute (blocking only one unlucky
// request to do so).
//
// Panics only if there's no cached value (i.e. it is the first call to this
// function in this process ever) and datastore operation fails. It is usually
// very unlikely.
func fetchCachedSettings(c context.Context) tsmonSettings {
	s := tsmonSettings{}
	switch err := settings.Get(c, settingsKey, &s); {
	case err == nil:
		return s
	case err == settings.ErrNoSettings:
		return defaultSettings
	default:
		panic(fmt.Errorf("could not fetch tsmon settings - %s", err))
	}
}

////////////////////////////////////////////////////////////////////////////////
// UI for Tsmon settings.

type settingsUIPage struct {
	settings.BaseUIPage
}

func (settingsUIPage) Title(c context.Context) (string, error) {
	return "Time series monitoring settings", nil
}

func (settingsUIPage) Fields(c context.Context) ([]settings.UIField, error) {
	serviceAcc, err := info.ServiceAccount(c)
	if err != nil {
		return nil, err
	}
	return []settings.UIField{
		settings.YesOrNoField(settings.UIField{
			ID:    "Enabled",
			Title: "Enabled",
			Help: `If not enabled, all metrics manipulations are ignored and the ` +
				`monitoring has zero runtime overhead. If enabled, will keep track of metrics ` +
				`values in memory and will periodically flush them to tsmon backends (if the flush method ` +
				`is configured, see below) or GAE log (if not configured). Note that enabling ` +
				`this field requires an active housekeeping cron task to be installed. See ` +
				`<a href="https://godoc.org/go.chromium.org/luci/appengine/tsmon">the tsmon doc</a> for more information.`,
		}),
		{
			ID:    "ProdXAccount",
			Title: "ProdX Service Account",
			Type:  settings.UIFieldText,
			Help: template.HTML(fmt.Sprintf(
				`Name of a properly configured service account inside a ProdX-enabled `+
					`Cloud Project to use for sending metrics. "Google Identity and Access `+
					`Management (IAM) API" must be enabled for the GAE app, and app's `+
					`account (<b>%s</b>) must have <i>Service Account Actor</i> role `+
					`for the specified ProdX account. This works only for Google projects.`, serviceAcc)),
		},
		{
			ID:    "FlushIntervalSec",
			Title: "Flush interval, sec",
			Type:  settings.UIFieldText,
			Validator: func(v string) error {
				if i, err := strconv.Atoi(v); err != nil || i < 10 {
					return errors.New("expecting an integer larger than 9")
				}
				return nil
			},
			Help: "How often to flush metrics, in seconds. The default value (60 sec) " +
				"is highly recommended. Change it only if you know what you are doing.",
		},
		settings.YesOrNoField(settings.UIField{
			ID:    "ReportRuntimeStats",
			Title: "Report runtime stats",
			Help: "If enabled, Go runtime state (e.g. memory allocator statistics) " +
				"will be collected at each flush and sent to the monitoring as a bunch " +
				"of go/* metrics.",
		}),
	}, nil
}

func (settingsUIPage) ReadSettings(c context.Context) (map[string]string, error) {
	s := tsmonSettings{}
	switch err := settings.GetUncached(c, settingsKey, &s); {
	case err == settings.ErrNoSettings:
		s = defaultSettings
	case err != nil:
		return nil, err
	}
	return map[string]string{
		"Enabled":            s.Enabled.String(),
		"ProdXAccount":       s.ProdXAccount,
		"FlushIntervalSec":   strconv.Itoa(s.FlushIntervalSec),
		"ReportRuntimeStats": s.ReportRuntimeStats.String(),
	}, nil
}

func (settingsUIPage) WriteSettings(c context.Context, values map[string]string, who, why string) error {
	modified := tsmonSettings{}
	modified.ProdXAccount = values["ProdXAccount"]
	if err := modified.Enabled.Set(values["Enabled"]); err != nil {
		return err
	}
	var err error
	if modified.FlushIntervalSec, err = strconv.Atoi(values["FlushIntervalSec"]); err != nil {
		return err
	}
	if err := modified.ReportRuntimeStats.Set(values["ReportRuntimeStats"]); err != nil {
		return err
	}

	// Verify ProdXAccount is usable before saving the settings.
	if modified.ProdXAccount != "" {
		if err := canActAsProdX(c, modified.ProdXAccount); err != nil {
			return fmt.Errorf("Can't use given ProdX Service Account %q, check its configuration - %s", modified.ProdXAccount, err)
		}
	}

	return settings.SetIfChanged(c, settingsKey, &modified, who, why)
}

// canActAsProdX attempts to grab ProdX scoped access token for the given
// account.
func canActAsProdX(c context.Context, account string) error {
	ts, err := auth.GetTokenSource(
		c, auth.AsActor,
		auth.WithServiceAccount(account),
		auth.WithScopes(monitor.ProdxmonScopes...))
	if err != nil {
		return err
	}
	_, err = ts.Token()
	return err
}

func init() {
	settings.RegisterUIPage(settingsKey, settingsUIPage{})
}
