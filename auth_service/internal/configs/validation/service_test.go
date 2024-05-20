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

	"go.chromium.org/luci/config/validation"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestAllowlistConfigValidation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("Validate Config", t, func() {
		vctx := &validation.Context{Context: ctx}
		path := "ip_allowlist.cfg"
		configSet := ""

		Convey("Loading bad proto", func() {
			content := []byte(` bad: "config" `)
			So(validateAllowlist(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize().Error(), ShouldContainSubstring, "unknown field")
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

		Convey("OK", func() {
			Convey("fully loaded", func() {
				So(validateAllowlist(vctx, configSet, path, []byte(okCfg)), ShouldBeNil)
				So(vctx.Finalize(), ShouldBeNil)
			})
			Convey("empty", func() {
				So(validateAllowlist(vctx, configSet, path, []byte{}), ShouldBeNil)
				So(vctx.Finalize(), ShouldBeNil)
			})
		})

		Convey("Catches regexp bugs", func() {
			badCfg := `
				ip_allowlists {
					name: "?!chromium-test-dev-bots"
					includes: "bots"
				}
			`
			So(validateAllowlist(vctx, configSet, path, []byte(badCfg)), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, "invalid IP allowlist name")
		})

		Convey("Catches duplicate allowlist bug", func() {
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
			So(validateAllowlist(vctx, configSet, path, []byte(badCfg)), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, "IP allowlist is defined twice")
		})

		Convey("Bad CIDR format", func() {
			badCfg := `
				ip_allowlists {
					name: "bots"
					subnets: "not a subnet/"
				}
			`
			So(validateAllowlist(vctx, configSet, path, []byte(badCfg)), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, "invalid CIDR address")
		})

		Convey("Bad standard IP format", func() {
			badCfg := `
				ip_allowlists {
					name: "bots"
					subnets: "not a subnet"
				}
			`
			So(validateAllowlist(vctx, configSet, path, []byte(badCfg)), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, "unable to parse IP for subnet")
		})

		Convey("Bad Identity format", func() {
			badCfg := `
				assignments {
					identity: "test-user@example.com"
					ip_allowlist_name: "bots"
				}
			`
			So(validateAllowlist(vctx, configSet, path, []byte(badCfg)), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, "bad identity")
		})

		Convey("Unknown allowlist in assignment", func() {
			badCfg := `
				assignments {
					identity: "user:test-user@example.com"
					ip_allowlist_name: "bots"
				}
			`
			So(validateAllowlist(vctx, configSet, path, []byte(badCfg)), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, "unknown allowlist")
		})

		Convey("Identity defined twice", func() {
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
			So(validateAllowlist(vctx, configSet, path, []byte(badCfg)), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, "defined twice")
		})

		Convey("Validate allowlist unknown includes", func() {
			badCfg := `
				ip_allowlists {
					name: "bots"
					subnets: []
					includes: ["unknown"]
				}
			`
			So(validateAllowlist(vctx, configSet, path, []byte(badCfg)), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, "contains unknown allowlist")
		})

		Convey("Validate allowlist includes cycle 1", func() {
			badCfg := `
				ip_allowlists {
					name: "abc"
					subnets: []
					includes: ["abc"]
				}
			`
			So(validateAllowlist(vctx, configSet, path, []byte(badCfg)), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, "part of an included cycle")
		})

		Convey("Validate allowlist includes cycle 2", func() {
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
			So(validateAllowlist(vctx, configSet, path, []byte(badCfg)), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, "part of an included cycle")
		})

		Convey("Validate allowlist includes diamond", func() {
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
			So(validateAllowlist(vctx, configSet, path, []byte(goodCfg)), ShouldBeNil)
			So(vctx.Finalize(), ShouldBeNil)
		})
	})
}

func TestOAuthConfigValidation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("Validate oauth.cfg", t, func() {
		vctx := &validation.Context{Context: ctx}
		path := "oauth.cfg"
		configSet := ""

		Convey("Loading bad proto", func() {
			content := []byte(` bad: "config" `)
			So(validateOAuth(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize().Error(), ShouldContainSubstring, "unknown field")
		})

		const okCfg = `
			# Realistic config.
			token_server_url: "https://example-token-server.appspot.com"

			primary_client_id: "123456.apps.example.com"
			primary_client_secret: "superdupersecret"

			client_ids: "12345-ranodmtext.apps.example.com"
			client_ids: "6789-morerandomtext.apps.example.com"
		`

		Convey("OK", func() {
			Convey("Fully loaded", func() {
				So(validateOAuth(vctx, configSet, path, []byte(okCfg)), ShouldBeNil)
				So(vctx.Finalize(), ShouldBeNil)
			})

			Convey("empty", func() {
				So(validateOAuth(vctx, configSet, path, []byte{}), ShouldBeNil)
				So(vctx.Finalize(), ShouldBeNil)
			})
		})

		Convey("Bad URL Scheme in Token Server URL (appspot)", func() {
			badCfg := `
				token_server_url: "http://example-token-server.appspot.com"
			`
			So(validateOAuth(vctx, configSet, path, []byte(badCfg)), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, "http:// can only be used with localhost servers")
		})

		Convey("Bad URL Scheme in Token Server URL ", func() {
			badCfg := `
				token_server_url: "ftp://example-token-server.appspot.com"
			`
			So(validateOAuth(vctx, configSet, path, []byte(badCfg)), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, "only http:// or https:// scheme is accepted")
		})
	})
}

func TestSecurityConfigValidation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("Validate security.cfg", t, func() {
		vctx := &validation.Context{Context: ctx}
		path := "security.cfg"
		configSet := ""

		Convey("Loading bad proto", func() {
			content := []byte(` bad: "config" `)
			So(validateSecurityCfg(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize().Error(), ShouldContainSubstring, "unknown field")
		})

		const okCfg = `
			# Realistic config.
			# Services running on GAE.
			internal_service_regexp: "(.*-dot-)?example-(dev|staging)\\.appspot\\.com"
			# Services running elsewhere.
			internal_service_regexp: "staging\\.example\\.api\\.dev"
		`

		Convey("OK", func() {
			Convey("Fully loaded", func() {
				So(validateSecurityCfg(vctx, configSet, path, []byte(okCfg)), ShouldBeNil)
				So(vctx.Finalize(), ShouldBeNil)
			})

			Convey("empty", func() {
				So(validateSecurityCfg(vctx, configSet, path, []byte{}), ShouldBeNil)
				So(vctx.Finalize(), ShouldBeNil)
			})
		})

		Convey("Bad regexp", func() {
			content := []byte(` internal_service_regexp: "???" `)
			So(validateSecurityCfg(vctx, configSet, path, content), ShouldBeNil)
			// Syntax is Perl, in Perl it is not allowed to stack repetition operators.
			So(vctx.Finalize().Error(), ShouldContainSubstring, "invalid nested repetition operator")
		})

	})
}

func TestImportsConfigValidation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("validate imports.cfg", t, func() {
		vctx := &validation.Context{Context: ctx}
		path := "imports.cfg"
		configSet := ""

		Convey("Loading bad proto", func() {
			content := []byte(` bad: "config" `)
			So(validateImportsCfg(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize().Error(), ShouldContainSubstring, "unknown field")
		})

		Convey("load config bad config structure", func() {
			Convey("no urls", func() {
				Convey("url field required in tarball entry", func() {
					content := []byte(`
						tarball {
							systems: "s1"
						}
					`)
					So(validateImportsCfg(vctx, configSet, path, content), ShouldBeNil)
					So(vctx.Finalize().Error(), ShouldContainSubstring, "url field required")
				})

				Convey("url field required in plainlist entry", func() {
					content := []byte(`
						plainlist {
							group: "test-group"
						}
					`)
					So(validateImportsCfg(vctx, configSet, path, content), ShouldBeNil)
					So(vctx.Finalize().Error(), ShouldContainSubstring, "url field required")
				})
			})

			Convey("tarball_upload entry \"ball\" is specified twice", func() {
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
				So(validateImportsCfg(vctx, configSet, path, content), ShouldBeNil)
				So(vctx.Finalize().Error(), ShouldContainSubstring, "specified twice")
			})

			Convey("bad authorized_uploader", func() {
				Convey("authorized_uploader is required in tarball_upload entry", func() {
					content := []byte(`
						tarball_upload {
							name: "ball"
							systems: "s"
						}
					`)
					So(validateImportsCfg(vctx, configSet, path, content), ShouldBeNil)
					So(vctx.Finalize().Error(), ShouldContainSubstring, "authorized_uploader is required in tarball_upload entry")
				})

				Convey("invalid email \"not an email\" in tarball_upload entry \"ball\"", func() {
					content := []byte(`
						tarball_upload {
							name: "ball"
							authorized_uploader: "not an email"
							systems: "s"
						}
					`)
					So(validateImportsCfg(vctx, configSet, path, content), ShouldBeNil)
					So(vctx.Finalize().Error(), ShouldContainSubstring, "not an email")
				})
			})

			Convey("bad systems", func() {
				Convey("tarball entry with URL \"https//example.com/tarball\" needs systems field", func() {
					content := []byte(`
						tarball {
							url: "http://example.com/tarball"
						}
					`)
					So(validateImportsCfg(vctx, configSet, path, content), ShouldBeNil)
					So(vctx.Finalize().Error(), ShouldContainSubstring, "needs a \"systems\" field")
				})

				Convey("tarball_upload entry with name \"ball\" needs systems field", func() {
					content := []byte(`
						tarball_upload {
							name: "ball"
							authorized_uploader: "abc@example.com"
						}
					`)
					So(validateImportsCfg(vctx, configSet, path, content), ShouldBeNil)
					So(vctx.Finalize().Error(), ShouldContainSubstring, "needs a \"systems\" field")
				})

				Convey("\"tarball_upload\" entry with name \"conflicting\" specifies duplicated system(s)", func() {
					content := []byte(`
						tarball {
							url: "http://example.com/tarball1"
							systems: "s1"
							systems: "s2"
						}

						tarball {
							url: "http://example.com/tarball2"
							systems: "s3"
							systems: "s4"
						}

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
					So(validateImportsCfg(vctx, configSet, path, content), ShouldBeNil)
					So(vctx.Finalize().Error(), ShouldContainSubstring, "\"conflicting\" is specifying a duplicated system(s): [external s1 s3 s5]")
				})
			})

			Convey("bad plainlists", func() {
				Convey("\"plainlist\" entry \"http://example.com/plainlist\" needs a \"group\" field", func() {
					content := []byte(`
						plainlist {
							url: "http://example.com/plainlist"
						}
					`)
					So(validateImportsCfg(vctx, configSet, path, content), ShouldBeNil)
					So(vctx.Finalize().Error(), ShouldContainSubstring, "entry \"http://example.com/plainlist\" needs a \"group\" field")
				})

				Convey("group \"gr\" is imported twice", func() {
					content := []byte(`
						plainlist {
							url: "http://example.com/plainlist1"
							group: "gr"
						}
						plainlist {
							url: "http://example.com/plainlist2"
							group: "gr"
						}
					`)
					So(validateImportsCfg(vctx, configSet, path, content), ShouldBeNil)
					So(vctx.Finalize().Error(), ShouldContainSubstring, "the group \"gr\" is imported twice")
				})
			})
		})

		Convey("load config happy config", func() {
			okCfg := []byte(`
				# Realistic config.
				tarball_upload {
					name: "should be ignored"
					authorized_uploader: "example-service-account@example.com"
					systems: "zzz"
				}
				tarball {
					url: "https://fake_tarball"
					groups: "ldap/new"
					oauth_scopes: "scope"
					systems: "ldap"
				}
				plainlist {
					url: "example.com"
					group: "test-group"
				}
			`)
			So(validateImportsCfg(vctx, configSet, path, okCfg), ShouldBeNil)
			So(vctx.Finalize(), ShouldBeNil)
		})
	})
}

func TestPermissionsConfigValidation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("validate permissions.cfg", t, func() {
		vctx := &validation.Context{Context: ctx}
		path := "permissions.cfg"
		configSet := ""

		Convey("loading bad proto", func() {
			content := []byte(` bad: "config" `)
			So(validatePermissionsCfg(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize().Error(), ShouldContainSubstring, "unknown field")
		})

		Convey("load config bad config structure", func() {
			Convey("name undefined", func() {
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
				So(validatePermissionsCfg(vctx, configSet, path, content), ShouldBeNil)
				So(vctx.Finalize().Error(), ShouldContainSubstring, "name is required")
			})

			Convey("testing prefixes", func() {
				content := []byte(`
					role {
						name: "notRole/test.role"
					}
				`)
				So(validatePermissionsCfg(vctx, configSet, path, content), ShouldBeNil)
				So(vctx.Finalize().Error(), ShouldContainSubstring, `invalid prefix, possible prefixes: ("role/", "customRole/", "role/luci.internal.")`)
			})

			Convey("role defined twice", func() {
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
				So(validatePermissionsCfg(vctx, configSet, path, content), ShouldBeNil)
				So(vctx.Finalize().Error(), ShouldContainSubstring, "role/test.role is already defined")
			})

			Convey("invalid permissions format", func() {
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
				So(validatePermissionsCfg(vctx, configSet, path, content), ShouldBeNil)
				So(vctx.Finalize().Error(), ShouldContainSubstring, "Permissions must have the form <service>.<subject>.<verb>")
			})

			Convey("invalid internal definition", func() {
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
				So(validatePermissionsCfg(vctx, configSet, path, content), ShouldBeNil)
				So(vctx.Finalize().Error(), ShouldContainSubstring, "invalid format: can only define internal permissions for internal roles")
			})

			Convey("role not defined in includes", func() {
				content := []byte(`
					role {
						name: "role/test.role"
						includes: [
							"role/not.defined"
						]
					}
				`)
				So(validatePermissionsCfg(vctx, configSet, path, content), ShouldBeNil)
				So(vctx.Finalize().Error(), ShouldContainSubstring, "role/not.defined not defined")
			})

			Convey("cycles", func() {
				Convey("reference self", func() {
					content := []byte(`
						role {
							name: "role/test.role"
							includes: [
								"role/test.role"
							]
						}
					`)
					So(validatePermissionsCfg(vctx, configSet, path, content), ShouldBeNil)
					So(vctx.Finalize().Error(), ShouldContainSubstring, "cycle found:")
				})

				Convey("small cycle", func() {
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
					So(validatePermissionsCfg(vctx, configSet, path, content), ShouldBeNil)
					So(vctx.Finalize().Error(), ShouldContainSubstring, "cycle found: role/test.role -> role/test.role2 -> role/test.role")
				})

				Convey("bigger cycle", func() {
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
					So(validatePermissionsCfg(vctx, configSet, path, content), ShouldBeNil)
					So(vctx.Finalize().Error(), ShouldContainSubstring, "cycle found: role/test.role -> role/test.role3 -> role/test.role4 -> role/test.role2 -> role/test.role")
				})

				Convey("cross edge", func() {
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
					So(validatePermissionsCfg(vctx, configSet, path, content), ShouldBeNil)
					So(vctx.Finalize(), ShouldBeNil)
				})
			})
		})

		Convey("valid configs", func() {

			Convey("1 entry", func() {
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
				So(validatePermissionsCfg(vctx, configSet, path, content), ShouldBeNil)
				So(vctx.Finalize(), ShouldBeNil)
			})

			Convey("multiple entries", func() {
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
				So(validatePermissionsCfg(vctx, configSet, path, content), ShouldBeNil)
				So(vctx.Finalize(), ShouldBeNil)
			})
		})
	})
}

func TestNormalizeSubnet(t *testing.T) {
	t.Parallel()

	Convey("Test normalizeSubnet works", t, func() {
		Convey("Invalid IP", func() {
			invalidValues := []string{
				"not a subnet",
				"still not/",
				"123.4.05.6",   // check error returned for leading zero
				"123.4.5.6/33", // IPv4 is 32 bits
				"123::/129",    // IPv6 is 128 bits
			}
			for _, value := range invalidValues {
				_, err := normalizeSubnet(value)
				So(err, ShouldNotBeNil)
			}
		})
		Convey("IPv4 CIDR", func() {
			subnet, err := normalizeSubnet("123.4.5.64/26")
			So(err, ShouldBeNil)
			So(subnet, ShouldEqual, "123.4.5.64/26")
		})
		Convey("IPv6 CIDR", func() {
			subnet, err := normalizeSubnet("123:0004:0050:0:0000:0:0:abcd/120")
			So(err, ShouldBeNil)
			So(subnet, ShouldEqual, "123:4:50::ab00/120")
		})
		Convey("Single IPv4", func() {
			subnet, err := normalizeSubnet("123.4.5.6")
			So(err, ShouldBeNil)
			So(subnet, ShouldEqual, "123.4.5.6/32")
		})
		Convey("Single IPv6", func() {
			subnet, err := normalizeSubnet("123:0004:0050:0:0000:0:0:abcd")
			So(err, ShouldBeNil)
			So(subnet, ShouldEqual, "123:4:50::abcd/128")
		})
	})
}
