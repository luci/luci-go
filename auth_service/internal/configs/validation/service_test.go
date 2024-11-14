// Copyright 2022 The LUCI Authors.
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
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"
)

func TestAllowlistConfigValidation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("Validate Config", t, func(t *ftt.Test) {
		vctx := &validation.Context{Context: ctx}
		path := "ip_allowlist.cfg"
		configSet := ""

		t.Run("Loading bad proto", func(t *ftt.Test) {
			content := []byte(` bad: "config" `)
			assert.Loosely(t, validateAllowlist(vctx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("unknown field"))
		})

		const okCfg = `
		# Realistic config.
			ip_allowlists {
				name: "bots"
				includes: "region99"
				subnets: "108.177.31.12"
			}
			ip_allowlists {
				name: "chromium-test-dev-bots"
				includes: "bots"
			}
			ip_allowlists {
				name: "region99"
				subnets: "127.0.0.1/20"
			}
			assignments {
				identity: "user:test-user@google.com"
				ip_allowlist_name: "bots"
			}
		`

		t.Run("OK", func(t *ftt.Test) {
			t.Run("fully loaded", func(t *ftt.Test) {
				assert.Loosely(t, validateAllowlist(vctx, configSet, path, []byte(okCfg)), should.BeNil)
				assert.Loosely(t, vctx.Finalize(), should.BeNil)
			})
			t.Run("empty", func(t *ftt.Test) {
				assert.Loosely(t, validateAllowlist(vctx, configSet, path, []byte{}), should.BeNil)
				assert.Loosely(t, vctx.Finalize(), should.BeNil)
			})
		})

		t.Run("Catches regexp bugs", func(t *ftt.Test) {
			badCfg := `
				ip_allowlists {
					name: "?!chromium-test-dev-bots"
					includes: "bots"
				}
			`
			assert.Loosely(t, validateAllowlist(vctx, configSet, path, []byte(badCfg)), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike("invalid IP allowlist name"))
		})

		t.Run("Catches duplicate allowlist bug", func(t *ftt.Test) {
			badCfg := `
				ip_allowlists {
					name: "bots"
				}
				ip_allowlists {
					name: "chromium-test-dev-bots"
					includes: "bots"
				}
				ip_allowlists {
					name: "bots"
				}
			`
			assert.Loosely(t, validateAllowlist(vctx, configSet, path, []byte(badCfg)), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike("IP allowlist is defined twice"))
		})

		t.Run("Bad CIDR format", func(t *ftt.Test) {
			badCfg := `
				ip_allowlists {
					name: "bots"
					subnets: "not a subnet/"
				}
			`
			assert.Loosely(t, validateAllowlist(vctx, configSet, path, []byte(badCfg)), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike("invalid CIDR address"))
		})

		t.Run("Bad standard IP format", func(t *ftt.Test) {
			badCfg := `
				ip_allowlists {
					name: "bots"
					subnets: "not a subnet"
				}
			`
			assert.Loosely(t, validateAllowlist(vctx, configSet, path, []byte(badCfg)), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike("unable to parse IP for subnet"))
		})

		t.Run("Bad Identity format", func(t *ftt.Test) {
			badCfg := `
				assignments {
					identity: "test-user@example.com"
					ip_allowlist_name: "bots"
				}
			`
			assert.Loosely(t, validateAllowlist(vctx, configSet, path, []byte(badCfg)), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike("bad identity"))
		})

		t.Run("Unknown allowlist in assignment", func(t *ftt.Test) {
			badCfg := `
				assignments {
					identity: "user:test-user@example.com"
					ip_allowlist_name: "bots"
				}
			`
			assert.Loosely(t, validateAllowlist(vctx, configSet, path, []byte(badCfg)), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike("unknown allowlist"))
		})

		t.Run("Identity defined twice", func(t *ftt.Test) {
			badCfg := `
				ip_allowlists {
					name: "bots"
				}
				assignments {
					identity: "user:test-user@example.com"
					ip_allowlist_name: "bots"
				}
				assignments {
					identity: "user:test-user@example.com"
					ip_allowlist_name: "bots"
				}
			`
			assert.Loosely(t, validateAllowlist(vctx, configSet, path, []byte(badCfg)), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike("defined twice"))
		})

		t.Run("Validate allowlist unknown includes", func(t *ftt.Test) {
			badCfg := `
				ip_allowlists {
					name: "bots"
					subnets: []
					includes: ["unknown"]
				}
			`
			assert.Loosely(t, validateAllowlist(vctx, configSet, path, []byte(badCfg)), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike("contains unknown allowlist"))
		})

		t.Run("Validate allowlist includes cycle 1", func(t *ftt.Test) {
			badCfg := `
				ip_allowlists {
					name: "abc"
					subnets: []
					includes: ["abc"]
				}
			`
			assert.Loosely(t, validateAllowlist(vctx, configSet, path, []byte(badCfg)), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike("part of an included cycle"))
		})

		t.Run("Validate allowlist includes cycle 2", func(t *ftt.Test) {
			badCfg := `
				ip_allowlists {
					name: "abc"
					subnets: []
					includes: ["def"]
				}
				ip_allowlists {
					name: "def"
					subnets: []
					includes: ["abc"]
				}
			`
			assert.Loosely(t, validateAllowlist(vctx, configSet, path, []byte(badCfg)), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike("part of an included cycle"))
		})

		t.Run("Validate allowlist includes diamond", func(t *ftt.Test) {
			goodCfg := `
				ip_allowlists {
					name: "abc"
					subnets: []
					includes: ["middle1", "middle2"]
				}
				ip_allowlists {
					name: "middle1"
					subnets: []
					includes: ["inner"]
				}
				ip_allowlists {
					name: "middle2"
					subnets: []
					includes: ["inner"]
				}
				ip_allowlists {
					name: "inner"
					subnets: []
				}
			`
			assert.Loosely(t, validateAllowlist(vctx, configSet, path, []byte(goodCfg)), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.BeNil)
		})
	})
}

