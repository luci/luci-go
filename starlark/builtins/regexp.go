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

package builtins

import (
	"regexp"
	"sync"

	"go.starlark.net/starlark"
)

// RegexpMatcher returns a function (with given name) that allows Starlark code
// to do regular expression matches:
//
//  def matches(pattern, str):
//    """Returns (True, tuple of submatches) with leftmost match of the regular
//    expression.
//
//    The submatches tuple has the full match as a first string, followed by
//    subexpression matches.
//
//    If there are no matches, returns (False, ()). Fails if the regular
//    expression can't be compiled.
//    """
//
// Uses Go regexp engine, which is slightly different from Python's. API also
// explicitly does NOT try to mimic Python's 're' module.
//
// Each separate instance of the builtin holds a cache of compiled regular
// expressions internally. The cache is never cleaned up.
//
// Safe for concurrent use.
func RegexpMatcher(name string) *starlark.Builtin {
	cache := regexpCache{r: make(map[string]*regexp.Regexp)}
	return starlark.NewBuiltin(name, func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var pattern, str starlark.String
		err := starlark.UnpackArgs(name, args, kwargs,
			"pattern", &pattern,
			"str", &str,
		)
		if err != nil {
			return nil, err
		}
		yes, groups, err := cache.matches(pattern.GoString(), str.GoString())
		if err != nil {
			return nil, err
		}
		tup := make(starlark.Tuple, len(groups))
		for i, s := range groups {
			tup[i] = starlark.String(s)
		}
		return starlark.Tuple{starlark.Bool(yes), tup}, nil
	})
}

type regexpCache struct {
	m sync.RWMutex
	r map[string]*regexp.Regexp
}

func (c *regexpCache) matches(pat, str string) (bool, []string, error) {
	exp, err := c.exp(pat)
	if err != nil {
		return false, nil, err
	}
	groups := exp.FindStringSubmatch(str)
	if groups == nil {
		return false, nil, nil
	}
	return true, groups, nil
}

func (c *regexpCache) exp(pat string) (*regexp.Regexp, error) {
	c.m.RLock()
	exp, _ := c.r[pat]
	c.m.RUnlock()
	if exp != nil {
		return exp, nil
	}

	c.m.Lock()
	defer c.m.Unlock()
	if exp, _ = c.r[pat]; exp != nil {
		return exp, nil
	}

	exp, err := regexp.Compile(pat)
	if err != nil {
		return nil, err
	}
	c.r[pat] = exp
	return exp, nil
}
