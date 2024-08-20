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
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/validate"
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/source_index/internal/validationutil"
	configpb "go.chromium.org/luci/source_index/proto/config"
)

func validateConfig(ctx *validation.Context, cfg *configpb.Config) {
	ctx.Enter("hosts")
	if len(cfg.Hosts) == 0 {
		ctx.Error(validate.Unspecified())
	}
	ctx.Exit()

	for i, host := range cfg.Hosts {
		ctx.Enter("hosts #%d", i+1)
		validateHost(ctx, host)
		ctx.Exit()
	}
}

func validateHost(ctx *validation.Context, host *configpb.Config_Host) {
	ctx.Enter("host")
	if err := gitiles.ValidateRepoHost(host.Host); err != nil {
		ctx.Error(err)
	}
	ctx.Exit()

	ctx.Enter("repositories")
	if len(host.Repositories) == 0 {
		ctx.Error(validate.Unspecified())
	}
	ctx.Exit()

	for i, repository := range host.Repositories {
		ctx.Enter("repositories #%d", i+1)
		validateRepositories(ctx, repository)
		ctx.Exit()
	}
}

func validateRepositories(ctx *validation.Context, repository *configpb.Config_Host_Repository) {
	ctx.Enter("name")
	if err := validationutil.ValidateRepoName(repository.Name); err != nil {
		ctx.Error(err)
	}
	ctx.Exit()

	ctx.Enter("include_ref_regexes")
	if len(repository.IncludeRefRegexes) == 0 {
		ctx.Error(validate.Unspecified())
	}
	ctx.Exit()

	for i, regex := range repository.IncludeRefRegexes {
		ctx.Enter("include_ref_regexes #%d", i+1)
		if err := validate.Regexp(regex); err != nil {
			ctx.Error(err)
		}
		if regex == "" {
			ctx.Errorf("regex should not be empty")
		}
		ctx.Exit()
	}
}
