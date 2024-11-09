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

package streamproto

import (
	"encoding/json"
	"flag"
	"sort"

	"go.chromium.org/luci/common/flag/stringmapflag"

	"go.chromium.org/luci/logdog/common/types"
)

// TagMap is a flags-compatible map used to store stream tags.
type TagMap stringmapflag.Value

var _ interface {
	json.Marshaler
	json.Unmarshaler
	flag.Value
} = (*TagMap)(nil)

// String implements flag.Value.
func (t *TagMap) String() string {
	return (*stringmapflag.Value)(t).String()
}

// Set implements flag.Value
func (t *TagMap) Set(key string) error {
	return (*stringmapflag.Value)(t).Set(key)
}

// SortedKeys returns a sorted slice of the keys in a TagMap.
func (t TagMap) SortedKeys() []string {
	if len(t) == 0 {
		return nil
	}

	keys := make([]string, 0, len(t))
	for k := range t {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// MarshalJSON implements the json.Marshaler interface.
func (t *TagMap) MarshalJSON() ([]byte, error) {
	m := make(map[string]string, len(*t))
	if len(*t) > 0 {
		for k, v := range *t {
			m[k] = v
		}
	}
	return json.Marshal(m)
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (t *TagMap) UnmarshalJSON(data []byte) error {
	m := map[string]string{}
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}

	if len(m) == 0 {
		*t = nil
		return nil
	}

	tm := make(TagMap, len(m))
	for k, v := range m {
		if err := types.ValidateTag(k, v); err != nil {
			return err
		}
		tm[k] = v
	}

	*t = tm
	return nil
}
