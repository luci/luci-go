// Copyright 2021 The LUCI Authors.
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

package config

import (
	"context"
	"regexp"
	"strings"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/tsmon/registry"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgcache"
	"go.chromium.org/luci/config/validation"

	pb "go.chromium.org/luci/buildbucket/proto"
)

const settingsCfgFilename = "settings.cfg"

var bbInternalMetrics = stringset.NewFromSlice([]string{
	// v1
	"/chrome/infra/buildbucket/builds/created",
	"/chrome/infra/buildbucket/builds/started",
	"/chrome/infra/buildbucket/builds/completed",
	"/chrome/infra/buildbucket/builds/cycle_durations",
	"/chrome/infra/buildbucket/builds/run_durations",
	"/chrome/infra/buildbucket/builds/scheduling_durations",
	"/chrome/infra/buildbucket/builds/lease_expired",
	"/chrome/infra/buildbucket/builds/max_age_scheduled",
	"/chrome/infra/buildbucket/builds/count",
	// V2
	"/chrome/infra/buildbucket/v2/builds/created",
	"/chrome/infra/buildbucket/v2/builds/started",
	"/chrome/infra/buildbucket/v2/builds/completed",
	"/chrome/infra/buildbucket/v2/builds/cycle_durations",
	"/chrome/infra/buildbucket/v2/builds/run_durations",
	"/chrome/infra/buildbucket/v2/builds/scheduling_durations",
	"/chrome/infra/buildbucket/v2/builds/max_age_scheduled",
	"/chrome/infra/buildbucket/v2/builds/count",
	"/chrome/infra/buildbucket/v2/builds/consecutive_failure_count",
}...)

// Cached settings config.
var cachedSettingsCfg = cfgcache.Register(&cfgcache.Entry{
	Path: settingsCfgFilename,
	Type: (*pb.SettingsCfg)(nil),
})

// init registers validation rules.
func init() {
	validation.Rules.Add("services/${appid}", settingsCfgFilename, validateSettingsCfg)
	validation.Rules.Add("regex:projects/.*", "${appid}.cfg", validateProjectCfg)
}

// validateSettingsCfg implements validation.Func and validates the content of
// the settings file.
//
// Validation errors are returned via validation.Context. An error directly
// returned by this function means a bug in the code.
func validateSettingsCfg(ctx *validation.Context, configSet, path string, content []byte) error {
	cfg := pb.SettingsCfg{}
	if err := prototext.Unmarshal(content, &cfg); err != nil {
		ctx.Errorf("invalid SettingsCfg proto message: %s", err)
		return nil
	}
	if s := cfg.Swarming; s != nil {
		ctx.Enter("swarming")
		validateSwarmingSettings(ctx, s)
		ctx.Exit()
	}

	for i, exp := range cfg.Experiment.GetExperiments() {
		ctx.Enter("experiment.experiments #%d", i)
		validateExperiment(ctx, exp)
		ctx.Exit()
	}

	for i, backend := range cfg.GetBackends() {
		ctx.Enter("Backends.BackendSetting #%d", i)
		validateHostname(ctx, "BackendSetting.hostname", backend.GetHostname())
		switch backend.Mode.(type) {
		case *pb.BackendSetting_FullMode_:
			validateBackendFullMode(ctx, backend.GetFullMode())
		case *pb.BackendSetting_LiteMode_:
		default:
			ctx.Errorf("mode field is not set or its type is unsupported")
		}
		ctx.Exit()
	}

	validateCustomMetrics(ctx, cfg.GetCustomMetrics())

	validateHostname(ctx, "logdog.hostname", cfg.Logdog.GetHostname())
	validateHostname(ctx, "resultdb.hostname", cfg.Resultdb.GetHostname())
	return nil
}

func validateCustomMetrics(ctx *validation.Context, cms []*pb.CustomMetric) {
	ctx.Enter("custom_metrics")
	metricNames := stringset.New(len(cms))
	for i, customMetric := range cms {
		ctx.Enter("custom_metrics #%d", i)
		validateCustomMetric(ctx, customMetric)

		if !metricNames.Add(customMetric.Name) {
			ctx.Errorf("duplicated name is not allowed: %s", customMetric.Name)
		}
		ctx.Exit()
	}
	ctx.Exit()
}

func validateCustomMetric(ctx *validation.Context, cm *pb.CustomMetric) {
	if !strings.HasPrefix(cm.GetName(), "/") {
		ctx.Errorf(`invalid metric name %q: must starts with "/"`, cm.GetName())
	}
	if err := registry.ValidateMetricName(cm.GetName()); err != nil {
		ctx.Errorf("%s", err)
	}

	if bbInternalMetrics.Has(cm.GetName()) {
		ctx.Errorf("%q is reserved by Buildbucket", cm.Name)
	}

	seen := stringset.New(len(cm.GetFields()))
	for _, field := range cm.GetFields() {
		if err := registry.ValidateMetricFieldName(field); err != nil {
			ctx.Errorf("%s", err)
		}
		if !seen.Add(field) {
			ctx.Errorf("%q is duplicated", field)
		}
	}
}