func TestOAuthConfigValidation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("Validate oauth.cfg", t, func(t *ftt.Test) {
		vctx := &validation.Context{Context: ctx}
		path := "oauth.cfg"
		configSet := ""

		t.Run("Loading bad proto", func(t *ftt.Test) {
			content := []byte(` bad: "config" `)
			assert.Loosely(t, validateOAuth(vctx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("unknown field"))
		})

		const okCfg = `
			# Realistic config.
			token_server_url: "https://example-token-server.appspot.com"

			primary_client_id: "123456.apps.example.com"
			primary_client_secret: "superdupersecret"

			client_ids: "12345-ranodmtext.apps.example.com"
			client_ids: "6789-morerandomtext.apps.example.com"
		`

		t.Run("OK", func(t *ftt.Test) {
			t.Run("Fully loaded", func(t *ftt.Test) {
				assert.Loosely(t, validateOAuth(vctx, configSet, path, []byte(okCfg)), should.BeNil)
				assert.Loosely(t, vctx.Finalize(), should.BeNil)
			})

			t.Run("empty", func(t *ftt.Test) {
				assert.Loosely(t, validateOAuth(vctx, configSet, path, []byte{}), should.BeNil)
				assert.Loosely(t, vctx.Finalize(), should.BeNil)
			})
		})

		t.Run("Bad URL Scheme in Token Server URL (appspot)", func(t *ftt.Test) {
			badCfg := `
				token_server_url: "http://example-token-server.appspot.com"
			`
			assert.Loosely(t, validateOAuth(vctx, configSet, path, []byte(badCfg)), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike("http:// can only be used with localhost servers"))
		})

		t.Run("Bad URL Scheme in Token Server URL ", func(t *ftt.Test) {
			badCfg := `
				token_server_url: "ftp://example-token-server.appspot.com"
			`
			assert.Loosely(t, validateOAuth(vctx, configSet, path, []byte(badCfg)), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike("only http:// or https:// scheme is accepted"))
		})
	})
}

