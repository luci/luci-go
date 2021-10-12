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

package validation

import (
	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/config/validation"

	migrationpb "go.chromium.org/luci/cv/api/migration"
)

// validateMigrationSettings validates a migration-settings file.
//
// Validation result is returned via validation ctx, while error returned
// directly implies only a bug in this code.
func validateMigrationSettings(ctx *validation.Context, configSet, path string, content []byte) error {
	ctx.SetFile(path)
	cfg := migrationpb.Settings{}
	if err := prototext.Unmarshal(content, &cfg); err != nil {
		ctx.Error(err)
		return nil
	}
	for i, a := range cfg.GetApiHosts() {
		ctx.Enter("api_hosts #%d", i+1)
		switch h := a.GetHost(); h {
		case "luci-change-verifier-dev.appspot.com":
		case "luci-change-verifier.appspot.com":
		default:
			ctx.Errorf("invalid host (given: %q)", h)
		}
		validateRegexp(ctx, "project_regexp", a.GetProjectRegexp())
		validateRegexp(ctx, "project_regexp_exclude", a.GetProjectRegexpExclude())
		ctx.Exit()
	}
	if u := cfg.GetUseCvStartMessage(); u != nil {
		ctx.Enter("use_cv_start_message")
		validateRegexp(ctx, "project_regexp", u.GetProjectRegexp())
		validateRegexp(ctx, "project_regexp_exclude", u.GetProjectRegexpExclude())
		ctx.Exit()
	}
	return nil
}
