// Copyright 2017 The LUCI Authors.
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
	"context"
	"errors"
	"fmt"
	"html"
	"html/template"
	"strconv"
	"strings"
	"sync"

	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/monitor"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/portal"
	"go.chromium.org/luci/server/settings"
)

// prodXEndpoint is endpoint to send metrics to.
//
// Hardcoded for now...
const prodXEndpoint = "prodxmon-pa.googleapis.com:443"

// settingsKey is key for tsmon settings (described by Settings struct)
// in the settings store. See go.chromium.org/luci/server/settings.
const settingsKey = "tsmon"

// Settings contain global tsmon settings for the application.
//
// They are usually stored in settings store.
type Settings struct {
	// Enabled is false to completely shutoff the monitoring.
	//
	// Default is false.
	Enabled portal.YesOrNo `json:"enabled"`

	// ProdXAccount is a service account to use to send metrics to ProdX endpoint.
	//
	// If not set, metrics will be logged to local GAE log. Default is "".
	ProdXAccount string `json:"prodx_account"`

	// FlushIntervalSec defines how often to flush metrics to the pubsub topic.
	//
	// Default is 60 sec.
	FlushIntervalSec int `json:"flush_interval_sec"`

	// FlushTimeoutSec defines how long to wait for the metrics to flush before
	// giving up.
	//
	// Default is 5 sec.
	FlushTimeoutSec int `json:"flush_timeout_sec"`

	// ReportRuntimeStats is true to enable reporting of Go RT stats on flush.
	//
	// Default is false.
	ReportRuntimeStats portal.YesOrNo `json:"report_runtime_stats"`
}

// Prefilled portion of settings.
var defaultSettings = Settings{
	FlushIntervalSec: 60,
	FlushTimeoutSec:  5,
}

