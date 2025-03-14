// Copyright 2025 The LUCI Authors.
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

package pkg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/armon/go-radix"
	"github.com/bazelbuild/buildtools/build"
	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/lucicfg/buildifier"
)

// rulesBasedFormatter implements buildifier.FormatterPolicy using rules from
// PACKAGE.star.
type rulesBasedFormatter struct {
	// Maps "//rel/directory/" to *build.Rewriter for this directory.
	rules *radix.Tree
}

// CheckValid is part of buildifier.FormatterPolicy interface.
func (f *rulesBasedFormatter) CheckValid(ctx context.Context) error {
	return nil // valid by construction
}

// RewriterForPath is part of buildifier.FormatterPolicy interface.
func (f *rulesBasedFormatter) RewriterForPath(ctx context.Context, path string) (*build.Rewriter, error) {
	if path != cleanPath(path) || strings.HasPrefix(path, "../") || strings.HasPrefix(path, "/") {
		panic(fmt.Sprintf("got path in expected format: %q", path))
	}
	_, entry, found := f.rules.LongestPrefix("//" + path)
	if !found {
		return nil, nil // no matching rules, use default
	}
	return entry.(*build.Rewriter), nil
}

// standardFormatter returns a formatter using given rules.
//
// The rules are assumed to be validated already. All paths are relative to
// the package. Returns nil if there are no rules.
func standardFormatter(rules []*FmtRule) buildifier.FormatterPolicy {
	radix := radix.New()
	for _, r := range rules {
		var rewriter *build.Rewriter
		if r.SortFunctionArgs {
			rewriter = buildifier.DefaultRewriter()
			rewriter.NamePriority = namePriorityTable(r.SortFunctionArgsOrder)
			rewriter.RewriteSet = append(rewriter.RewriteSet, "callsort")
		}
		for _, p := range r.Paths {
			if p == "." {
				p = "//"
			} else {
				p = "//" + p + "/" // e.g. "//some/dir/"
			}
			// Note that nil rewriter is possible here. It means "use defaults".
			radix.Insert(p, rewriter)
		}
	}
	if radix.Len() == 0 {
		return nil
	}
	return &rulesBasedFormatter{rules: radix}
}

// namePriorityTable assigns priorities to arguments.
//
// This sequentially gives the names a priority value in the range [-count, 0).
// This ensures that all names have distinct priority values that sort them in
// the specified order. Since all priority values are less than the default 0,
// all names present in the ordering will sort before names that don't appear
// in the ordering.
func namePriorityTable(ordering []string) map[string]int {
	count := len(ordering)
	table := make(map[string]int, count)
	for i, n := range ordering {
		table[n] = i - count
	}
	return table
}

////////////////////////////////////////////////////////////////////////////////

// lazyFormatter lazy-initializes buildifier.FormatterPolicy on first use.
//
// Useful to skip touching config file if no formatting is necessary.
type lazyFormatter struct {
	init func(context.Context) (buildifier.FormatterPolicy, error)

	once sync.Once
	fmt  buildifier.FormatterPolicy
	err  error
}

// get lazy-initializes the formatter.
//
// Can return nil to use the default rules.
func (l *lazyFormatter) get(ctx context.Context) (buildifier.FormatterPolicy, error) {
	l.once.Do(func() { l.fmt, l.err = l.init(ctx) })
	return l.fmt, l.err
}

// CheckValid is part of buildifier.FormatterPolicy interface.
func (l *lazyFormatter) CheckValid(ctx context.Context) error {
	switch f, err := l.get(ctx); {
	case err != nil:
		return err
	case f != nil:
		return f.CheckValid(ctx)
	default:
		return nil
	}
}

// RewriterForPath is part of buildifier.FormatterPolicy interface.
func (l *lazyFormatter) RewriterForPath(ctx context.Context, path string) (*build.Rewriter, error) {
	switch f, err := l.get(ctx); {
	case err != nil:
		return nil, err
	case f != nil:
		return f.RewriterForPath(ctx, path)
	default:
		return nil, nil // using default formatting rules
	}
}

////////////////////////////////////////////////////////////////////////////////

// legacyFormatter returns a formatter that uses legacy ".lucicfgfmtrc" config.
//
// The config must be located at "<root>/.lucicfgfmtrc". All paths passed to
// this formatter will be related to the root as well.
func legacyFormatter(root string) buildifier.FormatterPolicy {
	return &lazyFormatter{
		init: func(_ context.Context) (buildifier.FormatterPolicy, error) {
			configPath := filepath.Join(root, legacyConfig)
			cfg, err := readLegacyRules(configPath)
			switch {
			case err != nil:
				return nil, err
			case cfg == nil:
				return nil, nil // no config, use default rules
			}
			legacyRules, err := legacyRulesToFmtRules(cfg)
			if err != nil {
				return nil, errors.Annotate(err, "bad formatting rules at %s", configPath).Err()
			}
			return standardFormatter(legacyRules), nil
		},
	}
}

