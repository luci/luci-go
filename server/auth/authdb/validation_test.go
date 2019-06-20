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

package authdb

import (
	"testing"

	"go.chromium.org/luci/server/auth/service/protocol"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidation(t *testing.T) {
	t.Parallel()

	validate := func(db *protocol.AuthDB) error {
		_, err := NewSnapshotDB(db, "", 0, true)
		return err
	}

	Convey("Works", t, func() {
		So(validate(&protocol.AuthDB{}), ShouldBeNil)
		So(validate(&protocol.AuthDB{
			Groups: []*protocol.AuthGroup{
				{Name: "group"},
			},
			IpWhitelists: []*protocol.AuthIPWhitelist{
				{Name: "IP whitelist"},
			},
		}), ShouldBeNil)
	})

	Convey("Bad group", t, func() {
		So(validate(&protocol.AuthDB{
			Groups: []*protocol.AuthGroup{
				{
					Name:    "group",
					Members: []string{"bad identity"},
				},
			},
		}), ShouldErrLike, "invalid identity")
	})

	Convey("Bad IP whitelist", t, func() {
		So(validate(&protocol.AuthDB{
			IpWhitelists: []*protocol.AuthIPWhitelist{
				{
					Name:    "IP whitelist",
					Subnets: []string{"not a subnet"},
				},
			},
		}), ShouldErrLike, "bad IP whitlist")
	})

	Convey("Bad SecurityConfig", t, func() {
		So(validate(&protocol.AuthDB{
			SecurityConfig: []byte("not a serialized proto"),
		}), ShouldErrLike, "failed to deserialize SecurityConfig")
	})
}

func TestValidateAuthGroup(t *testing.T) {
	t.Parallel()

	Convey("validateAuthGroup works", t, func() {
		groups := map[string]*protocol.AuthGroup{
			"group1": {
				Name:    "group1",
				Members: []string{"user:abc@example.com"},
				Globs:   []string{"service:*"},
				Nested:  []string{"group2"},
			},
			"group2": {
				Name: "group2",
			},
		}
		So(validateAuthGroup("group1", groups), ShouldBeNil)
	})

	Convey("validateAuthGroup bad identity", t, func() {
		groups := map[string]*protocol.AuthGroup{
			"group1": {
				Name:    "group1",
				Members: []string{"blah"},
			},
		}
		So(validateAuthGroup("group1", groups), ShouldErrLike, "invalid identity")
	})

	Convey("validateAuthGroup bad glob", t, func() {
		groups := map[string]*protocol.AuthGroup{
			"group1": {
				Name:  "group1",
				Globs: []string{"blah"},
			},
		}
		So(validateAuthGroup("group1", groups), ShouldErrLike, "invalid glob")
	})

	Convey("validateAuthGroup missing nested group", t, func() {
		groups := map[string]*protocol.AuthGroup{
			"group1": {
				Name:   "group1",
				Nested: []string{"missing"},
			},
		}
		So(validateAuthGroup("group1", groups), ShouldErrLike, "unknown nested group")
	})

	Convey("validateAuthGroup dependency cycle", t, func() {
		groups := map[string]*protocol.AuthGroup{
			"group1": {
				Name:   "group1",
				Nested: []string{"group1"},
			},
		}
		So(validateAuthGroup("group1", groups), ShouldErrLike, "dependency cycle found")
	})
}

type groupGraph map[string][]string

func TestFindGroupCycle(t *testing.T) {
	t.Parallel()

	call := func(groups groupGraph) []string {
		asProto := make(map[string]*protocol.AuthGroup)
		for k, v := range groups {
			asProto[k] = &protocol.AuthGroup{
				Name:   k,
				Nested: v,
			}
		}
		return findGroupCycle("start", asProto)
	}

	Convey("Empty", t, func() {
		So(call(groupGraph{"start": nil}), ShouldBeEmpty)
	})

	Convey("No cycles", t, func() {
		So(call(groupGraph{
			"start": []string{"A"},
			"A":     []string{"B"},
			"B":     []string{"C"},
		}), ShouldBeEmpty)
	})

	Convey("Self reference", t, func() {
		So(call(groupGraph{
			"start": []string{"start"},
		}), ShouldResemble, []string{"start"})
	})

	Convey("Simple cycle", t, func() {
		So(call(groupGraph{
			"start": []string{"A"},
			"A":     []string{"start"},
		}), ShouldResemble, []string{"start", "A"})
	})

	Convey("Long cycle", t, func() {
		So(call(groupGraph{
			"start": []string{"A"},
			"A":     []string{"B"},
			"B":     []string{"C"},
			"C":     []string{"start"},
		}), ShouldResemble, []string{"start", "A", "B", "C"})
	})

	Convey("Diamond no cycles", t, func() {
		So(call(groupGraph{
			"start": []string{"A1", "A2"},
			"A1":    []string{"B"},
			"A2":    []string{"B"},
			"B":     nil,
		}), ShouldBeEmpty)
	})

	Convey("Diamond with cycles", t, func() {
		So(call(groupGraph{
			"start": []string{"A1", "A2"},
			"A1":    []string{"B"},
			"A2":    []string{"B"},
			"B":     []string{"start"},
		}), ShouldResemble, []string{"start", "A1", "B"})
	})
}
