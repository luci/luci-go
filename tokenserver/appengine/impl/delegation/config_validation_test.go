// Copyright 2016 The LUCI Authors.
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

package delegation

import (
	"context"
	"testing"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/policy"
)

func TestValidation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		Cfg    string
		Errors []string
	}{
		{
			// No errors, "normal looking" config.
			Cfg: `
				rules {
					name: "rule 1"
					requestor: "user:some-app@appspot.gserviceaccount.com"
					target_service: "service:some-service"
					allowed_to_impersonate: "group:some-group"
					allowed_audience: "REQUESTOR"
					max_validity_duration: 86400
				}

				rules {
					name: "rule 2"
					requestor: "group:some-group"
					target_service: "*"
					allowed_to_impersonate: "group:another-group"
					allowed_audience: "*"
					max_validity_duration: 86400
				}
			`,
		},

		{
			// Duplicate names.
			Cfg: `
				rules {
					name: "rule 1"
					requestor: "user:some-app@appspot.gserviceaccount.com"
					target_service: "service:some-service"
					allowed_to_impersonate: "group:some-group"
					allowed_audience: "REQUESTOR"
					max_validity_duration: 86400
				}

				rules {
					name: "rule 1"
					requestor: "group:some-group"
					target_service: "*"
					allowed_to_impersonate: "group:another-group"
					allowed_audience: "*"
					max_validity_duration: 86400
				}
			`,
			Errors: []string{`two rules with identical name "rule 1"`},
		},

		{
			// Missing required fields.
			Cfg: `
				rules {
				}
			`,
			Errors: []string{
				`"name" is required`,
				`"requestor" is required`,
				`"allowed_to_impersonate" is required`,
				`"allowed_audience" is required`,
				`"target_service" is required`,
				`"max_validity_duration" is required`,
			},
		},

		{
			// Validity duration out of range.
			Cfg: `
				rules {
					name: "rule 1"
					requestor: "user:some-app@appspot.gserviceaccount.com"
					target_service: "service:some-service"
					allowed_to_impersonate: "group:some-group"
					allowed_audience: "REQUESTOR"
					max_validity_duration: -1
				}
				rules {
					name: "rule 2"
					requestor: "user:some-app@appspot.gserviceaccount.com"
					target_service: "service:some-service"
					allowed_to_impersonate: "group:some-group"
					allowed_audience: "REQUESTOR"
					max_validity_duration: 86401
				}
			`,
			Errors: []string{
				`in "delegation.cfg" (rule #1: "rule 1"): "max_validity_duration" must be positive`,
				`in "delegation.cfg" (rule #2: "rule 2"): "max_validity_duration" must be smaller than 86401`,
			},
		},

		{
			// Bad requestor.
			Cfg: `
				rules {
					name: "rule 1"
					requestor: "user:some-app@appspot.gserviceaccount.com" # ok
					requestor: "service:blah" # ok
					requestor: "group:some-group" # ok
					requestor: "*" # not ok
					requestor: "some junk" # not ok
					requestor: "group:" # not ok
					target_service: "service:some-service"
					allowed_to_impersonate: "group:some-group"
					allowed_audience: "REQUESTOR"
					max_validity_duration: 3600
				}
			`,
			Errors: []string{
				`in "delegation.cfg" (rule #1: "rule 1" / "requestor"): auth: bad identity string "*"`,
				`in "delegation.cfg" (rule #1: "rule 1" / "requestor"): auth: bad identity string "some junk"`,
				`in "delegation.cfg" (rule #1: "rule 1" / "requestor"): bad group entry "group:"`,
			},
		},

		{
			// Bad allowed_to_impersonate.
			Cfg: `
				rules {
					name: "rule 1"
					requestor: "user:some-app@appspot.gserviceaccount.com"
					target_service: "service:some-service"
					allowed_to_impersonate: "user:abc@example.com" # ok
					allowed_to_impersonate: "group:some-group" # ok
					allowed_to_impersonate: "REQUESTOR" # ok
					allowed_to_impersonate: "*" # not OK
					allowed_audience: "REQUESTOR"
					max_validity_duration: 86400
				}
			`,
			Errors: []string{
				`in "delegation.cfg" (rule #1: "rule 1" / "allowed_to_impersonate"): auth: bad identity string "*"`,
			},
		},

		{
			// Bad allowed_audience.
			Cfg: `
				rules {
					name: "rule 1"
					requestor: "user:some-app@appspot.gserviceaccount.com"
					target_service: "service:some-service"
					allowed_to_impersonate: "user:abc@example.com"
					allowed_audience: "REQUESTOR" # ok
					allowed_audience: "*" # ok
					allowed_audience: "user:abc@example.com" # ok
					allowed_audience: "group:abc" # ok
					allowed_audience: "some junk" # not ok
					max_validity_duration: 86400
				}
			`,
			Errors: []string{
				`in "delegation.cfg" (rule #1: "rule 1" / "allowed_audience"): auth: bad identity string "some junk"`,
			},
		},

		{
			// Bad target_service.
			Cfg: `
				rules {
					name: "rule 1"
					requestor: "user:some-app@appspot.gserviceaccount.com"
					target_service: "service:some-service" # ok
					target_service: "user:abc@example.com" # not ok
					target_service: "group:some-group" # not ok
					allowed_to_impersonate: "user:abc@example.com"
					allowed_audience: "REQUESTOR"
					max_validity_duration: 86400
				}
			`,
			Errors: []string{
				`in "delegation.cfg" (rule #1: "rule 1" / "target_service"): identity of kind "user" is not allowed here`,
				`in "delegation.cfg" (rule #1: "rule 1" / "target_service"): group entries are not allowed`,
			},
		},
	}

	ftt.Run("Validation works", t, func(c *ftt.Test) {
		for idx, cs := range cases {
			c.Logf("Case #%d\n", idx)

			cfg := &admin.DelegationPermissions{}
			err := prototext.Unmarshal([]byte(cs.Cfg), cfg)
			assert.Loosely(c, err, should.BeNil)

			ctx := &validation.Context{Context: context.Background()}
			validateConfigBundle(ctx, policy.ConfigBundle{delegationCfg: cfg})
			verr := ctx.Finalize()

			if len(cs.Errors) == 0 { // no errors expected
				assert.Loosely(c, verr, should.BeNil)
			} else {
				verr := verr.(*validation.Error)
				assert.Loosely(c, len(verr.Errors), should.Equal(len(cs.Errors)))
				for i, err := range verr.Errors {
					assert.Loosely(c, err, should.ErrLike(cs.Errors[i]))
				}
			}
		}
	})
}
