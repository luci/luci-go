// Copyright 2020 The LUCI Authors.
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

// Package cfgmatcher efficiently matches a CL to 0+ ConfigGroupID for a single
// LUCI project.
package cfgmatcher

import (
	"fmt"
	"regexp"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/config"
)

// MakeGroup returns a new Group based on the Gerrit Project section of a
// ConfigGroup.
func MakeGroup(g *config.ConfigGroup, p *cfgpb.ConfigGroup_Gerrit_Project) *Group {
	return &Group{
		Id:       string(g.ID),
		Include:  p.RefRegexp,
		Exclude:  p.RefRegexpExclude,
		Fallback: g.Content.Fallback == cfgpb.Toggle_YES,
	}
}

// Match returns matching groups, obeying fallback config.
//
// If there are two groups that match, one fallback and one non-fallback, the
// non-fallback group is the one to use. The fallback group will be used if it's
// the only group that matches.
func (gs *Groups) Match(ref string) []*Group {
	var ret []*Group
	var fallback *Group
	for _, g := range gs.GetGroups() {
		switch {
		case !g.Match(ref):
			continue
		case g.GetFallback() && fallback != nil:
			// Valid config require at most 1 fallback group in a LUCI project.
			panic(fmt.Errorf("invalid Groups: %s and %s are both fallback", fallback, g))
		case g.GetFallback():
			fallback = g
		default:
			ret = append(ret, g)
		}
	}
	if len(ret) == 0 && fallback != nil {
		ret = []*Group{fallback}
	}
	return ret
}

// Match returns true iff ref matches given Group.
func (g *Group) Match(ref string) bool {
	return matchesAny(g.GetInclude(), ref) && !matchesAny(g.GetExclude(), ref)
}

// matchesAny returns true iff s matches any of the patterns.
//
// It is assumed that all patterns have been pre-validated and
// are valid regexps.
func matchesAny(patterns []string, s string) bool {
	for _, pattern := range patterns {
		if regexp.MustCompile(pattern).MatchString(s) {
			return true
		}
	}
	return false
}
