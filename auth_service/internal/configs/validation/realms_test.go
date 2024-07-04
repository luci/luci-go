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

		Convey("realms are validated", func() {
			Convey("unknown special realm name", func() {
				testCfg := &realmsconf.RealmsCfg{
					Realms: []*realmsconf.Realm{
						{Name: "@notspecialenough"},
					},
				}
				validator.Validate(vCtx, testCfg)
				So(vCtx.Finalize(), ShouldErrLike, "unknown special realm name")
			})

			Convey("invalid realm name", func() {
				testCfg := &realmsconf.RealmsCfg{
					Realms: []*realmsconf.Realm{
						{Name: "realmWithCapitals"},
					},
				}
				validator.Validate(vCtx, testCfg)
				So(vCtx.Finalize(), ShouldErrLike, "invalid realm name")
			})

			Convey("duplicate realm definitions", func() {
				testCfg := &realmsconf.RealmsCfg{
					Realms: []*realmsconf.Realm{
						{Name: "some"},
						{Name: "some"},
					},
				}
				validator.Validate(vCtx, testCfg)
				So(vCtx.Finalize(), ShouldErrLike, "realm already defined")
			})

			Convey("root realm extends", func() {
				testCfg := &realmsconf.RealmsCfg{
					Realms: []*realmsconf.Realm{
						{Name: "some"},
						{Name: "@root", Extends: []string{"some"}},
					},
				}
				validator.Validate(vCtx, testCfg)
				So(vCtx.Finalize(), ShouldErrLike, "root realm must not use `extends`")
			})

			Convey("extends unknown realm", func() {
				testCfg := &realmsconf.RealmsCfg{
					Realms: []*realmsconf.Realm{
						{Name: "some", Extends: []string{"unknown"}},
					},
				}
				validator.Validate(vCtx, testCfg)
				So(vCtx.Finalize(), ShouldErrLike, "referencing an undefined realm")
			})

			Convey("extends duplicate realm", func() {
				testCfg := &realmsconf.RealmsCfg{
					Realms: []*realmsconf.Realm{
						{Name: "base"},
						{Name: "some", Extends: []string{"base", "base"}},
					},
				}
				validator.Validate(vCtx, testCfg)
				So(vCtx.Finalize(), ShouldErrLike, "specified in `extends` more than once")
			})

			Convey("cyclically extends itself", func() {
				testCfg := &realmsconf.RealmsCfg{
					Realms: []*realmsconf.Realm{
						{Name: "a", Extends: []string{"b"}},
						{Name: "b", Extends: []string{"a"}},
					},
				}
				validator.Validate(vCtx, testCfg)
				So(vCtx.Finalize(), ShouldErrLike, "cyclically extends itself")
			})

			Convey("realm bindings are validated", func() {
				Convey("references unknown custom role", func() {
					testCfg := &realmsconf.RealmsCfg{
						Realms: []*realmsconf.Realm{{
							Name: "realm",
							Bindings: []*realmsconf.Binding{{
								Role: "customRole/unknown",
							}},
						}},
					}
					validator.Validate(vCtx, testCfg)
					So(vCtx.Finalize(), ShouldErrLike, "not defined in the realms config")
				})

				Convey("has invalid group name", func() {
					testCfg := &realmsconf.RealmsCfg{
						Realms: []*realmsconf.Realm{{
							Name: "realm",
							Bindings: []*realmsconf.Binding{{
								Role:       "role/dev.a",
								Principals: []string{"group:invalid!"},
							}},
						}},
					}
					validator.Validate(vCtx, testCfg)
					So(vCtx.Finalize(), ShouldErrLike, "invalid group name")
				})

				Convey("has invalid principal format", func() {
					testCfg := &realmsconf.RealmsCfg{
						Realms: []*realmsconf.Realm{{
							Name: "realm",
							Bindings: []*realmsconf.Binding{{
								Role:       "role/dev.a",
								Principals: []string{"notaprincipal"},
							}},
						}},
					}
					validator.Validate(vCtx, testCfg)
					So(vCtx.Finalize(), ShouldErrLike, "invalid principal format")
				})

				Convey("references unknown attribute", func() {
					testCfg := &realmsconf.RealmsCfg{
						Realms: []*realmsconf.Realm{{
							Name: "realm",
							Bindings: []*realmsconf.Binding{{
								Role:       "role/dev.a",
								Principals: []string{"group:aaa"},
								Conditions: []*realmsconf.Condition{{
									Op: &realmsconf.Condition_Restrict{
										Restrict: &realmsconf.Condition_AttributeRestriction{
											Attribute: "unknown.test.attribute",
										},
									},
								}},
							}},
						}},
					}
					validator.Validate(vCtx, testCfg)
					So(vCtx.Finalize(), ShouldErrLike, "unknown attribute")
				})
			})
		})

		Convey("complex valid config succeeds", func() {
			testCfg := &realmsconf.RealmsCfg{
				CustomRoles: []*realmsconf.CustomRole{
					{
						Name: "customRole/empty-is-ok",
					},
					{
						Name:    "customRole/extends-builtin",
						Extends: []string{"role/dev.a", "role/dev.b"},
					},
					{
						Name: "customRole/permissions-and-extension",
						Extends: []string{
							"role/dev.a",
							"customRole/extends-builtin",
							"customRole/empty-is-ok",
						},
						Permissions: []string{"luci.dev.p1", "luci.dev.p2"},
					},
				},
				Realms: []*realmsconf.Realm{
					{
						Name: "@root",
						Bindings: []*realmsconf.Binding{
							{
								Role: "customRole/permissions-and-extension",
								Principals: []string{
									"group:aaa",
									"user:a@example.com",
									"project:zzz",
								},
							},
							{
								Role:       "role/dev.a",
								Principals: []string{"group:bbb"},
							},
						},
					},
					{
						Name:    "@legacy",
						Extends: []string{"@root", "some-realm/a", "some-realm/b"},
					},
					{
						Name:    "@project",
						Extends: []string{"some-realm/b"},
					},
					{
						Name:    "some-realm/a",
						Extends: []string{"some-realm/b"},
						Bindings: []*realmsconf.Binding{
							{
								Role:       "role/dev.a",
								Principals: []string{"group:aaa"},
							},
							{
								Role:       "role/dev.a",
								Principals: []string{"group:bbb"},
								Conditions: []*realmsconf.Condition{{
									Op: &realmsconf.Condition_Restrict{
										Restrict: &realmsconf.Condition_AttributeRestriction{
											Attribute: "a1",
											Values:    []string{"x", "y", "z"},
										},
									},
								}},
							},
						},
					},
					{
						Name: "some-realm/b",
						Bindings: []*realmsconf.Binding{
							{
								Role:       "role/dev.b",
								Principals: []string{"group:bbb"},
							},
						},
					},
				},
			}
			validator.Validate(vCtx, testCfg)
			So(vCtx.Finalize(), ShouldBeNil)
		})
	})

	Convey("Internal references not allowed", t, func() {
		vCtx := &validation.Context{Context: context.Background()}
		validator := &realmsValidator{
			db:            testsupport.PermissionsDB(true),
			allowInternal: false,
		}

		Convey("references internal permission", func() {
			testCfg := &realmsconf.RealmsCfg{
				CustomRoles: []*realmsconf.CustomRole{
					{Name: "customRole/tester", Permissions: []string{"luci.dev.implicitRoot"}},
				},
			}
			validator.Validate(vCtx, testCfg)
			So(vCtx.Finalize(), ShouldErrLike, "is internal")
		})

		Convey("references internal role", func() {
			testCfg := &realmsconf.RealmsCfg{
				Realms: []*realmsconf.Realm{{
					Name: "realm",
					Bindings: []*realmsconf.Binding{{
						Role: "role/luci.internal.tester",
					}},
				}},
			}
			validator.Validate(vCtx, testCfg)
			So(vCtx.Finalize(), ShouldErrLike, "is internal")
		})
	})

	Convey("Internal references allowed", t, func() {
		vCtx := &validation.Context{Context: context.Background()}
		validator := &realmsValidator{
			db:            testsupport.PermissionsDB(true),
			allowInternal: true,
		}

		Convey("references internal permission", func() {
			testCfg := &realmsconf.RealmsCfg{
				CustomRoles: []*realmsconf.CustomRole{
					{Name: "customRole/tester", Permissions: []string{"luci.dev.implicitRoot"}},
				},
			}
			validator.Validate(vCtx, testCfg)
			So(vCtx.Finalize(), ShouldBeNil)
		})

		Convey("references internal role", func() {
			testCfg := &realmsconf.RealmsCfg{
				Realms: []*realmsconf.Realm{{
					Name: "realm",
					Bindings: []*realmsconf.Binding{{
						Role: "role/luci.internal.tester",
					}},
				}},
			}
			validator.Validate(vCtx, testCfg)
			So(vCtx.Finalize(), ShouldBeNil)
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
