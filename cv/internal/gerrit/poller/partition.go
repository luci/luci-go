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
	"crypto/sha256"
	"encoding/binary"
	"sort"
	"strings"

	"go.chromium.org/luci/common/data/stringset"

	"go.chromium.org/luci/cv/internal/configs/prjcfg"
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
// queries for efficient querying.
func partitionConfig(cgs []*prjcfg.ConfigGroup) []*QueryState {
	// 1 LUCI project typically watches 1-2 GoB hosts.
	hosts := make([]string, 0, 2)
	repos := make(map[string]stringset.Set, 2)
	for _, cg := range cgs {
		for _, g := range cg.Content.GetGerrit() {
			host := prjcfg.GerritHost(g)
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
	queries := make([]*QueryState, 0, len(hosts))
	for _, host := range hosts {
		queries = append(queries, partitionHostRepos(host, repos[host].ToSlice(), maxReposPerQuery)...)
	}
	return queries
}

// partitionHostRepos partitions repos of the same Gerrit host into queries.
// Mutates the passed repos slice.
func partitionHostRepos(host string, repos []string, maxReposPerQuery int) []*QueryState {
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

	queries := make([]*QueryState, 0, 1)
	remainingRepos := repos[:0] // re-use the slice.
	for _, prefix := range prefixes {
		if shared := byPrefix[prefix]; len(shared) < minReposPerPrefixQuery {
			remainingRepos = append(remainingRepos, shared...)
		} else {
			queries = append(queries, &QueryState{
				Host:                host,
				CommonProjectPrefix: prefix,
			})
		}
	}
	if len(remainingRepos) == 0 {
		return queries
	}

	// Split remainingRepos into queries minimizing max of repos per query.
	// TODO(crbug/1163177): take ref_regexp into account, since most LUCI projects
	// watching >1 project use the same ref(s) for each, which in turn allows to
	// specify `branch:` search term for Gerrit.
	// Note that rounding up positive int division is (x-1)/y + 1.
	neededQueries := (len(remainingRepos)-1)/maxReposPerQuery + 1
	maxPerQuery := (len(remainingRepos)-1)/neededQueries + 1
	sort.Strings(remainingRepos)
	for {
		q := &QueryState{Host: host}
		switch l := len(remainingRepos); {
		case l == 0:
			return queries
		case l <= maxPerQuery:
			q.OrProjects = remainingRepos
		default:
			q.OrProjects = remainingRepos[:maxPerQuery]
		}
		queries = append(queries, q)
		remainingRepos = remainingRepos[len(q.GetOrProjects()):]
	}
}

func reuseIfPossible(old, proposed []*QueryState) (use, discarded []*QueryState) {
	// Crypto quality hash is used to to infer equality.
	//
	// Each string is emitted as (<len>, string).
	// List of OrProjects is prefixed by its length.
	hash := func(s *QueryState) string {
		h := sha256.New()

		buf := make([]byte, 10) // varint uint64 will definitely fit.
		writeInt := func(l int) {
			n := binary.PutUvarint(buf, uint64(l))
			h.Write(buf[:n])
		}

		writeStr := func(s string) {
			writeInt(len(s))
			h.Write([]byte(s))
		}

		writeStr(s.GetHost())
		writeStr(s.GetCommonProjectPrefix())
		writeInt(len(s.GetOrProjects()))
		for _, p := range s.GetOrProjects() {
			writeStr(p)
		}
		return string(h.Sum(nil))
	}

	m := make(map[string]*QueryState, len(old))
	for _, o := range old {
		m[hash(o)] = o
	}
	for _, p := range proposed {
		h := hash(p)
		if o, exists := m[h]; exists {
			use = append(use, o)
			delete(m, h)
		} else {
			use = append(use, p)
		}
	}
	for _, o := range m {
		discarded = append(discarded, o)
	}
	return
}
