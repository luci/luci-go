// Copyright 2017 The LUCI Authors.
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

// Package strpair implements parsing and formatting of lists of
// colon-delimited key-value pair strings.
//
// Example of pairs:
//   master:tryserver.chromium.linux
//   builder:linux_chromium_rel_ng
//   buildset:patch/gerrit/chromium-review.googlesource.com/677784/5
package strpair

import (
	"sort"
	"strings"
)

// Parse parses a colon-delimited key-value pair.
//
// If pair does not have ":", the whole string becomes the key with an empty
// value.
func Parse(pair string) (k, v string) {
	parts := strings.SplitN(pair, ":", 2)
	k = parts[0]
	if len(parts) > 1 {
		v = parts[1]
	} else {
		// this pair is invalid. This should not happen in practice.
		// Do not panic because this function is used for externally-supplied
		// data.
	}
	return
}

// Format formats a pair from a key and a value.
func Format(k, v string) string {
	return k + ":" + v
}

// Map contains parsed string pairs.
type Map map[string][]string

// Get gets the first value associated with the given key.
// If there are no values associated with the key, Get returns
// the empty string. To access multiple values, use the map
// directly.
func (t Map) Get(key string) string {
	if t == nil {
		return ""
	}
	vs := t[key]
	if len(vs) == 0 {
		return ""
	}
	return vs[0]
}

// Set sets the key to value. It replaces any existing values.
func (t Map) Set(key, value string) {
	t[key] = []string{value}
}

// Add adds the value to key. It appends to any existing
// values associated with key.
func (t Map) Add(key, value string) {
	t[key] = append(t[key], value)
}

// Del deletes the values associated with key.
func (t Map) Del(key string) {
	delete(t, key)
}

// ParseMap parses a list of colon-delimited key-value pair strings.
func ParseMap(raw []string) Map {
	m := make(Map)
	for _, t := range raw {
		k, v := Parse(t)
		m[k] = append(m[k], v)
	}
	return m
}

// Format converts t to a sorted list of strings.
func (t Map) Format() []string {
	res := make([]string, 0, len(t))
	for k, values := range t {
		for _, v := range values {
			res = append(res, Format(k, v))
		}
	}
	sort.Strings(res)
	return res
}

// Copy returns a deep copy of t.
func (t Map) Copy() Map {
	t2 := make(Map, len(t))
	for k, vs := range t {
		t2[k] = append([]string(nil), vs...)
	}
	return t2
}

// Contains returns true if t contains the key-value pair.
func (t Map) Contains(key, value string) bool {
	for _, v := range t[key] {
		if v == value {
			return true
		}
	}
	return false
}