func validateBackendFullMode(ctx *validation.Context, m *pb.BackendSetting_FullMode) {
	if m.PubsubId == "" {
		ctx.Errorf("pubsub_id for UpdateBuildTask must be specified")
	}
	validateBuildSyncSetting(ctx, m.GetBuildSyncSetting())
}

func validateSwarmingSettings(ctx *validation.Context, s *pb.SwarmingSettings) {
	validateHostname(ctx, "milo_hostname", s.MiloHostname)
	for i, pkg := range s.UserPackages {
		ctx.Enter("user_packages #%d", i)
		validatePackage(ctx, pkg)
		ctx.Exit()
	}

	for i, pkg := range s.AlternativeAgentPackages {
		ctx.Enter("alternative_agent_packages #%d", i)
		validatePackage(ctx, pkg)
		if len(pkg.OmitOnExperiment) == 0 && len(pkg.IncludeOnExperiment) == 0 {
			ctx.Errorf("alternative_agent_package must set constraints on either omit_on_experiment or include_on_experiment")
		}
		ctx.Exit()
	}

	if bbPkg := s.BbagentPackage; bbPkg != nil {
		ctx.Enter("bbagent_package")
		validatePackage(ctx, bbPkg)
		if !strings.HasSuffix(bbPkg.PackageName, "/${platform}") {
			ctx.Errorf("package_name must end with '/${platform}'")
		}
		ctx.Exit()
	}

	if kitchen := s.KitchenPackage; kitchen != nil {
		ctx.Enter("kitchen_package")
		validatePackage(ctx, kitchen)
		ctx.Exit()
	}
}

func validateHostname(ctx *validation.Context, field string, host string) {
	if host == "" {
		ctx.Errorf("%s unspecified", field)
	}
	if strings.Contains(host, "://") {
		ctx.Errorf("%s must not contain '://'", field)
	}
}

func validateBuildSyncSetting(ctx *validation.Context, setting *pb.BackendSetting_BuildSyncSetting) {
	if setting.GetShards() < 0 {
		ctx.Errorf("shards must be greater than or equal to 0")
	}

	if setting.GetSyncIntervalSeconds() != 0 && setting.GetSyncIntervalSeconds() < 60 {
		ctx.Errorf("sync_interval_seconds must be greater than or equal to 60")
	}
}

func validatePackage(ctx *validation.Context, pkg *pb.SwarmingSettings_Package) {
	if pkg.PackageName == "" {
		ctx.Errorf("package_name is required")
	}
	if pkg.Version == "" {
		ctx.Errorf("version is required")
	}
	if pkg.Builders != nil {
		validateRegex(ctx, "builders.regex", pkg.Builders.Regex)
		validateRegex(ctx, "builders.regex_exclude", pkg.Builders.RegexExclude)
	}
}

func validateExperiment(ctx *validation.Context, exp *pb.ExperimentSettings_Experiment) {
	if exp.Name == "" {
		ctx.Errorf("name is required")
	}
	if exp.MinimumValue < 0 || exp.MinimumValue > 100 {
		ctx.Errorf("minimum_value must be in [0,100]")
	}
	if exp.DefaultValue < exp.MinimumValue || exp.DefaultValue > 100 {
		ctx.Errorf("default_value must be in [${minimum_value},100]")
	}
	if exp.Inactive && (exp.DefaultValue != 0 || exp.MinimumValue != 0) {
		ctx.Errorf("default_value and minimum_value must both be 0 when inactive is true")
	}
	if exp.Builders != nil {
		validateRegex(ctx, "builders.regex", exp.Builders.Regex)
		validateRegex(ctx, "builders.regex_exclude", exp.Builders.RegexExclude)
	}
}

func validateRegex(ctx *validation.Context, field string, patterns []string) {
	for _, p := range patterns {
		if _, err := regexp.Compile(p); err != nil {
			ctx.Errorf("%s %q: invalid regex", field, p)
		}
	}
}

// UpdateSettingsCfg is called from a cron periodically to import settings.cfg into datastore.
func UpdateSettingsCfg(ctx context.Context) error {
	_, err := cachedSettingsCfg.Update(ctx, nil)
	return err
}

// GetSettingsCfg fetches the settings.cfg from luci-config.
func GetSettingsCfg(ctx context.Context) (*pb.SettingsCfg, error) {
	cfg, err := cachedSettingsCfg.Get(ctx, nil)
	if err != nil {
		return nil, err
	}
	return cfg.(*pb.SettingsCfg), nil
}

// SetTestSettingsCfg is used in tests only.
func SetTestSettingsCfg(ctx context.Context, cfg *pb.SettingsCfg) error {
	return cachedSettingsCfg.Set(ctx, cfg, &config.Meta{Path: "settings.cfg"})
}
