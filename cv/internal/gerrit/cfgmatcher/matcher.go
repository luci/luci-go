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
	"context"
	"fmt"
	"regexp"
	"strings"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
)

// Matcher effieciently find matching ConfigGroupID for Gerrit CLs.
type Matcher struct {
	state                *MatcherState
	cachedConfigGroupIDs []prjcfg.ConfigGroupID
}

// LoadMatcher instantiates Matcher from config stored in Datastore.
func LoadMatcher(ctx context.Context, luciProject, configHash string) (*Matcher, error) {
	meta, err := prjcfg.GetHashMeta(ctx, luciProject, configHash)
	if err != nil {
		return nil, err
	}
	return LoadMatcherFrom(ctx, meta)
}

// LoadMatcherFrom instantiates Matcher from the given config.Meta.
func LoadMatcherFrom(ctx context.Context, meta prjcfg.Meta) (*Matcher, error) {
	configGroups, err := meta.GetConfigGroups(ctx)
	if err != nil {
		return nil, err
	}
	return LoadMatcherFromConfigGroups(ctx, configGroups, &meta), nil
}

// LoadMatcherFromConfigGroups instantiates Matcher.
//
// There must be at least 1 config group, which is true for all valid CV
// configs.
//
// meta, if not nil, must have been used to load the given ConfigGroups. It's an
// optimization to re-use memory since most callers typically have it.
func LoadMatcherFromConfigGroups(ctx context.Context, configGroups []*prjcfg.ConfigGroup, meta *prjcfg.Meta) *Matcher {
	m := &Matcher{
		state: &MatcherState{
			// 1-2 Gerrit hosts is typical as of 2020.
			Hosts:            make(map[string]*MatcherState_Projects, 2),
			ConfigGroupNames: make([]string, len(configGroups)),
		},
	}
	if meta != nil {
		m.state.ConfigHash = meta.Hash()
		m.cachedConfigGroupIDs = meta.ConfigGroupIDs
	} else {
		m.state.ConfigHash = configGroups[0].ID.Hash()
		m.cachedConfigGroupIDs = make([]prjcfg.ConfigGroupID, len(configGroups))
		for i, cg := range configGroups {
			m.cachedConfigGroupIDs[i] = cg.ID
		}
	}

	for i, cg := range configGroups {
		m.state.ConfigGroupNames[i] = cg.ID.Name()
		for _, gerrit := range cg.Content.GetGerrit() {
			host := prjcfg.GerritHost(gerrit)
			var projectsMap map[string]*Groups
			if ps, ok := m.state.GetHosts()[host]; ok {
				projectsMap = ps.GetProjects()
			} else {
				// Either 1 Gerrit project or lots of them is typical as 2020.
				projectsMap = make(map[string]*Groups, 1)
				m.state.GetHosts()[host] = &MatcherState_Projects{Projects: projectsMap}
			}

			for _, p := range gerrit.GetProjects() {
				g := MakeGroup(cg, p)
				// Don't store exact ID, it can be computed from the rest of matcher
				// state if index is known. This reduces RAM usage after
				// serialize/deserialize cycle.
				g.Id = ""
				g.Index = int32(i)
				if groups, ok := projectsMap[p.GetName()]; ok {
					groups.Groups = append(groups.GetGroups(), g)
				} else {
					projectsMap[p.GetName()] = &Groups{Groups: []*Group{g}}
				}
			}
		}
	}
	return m
}

func (m *Matcher) Serialize() ([]byte, error) {
	return proto.Marshal(m.state)
}

func Deserialize(buf []byte) (*Matcher, error) {
	m := &Matcher{state: &MatcherState{}}
	if err := proto.Unmarshal(buf, m.state); err != nil {
		return nil, errors.Fmt("failed to Deserialize Matcher: %w", err)
	}
	m.cachedConfigGroupIDs = make([]prjcfg.ConfigGroupID, len(m.state.ConfigGroupNames))
	hash := m.state.GetConfigHash()
	for i, name := range m.state.ConfigGroupNames {
		m.cachedConfigGroupIDs[i] = prjcfg.MakeConfigGroupID(hash, name)
	}
	return m, nil
}

// Match returns ConfigGroupIDs matched for a given triple.
func (m *Matcher) Match(host, project, ref string) []prjcfg.ConfigGroupID {
	ps, ok := m.state.GetHosts()[host]
	if !ok {
		return nil
	}
	gs, ok := ps.GetProjects()[project]
	if !ok {
		return nil
	}
	matched := gs.Match(ref)
	if len(matched) == 0 {
		return nil
	}
	ret := make([]prjcfg.ConfigGroupID, len(matched))
	for i, g := range matched {
		ret[i] = m.cachedConfigGroupIDs[g.GetIndex()]
	}
	return ret
}

// ConfigHash returns ConfigHash for which Matcher does matching.
func (m *Matcher) ConfigHash() string {
	return m.state.GetConfigHash()
}

// TODO(tandrii): add "main" branch too to ease migration once either:
//   - CQDaemon is no longer involved,
//   - CQDaemon does the same at the same time.
var defaultRefRegexpInclude = []string{"refs/heads/master"}
var defaultRefRegexpExclude = []string{"^$" /* matches nothing */}

// MakeGroup returns a new Group based on the Gerrit Project section of a
// ConfigGroup.
func MakeGroup(g *prjcfg.ConfigGroup, p *cfgpb.ConfigGroup_Gerrit_Project) *Group {
	var inc, exc []string
	if inc = p.GetRefRegexp(); len(inc) == 0 {
		inc = defaultRefRegexpInclude
	}
	if exc = p.GetRefRegexpExclude(); len(exc) == 0 {
		exc = defaultRefRegexpExclude
	}
	return &Group{
		Id:       string(g.ID),
		Include:  disjunctiveOfRegexps(inc),
		Exclude:  disjunctiveOfRegexps(exc),
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
	if !regexp.MustCompile(g.GetInclude()).MatchString(ref) {
		return false
	}
	return !regexp.MustCompile(g.GetExclude()).MatchString(ref)
}

func disjunctiveOfRegexps(rs []string) string {
	sb := strings.Builder{}
	sb.WriteString("^(")
	for i, r := range rs {
		if i > 0 {
			sb.WriteRune('|')
		}
		sb.WriteRune('(')
		sb.WriteString(r)
		sb.WriteRune(')')
	}
	sb.WriteString(")$")
	return sb.String()
}
