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
	"maps"
	"slices"
	"strings"
	"sync"

	"go.chromium.org/luci/common/data/stringset"
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

// MultiFunc performs config validation across a snapshot of matching files in a
// config set and stores the associated results in the Context.
//
// Implementations should call ctx.SetFile(path) to attribute recorded errors
// and warnings to the appropriate file.
//
// Returns an error if the validation process itself fails due to causes
// unrelated to the data being validated.
type MultiFunc func(ctx *Context, configSet string, files map[string][]byte) error

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
	configSet string    // pattern string with ${var} placeholders
	paths     []string  // pattern strings with ${var} placeholders
	cb        MultiFunc // validator function
}

type renderedRule struct {
	configSet pattern.Pattern
	paths     []pattern.Pattern
	cb        MultiFunc
}

func (rr *renderedRule) validate(ctx *Context, configSet string, files map[string][]byte, matched stringset.Set) error {
	if !rr.configSet.Match(configSet) {
		return nil
	}
	var subset map[string][]byte
	var firstPath string
	for path, content := range files {
		for _, pat := range rr.paths {
			if pat.Match(path) {
				if subset == nil {
					subset = make(map[string][]byte)
					firstPath = path
				}
				subset[path] = content
				matched.Add(path)
				break
			}
		}
	}
	if len(subset) == 0 {
		return nil
	}
	if len(subset) == 1 {
		ctx.SetFile(firstPath)
	} else {
		ctx.SetFile("")
	}
	return rr.cb(ctx, configSet, subset)
}

// NewRuleSet returns a RuleSet that uses its own new VarSet.
//
// Primarily useful in tests to create a self-contained RuleSet to avoid relying
// on global Rules and Vars.
func NewRuleSet() *RuleSet {
	return &RuleSet{Vars: &vars.VarSet{}}
}

func normalizePattern(pat string) string {
	// Pattern strings without ':' are magical: they are treated like exact
	// matches. Thus if a variable value has ':', it may change the meaning of the
	// pattern after the substitution. To avoid this, clarify the kind of the
	// pattern before the substitution.
	if !strings.ContainsRune(pat, ':') {
		return "exact:" + pat
	}
	return pat
}

// Add registers a validation function for the config set and path patterns.
//
// Patterns may contain placeholders (e.g. "${appid}") that will be resolved
// when doing the validation. All such placeholder variables must be registered
// in the VarSet before the rule set is used, but they may be registered after
// Add calls that reference them.
func (r *RuleSet) Add(configSet, path string, cb Func) {
	r.AddMulti(configSet, []string{path}, func(ctx *Context, configSet string, files map[string][]byte) error {
		for _, p := range slices.Sorted(maps.Keys(files)) {
			ctx.SetFile(p)
			if err := cb(ctx, configSet, p, files[p]); err != nil {
				return err
			}
		}
		return nil
	})
}

// AddMulti registers a multi-file validation function for a config set pattern
// and one or more path patterns.
//
// When ValidateConfigSet (or ValidateConfig) is called for a matching
// configSet, cb is invoked once if at least one file matches any of paths,
// receiving the subset of files that matched.
func (r *RuleSet) AddMulti(configSet string, paths []string, cb MultiFunc) {
	r.l.Lock()
	defer r.l.Unlock()

	normPaths := make([]string, len(paths))
	for i, p := range paths {
		normPaths[i] = normalizePattern(p)
	}
	r.r = append(r.r, &rule{
		configSet: normalizePattern(configSet),
		paths:     normPaths,
		cb:        cb,
	})
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
	rendered, err := r.renderedRules(ctx)
	if err != nil {
		return nil, err
	}

	var out []*ConfigPattern
	for _, rr := range rendered {
		for _, p := range rr.paths {
			out = append(out, &ConfigPattern{
				ConfigSet: rr.configSet,
				Path:      p,
			})
		}
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
	return r.ValidateConfigSet(ctx, configSet, map[string][]byte{path: content})
}

// ValidateConfigSet picks all rules matching the given config set and files and
// executes their validation callbacks.
//
// Single-file rules registered via [RuleSet.Add] are called once per matching
// file in `files`. Multi-file rules registered via [RuleSet.AddMulti] are
// called once with the subset of `files` matching any of the rule's path
// patterns (if non-empty).
func (r *RuleSet) ValidateConfigSet(ctx *Context, configSet string, files map[string][]byte) error {
	rendered, err := r.renderedRules(ctx.Context)
	if err != nil {
		return err
	}

	matched := stringset.New(len(files))

	for _, rr := range rendered {
		if err := rr.validate(ctx, configSet, files, matched); err != nil {
			return err
		}
	}

	for _, path := range slices.Sorted(maps.Keys(files)) {
		if !matched.Has(path) {
			logging.Warningf(ctx.Context, "No validation rule registered for file %q in config set %q", path, configSet)
		}
	}
	return nil
}

///

func (r *RuleSet) renderedRules(ctx context.Context) ([]renderedRule, error) {
	r.l.RLock()
	defer r.l.RUnlock()

	out := make([]renderedRule, len(r.r))
	var errs errors.MultiError

	for i, rule := range r.r {
		rr, err := r.renderRule(ctx, rule)
		out[i] = rr
		errs = append(errs, err...)
	}

	if len(errs) != 0 {
		return nil, errs
	}
	return out, nil
}

func (r *RuleSet) renderRule(ctx context.Context, rule *rule) (renderedRule, errors.MultiError) {
	var errs errors.MultiError

	configSet, err := r.renderPattern(ctx, rule.configSet)
	if err != nil {
		errs = append(errs, errors.Fmt("failed to compile config set pattern %q: %w", rule.configSet, err))
	}
	paths := make([]pattern.Pattern, len(rule.paths))
	for i, p := range rule.paths {
		paths[i], err = r.renderPattern(ctx, p)
		if err != nil {
			errs = append(errs, errors.Fmt("failed to compile path pattern %q: %w", p, err))
		}
	}

	if len(errs) != 0 {
		return renderedRule{}, errs
	}

	return renderedRule{
		configSet: configSet,
		paths:     paths,
		cb:        rule.cb,
	}, nil
}

func (r *RuleSet) renderPattern(ctx context.Context, pat string) (pattern.Pattern, error) {
	out, err := r.Vars.RenderTemplate(ctx, pat)
	if err != nil {
		return nil, err
	}
	return pattern.Parse(out)
}
