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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/impl/memory"

	"go.chromium.org/luci/auth_service/constants"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/permissionscfg"
	"go.chromium.org/luci/auth_service/testsupport"
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

	ftt.Run("Project realms config validation", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		assert.Loosely(t, permissionscfg.SetConfigWithMetadata(
			ctx, testsupport.PermissionsCfg(), testsupport.PermissionsCfgMeta(),
		), should.BeNil)

		vCtx := &validation.Context{Context: ctx}
		configSet := "projects/test-project"
		path := "realms.cfg"

		t.Run("loading bad proto", func(t *ftt.Test) {
			content := []byte(`bad: "config"`)
			assert.Loosely(t, validateProjectRealmsCfg(vCtx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vCtx.Finalize(), should.NotBeNil)
		})

		t.Run("returns error for invalid config", func(t *ftt.Test) {
			content := []byte(serviceRealmsCfgContent)
			assert.Loosely(t, validateProjectRealmsCfg(vCtx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vCtx.Finalize(), should.NotBeNil)
		})

		t.Run("succeeds for valid config", func(t *ftt.Test) {
			content := []byte(projectRealmsCfgContent)
			assert.Loosely(t, validateProjectRealmsCfg(vCtx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vCtx.Finalize(), should.BeNil)
		})
	})
}

func TestServiceRealmsCfgValidation(t *testing.T) {
	t.Parallel()

	ftt.Run("Service realms config validation", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		assert.Loosely(t, permissionscfg.SetConfigWithMetadata(
			ctx, testsupport.PermissionsCfg(), testsupport.PermissionsCfgMeta(),
		), should.BeNil)

		vCtx := &validation.Context{Context: ctx}
		configSet := "services/test-app-id"
		path := "realms.cfg"

		t.Run("loading bad proto", func(t *ftt.Test) {
			content := []byte(`bad: "config"`)
			assert.Loosely(t, validateServiceRealmsCfg(vCtx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vCtx.Finalize(), should.NotBeNil)
		})

		t.Run("returns error for invalid config", func(t *ftt.Test) {
			content := []byte(`custom_roles {name: "role/missing-custom-prefix"}`)
			assert.Loosely(t, validateServiceRealmsCfg(vCtx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vCtx.Finalize(), should.NotBeNil)
		})

		t.Run("succeeds for valid config", func(t *ftt.Test) {
			content := []byte(serviceRealmsCfgContent)
			assert.Loosely(t, validateServiceRealmsCfg(vCtx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vCtx.Finalize(), should.BeNil)
		})
	})
}

func TestRealmsValidator(t *testing.T) {
	t.Parallel()

	ftt.Run("Common realms config validation", t, func(t *ftt.Test) {
		vCtx := &validation.Context{Context: context.Background()}
		validator := &realmsValidator{
			db: testsupport.PermissionsDB(true),
		}

		t.Run("empty config is valid", func(t *ftt.Test) {
			testCfg := &realmsconf.RealmsCfg{}
			validator.Validate(vCtx, testCfg)
			assert.Loosely(t, vCtx.Finalize(), should.BeNil)
		})

		t.Run("custom roles are validated", func(t *ftt.Test) {
			t.Run("must have correct prefix", func(t *ftt.Test) {
				testCfg := &realmsconf.RealmsCfg{
					CustomRoles: []*realmsconf.CustomRole{
						{Name: "role/notCustom"},
					},
				}
				validator.Validate(vCtx, testCfg)
				assert.Loosely(t, vCtx.Finalize(), should.ErrLike(
					fmt.Sprintf("role name should have prefix %q", constants.PrefixCustomRole)))
			})

			t.Run("duplicate definitions", func(t *ftt.Test) {
				testCfg := &realmsconf.RealmsCfg{
					CustomRoles: []*realmsconf.CustomRole{
						{Name: "customRole/tester"},
						{Name: "customRole/tester"},
					},
				}
				validator.Validate(vCtx, testCfg)
				assert.Loosely(t, vCtx.Finalize(), should.ErrLike("custom role already defined"))
			})

			t.Run("references unknown permission", func(t *ftt.Test) {
				testCfg := &realmsconf.RealmsCfg{
					CustomRoles: []*realmsconf.CustomRole{
						{Name: "customRole/tester", Permissions: []string{"luci.dev.test"}},
					},
				}
				validator.Validate(vCtx, testCfg)
				assert.Loosely(t, vCtx.Finalize(), should.ErrLike("permission \"luci.dev.test\" is not defined"))
			})

			t.Run("references unknown builtin role", func(t *ftt.Test) {
				testCfg := &realmsconf.RealmsCfg{
					CustomRoles: []*realmsconf.CustomRole{
						{Name: "customRole/tester", Extends: []string{"role/dev.unknown"}},
					},
				}
				validator.Validate(vCtx, testCfg)
				assert.Loosely(t, vCtx.Finalize(), should.ErrLike("not defined in permissions DB"))
			})

			t.Run("references unknown custom role", func(t *ftt.Test) {
				testCfg := &realmsconf.RealmsCfg{
					CustomRoles: []*realmsconf.CustomRole{
						{Name: "customRole/tester", Extends: []string{"customRole/unknown"}},
					},
				}
				validator.Validate(vCtx, testCfg)
				assert.Loosely(t, vCtx.Finalize(), should.ErrLike("not defined in the realms config"))
			})

			t.Run("uses unrecognized role format", func(t *ftt.Test) {
				testCfg := &realmsconf.RealmsCfg{
					CustomRoles: []*realmsconf.CustomRole{
						{Name: "customRole/tester", Extends: []string{"foo/unknown"}},
					},
				}
				validator.Validate(vCtx, testCfg)
				assert.Loosely(t, vCtx.Finalize(), should.ErrLike("bad role reference"))
			})

			t.Run("duplicate extension of custom roles", func(t *ftt.Test) {
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
				assert.Loosely(t, vCtx.Finalize(), should.ErrLike("in `extends` more than once"))
			})

			t.Run("cyclically extends itself", func(t *ftt.Test) {
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
				assert.Loosely(t, vCtx.Finalize(), should.ErrLike("cyclically extends itself"))
			})
		})

		t.Run("realms are validated", func(t *ftt.Test) {
			t.Run("unknown special realm name", func(t *ftt.Test) {
				testCfg := &realmsconf.RealmsCfg{
					Realms: []*realmsconf.Realm{
						{Name: "@notspecialenough"},
					},
				}
				validator.Validate(vCtx, testCfg)
				assert.Loosely(t, vCtx.Finalize(), should.ErrLike("unknown special realm name"))
			})

			t.Run("invalid realm name", func(t *ftt.Test) {
				testCfg := &realmsconf.RealmsCfg{
					Realms: []*realmsconf.Realm{
						{Name: "realmWithCapitals"},
					},
				}
				validator.Validate(vCtx, testCfg)
				assert.Loosely(t, vCtx.Finalize(), should.ErrLike("invalid realm name"))
			})

			t.Run("duplicate realm definitions", func(t *ftt.Test) {
				testCfg := &realmsconf.RealmsCfg{
					Realms: []*realmsconf.Realm{
						{Name: "some"},
						{Name: "some"},
					},
				}
				validator.Validate(vCtx, testCfg)
				assert.Loosely(t, vCtx.Finalize(), should.ErrLike("realm already defined"))
			})

			t.Run("root realm extends", func(t *ftt.Test) {
				testCfg := &realmsconf.RealmsCfg{
					Realms: []*realmsconf.Realm{
						{Name: "some"},
						{Name: "@root", Extends: []string{"some"}},
					},
				}
				validator.Validate(vCtx, testCfg)
				assert.Loosely(t, vCtx.Finalize(), should.ErrLike("root realm must not use `extends`"))
			})

			t.Run("extends unknown realm", func(t *ftt.Test) {
				testCfg := &realmsconf.RealmsCfg{
					Realms: []*realmsconf.Realm{
						{Name: "some", Extends: []string{"unknown"}},
					},
				}
				validator.Validate(vCtx, testCfg)
				assert.Loosely(t, vCtx.Finalize(), should.ErrLike("referencing an undefined realm"))
			})

			t.Run("extends duplicate realm", func(t *ftt.Test) {
				testCfg := &realmsconf.RealmsCfg{
					Realms: []*realmsconf.Realm{
						{Name: "base"},
						{Name: "some", Extends: []string{"base", "base"}},
					},
				}
				validator.Validate(vCtx, testCfg)
				assert.Loosely(t, vCtx.Finalize(), should.ErrLike("specified in `extends` more than once"))
			})

			t.Run("cyclically extends itself", func(t *ftt.Test) {
				testCfg := &realmsconf.RealmsCfg{
					Realms: []*realmsconf.Realm{
						{Name: "a", Extends: []string{"b"}},
						{Name: "b", Extends: []string{"a"}},
					},
				}
				validator.Validate(vCtx, testCfg)
				assert.Loosely(t, vCtx.Finalize(), should.ErrLike("cyclically extends itself"))
			})

			t.Run("realm bindings are validated", func(t *ftt.Test) {
				t.Run("references unknown custom role", func(t *ftt.Test) {
					testCfg := &realmsconf.RealmsCfg{
						Realms: []*realmsconf.Realm{{
							Name: "realm",
							Bindings: []*realmsconf.Binding{{
								Role: "customRole/unknown",
							}},
						}},
					}
					validator.Validate(vCtx, testCfg)
					assert.Loosely(t, vCtx.Finalize(), should.ErrLike("not defined in the realms config"))
				})

				t.Run("has invalid group name", func(t *ftt.Test) {
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
					assert.Loosely(t, vCtx.Finalize(), should.ErrLike("invalid group name"))
				})

				t.Run("has invalid principal format", func(t *ftt.Test) {
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
					assert.Loosely(t, vCtx.Finalize(), should.ErrLike("invalid principal format"))
				})

				t.Run("references unknown attribute", func(t *ftt.Test) {
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
					assert.Loosely(t, vCtx.Finalize(), should.ErrLike("unknown attribute"))
				})
			})
		})

		t.Run("complex valid config succeeds", func(t *ftt.Test) {
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
			assert.Loosely(t, vCtx.Finalize(), should.BeNil)
		})
	})

	ftt.Run("Internal references not allowed", t, func(t *ftt.Test) {
		vCtx := &validation.Context{Context: context.Background()}
		validator := &realmsValidator{
			db:            testsupport.PermissionsDB(true),
			allowInternal: false,
		}

		t.Run("references internal permission", func(t *ftt.Test) {
			testCfg := &realmsconf.RealmsCfg{
				CustomRoles: []*realmsconf.CustomRole{
					{Name: "customRole/tester", Permissions: []string{"luci.dev.implicitRoot"}},
				},
			}
			validator.Validate(vCtx, testCfg)
			assert.Loosely(t, vCtx.Finalize(), should.ErrLike("is internal"))
		})

		t.Run("references internal role", func(t *ftt.Test) {
			testCfg := &realmsconf.RealmsCfg{
				Realms: []*realmsconf.Realm{{
					Name: "realm",
					Bindings: []*realmsconf.Binding{{
						Role: "role/luci.internal.tester",
					}},
				}},
			}
			validator.Validate(vCtx, testCfg)
			assert.Loosely(t, vCtx.Finalize(), should.ErrLike("is internal"))
		})
	})

	ftt.Run("Internal references allowed", t, func(t *ftt.Test) {
		vCtx := &validation.Context{Context: context.Background()}
		validator := &realmsValidator{
			db:            testsupport.PermissionsDB(true),
			allowInternal: true,
		}

		t.Run("references internal permission", func(t *ftt.Test) {
			testCfg := &realmsconf.RealmsCfg{
				CustomRoles: []*realmsconf.CustomRole{
					{Name: "customRole/tester", Permissions: []string{"luci.dev.implicitRoot"}},
				},
			}
			validator.Validate(vCtx, testCfg)
			assert.Loosely(t, vCtx.Finalize(), should.BeNil)
		})

		t.Run("references internal role", func(t *ftt.Test) {
			testCfg := &realmsconf.RealmsCfg{
				Realms: []*realmsconf.Realm{{
					Name: "realm",
					Bindings: []*realmsconf.Binding{{
						Role: "role/luci.internal.tester",
					}},
				}},
			}
			validator.Validate(vCtx, testCfg)
			assert.Loosely(t, vCtx.Finalize(), should.BeNil)
		})
	})
}

func TestFindCycle(t *testing.T) {
	t.Parallel()

	ftt.Run("Invalid start node", t, func(t *ftt.Test) {
		graph := map[string][]string{
			"A": {},
		}
		_, err := findCycle("B", graph)
		assert.Loosely(t, err, should.ErrLike("unrecognized"))
	})

	ftt.Run("No cycles", t, func(t *ftt.Test) {
		graph := map[string][]string{
			"A": {"B", "C"},
			"B": {"C"},
			"C": {},
		}
		cycle, err := findCycle("A", graph)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cycle, should.BeEmpty)
	})

	ftt.Run("Trivial cycle", t, func(t *ftt.Test) {
		graph := map[string][]string{
			"A": {"A"},
		}
		cycle, err := findCycle("A", graph)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cycle, should.Resemble([]string{"A", "A"}))
	})

	ftt.Run("Loop", t, func(t *ftt.Test) {
		graph := map[string][]string{
			"A": {"B"},
			"B": {"C"},
			"C": {"A"},
		}
		cycle, err := findCycle("A", graph)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cycle, should.Resemble([]string{"A", "B", "C", "A"}))
	})

	ftt.Run("Irrelevant cycle", t, func(t *ftt.Test) {
		graph := map[string][]string{
			"A": {"B"},
			"B": {"C"},
			"C": {"B"},
		}
		cycle, err := findCycle("A", graph)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cycle, should.BeEmpty)
	})

	ftt.Run("Only one relevant cycle", t, func(t *ftt.Test) {
		graph := map[string][]string{
			"A": {"B"},
			"B": {"C"},
			"C": {"B", "D"},
			"D": {"B", "C", "A"},
		}
		cycle, err := findCycle("A", graph)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cycle, should.Resemble([]string{"A", "B", "C", "D", "A"}))
	})

	ftt.Run("Diamond with no cycles", t, func(t *ftt.Test) {
		graph := map[string][]string{
			"A":  {"B1", "B2"},
			"B1": {"C"},
			"B2": {"C"},
			"C":  {},
		}
		cycle, err := findCycle("A", graph)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cycle, should.BeEmpty)
	})

	ftt.Run("Diamond with cycles", t, func(t *ftt.Test) {
		graph := map[string][]string{
			"A":  {"B2", "B1"},
			"B1": {"C"},
			"B2": {"C"},
			"C":  {"A"},
		}
		cycle, err := findCycle("A", graph)
		assert.Loosely(t, err, should.BeNil)
		// Note: this graph has two cycles, but the slice order dictates
		// the exploration order.
		assert.Loosely(t, cycle, should.Resemble([]string{"A", "B2", "C", "A"}))
	})
}
