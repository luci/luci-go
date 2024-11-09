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
	"sync"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cv/internal/configs/srvcfg"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap"
	listenerpb "go.chromium.org/luci/cv/settings/listener"
)

// projectFinder looks up LUCI projects watching a given Gerrit host and repo.
type projectFinder struct {
	mu                sync.RWMutex
	isListenerEnabled func(prj string) bool
}

// reload reloads the projectFinder with given project regexps.
func (pf *projectFinder) reload(s *listenerpb.Settings) error {
	chk, err := srvcfg.MakeListenerProjectChecker(s)
	if err != nil {
		// Must be a bug in the validator.
		return err
	}

	pf.mu.Lock()
	pf.isListenerEnabled = chk
	pf.mu.Unlock()
	return nil
}

// lookup looks up pubsub enabled LUCI projects for a given host and repo.
func (pf *projectFinder) lookup(ctx context.Context, host, repo string) ([]string, error) {
	// TODO: optimize lookup with in-memory cache.
	prjs, err := gobmap.LookupProjects(ctx, host, repo)
	if err != nil {
		return nil, errors.Annotate(err, "gobmap.LookupProjects").Err()
	}
	matched := make([]string, 0, len(prjs))

	pf.mu.RLock()
	defer pf.mu.RUnlock()
	for _, prj := range prjs {
		if pf.isListenerEnabled(prj) {
			matched = append(matched, prj)
		}
	}
	return matched, nil
}
