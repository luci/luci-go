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

	. "github.com/smartystreets/goconvey/convey"
	"gopkg.in/yaml.v2"
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

	Convey("Test IndexDefinition", t, func() {
		for _, tc := range indexDefinitionTests {
			Convey(tc.str, func() {
				So(tc.id.String(), ShouldEqual, tc.str)
				So(tc.id.Builtin(), ShouldEqual, tc.builtin)
				So(tc.id.Compound(), ShouldEqual, tc.compound)
				yaml, _ := tc.id.YAMLString()
				So(yaml, ShouldEqual, strings.Join(tc.yaml, "\n"))
			})
		}
	})

	Convey("Test MarshalYAML/UnmarshalYAML", t, func() {
		for _, tc := range indexDefinitionTests {
			Convey(fmt.Sprintf("marshallable index definition `%s` is marshalled and unmarshalled correctly", tc.str), func() {
				if tc.yaml != nil {
					// marshal
					_, err := yaml.Marshal(tc.id)
					So(err, ShouldBeNil)

					// unmarshal
					var ids []*IndexDefinition
					yaml.Unmarshal([]byte(strings.Join(tc.yaml, "\n")), &ids)
					So(ids[0], ShouldResemble, tc.id)
				}
			})
		}
	})
}
