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

package tumble

import (
	"context"
	"fmt"
	"html/template"
	"strconv"

	"go.chromium.org/luci/common/clock/clockflag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/portal"
	"go.chromium.org/luci/server/settings"
)

const (
	settingDisabled = "disabled"
	settingEnabled  = "enabled"
)

// settingsPage is a UI page to configure a static Tumble configuration.
type settingsPage struct {
	portal.BasePage
}

func (settingsPage) Title(c context.Context) (string, error) {
	return "Tumble settings", nil
}

func (settingsPage) Overview(c context.Context) (template.HTML, error) {
	return template.HTML(`<p>Configuration parameters for the
<a href="https://github.com/luci/luci-go/tree/master/tumble">tumble
service</a> can be found in its
<a href="https://godoc.org/go.chromium.org/luci/tumble">
documentation</a>.</p>.`), nil
}

func (settingsPage) Fields(c context.Context) ([]portal.Field, error) {
	return []portal.Field{
		{
			ID:          "NumShards",
			Title:       "Number of shards to use",
			Type:        portal.FieldText,
			Placeholder: strconv.FormatUint(defaultConfig.NumShards, 10),
			Validator:   intValidator(true),
		},
		{
			ID:          "NumGoroutines",
			Title:       "Number of goroutines per shard",
			Type:        portal.FieldText,
			Placeholder: strconv.Itoa(defaultConfig.NumGoroutines),
			Validator:   intValidator(true),
		},
		{
			ID:          "TemporalMinDelay",
			Title:       "Temporal minimum delay (s, m, h)",
			Type:        portal.FieldText,
			Placeholder: defaultConfig.TemporalMinDelay.String(),
			Validator:   validateDuration,
		},
		{
			ID:          "TemporalRoundFactor",
			Title:       "Temporal round factor, for batching (s, m, h)",
			Type:        portal.FieldText,
			Placeholder: defaultConfig.TemporalRoundFactor.String(),
			Validator:   validateDuration,
		},
		{
			ID:          "ProcessLoopDuration",
			Title:       "Maximum lifetime of batch processing loop",
			Type:        portal.FieldText,
			Placeholder: defaultConfig.ProcessLoopDuration.String(),
			Validator:   validateDuration,
		},
		{
			ID:          "DustSettleTimeout",
			Title:       "Amount of time to wait for datastore to settle in between mutation rounds (s, m, h)",
			Type:        portal.FieldText,
			Placeholder: defaultConfig.DustSettleTimeout.String(),
			Validator:   validateDuration,
		},
		{
			ID: "MaxNoWorkDelay",
			Title: "Maximum amount of time to sleep in between rounds if here was no work done " +
				"the previous round (s, m, h)",
			Type:        portal.FieldText,
			Placeholder: defaultConfig.MaxNoWorkDelay.String(),
			Validator:   validateDuration,
		},
		{
			ID: "NoWorkDelayGrowth",
			Title: "Growth factor for the delay in between loops when no work was done. " +
				"If <= 1, no growth will be applied. The delay is capped by MaxNoWorkDelay.",
			Type:        portal.FieldText,
			Placeholder: strconv.Itoa(int(defaultConfig.NoWorkDelayGrowth)),
			Validator:   intValidator(true),
		},
		{
			ID:          "ProcessMaxBatchSize",
			Title:       "Number of mutations to include per commit (negative for unlimited)",
			Type:        portal.FieldText,
			Placeholder: strconv.Itoa(int(defaultConfig.ProcessMaxBatchSize)),
			Validator:   intValidator(false),
		},
		{
			ID:             "DelayedMutations",
			Title:          "Delayed mutations (index MUST be present)",
			Type:           portal.FieldChoice,
			ChoiceVariants: []string{settingDisabled, settingEnabled},
		},
	}, nil
}

