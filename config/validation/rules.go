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

package validation

import (
	"context"
	"strings"
	"sync"

	"go.chromium.org/luci/common/data/text/pattern"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/config/vars"
)

// Rules is the default validation rule set used by the process.
//
// Individual packages may register rules here during init() time.
var Rules = RuleSet{Vars: &vars.Vars}

// Func performs the actual config validation and stores the associated results
// in the Context.
//
// Returns an error if the validation process itself fails due to causes
// unrelated to the data being validated. This will result in HTTP Internal
// Server Error reply, instructing the config service to retry.
type Func func(ctx *Context, configSet, path string, content []byte) error

// ConfigPattern is a pair of pattern.Pattern of config sets and paths that
// the service is responsible for validating.
type ConfigPattern struct {
	ConfigSet pattern.Pattern
	Path      pattern.Pattern
}

// RuleSet is a helper for building Validator from a set of rules: each rule
// specifies a pattern for config sets and file names, and a validation function
// to apply to corresponding configs.
//
// The primary use case is building the list of rules during init() time. Since
// not all information is available at that time (most crucially on GAE Standard
// App ID is not yet known), the rule patterns can have placeholders (such as
// "${appid}") that are substituted during actual config validation time via
// the given vars.VarSet instance.
type RuleSet struct {
	// Vars is a set of placeholder vars that can be used in patterns.
	Vars *vars.VarSet

	l sync.RWMutex
	r []*rule
}

type rule struct {
	configSet string // pattern string with ${var} placeholders
	path      string // same
	cb        Func   // a validator function to use for matching files
}

// NewRuleSet returns a RuleSet that uses its own new VarSet.
//
// Primarily useful in tests to create a self-contained RuleSet to avoid relying
// on global Rules and Vars.
func NewRuleSet() *RuleSet {
	return &RuleSet{Vars: &vars.VarSet{}}
}

// Add registers a validation function for the config set and path patterns.
//
// Patterns may contain placeholders (e.g. "${appid}") that will be resolved
// when doing the validation. All such placeholder variables must be registered
// in the VarSet before the rule set is used, but they may be registered after
// Add calls that reference them.
func (r *RuleSet) Add(configSet, path string, cb Func) {
	r.l.Lock()
	defer r.l.Unlock()

	// Pattern strings without ':' are magical: they are treated like exact
	// matches. Thus if a variable value has ':', it may change the meaning of the
	// pattern after the substitution. To avoid this, clarify the kind of the
	// pattern before the substitution.
	if !strings.ContainsRune(configSet, ':') {
		configSet = "exact:" + configSet
	}
	if !strings.ContainsRune(path, ':') {
		path = "exact:" + path
	}

	r.r = append(r.r, &rule{configSet, path, cb})
}

// ConfigPatterns renders all registered config patterns and returns them.
//
// Used by the metadata handler to notify the config service about config files
// we understand.
//
// Returns an error if some config patterns can't be rendered, e.g. if they
// reference placeholders that weren't registered in the VarSet, or if the
// placeholder value can't be resolved.
func (r *RuleSet) ConfigPatterns(ctx context.Context) ([]*ConfigPattern, error) {
	r.l.RLock()
	defer r.l.RUnlock()

	out := make([]*ConfigPattern, len(r.r))
	var errs errors.MultiError

	for i, rule := range r.r {
		var err errors.MultiError
		out[i], err = r.renderedConfigPattern(ctx, rule)
		errs = append(errs, err...)
	}

	if len(errs) != 0 {
		return nil, errs
	}
	return out, nil
}

// ValidateConfig picks all rules matching the given file and executes their
// validation callbacks.
//
// If there's no rule matching the file, the validation is skipped. If there
// are multiple rules that match the file, they all are used (in order of their)
// registration.
//
// Returns an error if the validation process itself fails due to causes
// unrelated to the data being validated (e.g. config patterns can't be rendered
// or some validation callback fails).
func (r *RuleSet) ValidateConfig(ctx *Context, configSet, path string, content []byte) error {
	switch cbs, err := r.matchingFuncs(ctx.Context, configSet, path); {
	case err != nil:
		return err
	case len(cbs) != 0:
		for _, cb := range cbs {
			if err := cb(ctx, configSet, path, content); err != nil {
				return err
			}
		}
	default:
		logging.Warningf(ctx.Context, "No validation rule registered for file %q in config set %q", path, configSet)
	}
	return nil
}

///

// matchingFuncs returns a validator callbacks matching the given file.
func (r *RuleSet) matchingFuncs(ctx context.Context, configSet, path string) ([]Func, error) {
	r.l.RLock()
	defer r.l.RUnlock()

	var out []Func
	var errs errors.MultiError

	for _, rule := range r.r {
		switch pat, err := r.renderedConfigPattern(ctx, rule); {
		case err != nil:
			errs = append(errs, err...)
		case pat.ConfigSet.Match(configSet) && pat.Path.Match(path):
			out = append(out, rule.cb)
		}
	}

	if len(errs) != 0 {
		return nil, errs
	}
	return out, nil
}

// renderedConfigPattern expands variables in the config patterns.
func (r *RuleSet) renderedConfigPattern(ctx context.Context, rule *rule) (*ConfigPattern, errors.MultiError) {
	var errs errors.MultiError

	configSet, err := r.renderPattern(ctx, rule.configSet)
	if err != nil {
		errs = append(errs, errors.Fmt("failed to compile config set pattern %q: %w", rule.configSet, err))
	}
	path, err := r.renderPattern(ctx, rule.path)
	if err != nil {
		errs = append(errs, errors.Fmt("failed to compile path pattern %q: %w", rule.path, err))
	}

	if len(errs) != 0 {
		return nil, errs
	}

	return &ConfigPattern{
		ConfigSet: configSet,
		Path:      path,
	}, nil
}

func (r *RuleSet) renderPattern(ctx context.Context, pat string) (pattern.Pattern, error) {
	out, err := r.Vars.RenderTemplate(ctx, pat)
	if err != nil {
		return nil, err
	}
	return pattern.Parse(out)
}
