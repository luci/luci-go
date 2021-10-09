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

// package migrationcfg contains config-related CQD migration funcs.
//
// This package is supposed to be deleted with its parent package after the end
// of migration.
package migrationcfg

import (
	"context"
	"regexp"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/srvcfg"
)

// IsCVInChargeOfPostingStartMessage returns true if CV is in charge of posting
// the start message on Gerrit CL.
func IsCVInChargeOfPostingStartMessage(ctx context.Context, env *common.Env, luciProject string) (bool, error) {
	res, err := isCQDInChargeOfPostingStartMessage(ctx, env, luciProject)
	return !res, err
}

func isCQDInChargeOfPostingStartMessage(ctx context.Context, env *common.Env, luciProject string) (bool, error) {
	cfg, err := srvcfg.GetMigrationConfig(ctx)
	if err != nil {
		return false, err
	}
	u := cfg.GetUseCvStatus()
	// NOTE: as https://crrev.com/i/4173580 documents in detail,
	// `use_cv_runs` is abused to actually mean "CQDaemon posts start message".
	if !matches(ctx, luciProject, u.GetProjectRegexp(), u.GetProjectRegexpExclude(), "use_cv_runs") {
		return false, nil
	}

	var all []*migrationpb.Settings_ApiHost
	var prod []*migrationpb.Settings_ApiHost
	for _, a := range cfg.GetApiHosts() {
		if !matches(ctx, luciProject, a.GetProjectRegexp(), a.GetProjectRegexpExclude(), "api_hosts") {
			continue
		}
		all = append(all, a)
		if a.GetProd() {
			prod = append(prod, a)
		}
	}

	switch {
	case len(all) == 1:
		return env.LogicalHostname == all[0].GetHost(), nil
	case len(prod) == 1:
		return env.LogicalHostname == prod[0].GetHost(), nil
	case len(prod) > 1:
		logging.Warningf(ctx, "%q matches %d prod api_hosts %s", luciProject, len(prod), prod)
	case len(all) > 1:
		logging.Debugf(ctx, "%q matches only %d non-prod hosts %s", luciProject, len(all), all)
	}
	return false, nil
}

// matches returns true iff the LUCI project matches at least one include and
// non of the excludes.
//
// Errs on the side of not accidentally matching a project, thus:
//   * invalid includes are ignored;
//   * invalid excludes are considered matching.
//
// Invalid regexps are logged with the given field name.
func matches(ctx context.Context, luciProject string, include, exclude []string, field string) bool {
	var errs errors.MultiError
	defer func() {
		if len(errs) > 0 {
			logging.Warningf(ctx, "Bad migration settings %q: %s", field, errs)
		}
	}()

	for _, re := range exclude {
		switch yes, err := matchesRegexp(luciProject, re); {
		case err != nil:
			errs = append(errs, err)
			return false
		case yes:
			return false
		}
	}
	for _, re := range include {
		switch yes, err := matchesRegexp(luciProject, re); {
		case err != nil:
			errs = append(errs, err)
		case yes:
			return true
		}
	}
	return false // by default, CQD handles everything.
}

func matchesRegexp(project, re string) (bool, error) {
	re = "^" + re + "$"
	r, err := regexp.Compile(re)
	if err != nil {
		return false, errors.Annotate(err, "invalid regexp %q", re).Err()
	}
	return r.Match([]byte(project)), nil
}
