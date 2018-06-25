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
)

// WatchedRefs is a type that allows to keep track of a set of watched refs.
type WatchedRefs interface {
	// HasRef checks if a given ref is watched.
	HasRef(ref string) bool
	// ForEachPrefix executes action func for each watched ref prefix.
	ForEachPrefix(action func(refsPath string))
}

type watchedRefPrefix struct {
	prefix string // no trailing "/".
	// exactChildren is a set of immediate children, not grandchildren. i.e., may
	// contain 'child', but not 'grand/child', which would be contained in
	// watchedRefsPrefix for (prefix + "/child").
	exactChildren stringset.Set
	// descendantRegexp is a regular expression matching all descendants.
	descendantRegexp *regexp.Regexp
}

func (w watchedRefPrefix) hasSuffix(suffix string) bool {
	switch {
	case w.descendantRegexp != nil && w.descendantRegexp.MatchString(suffix):
		return true
	case w.exactChildren == nil:
		return false
	default:
		return w.exactChildren.Has(suffix)
	}
}

func (w *watchedRefPrefix) addSuffix(suffix string) {
	if w.exactChildren == nil {
		w.exactChildren = stringset.New(1)
	}
	w.exactChildren.Add(suffix)
}

// Implements WatchedRefs.
type watchedRefs struct {
	prefixes map[string]*watchedRefPrefix
}

// SplitRef splits a ref into a ref prefix and suffix at the last slash.
func SplitRef(s string) (string, string) {
	i := strings.LastIndex(s, "/")
	if i <= 0 {
		return s, ""
	}
	return s[:i], s[i+1:]
}

// NewWatchedRefs creates an instance of the WatchedRefs based on a set of ref
// configs as described in proto config (see description for refs field in
// GitilesTask message in appengine/messages/config.proto)
func NewWatchedRefs(refConfigs []string) WatchedRefs {
	w := watchedRefs{map[string]*watchedRefPrefix{}}
	nsRegexps := map[string][]string{}
	for _, ref := range refConfigs {
		prefix, suffix, regexp := parseRefFromConfig(ref)
		if _, exists := w.prefixes[prefix]; !exists {
			w.prefixes[prefix] = &watchedRefPrefix{prefix: prefix}
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

func (w *watchedRefs) HasRef(ref string) bool {
	for prefix, wrp := range w.prefixes {
		nsPrefix := prefix + "/"
		if strings.HasPrefix(ref, nsPrefix) && wrp.hasSuffix(ref[len(nsPrefix):]) {
			return true
		}
	}

	return false
}

func (w *watchedRefs) ForEachPrefix(action func(refsPath string)) {
	for refsPath := range w.prefixes {
		action(refsPath)
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
