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

package projectscope

import (
	"sort"
	"strings"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config/validation"
)

func validateProjectsCfg(ctx *validation.Context, cfg *config.ProjectsCfg, useStagingEmail bool) {
	ctx.Enter("identity configuration")
	defer ctx.Exit()

	// Service account email => list of teams that use it.
	idents := map[string]stringset.Set{}

	for _, project := range cfg.Projects {
		ctx.Enter("Validate project %s IdentityConfig", project.Id)
		if email := projectIdentityEmail(project.IdentityConfig, useStagingEmail); email != "" {
			if set := idents[email]; set == nil {
				idents[email] = stringset.NewFromSlice(project.OwnedBy)
			} else {
				set.Add(project.OwnedBy)
			}
		}
		ctx.Exit()
	}

	// Error when projects share identities with different project owners.
	var shared []string
	for ident, projs := range idents {
		if len(projs) > 1 {
			shared = append(shared, ident)
		}
	}
	sort.Strings(shared)
	for _, ident := range shared {
		ctx.Errorf(
			"project-scoped account %s is used by multiple teams: %s",
			ident, strings.Join(idents[ident].ToSortedSlice(), ", "))
	}
}
