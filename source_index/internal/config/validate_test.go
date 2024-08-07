// Copyright 2024 The LUCI Authors.
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
	"testing"

	"go.chromium.org/luci/config/validation"

	configpb "go.chromium.org/luci/source_index/proto/config"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateConfig(t *testing.T) {
	t.Parallel()

	Convey("ValidateConfig", t, func() {
		var cfg = &configpb.Config{
			Hosts: []*configpb.Config_Host{
				{
					Host: "chromium.googlesource.com",
					Repositories: []*configpb.Config_Host_Repository{
						{
							Name:              "repo1",
							IncludeRefRegexes: []string{"^main$", "^main2$"},
						},
						{
							Name:              "repo2",
							IncludeRefRegexes: []string{"^main3$", "^main4$"},
						},
					},
				},
				{
					Host: "another-host.googlesource.com",
					Repositories: []*configpb.Config_Host_Repository{
						{
							Name:              "valid/repo",
							IncludeRefRegexes: []string{"^main$", "^main2$"},
						},
						{
							Name:              "another-repo2",
							IncludeRefRegexes: []string{"^main3$", "^main4$"},
						},
					},
				},
			},
		}

		validateCfg := func(cfg *configpb.Config) error {
			c := validation.Context{Context: context.Background()}
			validateConfig(&c, cfg)
			return c.Finalize()
		}

		Convey("accept valid config", func() {
			So(validateCfg(cfg), ShouldBeNil)
		})

		Convey("reject invalid gitiles host", func() {
			cfg.Hosts[1].Host = "chromium.not-googlesource.com"
			So(validateCfg(cfg), ShouldErrLike, "hosts #2 / host", "is not a valid Gitiles host")
		})

		Convey("reject invalid repo", func() {
			cfg.Hosts[0].Repositories[0].Name = "not-a-valid-repo/"
			So(validateCfg(cfg), ShouldErrLike, "hosts #1 / repositories #1 / name", "does not match pattern")
		})

		Convey("reject invalid regex", func() {
			cfg.Hosts[0].Repositories[0].IncludeRefRegexes[1] = "^main($"
			So(validateCfg(cfg), ShouldErrLike, "hosts #1 / repositories #1 / include_ref_regexes #2", "error parsing regexp")
		})

		Convey("reject empty regex", func() {
			cfg.Hosts[0].Repositories[0].IncludeRefRegexes[1] = ""
			So(validateCfg(cfg), ShouldErrLike, "hosts #1 / repositories #1 / include_ref_regexes #2", "regex should not be empty")
		})
	})
}