func TestSecurityConfigValidation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("Validate security.cfg", t, func(t *ftt.Test) {
		vctx := &validation.Context{Context: ctx}
		path := "security.cfg"
		configSet := ""

		t.Run("Loading bad proto", func(t *ftt.Test) {
			content := []byte(` bad: "config" `)
			assert.Loosely(t, validateSecurityCfg(vctx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("unknown field"))
		})

		const okCfg = `
			# Realistic config.
			# Services running on GAE.
			internal_service_regexp: "(.*-dot-)?example-(dev|staging)\\.appspot\\.com"
			# Services running elsewhere.
			internal_service_regexp: "staging\\.example\\.api\\.dev"
		`

		t.Run("OK", func(t *ftt.Test) {
			t.Run("Fully loaded", func(t *ftt.Test) {
				assert.Loosely(t, validateSecurityCfg(vctx, configSet, path, []byte(okCfg)), should.BeNil)
				assert.Loosely(t, vctx.Finalize(), should.BeNil)
			})

			t.Run("empty", func(t *ftt.Test) {
				assert.Loosely(t, validateSecurityCfg(vctx, configSet, path, []byte{}), should.BeNil)
				assert.Loosely(t, vctx.Finalize(), should.BeNil)
			})
		})

		t.Run("Bad regexp", func(t *ftt.Test) {
			content := []byte(` internal_service_regexp: "???" `)
			assert.Loosely(t, validateSecurityCfg(vctx, configSet, path, content), should.BeNil)
			// Syntax is Perl, in Perl it is not allowed to stack repetition operators.
			assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("invalid nested repetition operator"))
		})

	})
}

func TestImportsConfigValidation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("validate imports.cfg", t, func(t *ftt.Test) {
		vctx := &validation.Context{Context: ctx}
		path := "imports.cfg"
		configSet := ""

		t.Run("Loading bad proto", func(t *ftt.Test) {
			content := []byte(` bad: "config" `)
			assert.Loosely(t, validateImportsCfg(vctx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("unknown field"))
		})

		t.Run("load config bad config structure", func(t *ftt.Test) {
			t.Run("tarball_upload entry \"ball\" is specified twice", func(t *ftt.Test) {
				content := []byte(`
					tarball_upload {
						name: "ball"
						authorized_uploader: "abc@example.com"
						systems: "s"
					}
					tarball_upload {
						name: "ball"
						authorized_uploader: "abc@example.com"
						systems: "s"
					}
				`)
				assert.Loosely(t, validateImportsCfg(vctx, configSet, path, content), should.BeNil)
				assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("specified twice"))
			})

			t.Run("bad authorized_uploader", func(t *ftt.Test) {
				t.Run("authorized_uploader is required in tarball_upload entry", func(t *ftt.Test) {
					content := []byte(`
						tarball_upload {
							name: "ball"
							systems: "s"
						}
					`)
					assert.Loosely(t, validateImportsCfg(vctx, configSet, path, content), should.BeNil)
					assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("authorized_uploader is required in tarball_upload entry"))
				})

				t.Run("invalid email \"not an email\" in tarball_upload entry \"ball\"", func(t *ftt.Test) {
					content := []byte(`
						tarball_upload {
							name: "ball"
							authorized_uploader: "not an email"
							systems: "s"
						}
					`)
					assert.Loosely(t, validateImportsCfg(vctx, configSet, path, content), should.BeNil)
					assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("not an email"))
				})
			})

			t.Run("bad systems", func(t *ftt.Test) {
				t.Run("tarball_upload entry with name \"ball\" needs systems field", func(t *ftt.Test) {
					content := []byte(`
						tarball_upload {
							name: "ball"
							authorized_uploader: "abc@example.com"
						}
					`)
					assert.Loosely(t, validateImportsCfg(vctx, configSet, path, content), should.BeNil)
					assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("needs a \"systems\" field"))
				})

				t.Run("\"tarball_upload\" entry with name \"conflicting\" specifies duplicated system(s)", func(t *ftt.Test) {
					content := []byte(`
						tarball_upload {
							name: "tarball3"
							authorized_uploader: "abc@example.com"
							systems: "s5"
							systems: "s6"
						}

						tarball_upload {
							name: "conflicting"
							authorized_uploader: "abc@example.com"
							systems: "external" # this one is predefined
							systems: "s1"
							systems: "s3"
							systems: "s5"
							systems: "ok"
						}
					  `)
					assert.Loosely(t, validateImportsCfg(vctx, configSet, path, content), should.BeNil)
					assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("\"conflicting\" is specifying a duplicated system(s): [external s5]"))
				})
			})
		})

		t.Run("load config happy config", func(t *ftt.Test) {
			okCfg := []byte(`
				# Realistic config.
				tarball_upload {
					name: "should be ignored"
					authorized_uploader: "example-service-account@example.com"
					systems: "zzz"
				}
			`)
			assert.Loosely(t, validateImportsCfg(vctx, configSet, path, okCfg), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.BeNil)
		})
	})
}

