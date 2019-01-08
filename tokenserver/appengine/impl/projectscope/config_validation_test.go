// Copyright 2019 The LUCI Authors.
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

package projectscope

import (
	"context"
	"testing"

	"github.com/golang/protobuf/proto"
	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/testing/assertions"
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
			Cfg: `
				services {
					service: "scheduler"
				  cloud_project_id: "scheduler-project-1234"
				  max_validity_duration: 1800
				}
				services {
					service: "buildbucket"
				  cloud_project_id: "buildbucket-project-23434"
				  max_validity_duration: 3600
				}
				services {
					service: "scheduler2"
				  cloud_project_id: "scheduler-project"
				}
			`,
		},

		// Minimal config.
		{
			Cfg: `
				services {
					service: "scheduler"
					cloud_project_id: "scheduler-project-1234"
				}
			`,
		},

		// Duplicate
		{
			Cfg: `
				services {
					service: "scheduler"
					cloud_project_id: "scheduler-project-1234"
				}
				services {
					service: "scheduler"
					cloud_project_id: "scheduler-project-4321"
				}
			`,
			Errors: []string{"two rules with identical name"},
		},

		// Incomplete config
		{
			Cfg: `
				services {
					cloud_project_id: "scheduler-project-1234"
				}
			`,
			Errors: []string{`"service" is required`},
		},

		{
			Cfg: `
				services {
					service: "scheduler"
				}
			`,
			Errors: []string{`"cloud_project_id" is required`},
		},

		// Bad Minimal validity.
		{
			Cfg: `
				services {
					service: "scheduler"
					cloud_project_id: "scheduler-project-1234"
					max_validity_duration: 7200
				}
			`,
			Errors: []string{`"max_grant_validity_duration" must not exceed 3600`},
		},
	}

	Convey("Validation works", t, func(c C) {
		for idx, cs := range cases {
			c.Printf("Case #%d\n", idx)

			cfg := &admin.ProjectScopedServiceAccounts{}
			err := proto.UnmarshalText(cs.Cfg, cfg)
			So(err, ShouldBeNil)

			ctx := &validation.Context{Context: context.Background()}
			validateConfigBundle(ctx, policy.ConfigBundle{projectScopedServiceAccountsCfg: cfg})
			verr := ctx.Finalize()

			if len(cs.Errors) == 0 { // no errors expected
				So(verr, ShouldBeNil)
			} else {
				verr := verr.(*validation.Error)
				So(len(verr.Errors), ShouldEqual, len(cs.Errors))
				for i, err := range verr.Errors {
					So(err, assertions.ShouldErrLike, cs.Errors[i])
				}
			}
		}
	})
}
