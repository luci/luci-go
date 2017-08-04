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

	"github.com/luci/luci-go/tokenserver/api/admin/v1"
	"github.com/luci/luci-go/tokenserver/appengine/impl/utils/policy"

	"github.com/luci/luci-go/common/config/validation"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
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
					allowed_scope: "https://scope"
					end_user: "user:abc@example.com"
					end_user: "group:group-name"
					proxy: "user:proxy@example.com"
					max_grant_validity_duration: 3600
				}

				rules {
					name: "rule 2"
					owner: "developer@example.com"
					service_account: "def@robots.com"
					allowed_scope: "https://scope"
					end_user: "user:abc@example.com"
					end_user: "group:group-name"
					proxy: "user:proxy@example.com"
					max_grant_validity_duration: 3600
				}
			`,
		},

		// TODO(vadimsh): Add more cases.
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
