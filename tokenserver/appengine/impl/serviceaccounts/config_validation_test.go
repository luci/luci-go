// Copyright 2017 The LUCI Authors.
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

package serviceaccounts

import (
	"testing"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/policy"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/config/validation"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		Cfg    string
		Errors []string
	}{
		{
			Cfg: `
				rules {
					name: "rule 1"
					owner: "developer@example.com"
					service_account: "abc@robots.com"
					allowed_scope: "https://www.googleapis.com/scope"
					end_user: "user:abc@example.com"
					end_user: "group:enduser-group"
					proxy: "user:proxy@example.com"
					proxy: "group:proxy-group"
					max_grant_validity_duration: 3600
				}

				rules {
					name: "rule 2"
					owner: "developer@example.com"
					service_account: "def@robots.com"
					allowed_scope: "https://www.googleapis.com/scope"
					end_user: "user:abc@example.com"
					end_user: "group:enduser-group"
					proxy: "user:proxy@example.com"
					proxy: "group:proxy-group"
					max_grant_validity_duration: 3600
				}

				defaults {
					allowed_scope: "https://www.googleapis.com/scope"
					max_grant_validity_duration: 3600
				}
			`,
		},

		// Minimal config.
		{
			Cfg: `
				rules {
					name: "rule 1"
				}
			`,
		},

		{
			Cfg: `
				rules {
					name: "rule 1"
				}
				rules {
					name: "rule 1"
				}
			`,
			Errors: []string{"two rules with identical name"},
		},

		{
			Cfg: `
				rules {
					name: "rule 1"
					service_account: "abc@robots.com"
				}
				rules {
					name: "rule 2"
					service_account: "abc@robots.com"
				}
			`,
			Errors: []string{"mentioned by more than one rule"},
		},

		{
			Cfg: `
				rules {
					service_account: "abc@robots.com"
				}
			`,
			Errors: []string{`"name" is required`},
		},

		{
			Cfg: `
				rules {
					name: "rule 1"
					service_account: "not an email"
				}
			`,
			Errors: []string{"bad value"},
		},

		{
			Cfg: `
				rules {
					name: "rule 1"
					allowed_scope: "not a scope"
				}
			`,
			Errors: []string{"bad scope"},
		},

		{
			Cfg: `
				rules {
					name: "rule 1"
					end_user: "group:"
					end_user: "user:not an email"
				}
			`,
			Errors: []string{"bad group entry", "bad value"},
		},

		{
			Cfg: `
				rules {
					name: "rule 1"
					proxy: "group:"
					proxy: "user:not an email"
				}
			`,
			Errors: []string{"bad group entry", "bad value"},
		},

		{
			Cfg: `
				rules {
					name: "rule 1"
					max_grant_validity_duration: -1
				}
				rules {
					name: "rule 2"
					max_grant_validity_duration: 10000000
				}
			`,
			Errors: []string{"must be positive", "must not exceed"},
		},

		// Bad defaults.
		{
			Cfg: `
				defaults {
					allowed_scope: "not a scope"
					max_grant_validity_duration: -1
				}
			`,
			Errors: []string{"bad scope", "must be positive"},
		},
	}

	Convey("Validation works", t, func(c C) {
		for idx, cs := range cases {
			c.Printf("Case #%d\n", idx)

			cfg := &admin.ServiceAccountsPermissions{}
			err := proto.UnmarshalText(cs.Cfg, cfg)
			So(err, ShouldBeNil)

			ctx := validation.Context{}
			validateConfigs(policy.ConfigBundle{serviceAccountsCfg: cfg}, &ctx)
			verr := ctx.Finalize()

			if len(cs.Errors) == 0 { // no errors expected
				So(verr, ShouldBeNil)
			} else {
				verr := verr.(*validation.Error)
				So(len(verr.Errors), ShouldEqual, len(cs.Errors))
				for i, err := range verr.Errors {
					So(err, ShouldErrLike, cs.Errors[i])
				}
			}
		}
	})
}
