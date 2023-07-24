// Copyright 2023 The LUCI Authors.
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

package service

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/config_service/internal/model"
)

// Finder provides fast look up for services interested in the provided config.
//
// It loads `Service` entity for all registered services and pre-compute the
// ConfigPattern to in-memory data structures that promotes faster matching
// performance.
//
// See ConfigPattern syntax at https://pkg.go.dev/go.chromium.org/luci/common/proto/config#ConfigPattern
//
// It can be used as an one-off look up. However, to unlock its full ability,
// the caller should use a singleton and call `RefreshPeriodically` in the
// background to keep the singleton's pre-computed patterns up-to-date with.
type Finder struct {
	mu       sync.RWMutex // protects `services`
	services []serviceWrapper
}

// serviceWrapper is a wrapper around `Service` entity that contains
// pre-computed patterns to match config file.
type serviceWrapper struct {
	service  *model.Service
	patterns []pattern
}

// pattern is used to match a config file.
type pattern struct {
	configSetMatcher matcher
	pathMatcher      matcher
}

// matcher is what will be pre-computed from a ConfigPattern.
type matcher struct {
	exact string
	re    *regexp.Regexp
}

// match return true if the pattern matches the provided string.
func (m matcher) match(s string) bool {
	if m.exact != "" && m.exact == s {
		return true
	}
	if m.re != nil && m.re.MatchString(s) {
		return true
	}
	return false
}

// matchConfig returns true if any of the pre-computed pattern matches the
// provided config file.
//
// If the provided config set is a service, always returns false when the
// service name is different from the service wrapped by serviceWrapper.
// This is for tightening access control where service A doesn't get to
// validate the service config for service B.
func (sw *serviceWrapper) matchConfig(ctx context.Context, cs config.Set, filePath string) bool {
	if domain, target := cs.Split(); domain == config.ServiceDomain && sw.service.Name != target {
		for _, p := range sw.patterns {
			if p.configSetMatcher.match(string(cs)) {
				logging.Warningf(ctx, "service %q declares it is interested in the config of another service %q", sw.service.Name, target)
				return false
			}
		}
		return false
	}
	for _, p := range sw.patterns {
		if p.configSetMatcher.match(string(cs)) && p.pathMatcher.match(filePath) {
			return true
		}
	}
	return false
}

// NewFinder instantiate a new Finder that are ready for look up.
func NewFinder(ctx context.Context) (*Finder, error) {
	m := &Finder{}
	if err := m.refresh(ctx); err != nil {
		return nil, err
	}
	return m, nil
}

// FindInterestedServices look up for services interested in the given config.
//
// Look-up is performed in memory.
func (m *Finder) FindInterestedServices(ctx context.Context, cs config.Set, filePath string) []*model.Service {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var ret []*model.Service
	for _, service := range m.services {
		if service.matchConfig(ctx, cs, filePath) {
			ret = append(ret, service.service)
		}
	}
	if len(ret) > 0 {
		sort.Slice(ret, func(i, j int) bool {
			return strings.Compare(ret[i].Name, ret[j].Name) < 0
		})
	}
	return ret
}

// RefreshPeriodically refreshes the finder with the latest registered services
// information every 1 minute until context is cancelled.
//
// Log the error if refresh has failed.
func (m *Finder) RefreshPeriodically(ctx context.Context) {
	for {
		if r := <-clock.After(ctx, 1*time.Minute); r.Err != nil {
			return // the context is canceled
		}
		if err := m.refresh(ctx); err != nil {
			// TODO(yiwzhang): alert if there are consecutive refresh errors.
			logging.Errorf(ctx, "Failed to update config finder using service metadata: %s", err)
		}
	}
}

func (m *Finder) refresh(ctx context.Context) error {
	var services []*model.Service
	if err := datastore.GetAll(ctx, datastore.NewQuery(model.ServiceKind), &services); err != nil {
		return fmt.Errorf("failed to query all Services: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(services) == 0 {
		m.services = nil
		logging.Warningf(ctx, "no service found when updating finder")
		return nil
	}
	m.services = make([]serviceWrapper, 0, len(services))
	for _, service := range services {
		var patterns []pattern
		switch {
		case service.Metadata != nil:
			patterns = makePatterns(service.Metadata.GetConfigPatterns())
		case service.LegacyMetadata != nil:
			patterns = makePatterns(service.LegacyMetadata.GetValidation().GetPatterns())
		}
		if len(patterns) > 0 {
			m.services = append(m.services, serviceWrapper{
				service:  service,
				patterns: patterns,
			})
		}
	}
	return nil
}

// The patterns are ensured to be valid by `validateMetadata` before updating
// services.
func makePatterns(patterns []*cfgcommonpb.ConfigPattern) []pattern {
	if len(patterns) == 0 {
		return nil
	}
	ret := make([]pattern, len(patterns))
	for i, p := range patterns {
		ret[i] = pattern{
			configSetMatcher: makeMatcher(p.GetConfigSet()),
			pathMatcher:      makeMatcher(p.GetPath()),
		}
	}
	return ret
}

func makeMatcher(pattern string) matcher {
	switch p := strings.TrimSpace(pattern); {
	case strings.HasPrefix(p, "exact:"):
		return matcher{exact: p[len("exact:"):]}
	case strings.HasPrefix(p, "text:"):
		return matcher{exact: p[len("text:"):]}
	case strings.HasPrefix(p, "regex:"):
		expr := p[len("regex:"):]
		if !strings.HasPrefix(expr, "^") {
			expr = "^" + expr
		}
		if !strings.HasSuffix(expr, "$") {
			expr = expr + "$"
		}
		return matcher{re: regexp.MustCompile(expr)}
	default:
		return matcher{exact: p}
	}
}
