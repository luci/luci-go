// Copyright 2022 The LUCI Authors.
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

package listener

import (
	"context"
	"regexp"
	"sync"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap"
)

// projectFinder looks up LUCI projects watching a given Gerrit host and repo.
type projectFinder struct {
	mu                       sync.RWMutex
	enabledProjectRegexps    []*regexp.Regexp
	enabledProjectRegexpStrs []string
}

func isEqual(lhs, rhs []string) bool {
	if len(lhs) != len(rhs) {
		return false
	}
	for i := range lhs {
		if lhs[i] != rhs[i] {
			return false
		}
	}
	return true
}

// reload reloads the projectFinder with given project regexps.
func (pf *projectFinder) reload(reStrs []string) error {
	// optimistic; they are the same most likely.
	pf.mu.RLock()
	if isEqual(pf.enabledProjectRegexpStrs, reStrs) {
		pf.mu.RUnlock()
		return nil
	}
	pf.mu.RUnlock()

	// There is a change. Reload the cached regexps with the new regexps.
	pf.mu.Lock()
	defer pf.mu.Unlock()
	if isEqual(pf.enabledProjectRegexpStrs, reStrs) {
		return nil
	}
	newREs, err := anchorRegexps(reStrs)
	if err != nil {
		// The config validation shouldn't allow this to happen.
		return err
	}
	pf.enabledProjectRegexpStrs = reStrs
	pf.enabledProjectRegexps = newREs
	return nil
}

// lookup looks up pubsub enabled LUCI projects for a given host and repo.
func (pf *projectFinder) lookup(ctx context.Context, host, repo string) ([]string, error) {
	// TODO: optimize lookup with in-memory cache.
	prjs, err := gobmap.LookupProjects(ctx, host, repo)
	if err != nil {
		return nil, errors.Annotate(err, "gobmap.LookupProjects").Err()
	}
	pf.mu.RLock()
	defer pf.mu.RUnlock()
	matched := make([]string, 0, len(prjs))
	for _, prj := range prjs {
		for _, re := range pf.enabledProjectRegexps {
			if re.Match([]byte(prj)) {
				matched = append(matched, prj)
			}
		}
	}
	return matched, nil
}
