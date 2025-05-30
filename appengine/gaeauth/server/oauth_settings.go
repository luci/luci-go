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

package server

import (
	"context"
	"html/template"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/portal"
	"go.chromium.org/luci/server/settings"
)

const oauthSettingsKey = "oauth_config"

type oauthSettings struct {
	ClientID string `json:"client_id"`
}

// FetchFrontendClientID fetches the frontend client ID from the settings store.
func FetchFrontendClientID(ctx context.Context) (string, error) {
	cfg := &oauthSettings{}
	switch err := settings.Get(ctx, oauthSettingsKey, cfg); {
	case err == nil:
		return cfg.ClientID, nil
	case err == settings.ErrNoSettings:
		return "", nil
	default:
		return "", transient.Tag.Apply(errors.Fmt("failed to fetch OAuth settings: %w", err))
	}
}

////////////////////////////////////////////////////////////////////////////////
// UI for configuring frontend client ID.

type oauthSettingsPage struct {
	portal.BasePage
}

func (oauthSettingsPage) Title(ctx context.Context) (string, error) {
	return "OAuth 2.0 settings", nil
}

func (oauthSettingsPage) Overview(ctx context.Context) (template.HTML, error) {
	return `<p>This page allows to configure a Google OAuth 2.0 client ID to use
from the frontend (i.e. Javascript in the browser) that uses Google OAuth (or
OpenID Connect) web sign-in. If your service doesn't have a Javascript frontend,
or it doesn't use web sign-in, settings here have no effect.</p>`, nil
}

func (oauthSettingsPage) Fields(ctx context.Context) ([]portal.Field, error) {
	return []portal.Field{
		{
			ID:    "ClientID",
			Title: "Frontend OAuth client ID",
			Type:  portal.FieldText,
			Help: `This client ID will be available via /auth/api/v1/server/client_id
endpoint (so that the frontend can grab it), and the auth library will permit
access tokens with this client as an audience when authenticating requests.`,
		},
	}, nil
}

func (oauthSettingsPage) ReadSettings(ctx context.Context) (map[string]string, error) {
	s := oauthSettings{}
	err := settings.GetUncached(ctx, oauthSettingsKey, &s)
	if err != nil && err != settings.ErrNoSettings {
		return nil, err
	}
	return map[string]string{
		"ClientID": s.ClientID,
	}, nil
}

func (oauthSettingsPage) WriteSettings(ctx context.Context, values map[string]string) error {
	return settings.SetIfChanged(ctx, oauthSettingsKey, &oauthSettings{
		ClientID: values["ClientID"],
	})
}

func init() {
	portal.RegisterPage(oauthSettingsKey, oauthSettingsPage{})
}
