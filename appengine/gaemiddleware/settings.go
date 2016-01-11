// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gaemiddleware

import (
	"fmt"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/settings"
)

// settingsKey is key for global GAE settings (described by gaeSettings struct)
// in the settings store. See github.com/luci/luci-go/server/settings.
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
func fetchCachedSettings(c context.Context) gaeSettings {
	s := gaeSettings{}
	switch err := settings.Get(c, settingsKey, &s); {
	case err == nil:
		return s
	case err == settings.ErrNoSettings:
		// Defaults.
		return gaeSettings{
			LoggingLevel: logging.Debug,
		}
	default:
		panic(fmt.Errorf("could not fetch GAE settings - %s", err))
	}
}

////////////////////////////////////////////////////////////////////////////////
// UI for GAE settings.

type settingsUIPage struct {
	settings.BaseUIPage
}

func (settingsUIPage) Title(c context.Context) (string, error) {
	return "Appengine related settings", nil
}

func (settingsUIPage) Fields(c context.Context) ([]settings.UIField, error) {
	return []settings.UIField{
		{
			ID:    "LoggingLevel",
			Title: "Minimal logging level",
			Type:  settings.UIFieldChoice,
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
	}, nil
}

func (settingsUIPage) ReadSettings(c context.Context) (map[string]string, error) {
	s := gaeSettings{}
	err := settings.GetUncached(c, settingsKey, &s)
	if err != nil && err != settings.ErrNoSettings {
		return nil, err
	}
	return map[string]string{
		"LoggingLevel": s.LoggingLevel.String(),
	}, nil
}

func (settingsUIPage) WriteSettings(c context.Context, values map[string]string, who, why string) error {
	modified := gaeSettings{}
	if err := modified.LoggingLevel.Set(values["LoggingLevel"]); err != nil {
		return err
	}

	// Skip update if not really changed.
	existing := gaeSettings{}
	err := settings.GetUncached(c, settingsKey, &existing)
	if err != nil && err != settings.ErrNoSettings {
		return err
	}
	if existing == modified {
		return nil
	}

	logging.Warningf(c, "GAE settings changed from %q to %q", existing, modified)
	return settings.Set(c, settingsKey, &modified, who, why)
}

func init() {
	settings.RegisterUIPage(settingsKey, settingsUIPage{})
}
