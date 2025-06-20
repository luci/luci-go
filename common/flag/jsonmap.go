// Copyright 2019 The LUCI Authors.
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
	"flag"

	"go.chromium.org/luci/common/errors"
)

type jsonMap map[string]string

// JSONMap returns a flag.Value that can be passed to flag.Var to
// parse a JSON string into a map.
func JSONMap(m *map[string]string) flag.Value {
	return (*jsonMap)(m)
}

func (m *jsonMap) String() string {
	if m == nil {
		return "null"
	}
	d, err := json.Marshal(*m)
	if err != nil {
		// Marshaling a map[string]string shouldn't error.
		panic(err)
	}
	return string(d)
}

func (m *jsonMap) Set(s string) error {
	if m == nil {
		return errors.New("JSONMap pointer is nil")
	}
	d := []byte(s)
	return json.Unmarshal(d, m)
}
