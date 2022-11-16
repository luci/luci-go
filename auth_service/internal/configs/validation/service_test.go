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
			So(vctx.Finalize(), ShouldErrLike, "invalid ip allowlist name")
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
			So(vctx.Finalize(), ShouldErrLike, "ip allowlist is defined twice")
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
			So(vctx.Finalize(), ShouldErrLike, "unable to parse ip for subnet")
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
			So(vctx.Finalize(), ShouldErrLike, "only https:// scheme is accepted for appspot hosts, it can be omitted")
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
