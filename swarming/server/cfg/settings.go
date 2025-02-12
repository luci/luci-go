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

package cfg

import (
	"fmt"
	"net/url"
	"strings"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/config/validation"

	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/validate"
)

// withDefaultSettings fills in defaults in `cfg` and returns it.
func withDefaultSettings(cfg *configpb.SettingsCfg) *configpb.SettingsCfg {
	if cfg.ReusableTaskAgeSecs == 0 {
		cfg.ReusableTaskAgeSecs = 7 * 24 * 60 * 60
	}
	if cfg.BotDeathTimeoutSecs == 0 {
		cfg.BotDeathTimeoutSecs = 10 * 60
	}
	if cfg.Auth == nil {
		cfg.Auth = &configpb.AuthSettings{}
	}
	if cfg.Auth.AdminsGroup == "" {
		cfg.Auth.AdminsGroup = "administrators"
	}
	if cfg.Auth.BotBootstrapGroup == "" {
		cfg.Auth.BotBootstrapGroup = cfg.Auth.AdminsGroup
	}
	if cfg.Auth.PrivilegedUsersGroup == "" {
		cfg.Auth.PrivilegedUsersGroup = cfg.Auth.AdminsGroup
	}
	if cfg.Auth.UsersGroup == "" {
		cfg.Auth.UsersGroup = cfg.Auth.AdminsGroup
	}
	if cfg.Auth.ViewAllBotsGroup == "" {
		cfg.Auth.ViewAllBotsGroup = cfg.Auth.AdminsGroup
	}
	if cfg.Auth.ViewAllTasksGroup == "" {
		cfg.Auth.ViewAllTasksGroup = cfg.Auth.AdminsGroup
	}
	return cfg
}

// validateSettingsCfg validates settings.cfg, writing errors into `ctx`.
func validateSettingsCfg(ctx *validation.Context, cfg *configpb.SettingsCfg) {
	ctx.Enter("bot_death_timeout_secs")
	if cfg.BotDeathTimeoutSecs < 0 {
		ctx.Errorf("must be non-negative, got %d", cfg.BotDeathTimeoutSecs)
	}
	ctx.Exit()

	ctx.Enter("reusable_task_age_secs")
	if cfg.ReusableTaskAgeSecs < 0 {
		ctx.Errorf("must be non-negative, got %d", cfg.ReusableTaskAgeSecs)
	}
	ctx.Exit()

	if cfg.DisplayServerUrlTemplate != "" {
		if strings.Count(cfg.DisplayServerUrlTemplate, "%s") != 1 {
			ctx.Enter("display_server_url_template")
			ctx.Errorf("must have exactly one `%%s` term")
			ctx.Exit()
		} else {
			// Need to do the template substitution now because '%s' is not valid
			// inside as URL.
			validateHTTPS(ctx, "display_server_url_template", fmt.Sprintf(cfg.DisplayServerUrlTemplate, "..."))
		}
	}

	for _, url := range cfg.ExtraChildSrcCspUrl {
		validateHTTPS(ctx, "extra_child_src_csp_url", url)
	}

	if cfg.Cipd != nil {
		ctx.Enter("cipd")
		validateHTTPS(ctx, "default_server", cfg.Cipd.DefaultServer)
		ctx.Enter("default_client_package")
		if cfg.Cipd.DefaultClientPackage == nil {
			ctx.Errorf("this is a required field")
		} else {
			ctx.Enter("package_name")
			if err := validate.CIPDPackageName(cfg.Cipd.DefaultClientPackage.PackageName, true); err != nil {
				ctx.Errorf("%s", err)
			}
			ctx.Exit()
			ctx.Enter("version")
			if err := validate.CIPDPackageVersion(cfg.Cipd.DefaultClientPackage.Version); err != nil {
				ctx.Errorf("%s", err)
			}
			ctx.Exit()
		}
		ctx.Exit()
		ctx.Exit()
	}

	if cfg.Resultdb != nil {
		ctx.Enter("resultdb")
		validateHTTPS(ctx, "server", cfg.Resultdb.Server)
		ctx.Exit()
	}

	if cfg.Cas != nil {
		ctx.Enter("cas")
		validateHTTPS(ctx, "viewer_server", cfg.Cas.ViewerServer)
		ctx.Exit()
	}

	if cfg.TrafficMigration != nil {
		ctx.Enter("traffic_migration")
		validateTrafficMigration(ctx, cfg.TrafficMigration)
		ctx.Exit()
	}

	if cfg.BotDeployment != nil {
		ctx.Enter("bot_deployment")
		validateBotDeployment(ctx, cfg.BotDeployment)
		ctx.Exit()
	}
}

func validateBotDeployment(ctx *validation.Context, d *configpb.BotDeployment) {
	if d.Stable == nil {
		ctx.Errorf("missing required \"stable\" section")
	} else {
		validateBotPackage(ctx, "stable", d.Stable)
	}
	if d.Canary != nil {
		validateBotPackage(ctx, "canary", d.Canary)
	}
	if d.CanaryPercent < 0 || d.CanaryPercent > 100 {
		ctx.Errorf("canary_percent must be between 0 and 100")
	}
}

func validateBotPackage(ctx *validation.Context, channel string, pkg *configpb.BotDeployment_BotPackage) {
	ctx.Enter("%s", channel)
	defer ctx.Exit()

	ctx.Enter("server")
	if err := validate.CIPDServer(pkg.Server); err != nil {
		ctx.Error(err)
	}
	ctx.Exit()

	ctx.Enter("pkg")
	if err := validate.CIPDPackageName(pkg.Pkg, false); err != nil {
		ctx.Error(err)
	}
	ctx.Exit()

	ctx.Enter("version")
	if err := validate.CIPDPackageVersion(pkg.Version); err != nil {
		ctx.Error(err)
	}
	ctx.Exit()
}

func validateHTTPS(ctx *validation.Context, key, val string) {
	ctx.Enter("%s", key)
	defer ctx.Exit()

	if val == "" {
		ctx.Errorf("this is a required field")
		return
	}

	if !strings.HasPrefix(val, "https://") {
		ctx.Errorf("must be an https:// URL")
		return
	}

	switch parsed, err := url.Parse(val); {
	case err != nil:
		ctx.Errorf("%s", err)
	case parsed.Host == "":
		ctx.Errorf("URL %q doesn't have a host", val)
	}
}

func validateTrafficMigration(ctx *validation.Context, cfg *configpb.TrafficMigration) {
	seen := stringset.New(len(cfg.Routes))
	for _, r := range cfg.Routes {
		ctx.Enter("%q", r.Name)
		if !seen.Add(r.Name) {
			ctx.Errorf("duplicate route")
		} else {
			if !strings.HasPrefix(r.Name, "/") {
				ctx.Errorf("route name should start with /")
			}
			if r.RouteToGoPercent < 0 || r.RouteToGoPercent > 100 {
				ctx.Errorf("route_to_go_percent should be in range [0, 100]")
			}
		}
		ctx.Exit()
	}
}
