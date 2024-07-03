// Copyright 2023 The LUCI Authors.
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

package frontend

import (
	"text/template"

	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/milo/internal/config"
	configpb "go.chromium.org/luci/milo/proto/config"
	"go.chromium.org/luci/server/router"
)

// configsJSHandler serves /configs.js used by the browser-side.
func (s *HTTPService) configsJSHandler(c *router.Context) error {
	tmpl, err := template.ParseFiles("frontend/templates/configs.template.js")
	if err != nil {
		logging.Errorf(c.Request.Context(), "Failed to load configs.template.js: %s", err)
		return err
	}

	settings := config.GetSettings(c.Request.Context())
	// Reassign to make exposing props explicit.
	settings = &configpb.Settings{
		Swarming:       settings.Swarming,
		Buildbucket:    settings.Buildbucket,
		Resultdb:       settings.Resultdb,
		LuciAnalysis:   settings.LuciAnalysis,
		LuciBisection:  settings.LuciBisection,
		SheriffOMatic:  settings.SheriffOMatic,
		LuciTreeStatus: settings.LuciTreeStatus,
		LuciNotify:     settings.LuciNotify,
		AuthService:    settings.AuthService,
		CrRev:          settings.CrRev,
	}

	header := c.Writer.Header()
	header.Set("content-type", "application/javascript")

	// We don't need to cache the configs file because it is fetched and re-served
	// by the service worker.
	header.Set("cache-control", "no-cache")
	err = tmpl.Execute(c.Writer, map[string]any{
		"Version":      s.Server.Options.ImageVersion(),
		"SettingsJSON": protojson.Format(settings),
	})

	if err != nil {
		logging.Errorf(c.Request.Context(), "Failed to execute configs.template.js: %s", err)
		return err
	}

	return nil
}
