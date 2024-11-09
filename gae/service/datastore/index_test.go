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

// adapted from github.com/golang/appengine/datastore

package datastore

import (
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

var indexDefinitionTests = []struct {
	id       *IndexDefinition
	builtin  bool
	compound bool
	str      string
	yaml     []string
}{
	{
		id:      &IndexDefinition{Kind: "kind"},
		builtin: true,
		str:     "B:kind",
	},

	{
		id: &IndexDefinition{
			Kind:   "kind",
			SortBy: []IndexColumn{{Property: ""}},
		},
		builtin: true,
		str:     "B:kind/",
	},

	{
		id: &IndexDefinition{
			Kind:   "kind",
			SortBy: []IndexColumn{{Property: "prop"}},
		},
		builtin: true,
		str:     "B:kind/prop",
	},

	{
		id: &IndexDefinition{
			Kind:     "Kind",
			Ancestor: true,
			SortBy: []IndexColumn{
				{Property: "prop"},
				{Property: "other", Descending: true},
			},
		},
		compound: true,
		str:      "C:Kind|A/prop/-other",
		yaml: []string{
			"- kind: Kind",
			"  ancestor: yes",
			"  properties:",
			"  - name: prop",
			"  - name: other",
			"    direction: desc",
		},
	},
}

func TestIndexDefinition(t *testing.T) {
	t.Parallel()

	ftt.Run("Test IndexDefinition", t, func(t *ftt.Test) {
		for _, tc := range indexDefinitionTests {
			t.Run(tc.str, func(t *ftt.Test) {
				assert.Loosely(t, tc.id.String(), should.Equal(tc.str))
				assert.Loosely(t, tc.id.Builtin(), should.Equal(tc.builtin))
				assert.Loosely(t, tc.id.Compound(), should.Equal(tc.compound))
				yaml, _ := tc.id.YAMLString()
				assert.Loosely(t, yaml, should.Equal(strings.Join(tc.yaml, "\n")))
			})
		}
	})

	ftt.Run("Test MarshalYAML/UnmarshalYAML", t, func(t *ftt.Test) {
		for _, tc := range indexDefinitionTests {
			t.Run(fmt.Sprintf("marshallable index definition `%s` is marshalled and unmarshalled correctly", tc.str), func(t *ftt.Test) {
				if tc.yaml != nil {
					// marshal
					_, err := yaml.Marshal(tc.id)
					assert.Loosely(t, err, should.BeNil)

					// unmarshal
					var ids []*IndexDefinition
					yaml.Unmarshal([]byte(strings.Join(tc.yaml, "\n")), &ids)
					assert.Loosely(t, ids[0], should.Resemble(tc.id))
				}
			})
		}
	})
}