// legacyCompatibleFormatter returns a formatter that uses PACKAGE.star rules,
// but verifies they are identical to the legacy rules from ".lucicfgfmtrc"
// config (if it exists).
//
// If PACKAGE.star has no formatting rules, uses the legacy config.
//
// This is useful during the migration period to ensure all configs are in sync.
func legacyCompatibleFormatter(root string, rules []*FmtRule) buildifier.FormatterPolicy {
	return &lazyFormatter{
		init: func(_ context.Context) (buildifier.FormatterPolicy, error) {
			configPath := filepath.Join(root, legacyConfig)
			cfg, err := readLegacyRules(configPath)
			switch {
			case err != nil:
				return nil, err
			case cfg == nil:
				return standardFormatter(rules), nil // no legacy config, use new rules
			}
			legacyRules, err := legacyRulesToFmtRules(cfg)
			if err != nil {
				return nil, errors.Annotate(err, "bad formatting rules at %s", configPath).Err()
			}
			if len(rules) == 0 {
				// No new rules defined yet. Just use legacy ones.
				return standardFormatter(legacyRules), nil
			}
			// Have legacy and non-legacy rules. They must be 100% equal.
			eq := slices.EqualFunc(legacyRules, rules, func(a, b *FmtRule) bool { return a.Equal(b) })
			if !eq {
				return nil, errors.Reason(
					"Formatting rules in PACKAGE.star and legacy .lucicfgfmtrc should be identical. " +
						"Eventually pkg.options.fmt_rules(...) in PACKAGE.star will become authoritative and .lucicfgfmtrc " +
						"will be retired. Until then the rules must agree. Please update .lucicfgfmtrc.",
				).Err()
			}
			return standardFormatter(rules), nil
		},
	}
}

// readLegacyRules reads ".lucicfgfmtrc" from the given path if it exists.
//
// Returns (nil, nil) if it doesn't exist. Doesn't interpret the rules.
func readLegacyRules(path string) (*buildifier.LucicfgFmtConfig, error) {
	switch blob, err := os.ReadFile(path); {
	case errors.Is(err, os.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, err
	default:
		cfg := &buildifier.LucicfgFmtConfig{}
		if err := prototext.Unmarshal(blob, cfg); err != nil {
			return nil, errors.Annotate(err, "bad text proto at %s", path).Err()
		}
		return cfg, nil
	}
}

// legacyRulesToFmtRules converts legacy rules to non-legacy ones, verifying
// their correctness along the way.
func legacyRulesToFmtRules(cfg *buildifier.LucicfgFmtConfig) ([]*FmtRule, error) {
	seenPaths := stringset.New(0)

	rules := make([]*FmtRule, len(cfg.Rules))
	for i, r := range cfg.Rules {
		rule := &FmtRule{}
		rules[i] = rule

		if len(r.Path) == 0 {
			return nil, errors.Reason("rule #%d: paths should not be empty", i).Err()
		}
		if len(stringset.NewFromSlice(r.Path...)) != len(r.Path) {
			return nil, errors.Reason("rule #%d: has duplicate paths", i).Err()
		}
		for _, p := range r.Path {
			if p == "" {
				return nil, errors.Reason("rule #%d: empty path", i).Err()
			}
			if clean := cleanPath(p); clean != p {
				return nil, errors.Reason("rule #%d: path %q is not in normal form (e.g. %q)", i, p, clean).Err()
			}
			if !seenPaths.Add(p) {
				return nil, errors.Reason("rule #%d: path %q was already specified in another rule", i, p).Err()
			}
		}
		rule.Paths = r.Path // good

		if r.FunctionArgsSort != nil {
			args := r.FunctionArgsSort.Arg
			if len(stringset.NewFromSlice(args...)) != len(args) {
				return nil, errors.Reason("rule #%d: has duplicate args in function_args_sort", i).Err()
			}
			for _, arg := range args {
				if arg == "" {
					return nil, errors.Reason("rule #%d: empty arg in function_args_sort", i).Err()
				}
			}
			rule.SortFunctionArgs = true
			rule.SortFunctionArgsOrder = args
		}
	}

	return rules, nil
}
