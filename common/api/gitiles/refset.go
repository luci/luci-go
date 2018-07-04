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

package gitiles

import (
	"regexp"
	"strings"
	"sync"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/config/validation"
	"golang.org/x/net/context"
)

// RefSet efficiently resolves many refs, supporting regexps.
//
// RefSet groups refs by prefix and issues 1 refs RPC per prefix. This is more
// efficient that a single refs RPC for "refs/" prefix, because it would return
// all refs of the repo, incl. potentially huge number of refs in refs/changes/.
type RefSet struct {
	byPrefix map[string]*refSetPrefix
}

// NewRefSet creates an instance of the RefSet.
//
// Each entry in the refs parameter can be either
//   * a fully-qualified ref with at least 2 slashes, e.g. `refs/heads/master`,
//     `refs/tags/v1.2.3`, or
//   * a regular expression with "regexp:" prefix to match multiple refs, e.g.
//     `regexp:refs/heads/.*` or `regexp:refs/branch-heads/\d+\.\d+`.
//
// The regular expression must have:
//   * a literal prefix with at least 2 slashes, e.g. `refs/release-\d+/foo` is
//     not allowed, because the literal prefix `refs/release-` contains only one
//     slash, and
//   * must not start with ^ or end with $ as they will be added automatically.
//
// See also ValidateRefSet function.
func NewRefSet(refs []string) RefSet {
	w := RefSet{map[string]*refSetPrefix{}}
	nsRegexps := map[string][]string{}
	for _, ref := range refs {
		prefix, suffix, regexp := parseRef(ref)
		if _, exists := w.byPrefix[prefix]; !exists {
			w.byPrefix[prefix] = &refSetPrefix{prefix: prefix}
		}

		switch {
		case (suffix == "") == (regexp == ""):
			panic("exactly one must be defined")
		case regexp != "":
			nsRegexps[prefix] = append(nsRegexps[prefix], regexp)
		case suffix != "":
			w.byPrefix[prefix].addSuffix(suffix)
		}
	}

	for prefix, regexps := range nsRegexps {
		w.byPrefix[prefix].descendantRegexp = regexp.MustCompile(
			"^(" + strings.Join(regexps, ")|(") + ")$")
	}

	return w
}

// Has checks if a specific ref is in this set.
func (w RefSet) Has(ref string) bool {
	for prefix, wrp := range w.byPrefix {
		nsPrefix := prefix + "/"
		if strings.HasPrefix(ref, nsPrefix) && wrp.hasSuffix(ref[len(nsPrefix):]) {
			return true
		}
	}

	return false
}

// Resolve returns map from watched ref to its resolved tip.
//
// Refs, which don't exist or are not visible to requester, won't be included in
// the map.
func (w RefSet) Resolve(c context.Context, client gitiles.GitilesClient, project string) (map[string]string, error) {
	lock := sync.Mutex{} // for concurrent writes to the map
	refTips := map[string]string{}
	return refTips, parallel.FanOutIn(func(work chan<- func() error) {
		for prefix := range w.byPrefix {
			prefix := prefix
			work <- func() error {
				resp, err := client.Refs(c, &gitiles.RefsRequest{Project: project, RefsPath: prefix})
				if err != nil {
					return err
				}
				lock.Lock()
				defer lock.Unlock()
				for ref, tip := range resp.Revisions {
					if w.Has(ref) {
						refTips[ref] = tip
					}
				}
				return nil
			}
		}
	})
}

// ValidateRefSet validates strings representing a set of refs.
//
// It ensures that passed strings match the requirements as described in the
// documentation for the NewRefSet function. It is designed to work with config
// validation logic, hence one needs to pass in the validation.Context as well.
func ValidateRefSet(c *validation.Context, refs []string) {
	for _, ref := range refs {
		if strings.HasPrefix(ref, "regexp:") {
			validateRegexpRef(c, ref)
			continue
		}

		if !strings.HasPrefix(ref, "refs/") {
			c.Errorf("ref must start with 'refs/' not %q", ref)
		}

		if strings.Count(ref, "/") < 2 {
			c.Errorf(`fewer than 2 slashes in ref %q`, ref)
		}
	}
}

type refSetPrefix struct {
	prefix string // no trailing "/".
	// exactChildren is a set of immediate children, not grandchildren. i.e., may
	// contain 'child', but not 'grand/child', which would be contained in
	// refSetPrefix for (prefix + "/child").
	exactChildren stringset.Set
	// descendantRegexp is a regular expression matching all descendants.
	descendantRegexp *regexp.Regexp
}

func (w refSetPrefix) hasSuffix(suffix string) bool {
	switch {
	case w.descendantRegexp != nil && w.descendantRegexp.MatchString(suffix):
		return true
	case w.exactChildren == nil:
		return false
	default:
		return w.exactChildren.Has(suffix)
	}
}

func (w *refSetPrefix) addSuffix(suffix string) {
	if w.exactChildren == nil {
		w.exactChildren = stringset.New(1)
	}
	w.exactChildren.Add(suffix)
}

func validateRegexpRef(c *validation.Context, ref string) {
	c.Enter(ref)
	defer c.Exit()
	reStr := strings.TrimPrefix(ref, "regexp:")
	if strings.HasPrefix(reStr, "^") || strings.HasSuffix(reStr, "$") {
		c.Errorf("^ and $ qualifiers are added automatically, please remove them")
		return
	}
	r, err := regexp.Compile(reStr)
	if err != nil {
		c.Errorf("invalid regexp: %s", err)
		return
	}
	lp, complete := r.LiteralPrefix()
	if complete {
		c.Errorf("matches a single ref only, please use %q instead", lp)
	}
	if strings.Count(lp, "/") < 2 {
		c.Errorf(`fewer than 2 slashes in literal prefix %q, e.g., `+
			`"refs/heads/\d+" is accepted because of "refs/heads/" is the `+
			`literal prefix, while "refs/.*" is too short`, lp)
	}
	refPrefix := lp[:strings.LastIndex(lp, "/")+1]
	if !strings.HasPrefix(reStr, refPrefix) {
		c.Errorf("does not start with its ref prefix %q", refPrefix)
	}
	if !strings.HasPrefix(refPrefix, "refs/") {
		c.Errorf(`ref prefix %q must start with "refs/"`, refPrefix)
	}
}

func parseRef(ref string) (prefix, literalSuffix, regexpSuffix string) {
	if strings.HasPrefix(ref, "regexp:") {
		refRegexp := strings.TrimPrefix(ref, "regexp:")
		descendantRegexp, err := regexp.Compile(refRegexp)
		if err != nil {
			panic(err)
		}
		literalPrefix, _ := descendantRegexp.LiteralPrefix()
		prefix = literalPrefix[:strings.LastIndex(literalPrefix, "/")]
		regexpSuffix = refRegexp[len(prefix)+1:]
		return
	}

	// Plain ref name, just split it by the last slash into prefix and suffix.
	lastSlash := strings.LastIndex(ref, "/")
	prefix, literalSuffix = ref[:lastSlash], ref[lastSlash+1:]

	return
}
