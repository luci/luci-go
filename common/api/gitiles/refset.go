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
	"context"
	"regexp"
	"strings"
	"sync"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/config/validation"
)

// RefSet efficiently resolves many refs, supporting regexps.
//
// RefSet groups refs by prefix and issues 1 refs RPC per prefix. This is more
// efficient that a single refs RPC for "refs/" prefix, because it would return
// all refs of the repo, incl. potentially huge number of refs in refs/changes/.
type RefSet struct {
	byPrefix map[string]*refSetPrefix

	// These two fields are used by Resolve() method to compute missing refs.
	literalRefs []string
	regexpRefs  []struct {
		ref string
		re  *regexp.Regexp
	}
}

// NewRefSet creates an instance of the RefSet.
//
// Each entry in the refs parameter can be either
//   - a fully-qualified ref with at least 2 slashes, e.g. `refs/heads/master`,
//     `refs/tags/v1.2.3`, or
//   - a regular expression with "regexp:" prefix to match multiple refs, e.g.
//     `regexp:refs/heads/.*` or `regexp:refs/branch-heads/\d+\.\d+`.
//
// The regular expression must have:
//   - a literal prefix with at least 2 slashes, e.g. `refs/release-\d+/foo` is
//     not allowed, because the literal prefix `refs/release-` contains only one
//     slash, and
//   - must not start with ^ or end with $ as they will be added automatically.
//
// See also ValidateRefSet function.
func NewRefSet(refs []string) RefSet {
	w := RefSet{
		byPrefix: map[string]*refSetPrefix{},
	}
	nsRegexps := map[string][]string{}
	for _, ref := range refs {
		prefix, literalRef, refRegexp, compiledRegexp := parseRef(ref)
		if _, exists := w.byPrefix[prefix]; !exists {
			w.byPrefix[prefix] = &refSetPrefix{prefix: prefix}
		}

		switch {
		case (literalRef == "") == (refRegexp == ""):
			panic("exactly one must be defined")
		case refRegexp != "":
			nsRegexps[prefix] = append(nsRegexps[prefix], refRegexp)
			w.regexpRefs = append(w.regexpRefs, struct {
				ref string
				re  *regexp.Regexp
			}{ref: ref, re: compiledRegexp})
		case literalRef != "":
			w.byPrefix[prefix].addLiteralRef(literalRef)
			w.literalRefs = append(w.literalRefs, literalRef)
		}
	}

	for prefix, regexps := range nsRegexps {
		w.byPrefix[prefix].refRegexp = regexp.MustCompile(
			"^(" + strings.Join(regexps, ")|(") + ")$")
	}

	return w
}

// Has checks if a specific ref is in this set.
func (w RefSet) Has(ref string) bool {
	for prefix, wrp := range w.byPrefix {
		nsPrefix := prefix + "/"
		if strings.HasPrefix(ref, nsPrefix) && wrp.hasRef(ref) {
			return true
		}
	}

	return false
}

// Resolve queries gitiles to resolve watched refs to git SHA1 hash of their
// current tips.
//
// Returns map from individual ref to its SHA1 hash and a list of original refs,
// incl. regular expressions, which either don't exist or are not visible to the
// requester.
func (w RefSet) Resolve(ctx context.Context, client gitiles.GitilesClient, project string) (refTips map[string]string, missingRefs []string, err error) {
	lock := sync.Mutex{} // for concurrent writes to the map
	refTips = map[string]string{}
	err = parallel.FanOutIn(func(work chan<- func() error) {
		for prefix := range w.byPrefix {
			work <- func() error {
				resp, err := client.Refs(ctx, &gitiles.RefsRequest{Project: project, RefsPath: prefix})
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
	if err != nil {
		return
	}
	// Compute missingRefs as those for which no actual ref was found.
	for _, ref := range w.literalRefs {
		if _, ok := refTips[ref]; !ok {
			missingRefs = append(missingRefs, ref)
		}
	}
	for _, r := range w.regexpRefs {
		found := false
		// This loop isn't the most efficient way to perform this search, and may
		// result in executing MatchString O(refTips) times. If necessary to
		// optimize, store individual regexps inside relevant refSetPrefix,
		// and then mark corresponding regexps as "found" on the fly inside a
		// goroutine working with the refSetPrefix.
		for resolvedRef := range refTips {
			if r.re.MatchString(resolvedRef) {
				found = true
				break
			}
		}
		if !found {
			missingRefs = append(missingRefs, r.ref)
		}
	}
	return
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
	// literalRefs is a set of immediate children, not grandchildren. i.e., may
	// contain 'refs/prefix/child', but not 'refs/prefix/grand/child', which would
	// be contained in refSetPrefix for 'refs/prefix/grand'.
	literalRefs stringset.Set
	// refRegexp is a regular expression matching all descendants.
	refRegexp *regexp.Regexp
}

func (w refSetPrefix) hasRef(ref string) bool {
	switch {
	case w.refRegexp != nil && w.refRegexp.MatchString(ref):
		return true
	case w.literalRefs == nil:
		return false
	default:
		return w.literalRefs.Has(ref)
	}
}

func (w *refSetPrefix) addLiteralRef(literalRef string) {
	if w.literalRefs == nil {
		w.literalRefs = stringset.New(1)
	}
	w.literalRefs.Add(literalRef)
}

func validateRegexpRef(c *validation.Context, ref string) {
	c.Enter("%s", ref)
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
	lp, _ := r.LiteralPrefix()
	if strings.Count(lp, "/") < 2 {
		c.Errorf(`fewer than 2 slashes in literal prefix %q, e.g., `+
			`"refs/heads/\d+" is accepted because of "refs/heads/" is the `+
			`literal prefix, while "refs/.*" is too short`, lp)
	}
	if !strings.HasPrefix(lp, "refs/") {
		c.Errorf(`literal prefix %q must start with "refs/"`, lp)
	}
}

func parseRef(ref string) (prefix, literalRef, refRegexp string, compiledRegexp *regexp.Regexp) {
	if strings.HasPrefix(ref, "regexp:") {
		refRegexp = strings.TrimPrefix(ref, "regexp:")
		compiledRegexp = regexp.MustCompile("^" + refRegexp + "$")
		// Sometimes, LiteralPrefix(^regexp$) != LiteralPrefix(regexp)
		// See https://github.com/golang/go/issues/30425
		literalPrefix, complete := regexp.MustCompile(refRegexp).LiteralPrefix()
		prefix = literalPrefix[:strings.LastIndex(literalPrefix, "/")]
		if complete {
			// Trivial regexp which matches only and exactly literalPrefix.
			literalRef = literalPrefix
			compiledRegexp = nil
			refRegexp = ""
		}
		return
	}

	// Plain ref name, just extract the ref prefix from it.
	lastSlash := strings.LastIndex(ref, "/")
	prefix, literalRef = ref[:lastSlash], ref
	return
}
