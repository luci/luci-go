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

package buildbucket

import "strings"

const (
	// TagBuildSet is a key of a tag used to group related builds.
	// See also ParseChange.
	// When a build triggers a new build, the buildset tag must be copied.
	TagBuildSet = "buildset"
	// TagBuilder is the key of builder name tag.
	TagBuilder = "builder"
)

// ParseTag parses a buildbucket tag.
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

// Tags contains parsed buildbucket build tags.
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

// Set sets the key to value. It replaces any existing
// values.
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

func ParseTags(raw []string) Tags {
	tags := make(Tags)
	for _, t := range raw {
		k, v := ParseTag(t)
		tags[k] = append(tags[k], v)
	}
	return tags
}

func (t Tags) Format() []string {
	res := make([]string, len(t))
	for k, values := range t {
		for _, v := range values {
			res = append(res, FormatTag(k, v))
		}
	}
	return res
}
