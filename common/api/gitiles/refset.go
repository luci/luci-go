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

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/config/validation"
)

// RefSet is a type that allows to keep track of a set of refs.
type RefSet interface {
	// In checks if a given ref is tracked.
	Has(ref string) bool
	// ForEachPrefix executes action func for each tracked ref prefix.
	ForEachPrefix(action func(prefix string))
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

// Implements RefSet.
type refSets struct {
	prefixes map[string]*refSetPrefix
}

// SplitRef splits a ref into a ref prefix and suffix at the last slash.
func SplitRef(s string) (string, string) {
	i := strings.LastIndex(s, "/")
	if i <= 0 {
		return s, ""
	}
	return s[:i], s[i+1:]
}

func validateRegexpRef(c *validation.Context, ref string) {
	c.Enter(ref)
	defer c.Exit()
	reStr := strings.TrimPrefix(ref, "regexp:")
	if strings.HasPrefix(reStr, "^") || strings.HasSuffix(reStr, "$") {
		c.Errorf("regexp ^ and $ qualifiers are added automatically, " +
			"please remove them from the config")
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
	if !strings.HasPrefix(reStr, lp) {
		c.Errorf("does not start with its own literal prefix %q", lp)
	}
	if strings.Count(lp, "/") < 2 {
		c.Errorf(`fewer than 2 slashes in literal prefix %q, e.g., `+
			`"refs/heads/\d+" is accepted because of "refs/heads/" is the `+
			`literal prefix, while "refs/.*" is too short`, lp)
	}
	if !strings.HasPrefix(lp, "refs/") {
		c.Errorf(`literal prefix %q must start with "refs/"`, lp)
	}
}

// ValidateRefConfigs validates passed set of ref configs.
func ValidateRefConfigs(c *validation.Context, refConfigs []string) {
	for _, ref := range refConfigs {
		if strings.HasPrefix(ref, "regexp:") {
			validateRegexpRef(c, ref)
			continue
		}

		if !strings.HasPrefix(ref, "refs/") {
			c.Errorf("ref must start with 'refs/' not %q", ref)
		}
		cnt := strings.Count(ref, "*")
		if cnt > 0 {
			regexpRef := "regexp:" + strings.Replace(regexp.QuoteMeta(ref), `\*`, `[^/]+`, -1)
			c.Errorf("globs are not supported anymore, please use regexp instead: %q", regexpRef)
		}
	}
}

// NewRefSet creates an instance of the RefSet based on a set of ref
// configs as described in proto config (see description for refs field in
// GitilesTask message in appengine/messages/config.proto)
func NewRefSet(refConfigs []string) RefSet {
	w := refSets{map[string]*refSetPrefix{}}
	nsRegexps := map[string][]string{}
	for _, ref := range refConfigs {
		prefix, suffix, regexp := parseRefFromConfig(ref)
		if _, exists := w.prefixes[prefix]; !exists {
			w.prefixes[prefix] = &refSetPrefix{prefix: prefix}
		}

		switch {
		case (suffix == "") == (regexp == ""):
			panic("exactly one must be defined")
		case regexp != "":
			nsRegexps[prefix] = append(nsRegexps[prefix], regexp)
		case suffix != "":
			w.prefixes[prefix].addSuffix(suffix)
		}
	}

	for prefix, regexps := range nsRegexps {
		var err error
		w.prefixes[prefix].descendantRegexp, err = regexp.Compile(
			"^(" + strings.Join(regexps, ")|(") + ")$")
		if err != nil {
			panic(err)
		}
	}

	return &w
}

func (w *refSets) Has(ref string) bool {
	for prefix, wrp := range w.prefixes {
		nsPrefix := prefix + "/"
		if strings.HasPrefix(ref, nsPrefix) && wrp.hasSuffix(ref[len(nsPrefix):]) {
			return true
		}
	}

	return false
}

func (w *refSets) ForEachPrefix(action func(prefix string)) {
	for prefix := range w.prefixes {
		action(prefix)
	}
}

func parseRefFromConfig(ref string) (prefix, literalSuffix, regexpSuffix string) {
	if strings.HasPrefix(ref, "regexp:") {
		ref = strings.TrimPrefix(ref, "regexp:")
		descendantRegexp, err := regexp.Compile(ref)
		if err != nil {
			panic(err)
		}
		literalPrefix, _ := descendantRegexp.LiteralPrefix()
		prefix = literalPrefix[:strings.LastIndex(literalPrefix, "/")]
		regexpSuffix = ref[len(prefix)+1:]
		return
	}

	prefix, literalSuffix = SplitRef(ref)
	return
}