func (settingsPage) ReadSettings(c context.Context) (map[string]string, error) {
	var cfg Config
	switch err := settings.GetUncached(c, baseName, &cfg); err {
	case nil:
		break
	case settings.ErrNoSettings:
		logging.WithError(err).Infof(c, "No settings available, using defaults.")
		cfg = defaultConfig
	default:
		return nil, err
	}

	values := map[string]string{}

	// Only render values if they differ from our default config.
	if cfg.NumShards != defaultConfig.NumShards {
		values["NumShards"] = strconv.FormatUint(cfg.NumShards, 10)
	}
	if cfg.NumGoroutines != defaultConfig.NumGoroutines {
		values["NumGoroutines"] = strconv.Itoa(cfg.NumGoroutines)
	}
	if cfg.TemporalMinDelay != defaultConfig.TemporalMinDelay {
		values["TemporalMinDelay"] = cfg.TemporalMinDelay.String()
	}
	if cfg.TemporalRoundFactor != defaultConfig.TemporalRoundFactor {
		values["TemporalRoundFactor"] = cfg.TemporalRoundFactor.String()
	}
	if cfg.ProcessLoopDuration != defaultConfig.ProcessLoopDuration {
		values["ProcessLoopDuration"] = cfg.ProcessLoopDuration.String()
	}
	if cfg.DustSettleTimeout != defaultConfig.DustSettleTimeout {
		values["DustSettleTimeout"] = cfg.DustSettleTimeout.String()
	}
	if cfg.MaxNoWorkDelay != defaultConfig.MaxNoWorkDelay {
		values["MaxNoWorkDelay"] = cfg.MaxNoWorkDelay.String()
	}
	if cfg.NoWorkDelayGrowth != defaultConfig.NoWorkDelayGrowth {
		values["NoWorkDelayGrowth"] = strconv.Itoa(int(cfg.NoWorkDelayGrowth))
	}
	if cfg.ProcessMaxBatchSize != defaultConfig.ProcessMaxBatchSize {
		values["ProcessMaxBatchSize"] = strconv.Itoa(int(cfg.ProcessMaxBatchSize))
	}

	values["DelayedMutations"] = getToggleSetting(cfg.DelayedMutations)

	return values, nil
}

func (settingsPage) WriteSettings(c context.Context, values map[string]string, who, why string) error {
	// Start with our default config and shape it with populated values.
	cfg := defaultConfig

	var err error
	if v := values["NumShards"]; v != "" {
		cfg.NumShards, err = strconv.ParseUint(v, 10, 64)
		if err != nil {
			return fmt.Errorf("could not parse NumShards: %v", err)
		}
	}
	if v := values["NumGoroutines"]; v != "" {
		cfg.NumGoroutines, err = strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("could not parse NumGoroutines: %v", err)
		}
	}
	if v := values["TemporalMinDelay"]; v != "" {
		cfg.TemporalMinDelay, err = clockflag.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("could not parse TemporalMinDelay: %v", err)
		}
	}
	if v := values["TemporalRoundFactor"]; v != "" {
		cfg.TemporalRoundFactor, err = clockflag.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("could not parse TemporalRoundFactor: %v", err)
		}
	}
	if v := values["ProcessLoopDuration"]; v != "" {
		cfg.ProcessLoopDuration, err = clockflag.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("could not parse ProcessLoopDuration: %v", err)
		}
	}
	if v := values["DustSettleTimeout"]; v != "" {
		cfg.DustSettleTimeout, err = clockflag.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("could not parse DustSettleTimeout: %v", err)
		}
	}
	if v := values["MaxNoWorkDelay"]; v != "" {
		cfg.MaxNoWorkDelay, err = clockflag.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("could not parse MaxNoWorkDelay: %v", err)
		}
	}
	if v := values["NoWorkDelayGrowth"]; v != "" {
		val, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("could not parse ProcessMaxBatchSize: %v", err)
		}
		cfg.NoWorkDelayGrowth = val
	}
	if v := values["ProcessMaxBatchSize"]; v != "" {
		val, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("could not parse ProcessMaxBatchSize: %v", err)
		}
		cfg.ProcessMaxBatchSize = val
	}
	cfg.DelayedMutations = values["DelayedMutations"] == settingEnabled

	return settings.SetIfChanged(c, baseName, &cfg, who, why)
}

func intValidator(positive bool) func(string) error {
	return func(v string) error {
		if v == "" {
			return nil
		}
		i, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid integer %q - %s", v, err)
		}
		if positive && i <= 0 {
			return fmt.Errorf("value %q must be positive", v)
		}
		return nil
	}
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

func getToggleSetting(v bool) string {
	if v {
		return settingEnabled
	}
	return settingDisabled
}

func init() {
	portal.RegisterPage("tumble", settingsPage{})
}
