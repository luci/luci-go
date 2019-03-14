// Copyright 2019 The LUCI Authors.
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

// TODO(hinoka): Remove this file after migrating to the new archival tasking pipeline.
// These settings are used for adjusting how much traffic goes into the new archival tasking pipeline.
// 3 Settings are available for the new pipeline:
// * Optimistic archival delay - Controls the delay to set for optimistic archival.
// * Optimistic archival tasking percentage - Controls what % of terminated streams are tasked to the new pipeline
// * Pessimistic archival tasking percentage - Controls what % of registered streams are tasked to the new pipeline.

package coordinator

import (
	"context"
	"fmt"
	"html/template"
	"strconv"
	"time"

	"go.chromium.org/luci/common/clock/clockflag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/portal"
	"go.chromium.org/luci/server/settings"
)

const (
	settingDisabled = "disabled"
	settingEnabled  = "enabled"
	baseName        = "archivist"
)

type Settings struct {
	// OptimisticArchivalDelay controls the delay to set for optimistic archival.
	OptimisticArchivalDelay time.Duration

	// OptimisticArchivalPercent (0-100) Controls what percent of terminated streams
	// are tasked to the new pipeline
	OptimisticArchivalPercent uint32

	// PessimisticArchivalPercent (0-100) Controls what percent of registered
	// streams are tasked to the new pipeline after 47 hours.
	PessimisticArchivalPercent uint32

	// ArchivistBatchSize controls the batchsize per lease cycle.
	ArchivistBatchSize int64

	// ArchivistLeaseTime controls the lease time per cycle.
	ArchivistLeaseTime time.Duration
}

var defaultSettings = Settings{
	OptimisticArchivalDelay: 5 * time.Minute,
	ArchivistBatchSize:      50,
	ArchivistLeaseTime:      15 * time.Minute,
}

// settingsPage is a UI page to configure a static Tumble configuration.
type settingsPage struct {
	portal.BasePage
}

func (settingsPage) Title(c context.Context) (string, error) {
	return "LogDog Archivist Settings", nil
}

func (settingsPage) Overview(c context.Context) (template.HTML, error) {
	return template.HTML(`<p>Configuration parameters for the Archivist tasking pipeline.</p>.`), nil
}

func (settingsPage) Fields(c context.Context) ([]portal.Field, error) {
	return []portal.Field{
		{
			ID: "OptimisticArchivalDelay",
			Title: "The delay, of how the optimistic archival delay. " +
				"This should be set to compensate for potential collector pipeline delays.",
			Type:        portal.FieldText,
			Placeholder: defaultSettings.OptimisticArchivalDelay.String(),
			Validator:   validateDuration,
		},
		{
			ID:          "OptimisticArchivalPercent",
			Title:       "Percentage (0-100) of tasks to go to the new optimistic pipeline.",
			Type:        portal.FieldText,
			Placeholder: "0",
			Validator:   validatePercent,
		},
		{
			ID:          "PessimisticArchivalPercent",
			Title:       "Percentage (0-100) of tasks to go to the new pessimistic pipeline (47hr delay from registration).",
			Type:        portal.FieldText,
			Placeholder: "0",
			Validator:   validatePercent,
		},
		{
			ID: "ArchivistBatchSize",
			Title: "Number of Archive Tasks to lease in each LeaseTasks request. " +
				"The theoretical max Task processed per second is this number * 10, " +
				"due to the limitations of GAE TaskQueues.",
			Type:        portal.FieldText,
			Placeholder: fmt.Sprintf("%d", defaultSettings.ArchivistBatchSize),
			Validator:   validateInt,
		},
		{
			ID:          "ArchivistLeaseTime",
			Title:       "Lease time per cycle for the Archivist.",
			Type:        portal.FieldText,
			Placeholder: defaultSettings.ArchivistLeaseTime.String(),
			Validator:   validateDuration,
		},
	}, nil
}

