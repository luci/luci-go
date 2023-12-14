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

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/testutil"
	analysispb "go.chromium.org/luci/analysis/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSpan(t *testing.T) {
	Convey("With Spanner Test Database", t, func() {
		ctx := testutil.IntegrationTestContext(t)

		Convey("With test data", func() {
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
					Host:         "mysource-review.googlesource.com",
					Change:       123456,
					OwnerKind:    analysispb.ChangelistOwnerKind_CHANGELIST_OWNER_UNSPECIFIED,
					CreationTime: time.Date(2022, 2, 3, 4, 5, 6, 0, time.UTC),
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
			}
			err := SetGerritChangelistsForTesting(ctx, clsToCreate)
			So(err, ShouldBeNil)

			expectedCLs := make(map[Key]*GerritChangelist)
			for _, entry := range clsToCreate {
				key := Key{entry.Project, entry.Host, entry.Change}
				expectedCLs[key] = entry
			}

			Convey("ReadAll", func() {
				cls, err := ReadAll(span.Single(ctx))
				So(err, ShouldBeNil)
				So(cls, ShouldResemble, expectedCLs)
			})
			Convey("Read", func() {
				expectedCLs := make(map[Key]*GerritChangelist)
				keys := map[Key]struct{}{}
				Convey("Zero", func() {
					cls, err := Read(span.Single(ctx), keys)
					So(err, ShouldBeNil)
					So(cls, ShouldResemble, expectedCLs)
				})
				Convey("One", func() {
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
					So(err, ShouldBeNil)
					So(cls, ShouldResemble, expectedCLs)
				})
				Convey("Many", func() {
					for _, entry := range clsToCreate {
						key := Key{entry.Project, entry.Host, entry.Change}
						keys[key] = struct{}{}
						expectedCLs[key] = entry
					}

					cls, err := Read(span.Single(ctx), keys)
					So(err, ShouldBeNil)
					So(cls, ShouldResemble, expectedCLs)
				})
			})
		})
		Convey("Create", func() {
			save := func(g *GerritChangelist) (time.Time, error) {
				commitTime, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					m, err := Create(g)
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

			Convey("Baseline", func() {
				expectedCL := cl

				commitTime, err := save(&cl)
				So(err, ShouldBeNil)
				expectedCL.CreationTime = commitTime

				cls, err := Read(span.Single(ctx), keys)
				So(err, ShouldBeNil)
				So(cls, ShouldHaveLength, 1)
				So(cls[key], ShouldResemble, &expectedCL)
			})
			Convey("Owner kind unset", func() {
				// If we do not have permission to read the changelist, we still may
				// write a cache entry to avoid battering gerrit with requests.
				// In this case, the owner kind will be UNSPECIFIED.
				cl.OwnerKind = analysispb.ChangelistOwnerKind_CHANGELIST_OWNER_UNSPECIFIED

				expectedCL := cl

				commitTime, err := save(&cl)
				So(err, ShouldBeNil)
				expectedCL.CreationTime = commitTime

				cls, err := Read(span.Single(ctx), keys)
				So(err, ShouldBeNil)
				So(cls, ShouldHaveLength, 1)
				So(cls[key], ShouldResemble, &expectedCL)
			})

			Convey("Invalid GerritChangelists are rejected", func() {
				Convey("Project is empty", func() {
					cl.Project = ""
					_, err := save(&cl)
					So(err, ShouldErrLike, "project: unspecified")
				})
				Convey("Project is invalid", func() {
					cl.Project = "!invalid"
					_, err := save(&cl)
					So(err, ShouldErrLike, `project: must match ^[a-z0-9\-]{1,40}$`)
				})
				Convey("Host is empty", func() {
					cl.Host = ""
					_, err := save(&cl)
					So(err, ShouldErrLike, "host: must have a length between 1 and 255")
				})
				Convey("Host is too long", func() {
					cl.Host = strings.Repeat("h", 256)
					_, err := save(&cl)
					So(err, ShouldErrLike, "host: must have a length between 1 and 255")
				})
				Convey("Change is empty", func() {
					cl.Change = 0
					_, err := save(&cl)
					So(err, ShouldErrLike, "change: must be set and positive")
				})
				Convey("Change is negative", func() {
					cl.Change = -1
					_, err := save(&cl)
					So(err, ShouldErrLike, "change: must be set and positive")
				})
			})
		})
	})
}
