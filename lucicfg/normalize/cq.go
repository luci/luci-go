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

package normalize

import (
	"context"
	"sort"
	"strings"

	pb "go.chromium.org/luci/cq/api/config/v2"
)

// CQ normalizes cq.cfg config.
func CQ(c context.Context, cfg *pb.Config) error {
	// Normalize each ConfigGroup individually first.
	for _, cg := range cfg.ConfigGroups {
		for _, g := range cg.Gerrit {
			normalizeGerrit(g)
		}
		sort.Slice(cg.Gerrit, func(i, j int) bool {
			return cg.Gerrit[i].Url < cg.Gerrit[j].Url
		})
		if cg.Verifiers != nil {
			normalizeGerritCqAbility(cg.Verifiers.GerritCqAbility)
			normalizeTryjobs(cg.Verifiers.Tryjob)
		}
	}

	// Sort by some synthetic key. It doesn't really matter what it is exactly as
	// long as the final ordering is "stable" across multiple invocations. Note
	// that we rely here on sorting of Gerrit/Project/Refs done above.
	sortKeys := make(map[*pb.ConfigGroup]string, len(cfg.ConfigGroups))
	for _, cg := range cfg.ConfigGroups {
		sortKeys[cg] = cgSortKey(cg)
	}
	sort.Slice(cfg.ConfigGroups, func(i, j int) bool {
		return sortKeys[cfg.ConfigGroups[i]] < sortKeys[cfg.ConfigGroups[j]]
	})

	return nil
}

func normalizeGerrit(g *pb.ConfigGroup_Gerrit) {
	for _, p := range g.Projects {
		if len(p.RefRegexp) == 0 {
			p.RefRegexp = []string{"refs/heads/master"}
		} else {
			sort.Strings(p.RefRegexp)
		}
	}
	sort.Slice(g.Projects, func(i, j int) bool {
		return g.Projects[i].Name < g.Projects[j].Name
	})
}

func normalizeGerritCqAbility(v *pb.Verifiers_GerritCQAbility) {
	if v == nil {
		return
	}
	sort.Strings(v.CommitterList)
	sort.Strings(v.DryRunAccessList)
}

func normalizeTryjobs(v *pb.Verifiers_Tryjob) {
	if v == nil {
		return
	}
	sort.Slice(v.Builders, func(i, j int) bool {
		return v.Builders[i].Name < v.Builders[j].Name
	})
	for _, b := range v.Builders {
		sort.Strings(b.LocationRegexp)
		sort.Strings(b.LocationRegexpExclude)
	}
	if v.RetryConfig == nil {
		v.RetryConfig = &pb.Verifiers_Tryjob_RetryConfig{}
	}
}

func cgSortKey(cg *pb.ConfigGroup) string {
	out := strings.Builder{}
	for _, g := range cg.Gerrit {
		for _, p := range g.Projects {
			for _, r := range p.RefRegexp {
				out.WriteString(g.Url)
				out.WriteRune('\n')
				out.WriteString(p.Name)
				out.WriteRune('\n')
				out.WriteString(r)
				out.WriteRune('\n')
			}
		}
	}
	return out.String()
}
