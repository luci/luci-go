// Copyright 2024 The LUCI Authors.
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

package validation

import (
	"context"
	"fmt"
	"testing"

	realmsconf "go.chromium.org/luci/common/proto/realms"
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/auth_service/constants"
	"go.chromium.org/luci/auth_service/testsupport"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRealmsValidator(t *testing.T) {
	t.Parallel()

	Convey("Common realms config validation", t, func() {
		vCtx := &validation.Context{Context: context.Background()}
		validator := &realmsValidator{
			db: testsupport.PermissionsDB(true),
		}

		Convey("empty config is valid", func() {
			testCfg := &realmsconf.RealmsCfg{}
			validator.Validate(vCtx, testCfg)
			So(vCtx.Finalize(), ShouldBeNil)
		})

		Convey("custom roles are validated", func() {
			Convey("must have correct prefix", func() {
				testCfg := &realmsconf.RealmsCfg{
					CustomRoles: []*realmsconf.CustomRole{
						{Name: "role/notCustom"},
					},
				}
				validator.Validate(vCtx, testCfg)
				So(vCtx.Finalize(), ShouldErrLike,
					fmt.Sprintf("role name should have prefix %q", constants.PrefixCustomRole))
			})

			Convey("duplicate definitions", func() {
				testCfg := &realmsconf.RealmsCfg{
					CustomRoles: []*realmsconf.CustomRole{
						{Name: "customRole/tester"},
						{Name: "customRole/tester"},
					},
				}
				validator.Validate(vCtx, testCfg)
				So(vCtx.Finalize(), ShouldErrLike, "custom role already defined")
			})

			Convey("references unknown permission", func() {
				testCfg := &realmsconf.RealmsCfg{
					CustomRoles: []*realmsconf.CustomRole{
						{Name: "customRole/tester", Permissions: []string{"luci.dev.test"}},
					},
				}
				validator.Validate(vCtx, testCfg)
				So(vCtx.Finalize(), ShouldErrLike, "permission \"luci.dev.test\" is not defined")
			})

			Convey("references unknown builtin role", func() {
				testCfg := &realmsconf.RealmsCfg{
					CustomRoles: []*realmsconf.CustomRole{
						{Name: "customRole/tester", Extends: []string{"role/dev.unknown"}},
					},
				}
				validator.Validate(vCtx, testCfg)
				So(vCtx.Finalize(), ShouldErrLike, "not defined in permissions DB")
			})

			Convey("references unknown custom role", func() {
				testCfg := &realmsconf.RealmsCfg{
					CustomRoles: []*realmsconf.CustomRole{
						{Name: "customRole/tester", Extends: []string{"customRole/unknown"}},
					},
				}
				validator.Validate(vCtx, testCfg)
				So(vCtx.Finalize(), ShouldErrLike, "not defined in the realms config")
			})

			Convey("uses unrecognized role format", func() {
				testCfg := &realmsconf.RealmsCfg{
					CustomRoles: []*realmsconf.CustomRole{
						{Name: "customRole/tester", Extends: []string{"foo/unknown"}},
					},
				}
				validator.Validate(vCtx, testCfg)
				So(vCtx.Finalize(), ShouldErrLike, "bad role reference")
			})

			Convey("duplicate extension of custom roles", func() {
				testCfg := &realmsconf.RealmsCfg{
					CustomRoles: []*realmsconf.CustomRole{
						{
							Name: "customRole/base",
						},
						{
							Name:    "customRole/tester",
							Extends: []string{"customRole/base", "customRole/base"},
						},
					},
				}
				validator.Validate(vCtx, testCfg)
				So(vCtx.Finalize(), ShouldErrLike, "in `extends` more than once")
			})

			Convey("cyclically extends itself", func() {
				testCfg := &realmsconf.RealmsCfg{
					CustomRoles: []*realmsconf.CustomRole{
						{
							Name: "customRole/a", Extends: []string{"customRole/b"},
						},
						{
							Name: "customRole/b", Extends: []string{"customRole/a"},
						},
					},
				}
				validator.Validate(vCtx, testCfg)
				So(vCtx.Finalize(), ShouldErrLike, "cyclically extends itself")
			})
		})
	})
}

func TestFindCycle(t *testing.T) {
	t.Parallel()

	Convey("Invalid start node", t, func() {
		graph := map[string][]string{
			"A": {},
		}
		_, err := findCycle("B", graph)
		So(err, ShouldErrLike, "unrecognized")
	})

	Convey("No cycles", t, func() {
		graph := map[string][]string{
			"A": {"B", "C"},
			"B": {"C"},
			"C": {},
		}
		cycle, err := findCycle("A", graph)
		So(err, ShouldBeNil)
		So(cycle, ShouldBeEmpty)
	})

	Convey("Trivial cycle", t, func() {
		graph := map[string][]string{
			"A": {"A"},
		}
		cycle, err := findCycle("A", graph)
		So(err, ShouldBeNil)
		So(cycle, ShouldEqual, []string{"A", "A"})
	})

	Convey("Loop", t, func() {
		graph := map[string][]string{
			"A": {"B"},
			"B": {"C"},
			"C": {"A"},
		}
		cycle, err := findCycle("A", graph)
		So(err, ShouldBeNil)
		So(cycle, ShouldEqual, []string{"A", "B", "C", "A"})
	})

	Convey("Irrelevant cycle", t, func() {
		graph := map[string][]string{
			"A": {"B"},
			"B": {"C"},
			"C": {"B"},
		}
		cycle, err := findCycle("A", graph)
		So(err, ShouldBeNil)
		So(cycle, ShouldBeEmpty)
	})

	Convey("Only one relevant cycle", t, func() {
		graph := map[string][]string{
			"A": {"B"},
			"B": {"C"},
			"C": {"B", "D"},
			"D": {"B", "C", "A"},
		}
		cycle, err := findCycle("A", graph)
		So(err, ShouldBeNil)
		So(cycle, ShouldEqual, []string{"A", "B", "C", "D", "A"})
	})

	Convey("Diamond with no cycles", t, func() {
		graph := map[string][]string{
			"A":  {"B1", "B2"},
			"B1": {"C"},
			"B2": {"C"},
			"C":  {},
		}
		cycle, err := findCycle("A", graph)
		So(err, ShouldBeNil)
		So(cycle, ShouldBeEmpty)
	})

	Convey("Diamond with cycles", t, func() {
		graph := map[string][]string{
			"A":  {"B2", "B1"},
			"B1": {"C"},
			"B2": {"C"},
			"C":  {"A"},
		}
		cycle, err := findCycle("A", graph)
		So(err, ShouldBeNil)
		// Note: this graph has two cycles, but the slice order dictates
		// the exploration order.
		So(cycle, ShouldEqual, []string{"A", "B2", "C", "A"})
	})
}
