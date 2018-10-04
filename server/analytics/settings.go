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

package analytics

import (
	"context"
	"fmt"
	"html/template"
	"regexp"

	"go.chromium.org/luci/server/portal"
	"go.chromium.org/luci/server/settings"
)

// settingsKey is key for global GAE settings (described by analyticsSettings struct)
// in the settings store. See go.chromium.org/luci/server/settings.
const settingsKey = "analytics"

// analyticsSettings contain settings to enable Google Analytics.
type analyticsSettings struct {
	// AnalyticsID is a Google Analytics ID an admin can set to enable Analytics.
	// The app must support analytics for this to work.
	AnalyticsID string `json:"analytics_id"`
}

// fetchCachedSettings fetches analyticsSettings from the settings store or panics.
//
// Uses in-process global cache to avoid hitting datastore often. The cache
// expiration time is 1 min (see analyticsSettings.expirationTime), meaning
// the instance will refetch settings once a minute (blocking only one unlucky
// request to do so).
//
// Panics only if there's no cached value (i.e. it is the first call to this
// function in this process ever) and datastore operation fails. It is a good
// idea to implement /_ah/warmup to warm this up.
func fetchCachedSettings(c context.Context) analyticsSettings {
	s := analyticsSettings{}
	switch err := settings.Get(c, settingsKey, &s); {
	case err == nil:
		return s
	case err == settings.ErrNoSettings:
		// Defaults.
		return analyticsSettings{
			AnalyticsID: "",
		}
	default:
		panic(fmt.Errorf("could not fetch GAE settings - %s", err))
	}
}

var rAllowed = regexp.MustCompile("UA-\\d+-\\d+")

////////////////////////////////////////////////////////////////////////////////
// UI for GAE settings.

type settingsPage struct {
	portal.BasePage
}

func (settingsPage) Title(c context.Context) (string, error) {
	return "Google Analytics Related Settings", nil
}

func (settingsPage) Overview(c context.Context) (template.HTML, error) {
	return template.HTML(`<p>To generate a Google Analytics Tracking ID</p>
<ul>
<li> Sign in to <a href="https://www.google.com/analytics/web/#home/">your Analytics account.</a></li>
<li>Select the Admin tab.</li>
<li>Select an account from the drop-down menu in the <i>ACCOUNT</i> column.</li>
<li>Select a property from the drop-down menu in the <i>PROPERTY</i> column.</li>
<li>Under <i>PROPERTY</i>, click <b>Tracking Info > Tracking Code.</b></li>
</ul>`), nil
}

func (settingsPage) Fields(c context.Context) ([]portal.Field, error) {
	return []portal.Field{
		{
			ID:    "AnalyticsID",
			Title: "Google Analytics Tracking ID",
			Type:  portal.FieldText,
			Help: `Tracking ID used for Google Analytics. Filling this in enables
Google Analytics tracking across the app.`,
		},
	}, nil
}

func (settingsPage) ReadSettings(c context.Context) (map[string]string, error) {
	s := analyticsSettings{}
	err := settings.GetUncached(c, settingsKey, &s)
	if err != nil && err != settings.ErrNoSettings {
		return nil, err
	}
	return map[string]string{
		"AnalyticsID": s.AnalyticsID,
	}, nil
}

func (settingsPage) WriteSettings(c context.Context, values map[string]string, who, why string) error {
	modified := analyticsSettings{}
	id := values["AnalyticsID"]
	if id != "" {
		if !rAllowed.MatchString(id) {
			return fmt.Errorf("Analytics ID %s does not match format UA-\\d+-\\d+", id)
		}
		modified.AnalyticsID = id
	}

	return settings.SetIfChanged(c, settingsKey, &modified, who, why)
}

func init() {
	portal.RegisterPage(settingsKey, settingsPage{})
}
