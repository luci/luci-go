// Copyright 2020 The LUCI Authors.
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
	"context"
	"testing"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/policy"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		Cfg    string
		Errors []string
	}{
		{
			// Non-trivial good config.
			Cfg: `
				mapping {
					project: "proj1"
					service_account: "exclusive-1@example.com"
					service_account: "exclusive-2@example.com"
				}

				mapping {
					project: "proj1"
					project: "proj2"
					service_account: "shared-1@example.com"
					service_account: "shared-2@example.com"
				}

				mapping {
					project: "proj3"
					service_account: "exclusive-3@example.com"
				}

				mapping {
					project: "@internal"
					service_account: "exclusive-4@example.com"
				}
			`,
		},

		// Minimal config.
		{
			Cfg: ``,
		},

		// Empty list of project is not OK.
		{
			Cfg: `
				mapping {
					service_account: "sa1@example.com"
				}
			`,
			Errors: []string{"at least one project must be given"},
		},

		// Empty list of accounts is not OK.
		{
			Cfg: `
				mapping {
					project: "proj"
				}
			`,
			Errors: []string{"at least one service account must be given"},
		},

		// Bad project names.
		{
			Cfg: `
				mapping {
					project: ""
					project: "  "
					service_account: "sa@example.com"
				}
			`,
			Errors: []string{
				`bad project ""`,
				`bad project "  "`,
			},
		},

		// Bad service account names.
		{
			Cfg: `
				mapping {
					project: "proj"
					service_account: ""
					service_account: "not-email"
				}
			`,
			Errors: []string{
				`bad service_account ""`,
				`bad service_account "not-email"`,
			},
		},

		// Multiple mappings with the same account.
		{
			Cfg: `
				mapping {
					project: "proj1"
					service_account: "sa@example.com"
				}
				mapping {
					project: "proj2"
					service_account: "sa@example.com"
				}
			`,
			Errors: []string{
				`service_account "sa@example.com" appears in more that one mapping`,
			},
		},
	}

	Convey("Validation works", t, func(c C) {
		for idx, cs := range cases {
			c.Printf("Case #%d\n", idx)

			cfg := &admin.ServiceAccountsProjectMapping{}
			err := prototext.Unmarshal([]byte(cs.Cfg), cfg)
			So(err, ShouldBeNil)

			ctx := &validation.Context{Context: context.Background()}
			validateConfigBundle(ctx, policy.ConfigBundle{configFileName: cfg})
			verr := ctx.Finalize()

			if len(cs.Errors) == 0 {
				So(verr, ShouldBeNil)
			} else {
				verr := verr.(*validation.Error)
				So(verr.Errors, ShouldHaveLength, len(cs.Errors))
				for i, err := range verr.Errors {
					So(err, ShouldErrLike, cs.Errors[i])
				}
			}
		}
	})
}
