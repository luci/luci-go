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

package flag

import (
	"encoding/json"
)

// JSONMap is an implementation of flag.Value for parsing a JSON
// object into a map.
type JSONMap struct {
	m *map[string]string
}

// NewJSONMap creates a JSONMap value
func NewJSONMap(m *map[string]string) JSONMap {
	return JSONMap{m: m}
}

// String implements the flag.Value interface.
func (f JSONMap) String() string {
	d, err := json.Marshal(f.m)
	if err != nil {
		// Marshaling a map[string]string shouldn't error.
		panic(err)
	}
	return string(d)
}

// Set implements the flag.Value interface.
func (f JSONMap) Set(s string) error {
	return json.Unmarshal([]byte(s), f.m)
}
