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

package poller

import (
	"sort"
	"strings"

	"go.chromium.org/luci/common/data/stringset"

	"go.chromium.org/luci/cv/internal/config"
)

const (
	// maxReposPerQuery is the maximum number of Gerrit projects that will be
	// polled in 1 OR-query.
	//
	// Gerrit-on-googlesource has a 100 term limit in query after expansion,
	// but some query terms are actually composite.
	maxReposPerQuery = 20

	// minReposPerPrefixQuery is the minimum number of Gerrit projects with shared
	// prefix to poll using shared prefix query.
	//
	// Because prefix may match other projects not actually relevant to the LUCI
	// project, this constant should not be too low.
	//
	// TODO(tandrii): consider querying Gerrit host to see how many projects
	// actually match this prefix.
	minReposPerPrefixQuery = 20
)

// partitionConfig partitions LUCI Project config into minimal number of
// SubPollers for efficient querying.
func partitionConfig(cgs []*config.ConfigGroup) []*SubPoller {
	// 1 LUCI project typically watches 1-2 GoB hosts.
	hosts := make([]string, 0, 2)
	repos := make(map[string]stringset.Set, 2)
	for _, cg := range cgs {
		for _, g := range cg.Content.GetGerrit() {
			host := config.GerritHost(g)
			if repos[host] == nil {
				hosts = append(hosts, host)
				repos[host] = stringset.New(len(g.GetProjects()))
			}
			for _, pr := range g.GetProjects() {
				repos[host].Add(pr.GetName())
			}
		}
	}
	sort.Strings(hosts)
	subpollers := make([]*SubPoller, 0, len(hosts))
	for _, host := range hosts {
		subpollers = append(subpollers,
			partitionHostRepos(host, repos[host].ToSlice(), maxReposPerQuery)...)
	}
	return subpollers
}

// partitionHostRepos partitions repos of the same Gerrit host into SubPollers.
// Mutates the passed repos slice.
func partitionHostRepos(host string, repos []string, maxReposPerQuery int) []*SubPoller {
	// Heuristic targeting ChromeOS like structure with lots of repos under
	// chromiumos/ prefix.
	byPrefix := make(map[string][]string, 2)
	for _, r := range repos {
		prefix := strings.SplitN(r, "/", 2)[0]
		byPrefix[prefix] = append(byPrefix[prefix], r)
	}
	prefixes := make([]string, len(byPrefix))
	for prefix := range byPrefix {
		prefixes = append(prefixes, prefix)
	}
	sort.Strings(prefixes)

	subpollers := make([]*SubPoller, 0, 1)
	remainingRepos := repos[:0] // re-use the slice.
	for _, prefix := range prefixes {
		if shared := byPrefix[prefix]; len(shared) < minReposPerPrefixQuery {
			remainingRepos = append(remainingRepos, shared...)
		} else {
			subpollers = append(subpollers, &SubPoller{
				Host:                host,
				CommonProjectPrefix: prefix,
			})
		}
	}
	if len(remainingRepos) == 0 {
		return subpollers
	}

	// Split remainingRepos into SubPollers minimizing max of repos per SubPoller.
	// Note that rounding up positive int division is (x-1)/y + 1.
	neededSubPollers := (len(remainingRepos)-1)/maxReposPerQuery + 1
	maxPerSubPoller := (len(remainingRepos)-1)/neededSubPollers + 1
	sort.Strings(remainingRepos)
	for {
		sp := &SubPoller{Host: host}
		switch l := len(remainingRepos); {
		case l == 0:
			return subpollers
		case l <= maxPerSubPoller:
			sp.OrProjects = remainingRepos
		default:
			sp.OrProjects = remainingRepos[:maxPerSubPoller]
		}
		subpollers = append(subpollers, sp)
		remainingRepos = remainingRepos[len(sp.GetOrProjects()):]
	}
}
