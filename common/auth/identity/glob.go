// Copyright 2015 The LUCI Authors.
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

package identity

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// Glob is glob like pattern that matches identity strings of some kind.
//
// It is a string of the form "kind:<pattern>" where 'kind' is one of Kind
// constants and 'pattern' is a wildcard pattern to apply to identity name.
//
// The only supported glob syntax is '*', which matches zero or more characters.
//
// Case sensitive. Doesn't support multi-line strings or patterns. There's no
// way to match '*' itself.
type Glob string

// MakeGlob ensures 'glob' string looks like a valid identity glob and
// returns it as Glob value.
func MakeGlob(glob string) (Glob, error) {
	g := Glob(glob)
	if err := g.Validate(); err != nil {
		return "", err
	}
	return g, nil
}

// Validate checks that the identity glob string is well-formed.
func (g Glob) Validate() error {
	chunks := strings.SplitN(string(g), ":", 2)
	if len(chunks) != 2 {
		return fmt.Errorf("auth: bad identity glob string %q", g)
	}
	if knownKinds[Kind(chunks[0])] == nil {
		return fmt.Errorf("auth: bad identity glob kind %q", chunks[0])
	}
	if chunks[1] == "" {
		return fmt.Errorf("auth: identity glob can't be empty")
	}
	if _, err := cache.translate(chunks[1]); err != nil {
		return fmt.Errorf("auth: bad identity glob pattern %q - %s", chunks[1], err)
	}
	return nil
}

// Kind returns identity glob kind. If identity glob string is invalid returns
// Anonymous.
func (g Glob) Kind() Kind {
	chunks := strings.SplitN(string(g), ":", 2)
	if len(chunks) != 2 {
		return Anonymous
	}
	return Kind(chunks[0])
}

// Pattern returns a pattern part of the identity glob. If the identity glob
// string is invalid returns empty string.
func (g Glob) Pattern() string {
	chunks := strings.SplitN(string(g), ":", 2)
	if len(chunks) != 2 {
		return ""
	}
	return chunks[1]
}

// Match returns true if glob matches an identity. If identity string
// or identity glob string are invalid, returns false.
func (g Glob) Match(id Identity) bool {
	globChunks := strings.SplitN(string(g), ":", 2)
	if len(globChunks) != 2 || knownKinds[Kind(globChunks[0])] == nil {
		return false
	}
	globKind := globChunks[0]
	pattern := globChunks[1]

	idChunks := strings.SplitN(string(id), ":", 2)
	if len(idChunks) != 2 || knownKinds[Kind(idChunks[0])] == nil {
		return false
	}
	idKind := idChunks[0]
	name := idChunks[1]

	if idKind != globKind {
		return false
	}
	if strings.ContainsRune(name, '\n') {
		return false
	}

	re, err := cache.translate(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(name)
}

////

var cache patternCache

// patternCache implements caching for compiled pattern regexps, similar to how
// python does that. Uses extremely dumb algorithm that assumes there are less
// than 500 active patterns in the process. That's how python runtime does it
// too.
type patternCache struct {
	sync.RWMutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	re  *regexp.Regexp
	err error
}

// translate grabs converted regexp from cache or calls 'translate' to get it.
func (c *patternCache) translate(pat string) (*regexp.Regexp, error) {
	c.RLock()
	val, ok := c.cache[pat]
	c.RUnlock()
	if ok {
		return val.re, val.err
	}

	c.Lock()
	defer c.Unlock()
	if val, ok := c.cache[pat]; ok {
		return val.re, val.err
	}

	if c.cache == nil || len(c.cache) > 500 {
		c.cache = map[string]cacheEntry{}
	}

	var re *regexp.Regexp
	reStr, err := translate(pat)
	if err == nil {
		re, err = regexp.Compile(reStr)
	}

	c.cache[pat] = cacheEntry{re, err}
	return re, err
}

// translate converts glob pattern to a regular expression string.
func translate(pat string) (string, error) {
	res := "^"
	for _, runeVal := range pat {
		switch runeVal {
		case '\n':
			return "", fmt.Errorf("new lines are not supported in globs")
		case '*':
			res += ".*"
		default:
			res += regexp.QuoteMeta(string(runeVal))
		}
	}
	return res + "$", nil
}
