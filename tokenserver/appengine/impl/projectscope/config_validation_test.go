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

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config/validation"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		Cfg      string
		Warnings []string
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
		},
		{
			// Identity double assignment, produces a warning.
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
					id: "id2"
					config_location {
						url: "https://some/repo"
						storage_type: GITILES
					}
					identity_config {
						service_account_email: "foo@bar.com"
					}
				}
			`,
			Warnings: []string{
				`project-scoped account foo@bar.com is used by multiple projects: id1, id2`,
			},
		},
	}

	Convey("Validation works", t, func(c C) {
		for idx, cs := range cases {
			c.Printf("Case #%d\n", idx)

			cfg := &config.ProjectsCfg{}
			err := prototext.Unmarshal([]byte(cs.Cfg), cfg)
			So(err, ShouldBeNil)

			ctx := &validation.Context{Context: context.Background()}
			ctx.SetFile(projectsCfg)
			validateSingleIdentityProjectAssignment(ctx, cfg)
			verr := ctx.Finalize()

			if len(cs.Warnings) == 0 {
				So(verr, ShouldBeNil)
			} else {
				verr := verr.(*validation.Error)
				So(verr.Errors, ShouldHaveLength, len(cs.Warnings))
				for i, err := range verr.Errors {
					sev, _ := validation.SeverityTag.In(err)
					So(sev, ShouldEqual, validation.Warning)
					So(err, ShouldErrLike, cs.Warnings[i])
				}
			}
		}
	})
}
