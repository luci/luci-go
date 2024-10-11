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

package gaeconfig

import (
	"context"
	"fmt"
	"html"
	"html/template"
	"net/url"
	"strings"

	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/portal"
	"go.chromium.org/luci/server/settings"
)

// configServiceAdmins is the default value for settings.AdministratorsGroup
// config setting below.
const configServiceAdmins = "administrators"

// Settings are stored in the datastore via appengine/gaesettings package.
type Settings struct {
	// ConfigServiceHost is host name (and port) of the luci-config service to
	// fetch configs from.
	//
	// For legacy reasons, the JSON value is "config_service_url".
	ConfigServiceHost string `json:"config_service_url"`

	// Administrators is the auth group of users that can call the validation
	// endpoint.
	AdministratorsGroup string `json:"administrators_group"`
}

// SetIfChanged sets "s" to be the new Settings if it differs from the current
// settings value.
func (s *Settings) SetIfChanged(c context.Context) error {
	return settings.SetIfChanged(c, settingsKey, s)
}

// FetchCachedSettings fetches Settings from the settings store.
//
// Uses in-process global cache to avoid hitting datastore often. The cache
// expiration time is 1 min (see gaesettings.expirationTime), meaning
// the instance will refetch settings once a minute (blocking only one unlucky
// request to do so).
//
// Returns errors only if there's no cached value (i.e. it is the first call
// to this function in this process ever) and datastore operation fails.
func FetchCachedSettings(c context.Context) (Settings, error) {
	s := Settings{}
	switch err := settings.Get(c, settingsKey, &s); err {
	case nil:
		// Backwards-compatibility with full URL: translate to host.
		s.ConfigServiceHost = translateConfigURLToHost(s.ConfigServiceHost)
		return s, nil
	case settings.ErrNoSettings:
		return DefaultSettings(c), nil
	default:
		return Settings{}, err
	}
}

func mustFetchCachedSettings(c context.Context) *Settings {
	settings, err := FetchCachedSettings(c)
	if err != nil {
		panic(err)
	}
	return &settings
}

// DefaultSettings returns Settings to use if setting store is empty.
func DefaultSettings(c context.Context) Settings {
	return Settings{AdministratorsGroup: configServiceAdmins}
}

////////////////////////////////////////////////////////////////////////////////
// UI for settings.

// settingsKey is used internally to identify gaeconfig settings in settings
// store.
const settingsKey = "gaeconfig"

type settingsPage struct {
	portal.BasePage
}

func (settingsPage) Title(c context.Context) (string, error) {
	return "Configuration service settings", nil
}

func (settingsPage) Overview(c context.Context) (template.HTML, error) {
	serviceAcc, err := info.ServiceAccount(c)
	if err != nil {
		return "", err
	}
	return template.HTML(fmt.Sprintf(`
<p>This service may fetch configuration files stored centrally in an instance of
<a href="https://chromium.googlesource.com/infra/luci/luci-go/+/refs/heads/main/config_service">luci-config</a>
service. This page can be used to configure the location of the config service
as well as parameters of the local cache that holds the fetched configuration
files.</p>
<p>Before your service can start fetching configs, it should be registered in
the config service's registry of known services (services.cfg config file), by
adding something similar to this:</p>
<pre>
services {
  id: "%s"
  owners: &lt;your email&gt;
  access: "%s"
}
</pre>
<p>Refer to the documentation in the services.cfg file for more info.</p>`,
		html.EscapeString(info.TrimmedAppID(c)),
		html.EscapeString(serviceAcc)),
	), nil
}

func (settingsPage) Fields(c context.Context) ([]portal.Field, error) {
	return []portal.Field{
		{
			ID:    "ConfigServiceHost",
			Title: `Config service host`,
			Type:  portal.FieldText,
			Validator: func(v string) error {
				if strings.ContainsRune(v, '/') {
					return fmt.Errorf("host must be a host name, not a URL")
				}
				return nil
			},
			Help: `<p>Host name (e.g., "config.luci.app") of a config service to fetch configuration files from.</p>`,
		},
		{
			ID:    "AdministratorsGroup",
			Title: "Administrator group",
			Type:  portal.FieldText,
			Validator: func(v string) error {
				if v == "" {
					return fmt.Errorf("administrator group cannot be an empty string")
				}
				return nil
			},
			Help: `<p>Members of this group can directly call the validation endpoint
of this service. Usually it is called only indirectly by the config service,
but it may be useful (e.g. for debugging) to call it directly.</p>`,
		},
	}, nil
}

func (settingsPage) ReadSettings(c context.Context) (map[string]string, error) {
	s := DefaultSettings(c)
	err := settings.GetUncached(c, settingsKey, &s)
	if err != nil && err != settings.ErrNoSettings {
		return nil, err
	}
	return map[string]string{
		"ConfigServiceHost":   s.ConfigServiceHost,
		"AdministratorsGroup": s.AdministratorsGroup,
	}, nil
}

func (settingsPage) WriteSettings(c context.Context, values map[string]string) error {
	modified := Settings{
		ConfigServiceHost:   translateConfigURLToHost(values["ConfigServiceHost"]),
		AdministratorsGroup: values["AdministratorsGroup"],
	}
	return modified.SetIfChanged(c)
}

func translateConfigURLToHost(v string) string {
	// If the host is a full URL, extract just the host component.
	switch u, err := url.Parse(v); {
	case err != nil:
		return v
	case u.Host != "":
		// If we have a host (e.g., "example.com"), this will parse into the "Path"
		// field with an empty host value. Therefore, if we have a "Host" value,
		// we will use it directly (e.g., "http://example.com")
		return u.Host
	case u.Path != "":
		// If this was just an empty (correct) host, it will have parsed into the
		// Path field with an empty Host value.
		return u.Path
	default:
		return v
	}
}

func init() {
	portal.RegisterPage(settingsKey, settingsPage{})
}
