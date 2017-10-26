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

package frontend

import (
	"html/template"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/settings"

	"golang.org/x/net/context"
)

// settingsKey is the name used to read/write these settings.
const settingsKey = "database"

// DatabaseSettings are app-level settings configuring how to connect to the
// Cloud SQL database.
type DatabaseSettings struct {
	// Database is the name of the database to connect to.
	Database string `json:"database"`
	// Username is the username to authenticate to the database with.
	Username string `json:"username"`
	// Password is the password to authenticate to the database with.
	Password string `json:"password"`
}

// New returns a new instance of DatabaseSettings.
func New(c context.Context) *DatabaseSettings {
	return &DatabaseSettings {
		Database: "",
		Username: "",
		Password: "",
	}
}

// GetUncached returns the latest settings, bypassing the cache.
func GetUncached(c context.Context) (*DatabaseSettings, error) {
	databaseSettings := &DatabaseSettings{}
	switch err := settings.GetUncached(c, settingsKey, databaseSettings); err {
	case nil:
		return databaseSettings, nil
	case settings.ErrNoSettings:
		return New(c), nil
	default:
		return nil, err
	}
}

// DatabaseSettingsUI implements settings.UIPage around DatabaseSettings.
type DatabaseSettingsUI struct {
}

func (DatabaseSettings) Fields(c context.Context) ([]settings.UIField, error) {
	fields := []settings.UIField {
		settings.UIField {
			ID: "Database",
			Title: "Database",
			Type: settings.UIFieldText,
			Help: template.HTML("<p>Cloud SQL database to use, should be of the form project:region:database</p>"),
		},
		settings.UIField {
			ID: "Username",
			Title: "Username",
			Type: settings.UIFieldText,
			Help: template.HTML("<p>Username to authenticate to the Cloud SQL database with</p>"),
		},
		settings.UIField {
			ID: "Password",
			Title: "Password",
			Type: settings.UIFieldText,
			Help: template.HTML("<p>Password to authenticate to the Cloud SQL database with</p>"),
		},
	}
	return fields, nil
}

func (DatabaseSettings) Overview(c context.Context) (template.HTML, error) {
	return template.HTML("<p>Cloud SQL database configuration</p>"), nil
}

func (DatabaseSettings) ReadSettings(c context.Context) (map[string]string, error) {
	databaseSettings, err := GetUncached(c)
	logging.Infof(c, "ReadSettings, error: %s, settings: %s", err, databaseSettings)
	if err != nil {
		return nil, err
	}

	settingsMap := map[string]string {
		"Database": databaseSettings.Database,
		"Username": databaseSettings.Username,
		"Password": databaseSettings.Password,
	}
	return settingsMap, nil
}

func (DatabaseSettings) Title(c context.Context) (string, error) {
	return "Database settings", nil
}

func (DatabaseSettings) WriteSettings(c context.Context, values map[string]string, who, why string) error {
	databaseSettings := &DatabaseSettings{
		Database: values["Database"],
		Username: values["Username"],
		Password: values["Password"],
	}

	return settings.Set(c, settingsKey, databaseSettings, who, why)
}

func InstallSettings() {
	settings.RegisterUIPage(settingsKey, DatabaseSettings{})
}
