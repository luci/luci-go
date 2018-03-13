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
	"fmt"
	"regexp"
	"strings"
	"sync"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/data/text/pattern"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

// Rules is the default validation rule set used by the process.
//
// Individual packages may register vars and rules there during init() time.
var Rules RuleSet

// RuleSet is a helper for building Validator from a set of rules: each rule
// specifies a pattern for config set and file names, and a validation function
// to apply to corresponding configs.
//
// The primary use case is building the list of rules during init() time. Since
// not all information is available at that time (most crucially on GAE Standard
// App ID is not yet known), the rule patterns can have placeholders (such as
// "${appid}") that are substituted during actual config validation time.
type RuleSet struct {
	l sync.Mutex
	v map[string]func(context.Context) string
	r []*rule
}

type rule struct {
	configSet string         // pattern string with ${var} placeholders
	path      string         // same
	cb        Func           // a validator function to use for matching files
	rendered  *ConfigPattern // lazily-populated rendered and compiled pattern
}

// RegisterVar registers a placeholder that can be used in patterns as ${name}.
//
// Such placeholder is rendered into an actual value via the given callback
// before the validation starts. The value of the placeholder is injected into
// the pattern string as is. So for example if the pattern is 'regex:...',
// the placeholder value can be a chunk of regexp.
//
// It is assumed the returned value doesn't change (in fact, it is getting
// cached after the first call).
//
// The primary use case for this mechanism is too allow to register rule
// patterns that depend on not-yet known values during init() time.
//
// Panics if such variable is already registered.
func (r *RuleSet) RegisterVar(name string, value func(context.Context) string) {
	r.l.Lock()
	defer r.l.Unlock()
	if r.v == nil {
		r.v = make(map[string]func(context.Context) string, 1)
	}
	if r.v[name] != nil {
		panic(fmt.Sprintf("variable %q is already registered", name))
	}
	r.v[name] = value
}

// Add registers a validation function for given configSet and path patterns.
//
// The pattern may contain placeholders (e.g. "${appid}") that will be
// resolved before the actual validation starts. All such placeholder variables
// must be registered prior to adding rules that reference them (or 'Add' will
// panic).
//
// 'Add' will also try to validate the patterns by substituting all placeholders
// in them with empty strings and trying to render the resulting pattern. It
// will panic if the pattern is invalid.
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

	// Validate the patterns syntax by rendering them with some fake variable
	// values.
	nilSub := func(name string) (string, error) {
		if r.v[name] == nil {
			return "", fmt.Errorf("no placeholder named %q is registered", name)
		}
		// We have no context to actually render the placeholder. Substituting with
		// empty string is good enough for the purpose of the preliminary pattern
		// syntax validation.
		return "", nil
	}
	if _, err := renderPatternString(configSet, nilSub); err != nil {
		panic(fmt.Sprintf("bad config set pattern %q - %s", configSet, err))
	}
	if _, err := renderPatternString(path, nilSub); err != nil {
		panic(fmt.Sprintf("bad path pattern %q - %s", path, err))
	}

	r.r = append(r.r, &rule{configSet, path, cb, nil})
}

// Validator returns an actual validator that uses the registered rules.
//
// Note that the returned Validator holds references back to Rules object, so
// it will pick up rules registered after Validator() call too.
func (r *RuleSet) Validator() *Validator {
	return &Validator{
		ConfigPatterns: r.patterns,
		Func:           r.validate,
	}
}

///

// patterns lazily renders all registered patterns and returns them.
func (r *RuleSet) patterns(c context.Context) ([]*ConfigPattern, error) {
	r.l.Lock()
	defer r.l.Unlock()

	out := make([]*ConfigPattern, len(r.r))
	for i, rule := range r.r {
		var err error
		if out[i], err = r.renderedConfigPattern(c, rule); err != nil {
			return nil, err
		}
	}

	return out, nil
}

// validate picks a rule matching the given file and executes its callback.
func (r *RuleSet) validate(ctx *Context, configSet, path string, content []byte) error {
	switch cb, err := r.matchingFunc(ctx.Context, configSet, path); {
	case err != nil:
		return err
	case cb != nil:
		return cb(ctx, configSet, path, content)
	default:
		logging.Warningf(ctx.Context, "No validation rule registered for file %q in config set %q", path, configSet)
	}
	return nil
}

// matchingFunc returns a validator callback matching the given file.
func (r *RuleSet) matchingFunc(c context.Context, configSet, path string) (Func, error) {
	r.l.Lock()
	defer r.l.Unlock()

	for _, rule := range r.r {
		switch pat, err := r.renderedConfigPattern(c, rule); {
		case err != nil:
			return nil, err
		case pat.ConfigSet.Match(configSet) && pat.Path.Match(path):
			return rule.cb, nil
		}
	}

	return nil, nil
}

// renderedConfigPattern lazily populates rule.rendered and returns it.
//
// Must be called with r.l held.
func (r *RuleSet) renderedConfigPattern(c context.Context, rule *rule) (*ConfigPattern, error) {
	if rule.rendered != nil {
		return rule.rendered, nil
	}

	sub := func(name string) (string, error) {
		if cb := r.v[name]; cb != nil {
			return cb(c), nil
		}
		return "", fmt.Errorf("no placeholder named %q is registered", name)
	}

	configSet, err := renderPatternString(rule.configSet, sub)
	if err != nil {
		return nil, errors.Annotate(err, "failed to compile config set pattern %q", rule.configSet).Err()
	}
	path, err := renderPatternString(rule.path, sub)
	if err != nil {
		return nil, errors.Annotate(err, "failed to compile path pattern %q", rule.path).Err()
	}

	rule.rendered = &ConfigPattern{
		ConfigSet: configSet,
		Path:      path,
	}
	return rule.rendered, nil
}

var placeholderRe = regexp.MustCompile(`\${[^}]*}`)

// renderPatternString substitutes all ${name} placeholders via given callback
// and compiles the resulting pattern.
func renderPatternString(pat string, sub func(name string) (string, error)) (pattern.Pattern, error) {
	var errs errors.MultiError
	out := placeholderRe.ReplaceAllStringFunc(pat, func(match string) string {
		name := match[2 : len(match)-1] // strip ${...}
		val, err := sub(name)
		if err != nil {
			errs = append(errs, err)
		}
		return val
	})
	if len(errs) != 0 {
		return nil, errs
	}
	return pattern.Parse(out)
}
