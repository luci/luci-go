// Copyright 2023 The LUCI Authors.
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

package gerritchangelists

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/testutil"
	analysispb "go.chromium.org/luci/analysis/proto/v1"
)

func TestSpan(t *testing.T) {
	ftt.Run("With Spanner Test Database", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)

		t.Run("With test data", func(t *ftt.Test) {
			clsToCreate := []*GerritChangelist{
				{
					Project:      "testprojecta",
					Host:         "mysource-review.googlesource.com",
					Change:       123456,
					OwnerKind:    analysispb.ChangelistOwnerKind_AUTOMATION,
					CreationTime: time.Date(2021, 2, 3, 4, 5, 6, 0, time.UTC),
				},
				{
					Project:      "testprojectb",
					Host:         "mysource-internal-review.googlesource.com",
					Change:       123456,
					OwnerKind:    analysispb.ChangelistOwnerKind_HUMAN,
					CreationTime: time.Date(2023, 2, 3, 4, 5, 6, 0, time.UTC),
				},
				{
					Project:      "testprojectb",
					Host:         "mysource-internal-review.googlesource.com",
					Change:       123457,
					OwnerKind:    analysispb.ChangelistOwnerKind_HUMAN,
					CreationTime: time.Date(2020, 2, 3, 4, 5, 6, 0, time.UTC),
				},
				{
					Project:      "testprojectb",
					Host:         "mysource-review.googlesource.com",
					Change:       123456,
					OwnerKind:    analysispb.ChangelistOwnerKind_CHANGELIST_OWNER_UNSPECIFIED,
					CreationTime: time.Date(2022, 2, 3, 4, 5, 6, 0, time.UTC),
				},
			}
			err := SetGerritChangelistsForTesting(ctx, t, clsToCreate)
			assert.Loosely(t, err, should.BeNil)

			t.Run("ReadAll", func(t *ftt.Test) {
				cls, err := ReadAll(span.Single(ctx))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cls, should.Match(clsToCreate))
			})
			t.Run("Read", func(t *ftt.Test) {
				expectedCLs := make(map[Key]*GerritChangelist)
				keys := map[Key]struct{}{}
				t.Run("Zero", func(t *ftt.Test) {
					cls, err := Read(span.Single(ctx), keys)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, cls, should.Match(expectedCLs))
				})
				t.Run("One", func(t *ftt.Test) {
					k := Key{
						Project: "testprojectb",
						Host:    "mysource-internal-review.googlesource.com",
						Change:  123456,
					}
					keys[k] = struct{}{}
					expectedCLs[k] = &GerritChangelist{
						Project:      "testprojectb",
						Host:         "mysource-internal-review.googlesource.com",
						Change:       123456,
						OwnerKind:    analysispb.ChangelistOwnerKind_HUMAN,
						CreationTime: time.Date(2023, 2, 3, 4, 5, 6, 0, time.UTC),
					}

					cls, err := Read(span.Single(ctx), keys)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, cls, should.Match(expectedCLs))
				})
				t.Run("Many", func(t *ftt.Test) {
					for _, entry := range clsToCreate {
						key := Key{entry.Project, entry.Host, entry.Change}
						keys[key] = struct{}{}
						expectedCLs[key] = entry
					}

					cls, err := Read(span.Single(ctx), keys)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, cls, should.Match(expectedCLs))
				})
			})
		})
		t.Run("CreateOrUpdate", func(t *ftt.Test) {
			save := func(g *GerritChangelist) (time.Time, error) {
				commitTime, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					m, err := CreateOrUpdate(g)
					if err != nil {
						return err
					}
					span.BufferWrite(ctx, m)
					return nil
				})
				return commitTime.In(time.UTC), err
			}

			cl := GerritChangelist{
				Project:   "testprojectb",
				Host:      "mysource-internal-review.googlesource.com",
				Change:    123456,
				OwnerKind: analysispb.ChangelistOwnerKind_HUMAN,
			}
			key := Key{
				Project: "testprojectb",
				Host:    "mysource-internal-review.googlesource.com",
				Change:  123456,
			}
			keys := map[Key]struct{}{}
			keys[key] = struct{}{}

			t.Run("Baseline", func(t *ftt.Test) {
				expectedCL := cl

				commitTime, err := save(&cl)
				assert.Loosely(t, err, should.BeNil)
				expectedCL.CreationTime = commitTime

				cls, err := Read(span.Single(ctx), keys)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cls, should.HaveLength(1))
				assert.Loosely(t, cls[key], should.Match(&expectedCL))
			})
			t.Run("Owner kind unset", func(t *ftt.Test) {
				// If we do not have permission to read the changelist, we still may
				// write a cache entry to avoid battering gerrit with requests.
				// In this case, the owner kind will be UNSPECIFIED.
				cl.OwnerKind = analysispb.ChangelistOwnerKind_CHANGELIST_OWNER_UNSPECIFIED

				expectedCL := cl

				commitTime, err := save(&cl)
				assert.Loosely(t, err, should.BeNil)
				expectedCL.CreationTime = commitTime

				cls, err := Read(span.Single(ctx), keys)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cls, should.HaveLength(1))
				assert.Loosely(t, cls[key], should.Match(&expectedCL))
			})

			t.Run("Invalid GerritChangelists are rejected", func(t *ftt.Test) {
				t.Run("Project is empty", func(t *ftt.Test) {
					cl.Project = ""
					_, err := save(&cl)
					assert.Loosely(t, err, should.ErrLike("project: unspecified"))
				})
				t.Run("Project is invalid", func(t *ftt.Test) {
					cl.Project = "!invalid"
					_, err := save(&cl)
					assert.Loosely(t, err, should.ErrLike(`project: must match ^[a-z0-9\-]{1,40}$`))
				})
				t.Run("Host is empty", func(t *ftt.Test) {
					cl.Host = ""
					_, err := save(&cl)
					assert.Loosely(t, err, should.ErrLike("host: must have a length between 1 and 255"))
				})
				t.Run("Host is too long", func(t *ftt.Test) {
					cl.Host = strings.Repeat("h", 256)
					_, err := save(&cl)
					assert.Loosely(t, err, should.ErrLike("host: must have a length between 1 and 255"))
				})
				t.Run("Change is empty", func(t *ftt.Test) {
					cl.Change = 0
					_, err := save(&cl)
					assert.Loosely(t, err, should.ErrLike("change: must be set and positive"))
				})
				t.Run("Change is negative", func(t *ftt.Test) {
					cl.Change = -1
					_, err := save(&cl)
					assert.Loosely(t, err, should.ErrLike("change: must be set and positive"))
				})
			})
		})
	})
}
