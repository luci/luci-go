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
	"fmt"
	"testing"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"
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
				projects {
					id: "id1"
					owned_by: "team A"
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
			// Identity double assignment across teams, produces an error.
			Cfg: `
				projects {
					id: "id1"
					owned_by: "team A"
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
					owned_by: "team B"
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
				`project-scoped account foo@bar.com is used by multiple teams: team A, team B`,
			},
		},
		{
			// Identity double assignment within the same team, no error.
			Cfg: `
				projects {
					id: "id1"
					owned_by: "team A"
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
					owned_by: "team A"
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
	}

	ftt.Run("Validation works", t, func(c *ftt.Test) {
		for idx, cs := range cases {
			c.Run(fmt.Sprintf("Case #%d", idx), func(c *ftt.Test) {
				cfg := &config.ProjectsCfg{}
				err := prototext.Unmarshal([]byte(cs.Cfg), cfg)
				assert.Loosely(c, err, should.BeNil)

				ctx := &validation.Context{Context: context.Background()}
				ctx.SetFile(projectsCfg)
				validateSingleIdentityProjectAssignment(ctx, cfg)
				verr := ctx.Finalize()

				if len(cs.Errors) == 0 {
					assert.Loosely(c, verr, should.BeNil)
				} else {
					verr := verr.(*validation.Error)
					assert.Loosely(c, verr.Errors, should.HaveLength(len(cs.Errors)))
					for i, err := range verr.Errors {
						sev, _ := validation.SeverityTag.In(err)
						assert.Loosely(c, sev, should.Equal(validation.Blocking))
						assert.Loosely(c, err, should.ErrLike(cs.Errors[i]))
					}
				}
			})
		}
	})
}
