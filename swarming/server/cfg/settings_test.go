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
	"go.chromium.org/luci/config/validation"

	configpb "go.chromium.org/luci/swarming/proto/config"

	. "github.com/smartystreets/goconvey/convey"
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

	Convey("Empty", t, func() {
		So(call(&configpb.SettingsCfg{}), ShouldBeNil)
	})

	Convey("Good", t, func() {
		So(call(withDefaultSettings(&configpb.SettingsCfg{
			DisplayServerUrlTemplate: "https://something.example.com/swarming/task/%s",
			ExtraChildSrcCspUrl:      []string{"https://something.example.com/raw/build/"},
			Cipd: &configpb.CipdSettings{
				DefaultServer: "https://something.example.com",
				DefaultClientPackage: &configpb.CipdPackage{
					PackageName: "some/pkg/${platform}",
					Version:     "some:version",
				},
			},
		})), ShouldBeNil)
	})

	Convey("Errors", t, func() {
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
				err: `(cipd / default_client_package): bad package name template "some/pkg/${zzz}": unknown variable "${zzz}"`,
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
				err: `(cipd / default_client_package): invalid package name "some/pkg//pkg": must be a slash-separated path where each component matches "[a-z0-9_\-\.]+"`,
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
				err: `(cipd / default_client_package): bad version "???": not an instance ID, a ref or a tag`,
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
		}
		for _, cs := range testCases {
			So(call(cs.cfg), ShouldResemble, []string{`in "settings.cfg" ` + cs.err})
		}
	})
}
