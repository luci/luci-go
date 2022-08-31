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

		Convey("Catches duplicate allowlist bug", func(){
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

		Convey("Catches multiple errors", func() {
			badCfg := `
				ip_allowlists {
					name: "bots"
				}
				ip_allowlists {
					name: "?!chromium-test-dev-bots"
					includes: "bots"
				}
				ip_allowlists {
					name: "bots"
				}
			`
			So(validateAllowlist(vctx, configSet, path, []byte(badCfg)), ShouldBeNil)
			errs := vctx.Finalize().(*validation.Error).Errors
			So(errs, ShouldContainErr, "ip allowlist is defined twice")
			So(errs, ShouldContainErr, "invalid ip allowlist name")
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

		Convey("Multiple subnet errors in one vctx", func() {
			badCfg := `
				ip_allowlists {
					name: "bots"
					subnets: "not a subnet/"
					subnets: "not a subnet"
				}
			`
			So(validateAllowlist(vctx, configSet, path, []byte(badCfg)), ShouldBeNil)
			errs := vctx.Finalize().(*validation.Error).Errors
			So(errs, ShouldContainErr, "invalid CIDR address")
			So(errs, ShouldContainErr, "unable to parse ip for subnet")
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
	})
}