// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package tsmon

import (
	"errors"
	"fmt"
	"html/template"
	"strconv"

	"golang.org/x/net/context"

	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/common/tsmon/monitor"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/settings"
)

// settingsKey is key for tsmon settings (described by tsmonSettings struct)
// in the settings store. See github.com/luci/luci-go/server/settings.
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

	// BackendKind defines how to flush metrics.
	//
	// TEMPORARY. It exists only until we fully remove PubSub flush mechanism
	// and switch to prodx one.
	//
	// Default is "prodx" for new installs, "pubsub" for converted ones.
	BackendKind string `json:"backend_kind"`

	// PubsubProject is Cloud Project that hosts metrics ingestion PubSub topic.
	//
	// DEPRECATED. Will be removed.
	//
	// If not set, metrics will be logged to local GAE log. Default is "".
	PubsubProject string `json:"pubsub_project"`

	// PubsubTopic is a topic inside PubsubProject to send metrics to.
	//
	// DEPRECATED. Will be removed.
	//
	// If not set, metrics will be logged to local GAE log. Default is "monacq".
	PubsubTopic string `json:"pubsub_topic"`

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

// configured is true if flush-related paramters of settings are filled in.
func (s *tsmonSettings) configured() bool {
	switch s.BackendKind {
	case "pubsub":
		return s.PubsubProject != "" && s.PubsubTopic != ""
	case "prodx":
		return s.ProdXAccount != ""
	default:
		return false
	}
}

// Prefilled portion of settings.
var defaultSettings = tsmonSettings{
	BackendKind:      "prodx", // for completely new installs
	PubsubProject:    "chrome-infra-mon-pubsub",
	PubsubTopic:      "monacq",
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
		// Settings exist in the datastore, but no method defined => old instance
		// that uses PubSub. New instances get 'prodx' there via defaultSettings.
		if s.BackendKind == "" {
			s.BackendKind = "pubsub"
		}
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
				`<a href="https://godoc.org/github.com/luci/luci-go/appengine/tsmon">the tsmon doc</a> for more information.`,
		}),
		{
			ID:    "BackendKind",
			Title: "Backend",
			Type:  settings.UIFieldChoice,
			ChoiceVariants: []string{
				"pubsub",
				"prodx",
			},
			Help: "What backend to use. Switch to <b>prodx</b> ASAP if still using PubSub.",
		},
		{
			ID:    "ProdXAccount",
			Title: "ProdX Service Account",
			Type:  settings.UIFieldText,
			Help: template.HTML(fmt.Sprintf(
				`Name of a properly configured service account inside a ProdX-enabled `+
					`project to use for sending metrics. The GAE app's account (<b>%s</b>) `+
					`must have <i>Service Account Actor</i> role for this service account `+
					`to be able to send metrics. This works only for Google projects.`, serviceAcc)),
		},
		{
			ID:    "PubsubProject",
			Title: "[DEPRECATED] PubSub project",
			Type:  settings.UIFieldText,
			Help:  "Pick <b>prodx</b> backend and configure ProdX Service Account instead.",
		},
		{
			ID:    "PubsubTopic",
			Title: "[DEPRECATED] PubSub topic",
			Type:  settings.UIFieldText,
			Help:  "Pick <b>prodx</b> backend and configure ProdX Service Account instead.",
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
	if s.BackendKind == "" {
		s.BackendKind = "pubsub" // existing non-updated installs
	}
	return map[string]string{
		"Enabled":            s.Enabled.String(),
		"BackendKind":        s.BackendKind,
		"ProdXAccount":       s.ProdXAccount,
		"PubsubProject":      s.PubsubProject,
		"PubsubTopic":        s.PubsubTopic,
		"FlushIntervalSec":   strconv.Itoa(s.FlushIntervalSec),
		"ReportRuntimeStats": s.ReportRuntimeStats.String(),
	}, nil
}

func (settingsUIPage) WriteSettings(c context.Context, values map[string]string, who, why string) error {
	modified := tsmonSettings{}
	modified.BackendKind = values["BackendKind"]
	modified.ProdXAccount = values["ProdXAccount"]
	modified.PubsubProject = values["PubsubProject"]
	modified.PubsubTopic = values["PubsubTopic"]
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
	if modified.BackendKind == "prodx" && modified.ProdXAccount != "" {
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
