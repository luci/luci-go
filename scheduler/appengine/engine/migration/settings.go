// Copyright 2018 The LUCI Authors.
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

package migration

import (
	"regexp"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/portal"
	"go.chromium.org/luci/server/settings"
)

const settingsKey = "scheduler_v2_migration"

// Settings contain the settings related to v1 -> v2 migration.
type Settings struct {
	EligibleJobs string // regexp against full job ID
}

// GetSettings return settings related to v1 -> v2 migration.
func GetSettings(ctx context.Context) (*Settings, error) {
	s := &Settings{}
	if err := settings.Get(ctx, settingsKey, s); err != nil && err != settings.ErrNoSettings {
		return nil, errors.Annotate(err, "failed to fetch migration settings").Tag(transient.Tag).Err()
	}
	return s, nil
}

type settingsPage struct {
	portal.BasePage
}

func (settingsPage) Title(ctx context.Context) (string, error) {
	return "Scheduler v1 -> v2 migration", nil
}

func (settingsPage) Fields(ctx context.Context) ([]portal.Field, error) {
	return []portal.Field{
		{
			ID:    "EligibleJobs",
			Title: "Jobs eligible for the v2 conversion",
			Type:  portal.FieldText,
			Help:  "Regexp against the full job names (project/id).",
			Validator: func(v string) error {
				_, err := regexp.Compile(v)
				return err
			},
		},
	}, nil
}

func (settingsPage) ReadSettings(ctx context.Context) (map[string]string, error) {
	s := &Settings{}
	if err := settings.GetUncached(ctx, settingsKey, s); err != nil && err != settings.ErrNoSettings {
		return nil, err
	}
	return map[string]string{"EligibleJobs": s.EligibleJobs}, nil
}

func (settingsPage) WriteSettings(ctx context.Context, values map[string]string, who, why string) error {
	return settings.SetIfChanged(ctx, settingsKey, &Settings{
		EligibleJobs: values["EligibleJobs"],
	}, who, why)
}

func init() {
	portal.RegisterPage(settingsKey, settingsPage{})
}