func TestPermissionsConfigValidation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("validate permissions.cfg", t, func(t *ftt.Test) {
		vctx := &validation.Context{Context: ctx}
		path := "permissions.cfg"
		configSet := ""

		t.Run("loading bad proto", func(t *ftt.Test) {
			content := []byte(` bad: "config" `)
			assert.Loosely(t, validatePermissionsCfg(vctx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("unknown field"))
		})

		t.Run("load config bad config structure", func(t *ftt.Test) {
			t.Run("name undefined", func(t *ftt.Test) {
				content := []byte(`
					role {
						permissions: [
							{
								name: "test.state.create",
							},
							{
								name: "test.state.delete",
							}
						]
						includes: [
							"role/testproject.state.creator"
						]
					}
				`)
				assert.Loosely(t, validatePermissionsCfg(vctx, configSet, path, content), should.BeNil)
				assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("name is required"))
			})

			t.Run("testing prefixes", func(t *ftt.Test) {
				content := []byte(`
					role {
						name: "notRole/test.role"
					}
				`)
				assert.Loosely(t, validatePermissionsCfg(vctx, configSet, path, content), should.BeNil)
				assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring(`invalid prefix, possible prefixes: ("role/", "customRole/", "role/luci.internal.")`))
			})

			t.Run("role defined twice", func(t *ftt.Test) {
				content := []byte(`
					role {
						name: "role/test.role"
					}
					role {
						name: "role/role.two"
					}
					role {
						name: "role/test.role"
					}
				`)
				assert.Loosely(t, validatePermissionsCfg(vctx, configSet, path, content), should.BeNil)
				assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("role/test.role is already defined"))
			})

			t.Run("invalid permissions format", func(t *ftt.Test) {
				content := []byte(`
					role {
						name: "role/test.role"
						permissions: [
							{
								name: "examplePermission",
							}
						]
					}
				`)
				assert.Loosely(t, validatePermissionsCfg(vctx, configSet, path, content), should.BeNil)
				assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("Permissions must have the form <service>.<subject>.<verb>"))
			})

			t.Run("invalid internal definition", func(t *ftt.Test) {
				content := []byte(`
					role {
						name: "role/test.role"
						permissions: [
							{
								name: "testinternal.state.create",
								internal: true
							}
						]
					}
				`)
				assert.Loosely(t, validatePermissionsCfg(vctx, configSet, path, content), should.BeNil)
				assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("invalid format: can only define internal permissions for internal roles"))
			})

			t.Run("role not defined in includes", func(t *ftt.Test) {
				content := []byte(`
					role {
						name: "role/test.role"
						includes: [
							"role/not.defined"
						]
					}
				`)
				assert.Loosely(t, validatePermissionsCfg(vctx, configSet, path, content), should.BeNil)
				assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("role/not.defined not defined"))
			})

			t.Run("cycles", func(t *ftt.Test) {
				t.Run("reference self", func(t *ftt.Test) {
					content := []byte(`
						role {
							name: "role/test.role"
							includes: [
								"role/test.role"
							]
						}
					`)
					assert.Loosely(t, validatePermissionsCfg(vctx, configSet, path, content), should.BeNil)
					assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("cycle found:"))
				})

				t.Run("small cycle", func(t *ftt.Test) {
					content := []byte(`
						role {
							name: "role/test.role"
							includes: [
								"role/test.role2"
							]
						}
						role {
							name: "role/test.role2"
							includes: [
								"role/test.role"
							]
						}
					`)
					assert.Loosely(t, validatePermissionsCfg(vctx, configSet, path, content), should.BeNil)
					assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("cycle found: role/test.role -> role/test.role2 -> role/test.role"))
				})

				t.Run("bigger cycle", func(t *ftt.Test) {
					content := []byte(`
						role {
							name: "role/test.role"
							includes: [
								"role/test.role3"
							]
						}
						role {
							name: "role/test.role2"
							includes: [
								"role/test.role"
							]
						}
						role {
							name: "role/test.role3"
							includes: [
								"role/test.role4"
							]
						}
						role {
							name: "role/test.role4"
							includes: [
								"role/test.role2"
							]
						}
					`)
					assert.Loosely(t, validatePermissionsCfg(vctx, configSet, path, content), should.BeNil)
					assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("cycle found: role/test.role -> role/test.role3 -> role/test.role4 -> role/test.role2 -> role/test.role"))
				})

				t.Run("cross edge", func(t *ftt.Test) {
					// This is ok!
					//          (r1)
					//          /  \
					//       (r2)  (r3)
					//        /      \
					//      (r4) --- (r5) r5 includes r4 here
					//      /        /  \
					//    (r6)     (r7) (r8)
					content := []byte(`
						role {
							name: "role/test.role1"
							includes: [
								"role/test.role2",
								"role/test.role3"
							]
						}
						role {
							name: "role/test.role2"
							includes: [
								"role/test.role4"
							]
						}
						role {
							name: "role/test.role3"
							includes: [
								"role/test.role5"
							]
						}
						role {
							name: "role/test.role4"
							includes: [
								"role/test.role6"
							]
						}
						role {
							name: "role/test.role5"
							includes: [
								"role/test.role4",
								"role/test.role7",
								"role/test.role8"
							]
						}
						role {
							name: "role/test.role6"
						}
						role {
							name: "role/test.role7"
						}
						role {
							name: "role/test.role8"
						}
					`)
					assert.Loosely(t, validatePermissionsCfg(vctx, configSet, path, content), should.BeNil)
					assert.Loosely(t, vctx.Finalize(), should.BeNil)
				})
			})
		})

		t.Run("valid configs", func(t *ftt.Test) {

			t.Run("1 entry", func(t *ftt.Test) {
				content := []byte(`
					role {
						name: "role/test.role.writer"
						permissions: [
							{
								name: "testproject.state.create",
							},
							{
								name: "testproject.state.update"
							}
						]
					}
				`)
				assert.Loosely(t, validatePermissionsCfg(vctx, configSet, path, content), should.BeNil)
				assert.Loosely(t, vctx.Finalize(), should.BeNil)
			})

			t.Run("multiple entries", func(t *ftt.Test) {
				content := []byte(`
					role {
						name: "role/test.role.writer"
						permissions: [
							{
								name: "testproject.state.create",
							},
							{
								name: "testproject.state.update"
							}
						]
					}
					role {
						name: "role/test.role.reader"
						permissions: [
							{
								name: "testproject.state.get",
							},
							{
								name: "testproject.state.list"
							}
						]
					}
					role {
						name: "role/test.role.owner"
						permissions: [
							{
								name: "testproject.state.delete"
							}
						]
						includes: [
							"role/test.role.writer",
							"role/test.role.reader"
						]
					}
					role {
						name: "role/luci.internal.test.role.owner",
						permissions: [
							{
								name: "testinternal.state.create",
								internal: true
							}
						]
					}
				`)
				assert.Loosely(t, validatePermissionsCfg(vctx, configSet, path, content), should.BeNil)
				assert.Loosely(t, vctx.Finalize(), should.BeNil)
			})
		})
	})
}