// fetchCachedSettings fetches Settings from the settings store or panics.
//
// Uses in-process global cache to avoid hitting datastore often. The cache
// expiration time is 1 min (see gaesettings.expirationTime), meaning
// the instance will refetch settings once a minute (blocking only one unlucky
// request to do so).
//
// Panics only if there's no cached value (i.e. it is the first call to this
// function in this process ever) and datastore operation fails. It is usually
// very unlikely.
func fetchCachedSettings(c context.Context) Settings {
	s := Settings{}
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

type settingsPage struct {
	portal.BasePage

	m        sync.Mutex
	readOnly *Settings
	banner   string // displayed on top if readOnly != nil
}

// Make some aspects of the UI configurable by external packages.
var PortalPage interface {
	// SetReadOnlySettings switches the portal page to always display the given
	// settings instead of attempting to fetch them from the settings store.
	SetReadOnlySettings(s *Settings, banner string)

	// readOnlySettings returns settings set with SetReadOnlySettings or nil.
	readOnlySettings() *Settings
} = &settingsPage{}

func (p *settingsPage) SetReadOnlySettings(s *Settings, banner string) {
	p.m.Lock()
	defer p.m.Unlock()
	p.readOnly = s
	p.banner = banner
}

// readOnlySettings returns settings set with SetReadOnlySettings or nil.
func (p *settingsPage) readOnlySettings() *Settings {
	p.m.Lock()
	defer p.m.Unlock()
	return p.readOnly
}

func (p *settingsPage) Title(c context.Context) (string, error) {
	return "Time series monitoring", nil
}

func (p *settingsPage) Overview(c context.Context) (template.HTML, error) {
	p.m.Lock()
	defer p.m.Unlock()

	buf := strings.Builder{}
	buf.WriteString("<p>")
	buf.WriteString("This page displays settings of the time series metrics collection library (aka tsmon).")
	if p.readOnly != nil {
		buf.WriteString(" Note that this page is <b>read only</b>. ")
		buf.WriteString(template.HTMLEscapeString(p.banner))
	}
	buf.WriteString("</p>")

	return template.HTML(buf.String()), nil
}

func (p *settingsPage) Fields(c context.Context) ([]portal.Field, error) {
	serviceAcc := "<unknown>"
	if signer := auth.GetSigner(c); signer != nil {
		info, err := signer.ServiceInfo(c)
		if err != nil {
			return nil, err
		}
		serviceAcc = info.ServiceAccountName
	}

	p.m.Lock()
	ro := p.readOnly != nil
	p.m.Unlock()

	return []portal.Field{
		portal.YesOrNoField(portal.Field{
			ID:       "Enabled",
			Title:    "Enabled",
			ReadOnly: ro,
			Help: `If not enabled, all metrics manipulations are ignored and the ` +
				`monitoring has zero runtime overhead. If enabled, will keep track of metrics ` +
				`values in memory and will periodically flush them to tsmon backends (if the flush method ` +
				`is configured, see below) or GAE log (if not configured). Note that enabling ` +
				`this field requires an active housekeeping cron task to be installed. See ` +
				`<a href="https://godoc.org/go.chromium.org/luci/appengine/tsmon">the tsmon doc</a> for more information.`,
		}),
		{
			ID:       "ProdXAccount",
			Title:    "ProdX Service Account",
			Type:     portal.FieldText,
			ReadOnly: ro,
			Help: template.HTML(fmt.Sprintf(
				`Name of a properly configured service account inside a ProdX-enabled `+
					`Cloud Project to use for sending metrics. "Google Identity and Access `+
					`Management (IAM) API" must be enabled for the GAE app, and app's `+
					`account (<b>%s</b>) must have <i>Service Account Token Creator</i> role `+
					`for the specified ProdX account. This works only for Google projects.`,
				html.EscapeString(serviceAcc))),
		},
		{
			ID:       "FlushIntervalSec",
			Title:    "Flush interval, sec",
			Type:     portal.FieldText,
			ReadOnly: ro,
			Validator: func(v string) error {
				if i, err := strconv.Atoi(v); err != nil || i < 10 {
					return errors.New("expecting an integer larger than 9")
				}
				return nil
			},
			Help: "How often to flush metrics, in seconds. The default value (60 sec) " +
				"is highly recommended. Change it only if you know what you are doing.",
		},
		{
			ID:       "FlushTimeoutSec",
			Title:    "Flush timeout, sec",
			Type:     portal.FieldText,
			ReadOnly: ro,
			Validator: func(v string) error {
				if i, err := strconv.Atoi(v); err != nil || i < 0 {
					return errors.New("expecting a non-negative integer")
				}
				return nil
			},
			Help: "How long to wait for the metrics to flush before giving up, in seconds. " +
				"Change it only if you know what you are doing.",
		},
		portal.YesOrNoField(portal.Field{
			ID:       "ReportRuntimeStats",
			Title:    "Report runtime stats",
			ReadOnly: ro,
			Help: "If enabled, Go runtime state (e.g. memory allocator statistics) " +
				"will be collected at each flush and sent to the monitoring as a bunch " +
				"of go/* metrics.",
		}),
	}, nil
}

func (p *settingsPage) ReadSettings(c context.Context) (map[string]string, error) {
	p.m.Lock()
	defer p.m.Unlock()

	s := Settings{}
	if p.readOnly == nil {
		switch err := settings.GetUncached(c, settingsKey, &s); {
		case err == settings.ErrNoSettings:
			s = defaultSettings
		case err != nil:
			return nil, err
		}
	} else {
		s = *p.readOnly
	}

	return map[string]string{
		"Enabled":            s.Enabled.String(),
		"ProdXAccount":       s.ProdXAccount,
		"FlushIntervalSec":   strconv.Itoa(s.FlushIntervalSec),
		"FlushTimeoutSec":    strconv.Itoa(s.FlushTimeoutSec),
		"ReportRuntimeStats": s.ReportRuntimeStats.String(),
	}, nil
}

func (p *settingsPage) WriteSettings(c context.Context, values map[string]string) error {
	p.m.Lock()
	ro := p.readOnly != nil
	p.m.Unlock()
	if ro {
		return fmt.Errorf("Can't modify read-only settings")
	}

	modified := Settings{}
	modified.ProdXAccount = values["ProdXAccount"]
	if err := modified.Enabled.Set(values["Enabled"]); err != nil {
		return err
	}
	var err error
	if modified.FlushIntervalSec, err = strconv.Atoi(values["FlushIntervalSec"]); err != nil {
		return err
	}
	if modified.FlushTimeoutSec, err = strconv.Atoi(values["FlushTimeoutSec"]); err != nil {
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

	return settings.SetIfChanged(c, settingsKey, &modified)
}

func (p *settingsPage) Actions(ctx context.Context) ([]portal.Action, error) {
	return []portal.Action{
		{
			ID:            "metrics",
			Title:         "Show buffered metrics",
			NoSideEffects: true,
			Callback: func(ctx context.Context) (string, template.HTML, error) {
				if state := tsmon.GetState(ctx); state != nil {
					cells := state.Store().GetAll(ctx)              // note: it is a mutable copy
					return "Metrics", formatCellsAsHTML(cells), nil // see dump.go
				}
				return "", "", fmt.Errorf("no tsmon state in the context")
			},
		},
	}, nil
}

// canActAsProdX attempts to grab ProdX scoped access token for the given
// account.
func canActAsProdX(c context.Context, account string) error {
	ts, err := auth.GetTokenSource(
		c, auth.AsActor,
		auth.WithServiceAccount(account),
		auth.WithScopes(monitor.ProdXMonScope))
	if err != nil {
		return err
	}
	_, err = ts.Token()
	return err
}

func init() {
	portal.RegisterPage(settingsKey, PortalPage.(*settingsPage))
}
