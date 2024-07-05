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
	"go.chromium.org/luci/gae/impl/memory"

	"go.chromium.org/luci/auth_service/constants"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/permissionscfg"
	"go.chromium.org/luci/auth_service/testsupport"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

const (
	serviceRealmsCfgContent = `
	custom_roles {
		name: "customRole/with-int-perm"
		permissions: "luci.dev.p1"
		permissions: "luci.dev.implicitRoot"
	}
	realms {
		name: "realm"
		bindings {
			role: "role/dev.a"
			principals: "group:aaa"
		}
		bindings {
			role: "role/luci.internal.tester"
			principals: "group:aaa"
		}
		bindings {
			role: "customRole/with-int-perm"
			principals: "group:aaa"
		}
	}
	`

	projectRealmsCfgContent = `
	custom_roles {
		name: "customRole/empty-is-ok"
	}
	custom_roles {
		name: "customRole/extends-builtin"
		extends: "role/dev.a"
		extends: "role/dev.b"
	}
	custom_roles {
		name: "customRole/permissions-and-extension"
		extends: "role/dev.a"
		extends: "customRole/extends-builtin"
		extends: "customRole/empty-is-ok"
		permissions: "luci.dev.p1"
		permissions: "luci.dev.p2"
	}
	realms {
		name: "@root"
		bindings {
			role: "customRole/permissions-and-extension"
			principals: "group:aaa"
			principals: "user:a@example.com"
			principals: "project:zzz"
		}
		bindings {
			role: "role/dev.a"
			principals: "group:bbb"
		}
	}
	realms {
		name: "@legacy"
		extends: "@root"
		extends: "some-realm/a"
		extends: "some-realm/b"
	}
	realms {
		name: "@project"
		extends: "some-realm/b"
	}
	realms {
		name: "some-realm/a"
		extends: "some-realm/b"
		bindings {
			role: "role/dev.a"
			principals: "group:aaa"
		}
		bindings {
			role: "role/dev.a"
			principals: "group:bbb"
			conditions {
				restrict {
					attribute: "a1"
					values: "x"
					values: "y"
					values: "z"
				}
			}
		}
	}
	realms {
		name: "some-realm/b"
		bindings {
			role: "role/dev.b"
			principals: "group:bbb"
		}
	}
	`
)

func TestProjectRealmsCfgValidation(t *testing.T) {
	t.Parallel()

	Convey("Project realms config validation", t, func() {
		ctx := memory.Use(context.Background())
		So(permissionscfg.SetConfigWithMetadata(
			ctx, testsupport.PermissionsCfg(), testsupport.PermissionsCfgMeta(),
		), ShouldBeNil)

		vCtx := &validation.Context{Context: ctx}
		configSet := "projects/test-project"
		path := "realms.cfg"

		Convey("loading bad proto", func() {
			content := []byte(`bad: "config"`)
			So(validateProjectRealmsCfg(vCtx, configSet, path, content), ShouldBeNil)
			So(vCtx.Finalize(), ShouldNotBeNil)
		})

		Convey("returns error for invalid config", func() {
			content := []byte(serviceRealmsCfgContent)
			So(validateProjectRealmsCfg(vCtx, configSet, path, content), ShouldBeNil)
			So(vCtx.Finalize(), ShouldNotBeNil)
		})

		Convey("succeeds for valid config", func() {
			content := []byte(projectRealmsCfgContent)
			So(validateProjectRealmsCfg(vCtx, configSet, path, content), ShouldBeNil)
			So(vCtx.Finalize(), ShouldBeNil)
		})
	})
}

func TestServiceRealmsCfgValidation(t *testing.T) {
	t.Parallel()

	Convey("Service realms config validation", t, func() {
		ctx := memory.Use(context.Background())
		So(permissionscfg.SetConfigWithMetadata(
			ctx, testsupport.PermissionsCfg(), testsupport.PermissionsCfgMeta(),
		), ShouldBeNil)

		vCtx := &validation.Context{Context: ctx}
		configSet := "services/test-app-id"
		path := "realms.cfg"

		Convey("loading bad proto", func() {
			content := []byte(`bad: "config"`)
			So(validateServiceRealmsCfg(vCtx, configSet, path, content), ShouldBeNil)
			So(vCtx.Finalize(), ShouldNotBeNil)
		})

		Convey("returns error for invalid config", func() {
			content := []byte(`custom_roles {name: "role/missing-custom-prefix"}`)
			So(validateServiceRealmsCfg(vCtx, configSet, path, content), ShouldBeNil)
			So(vCtx.Finalize(), ShouldNotBeNil)
		})

		Convey("succeeds for valid config", func() {
			content := []byte(serviceRealmsCfgContent)
			So(validateServiceRealmsCfg(vCtx, configSet, path, content), ShouldBeNil)
			So(vCtx.Finalize(), ShouldBeNil)
		})
	})
}

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
