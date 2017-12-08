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

package service

import (
	"testing"

	"go.chromium.org/luci/server/auth/service/protocol"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateAuthDB(t *testing.T) {
	Convey("validateAuthDB works", t, func() {
		So(validateAuthDB(&protocol.AuthDB{}), ShouldBeNil)
		So(validateAuthDB(&protocol.AuthDB{
			Groups: []*protocol.AuthGroup{
				{Name: strPtr("group")},
			},
			IpWhitelists: []*protocol.AuthIPWhitelist{
				{Name: strPtr("IP whitelist")},
			},
		}), ShouldBeNil)
	})

	Convey("validateAuthDB bad group", t, func() {
		So(validateAuthDB(&protocol.AuthDB{
			Groups: []*protocol.AuthGroup{
				{
					Name:    strPtr("group"),
					Members: []string{"bad identity"},
				},
			},
		}), ShouldErrLike, "invalid identity")
	})

	Convey("validateAuthDB bad IP whitelist", t, func() {
		So(validateAuthDB(&protocol.AuthDB{
			IpWhitelists: []*protocol.AuthIPWhitelist{
				{
					Name:    strPtr("IP whitelist"),
					Subnets: []string{"not a subnet"},
				},
			},
		}), ShouldErrLike, "bad IP whitlist")
	})
}

func TestValidateAuthGroup(t *testing.T) {
	Convey("validateAuthGroup works", t, func() {
		groups := map[string]*protocol.AuthGroup{
			"group1": {
				Name:    strPtr("group1"),
				Members: []string{"user:abc@example.com"},
				Globs:   []string{"service:*"},
				Nested:  []string{"group2"},
			},
			"group2": {
				Name: strPtr("group2"),
			},
		}
		So(validateAuthGroup("group1", groups), ShouldBeNil)
	})

	Convey("validateAuthGroup bad identity", t, func() {
		groups := map[string]*protocol.AuthGroup{
			"group1": {
				Name:    strPtr("group1"),
				Members: []string{"blah"},
			},
		}
		So(validateAuthGroup("group1", groups), ShouldErrLike, "invalid identity")
	})

	Convey("validateAuthGroup bad glob", t, func() {
		groups := map[string]*protocol.AuthGroup{
			"group1": {
				Name:  strPtr("group1"),
				Globs: []string{"blah"},
			},
		}
		So(validateAuthGroup("group1", groups), ShouldErrLike, "invalid glob")
	})

	Convey("validateAuthGroup missing nested group", t, func() {
		groups := map[string]*protocol.AuthGroup{
			"group1": {
				Name:   strPtr("group1"),
				Nested: []string{"missing"},
			},
		}
		So(validateAuthGroup("group1", groups), ShouldErrLike, "unknown nested group")
	})

	Convey("validateAuthGroup dependency cycle", t, func() {
		groups := map[string]*protocol.AuthGroup{
			"group1": {
				Name:   strPtr("group1"),
				Nested: []string{"group1"},
			},
		}
		So(validateAuthGroup("group1", groups), ShouldErrLike, "dependency cycle found")
	})
}

type groupGraph map[string][]string

func TestFindGroupCycle(t *testing.T) {
	call := func(groups groupGraph) []string {
		asProto := make(map[string]*protocol.AuthGroup)
		for k, v := range groups {
			asProto[k] = &protocol.AuthGroup{
				Name:   strPtr(k),
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

///

func strPtr(s string) *string {
	return &s
}