func TestNormalizeSubnet(t *testing.T) {
	t.Parallel()

	ftt.Run("Test normalizeSubnet works", t, func(t *ftt.Test) {
		t.Run("Invalid IP", func(t *ftt.Test) {
			invalidValues := []string{
				"not a subnet",
				"still not/",
				"123.4.05.6",   // check error returned for leading zero
				"123.4.5.6/33", // IPv4 is 32 bits
				"123::/129",    // IPv6 is 128 bits
			}
			for _, value := range invalidValues {
				_, err := normalizeSubnet(value)
				assert.Loosely(t, err, should.NotBeNil)
			}
		})
		t.Run("IPv4 CIDR", func(t *ftt.Test) {
			subnet, err := normalizeSubnet("123.4.5.64/26")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, subnet, should.Equal("123.4.5.64/26"))
		})
		t.Run("IPv6 CIDR", func(t *ftt.Test) {
			subnet, err := normalizeSubnet("123:0004:0050:0:0000:0:0:abcd/120")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, subnet, should.Equal("123:4:50::ab00/120"))
		})
		t.Run("Single IPv4", func(t *ftt.Test) {
			subnet, err := normalizeSubnet("123.4.5.6")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, subnet, should.Equal("123.4.5.6/32"))
		})
		t.Run("Single IPv6", func(t *ftt.Test) {
			subnet, err := normalizeSubnet("123:0004:0050:0:0000:0:0:abcd")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, subnet, should.Equal("123:4:50::abcd/128"))
		})
	})
}
