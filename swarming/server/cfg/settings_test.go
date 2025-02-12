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
	"context"
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"

	configpb "go.chromium.org/luci/swarming/proto/config"
)

func TestSettingsValidation(t *testing.T) {
	t.Parallel()

	call := func(cfg *configpb.SettingsCfg) []string {
		ctx := validation.Context{Context: context.Background()}
		ctx.SetFile("settings.cfg")
		validateSettingsCfg(&ctx, cfg)
		if err := ctx.Finalize(); err != nil {
			var verr *validation.Error
			errors.As(err, &verr)
			out := make([]string, len(verr.Errors))
			for i, err := range verr.Errors {
				out[i] = err.Error()
			}
			return out
		}
		return nil
	}

	ftt.Run("Empty", t, func(t *ftt.Test) {
		assert.Loosely(t, call(&configpb.SettingsCfg{}), should.BeNil)
	})

	ftt.Run("Good", t, func(t *ftt.Test) {
		assert.Loosely(t, call(withDefaultSettings(&configpb.SettingsCfg{
			DisplayServerUrlTemplate: "https://something.example.com/swarming/task/%s",
			ExtraChildSrcCspUrl:      []string{"https://something.example.com/raw/build/"},
			Cipd: &configpb.CipdSettings{
				DefaultServer: "https://something.example.com",
				DefaultClientPackage: &configpb.CipdPackage{
					PackageName: "some/pkg/${platform}",
					Version:     "some:version",
				},
			},
			TrafficMigration: &configpb.TrafficMigration{
				Routes: []*configpb.TrafficMigration_Route{
					{Name: "/prpc/service/method1", RouteToGoPercent: 0},
					{Name: "/prpc/service/method2", RouteToGoPercent: 50},
					{Name: "/prpc/service/method3", RouteToGoPercent: 100},
					{Name: "/bot_code", RouteToGoPercent: 100},
					{Name: "/swarming/api/v1/bot/something", RouteToGoPercent: 100},
				},
			},
			BotDeployment: &configpb.BotDeployment{
				Stable: &configpb.BotDeployment_BotPackage{
					Server:  "https://example.com",
					Pkg:     "some/pkg",
					Version: "stable",
				},
				Canary: &configpb.BotDeployment_BotPackage{
					Server:  "https://example.com",
					Pkg:     "some/pkg",
					Version: "canary",
				},
				CanaryPercent: 20,
			},
		})), should.BeNil)
	})

	ftt.Run("Errors", t, func(t *ftt.Test) {
		testCases := []struct {
			cfg *configpb.SettingsCfg
			err string
		}{
			{
				cfg: &configpb.SettingsCfg{BotDeathTimeoutSecs: -100},
				err: "(bot_death_timeout_secs): must be non-negative, got -100",
			},
			{
				cfg: &configpb.SettingsCfg{ReusableTaskAgeSecs: -100},
				err: "(reusable_task_age_secs): must be non-negative, got -100",
			},
			{
				cfg: &configpb.SettingsCfg{DisplayServerUrlTemplate: "example.com"},
				err: "(display_server_url_template): must have exactly one `%s` term",
			},
			{
				cfg: &configpb.SettingsCfg{DisplayServerUrlTemplate: "example.com/%s/%s"},
				err: "(display_server_url_template): must have exactly one `%s` term",
			},
			{
				cfg: &configpb.SettingsCfg{DisplayServerUrlTemplate: "example.com/%s"},
				err: "(display_server_url_template): must be an https:// URL",
			},
			{
				cfg: &configpb.SettingsCfg{DisplayServerUrlTemplate: "https://example.com/%s/%%"},
				err: `(display_server_url_template): parse "https://example.com/.../%": invalid URL escape "%"`,
			},
			{
				cfg: &configpb.SettingsCfg{ExtraChildSrcCspUrl: []string{"example.com"}},
				err: "(extra_child_src_csp_url): must be an https:// URL",
			},
			{
				cfg: &configpb.SettingsCfg{
					Cipd: &configpb.CipdSettings{
						DefaultClientPackage: &configpb.CipdPackage{
							PackageName: "some/pkg/${platform}",
							Version:     "some:version",
						},
					},
				},
				err: "(cipd / default_server): this is a required field",
			},
			{
				cfg: &configpb.SettingsCfg{
					Cipd: &configpb.CipdSettings{
						DefaultServer: "https://something.example.com",
						DefaultClientPackage: &configpb.CipdPackage{
							PackageName: "some/pkg/${zzz}",
							Version:     "some:version",
						},
					},
				},
				err: `(cipd / default_client_package / package_name): bad package name template "some/pkg/${zzz}": unknown variable "${zzz}"`,
			},
			{
				cfg: &configpb.SettingsCfg{
					Cipd: &configpb.CipdSettings{
						DefaultServer: "https://something.example.com",
						DefaultClientPackage: &configpb.CipdPackage{
							PackageName: "some/pkg//pkg",
							Version:     "some:version",
						},
					},
				},
				err: `(cipd / default_client_package / package_name): invalid package name "some/pkg//pkg": must be a slash-separated path where each component matches "[a-z0-9_\-\.]+"`,
			},
			{
				cfg: &configpb.SettingsCfg{
					Cipd: &configpb.CipdSettings{
						DefaultServer: "https://something.example.com",
						DefaultClientPackage: &configpb.CipdPackage{
							PackageName: "some/pkg/${platform}",
							Version:     "???",
						},
					},
				},
				err: `(cipd / default_client_package / version): bad version "???": not an instance ID, a ref or a tag`,
			},
			{
				cfg: &configpb.SettingsCfg{
					Resultdb: &configpb.ResultDBSettings{
						Server: "example.com",
					},
				},
				err: "(resultdb / server): must be an https:// URL",
			},
			{
				cfg: &configpb.SettingsCfg{
					Cas: &configpb.CASSettings{
						ViewerServer: "example.com",
					},
				},
				err: "(cas / viewer_server): must be an https:// URL",
			},
			{
				cfg: &configpb.SettingsCfg{
					TrafficMigration: &configpb.TrafficMigration{
						Routes: []*configpb.TrafficMigration_Route{
							{Name: "/prpc/service/method1", RouteToGoPercent: 0},
							{Name: "/prpc/service/method2", RouteToGoPercent: 50},
							{Name: "/prpc/service/method1", RouteToGoPercent: 100},
						},
					},
				},
				err: `(traffic_migration / "/prpc/service/method1"): duplicate route`,
			},
			{
				cfg: &configpb.SettingsCfg{
					TrafficMigration: &configpb.TrafficMigration{
						Routes: []*configpb.TrafficMigration_Route{
							{Name: "zzz/service/method1", RouteToGoPercent: 0},
						},
					},
				},
				err: `(traffic_migration / "zzz/service/method1"): route name should start with /`,
			},
			{
				cfg: &configpb.SettingsCfg{
					TrafficMigration: &configpb.TrafficMigration{
						Routes: []*configpb.TrafficMigration_Route{
							{Name: "/prpc/service/method1", RouteToGoPercent: -1},
						},
					},
				},
				err: `(traffic_migration / "/prpc/service/method1"): route_to_go_percent should be in range [0, 100]`,
			},
			{
				cfg: &configpb.SettingsCfg{
					TrafficMigration: &configpb.TrafficMigration{
						Routes: []*configpb.TrafficMigration_Route{
							{Name: "/prpc/service/method1", RouteToGoPercent: 101},
						},
					},
				},
				err: `(traffic_migration / "/prpc/service/method1"): route_to_go_percent should be in range [0, 100]`,
			},
			{
				cfg: &configpb.SettingsCfg{
					BotDeployment: &configpb.BotDeployment{},
				},
				err: `(bot_deployment): missing required "stable" section`,
			},
			{
				cfg: &configpb.SettingsCfg{
					BotDeployment: &configpb.BotDeployment{
						Stable: &configpb.BotDeployment_BotPackage{
							Server:  "not-https",
							Pkg:     "some/pkg",
							Version: "latest",
						},
					},
				},
				err: `(bot_deployment / stable / server): invalid URL "not-https"`,
			},
			{
				cfg: &configpb.SettingsCfg{
					BotDeployment: &configpb.BotDeployment{
						Stable: &configpb.BotDeployment_BotPackage{
							Server:  "https://example.com",
							Pkg:     "some/pkg/${platform}", // needs to be a concrete package
							Version: "latest",
						},
					},
				},
				err: `(bot_deployment / stable / pkg): package name template "some/pkg/${platform}" is not allowed here`,
			},
			{
				cfg: &configpb.SettingsCfg{
					BotDeployment: &configpb.BotDeployment{
						Stable: &configpb.BotDeployment_BotPackage{
							Server:  "https://example.com",
							Pkg:     "some/pkg",
							Version: "!!!",
						},
					},
				},
				err: `(bot_deployment / stable / version): bad version "!!!": not an instance ID, a ref or a tag`,
			},
			{
				cfg: &configpb.SettingsCfg{
					BotDeployment: &configpb.BotDeployment{
						Stable: &configpb.BotDeployment_BotPackage{
							Server:  "https://example.com",
							Pkg:     "some/pkg",
							Version: "latest",
						},
						Canary: &configpb.BotDeployment_BotPackage{
							Server:  "not-https",
							Pkg:     "some/pkg",
							Version: "latest",
						},
					},
				},
				err: `(bot_deployment / canary / server): invalid URL "not-https"`,
			},
			{
				cfg: &configpb.SettingsCfg{
					BotDeployment: &configpb.BotDeployment{
						Stable: &configpb.BotDeployment_BotPackage{
							Server:  "https://example.com",
							Pkg:     "some/pkg",
							Version: "latest",
						},
						CanaryPercent: -1,
					},
				},
				err: `(bot_deployment): canary_percent must be between 0 and 100`,
			},
		}
		for _, cs := range testCases {
			assert.Loosely(t, call(cs.cfg), should.Match([]string{`in "settings.cfg" ` + cs.err}))
		}
	})
}
