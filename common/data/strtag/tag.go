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

// Package strtag implements parsing and formatting of lists of
// colon-delimited key-value pair strings.
//
// Example of tags:
//   master:tryserver.chromium.linux
//   builder:linux_chromium_rel_ng
//   buildset:patch/gerrit/chromium-review.googlesource.com/677784/5
package strtag

import (
	"sort"
	"strings"
)

// ParseTag parses a colon-delimited key-value pair.
//
// If tag does not have ":", the whole tag becomes the key with an empty
// value.
func ParseTag(tag string) (k, v string) {
	parts := strings.SplitN(tag, ":", 2)
	k = parts[0]
	if len(parts) > 1 {
		v = parts[1]
	} else {
		// this tag is invalid. This should not happen in practice.
		// Do not panic because this function is used for externally-supplied
		// data.
	}
	return
}

// FormatTag formats a tag from a key-value pair.
func FormatTag(k, v string) string {
	return k + ":" + v
}

// Tags contains parsed string tags.
type Tags map[string][]string

// Get gets the first value associated with the given key.
// If there are no values associated with the key, Get returns
// the empty string. To access multiple values, use the map
// directly.
func (t Tags) Get(key string) string {
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
func (t Tags) Set(key, value string) {
	t[key] = []string{value}
}

// Add adds the value to key. It appends to any existing
// values associated with key.
func (t Tags) Add(key, value string) {
	t[key] = append(t[key], value)
}

// Del deletes the values associated with key.
func (t Tags) Del(key string) {
	delete(t, key)
}

// Parse parses a list of colon-delimited key-value pair strings.
func Parse(raw []string) Tags {
	tags := make(Tags)
	for _, t := range raw {
		k, v := ParseTag(t)
		tags[k] = append(tags[k], v)
	}
	return tags
}

// Format converts t to a sorted list of strings.
func (t Tags) Format() []string {
	res := make([]string, 0, len(t))
	for k, values := range t {
		for _, v := range values {
			res = append(res, FormatTag(k, v))
		}
	}
	sort.Strings(res)
	return res
}

// Copy returns a deep copy of t.
func (t Tags) Copy() Tags {
	t2 := make(Tags, len(t))
	for k, vs := range t {
		t2[k] = append([]string(nil), vs...)
	}
	return t2
}

// Contains returns true if t contains the key-value pair.
func (t Tags) Contains(key, value string) bool {
	for _, v := range t[key] {
		if v == value {
			return true
		}
	}
	return false
}
