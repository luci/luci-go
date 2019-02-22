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
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/projectidentity"
	"testing"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config/validation"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		Cfg              string
		Errors           []string
		ExpectIdentities []*projectidentity.ProjectIdentity
	}{
		{
			// No errors, "normal looking" config.
			Cfg: `
				projects {
					id: "id1"
					config_location {
						url: "https://some/repo"
						storage_type: GITILES
					}
					identity_config {
						service_account_email: "foo@bar.com"
					}
				}
			`,
			ExpectIdentities: []*projectidentity.ProjectIdentity{
				{
					Project: "id1",
					Email:   "foo@bar.com",
				},
			},
		},
		{
			// Identity double assignment, broken config.
			Cfg: `
				projects {
					id: "id1"
					config_location {
						url: "https://some/repo"
						storage_type: GITILES
					}
					identity_config {
						service_account_email: "foo@bar.com"
					}
				}
				projects {
					id: "id1"
					config_location {
						url: "https://some/repo"
						storage_type: GITILES
					}
					identity_config {
						service_account_email: "foo@bar.com"
					}
				}
			`,
			Errors: []string{
				`in "projects.cfg" (identity configuration): at least two projects sharing the same identity`,
			},
		},
	}

	Convey("Validation works", t, func(c C) {
		for idx, cs := range cases {
			c.Printf("Case #%d\n", idx)

			cfg := &config.ProjectsCfg{}
			err := proto.UnmarshalText(cs.Cfg, cfg)
			So(err, ShouldBeNil)

			ctx := &validation.Context{Context: gaetesting.TestingContext()}
			ctx.SetFile(projectsCfg)
			validateSingleIdentityProjectAssignment(ctx, cfg)
			verr := ctx.Finalize()

			if len(cs.Errors) == 0 { // no errors expected
				So(verr, ShouldBeNil)
				updateIdentities(ctx, cfg)
				storage := projectidentity.ProjectIdentities(ctx.Context)
				for _, identity := range cs.ExpectIdentities {
					foundIdentity, err := storage.LookupByProject(ctx.Context, identity.Project)
					So(err, ShouldBeNil)
					So(foundIdentity, ShouldResemble, identity)
				}
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
