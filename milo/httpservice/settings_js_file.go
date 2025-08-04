// Copyright 2024 The LUCI Authors.
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

package httpservice

import (
	"os"
	"text/template"

	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/milo/internal/config"
	"go.chromium.org/luci/milo/internal/hosts"
	configpb "go.chromium.org/luci/milo/proto/config"
)

var settingsJsTemplateStr = `
	self.SETTINGS = Object.freeze({{.SettingsJSON}});
`

var settingsJsTemplate = template.Must(template.New("settings.js").Parse(settingsJsTemplateStr))

func constructMiloConfigs(c *router.Context, settings *configpb.Settings) (*configpb.Settings_Milo, error) {
	// Fetched from command line arguments to the server binary instead of
	// config. This facilitates testing locally developed UI with locally
	// developed backend uploaded via gae.py.
	miloAPIHost, err := hosts.APIHost(c.Request.Context())
	if err != nil {
		logging.Errorf(c.Request.Context(), "Failed to load MILO Host: %s", err)
		return nil, err
	}

	errorReportingApiKey := ""
	// Previously, the Milo settings object was constructed dynamically and only
	// contained the Host field, which was fetched from command-line arguments.
	// Now that a new field is read from the config file (ErrorReportingApiKey),
	// a nil pointer exception can occur if the config is missing or empty.
	// This check prevents that.
	if settings.Milo != nil {
		errorReportingApiKey = settings.Milo.ErrorReportingApiKey
	}

	return &configpb.Settings_Milo{
		Host:                 miloAPIHost,
		Project:              os.Getenv("GOOGLE_CLOUD_PROJECT"),
		ErrorReportingApiKey: errorReportingApiKey,
	}, nil
}

// settingsJSHandler serves /settings.js used by the browser-side.
func (s *HTTPService) settingsJSHandler(c *router.Context) error {
	settings := config.GetSettings(c.Request.Context())
	miloConfigs, err := constructMiloConfigs(c, settings)
	if err != nil {
		return err
	}

	// Reassign to make exposing props explicit.
	settings = &configpb.Settings{
		Swarming:        settings.Swarming,
		Buildbucket:     settings.Buildbucket,
		Resultdb:        settings.Resultdb,
		LuciAnalysis:    settings.LuciAnalysis,
		LuciBisection:   settings.LuciBisection,
		SheriffOMatic:   settings.SheriffOMatic,
		LuciTreeStatus:  settings.LuciTreeStatus,
		LuciNotify:      settings.LuciNotify,
		AuthService:     settings.AuthService,
		CrRev:           settings.CrRev,
		Milo:            miloConfigs,
		LuciSourceIndex: settings.LuciSourceIndex,
		FleetConsole:    settings.FleetConsole,
		Ufs:             settings.Ufs,
		TestInvestigate: settings.TestInvestigate,
	}

	header := c.Writer.Header()
	header.Set("content-type", "text/javascript")

	// We don't need to cache the settings file because it is fetched and re-served
	// by the service worker.
	header.Set("cache-control", "no-cache")
	err = settingsJsTemplate.Execute(c.Writer, map[string]any{
		"SettingsJSON": protojson.Format(settings),
	})
	if err != nil {
		logging.Errorf(c.Request.Context(), "Failed to execute settings.js template: %s", err)
		return err
	}

	return nil
}
