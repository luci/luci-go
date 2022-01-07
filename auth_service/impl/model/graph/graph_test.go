// Copyright 2021 The LUCI Authors.
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

package graph

import (
	"strings"
	"testing"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/auth_service/impl/model"
	_ "go.chromium.org/luci/gae/service/datastore/crbug1242998safeget"

	. "github.com/smartystreets/goconvey/convey"
)

////////////////////////////////////////////////////////////////////////////////////////
// Helper functions for tests.
func testAuthGroup(name string, items ...string) *model.AuthGroup {
	var globs, identities, nested []string
	for _, item := range items {
		if strings.Contains(item, "*") {
			globs = append(globs, item)
		} else if strings.Contains(item, ":") {
			identities = append(identities, item)
		} else {
			nested = append(nested, item)
		}
	}

	return &model.AuthGroup{
		ID:      name,
		Members: identities,
		Globs:   globs,
		Nested:  nested,
		Owners:  "owners-" + name,
	}
}

////////////////////////////////////////////////////////////////////////////////////////

func TestGraphBuilding(t *testing.T) {
	t.Parallel()

	Convey("Testing basic Graph Building.", t, func() {

		authGroups := []*model.AuthGroup{
			testAuthGroup("group-0", "user:m1@example.com", "user:*@example.com"),
			testAuthGroup("group-1", "user:m1@example.com", "user:m2@example.com"),
			testAuthGroup("group-2", "user:*@example.com"),
		}

		actualGraph, err := NewGraph(authGroups)

		So(err, ShouldBeNil)

		expectedGraph := &Graph{
			groups: map[string]*groupNode{
				"group-0": {
					group: authGroups[0],
				},
				"group-1": {
					group: authGroups[1],
				},
				"group-2": {
					group: authGroups[2],
				},
			},
			globs: []identity.Glob{
				identity.Glob("user:*@example.com"),
			},
			membersIndex: map[identity.Identity][]string{
				identity.Identity("user:m1@example.com"): {"group-0", "group-1"},
				identity.Identity("user:m2@example.com"): {"group-1"},
			},
			globsIndex: map[identity.Glob][]string{
				identity.Glob("user:*@example.com"): {"group-0", "group-2"},
			},
		}

		So(actualGraph, ShouldResemble, expectedGraph)
	})

	Convey("Testing group nesting.", t, func() {
		authGroups := []*model.AuthGroup{
			testAuthGroup("group-0"),
			testAuthGroup("group-1", "group-0"),
			testAuthGroup("group-2", "group-1"),
		}

		actualGraph, err := NewGraph(authGroups)

		So(err, ShouldBeNil)

		So(actualGraph.groups["group-0"].included[0].group, ShouldResemble, authGroups[1])
		So(actualGraph.groups["group-1"].included[0].group, ShouldResemble, authGroups[2])
		So(actualGraph.groups["group-1"].includes[0].group, ShouldResemble, authGroups[0])
		So(actualGraph.groups["group-2"].includes[0].group, ShouldResemble, authGroups[1])
	})
}