func (settingsPage) ReadSettings(c context.Context) (map[string]string, error) {
	var set Settings
	switch err := settings.GetUncached(c, baseName, &set); err {
	case nil:
		break
	case settings.ErrNoSettings:
		logging.WithError(err).Infof(c, "No settings available, using defaults.")
		set = defaultSettings
	default:
		return nil, err
	}

	values := map[string]string{}

	// Only render values if they differ from our default config.
	if set.OptimisticArchivalDelay != defaultSettings.OptimisticArchivalDelay {
		values["OptimisticArchivalDelay"] = set.OptimisticArchivalDelay.String()
	}
	if set.OptimisticArchivalPercent != defaultSettings.OptimisticArchivalPercent {
		values["OptimisticArchivalPercent"] = fmt.Sprintf("%d", set.OptimisticArchivalPercent)
	}
	if set.PessimisticArchivalPercent != defaultSettings.PessimisticArchivalPercent {
		values["PessimisticArchivalPercent"] = fmt.Sprintf("%d", set.PessimisticArchivalPercent)
	}
	if set.ArchivistBatchSize != defaultSettings.ArchivistBatchSize {
		values["ArchivistBatchSize"] = fmt.Sprintf("%d", set.ArchivistBatchSize)
	}
	if set.ArchivistLeaseTime != defaultSettings.ArchivistLeaseTime {
		values["ArchivistLeaseTime"] = set.ArchivistLeaseTime.String()
	}

	return values, nil
}

func (settingsPage) WriteSettings(c context.Context, values map[string]string, who, why string) error {
	// Start with our default config and shape it with populated values.
	set := defaultSettings

	if v := values["OptimisticArchivalDelay"]; v != "" {
		t, err := clockflag.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("could not parse OptimisticArchivalDelay: %v", err)
		}
		set.OptimisticArchivalDelay = time.Duration(t)
	}
	if v := values["OptimisticArchivalPercent"]; v != "" {
		i, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("could not parse OptimisticArchivalPercent: %v", err)
		}
		set.OptimisticArchivalPercent = uint32(i)
	}
	if v := values["PessimisticArchivalPercent"]; v != "" {
		i, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("could not parse PessimisticArchivalPercent: %v", err)
		}
		set.PessimisticArchivalPercent = uint32(i)
	}
	if v := values["ArchivistBatchSize"]; v != "" {
		i, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("could not parse ArchivistBatchSize : %v", err)
		}
		set.ArchivistBatchSize = int64(i)
	}
	if v := values["ArchivistLeaseTime"]; v != "" {
		t, err := clockflag.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("could not parse ArchivistLeaseTime : %v", err)
		}
		set.ArchivistLeaseTime = time.Duration(t)
	}

	return settings.SetIfChanged(c, baseName, &set, who, why)
}

// validatePercent validates a string is empty or an integer between 0-100.
func validatePercent(v string) error {
	if v == "" {
		return nil
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return fmt.Errorf("invalid integer %q - %s", v, err)
	}
	if i < 0 || i > 100 {
		return fmt.Errorf("%d is out of range (0-100)", i)
	}
	return nil
}

// validateInt validates that a string is empty or an integer.
func validateInt(v string) error {
	if v == "" {
		return nil
	}
	_, err := strconv.Atoi(v)
	return err
}

func validateDuration(v string) error {
	if v == "" {
		return nil
	}

	var cf clockflag.Duration
	if err := cf.Set(v); err != nil {
		return fmt.Errorf("bad duration %q - %s", v, err)
	}
	if cf <= 0 {
		return fmt.Errorf("duration %q must be positive", v)
	}
	return nil
}

// GetSettings returns the current settings.
//
// It first tries to load it from settings. If no settings is installed, or if
// there is no configuration in settings, defaultSettings is returned.
func GetSettings(c context.Context) *Settings {
	set := Settings{}
	switch err := settings.Get(c, baseName, &set); err {
	case nil:
		break
	case settings.ErrNoSettings:
		// Defaults.
		set = defaultSettings
	default:
		panic(fmt.Errorf("could not fetch Archivist settings - %s", err))
	}
	return &set
}

func init() {
	portal.RegisterPage(baseName, settingsPage{})
}
