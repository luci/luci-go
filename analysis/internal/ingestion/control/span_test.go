// Copyright 2024 The LUCI Authors.
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

package control

import (
	"context"
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
	ftt.Run(`With Spanner Test Database`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		t.Run(`Read`, func(t *ftt.Test) {
			entriesToCreate := []*Entry{
				NewEntry(0).WithIngestionID("rdb-host/1").Build(),
				NewEntry(2).WithIngestionID("rdb-host/2").WithBuildResult(nil).Build(),
				NewEntry(3).WithIngestionID("rdb-host/3").WithPresubmitResult(nil).Build(),
				NewEntry(4).WithIngestionID("rdb-host/4").WithInvocationResult(nil).Build(),
			}
			_, err := SetEntriesForTesting(ctx, t, entriesToCreate...)
			assert.Loosely(t, err, should.BeNil)

			t.Run(`None exist`, func(t *ftt.Test) {
				ingestionIDs := []IngestionID{"rdb-host/5"}
				results, err := Read(span.Single(ctx), ingestionIDs)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(results), should.Equal(1))
				assert.Loosely(t, results[0], should.BeNil)
			})
			t.Run(`Some exist`, func(t *ftt.Test) {
				ingestionIDs := []IngestionID{
					"rdb-host/3",
					"rdb-host/4",
					"rdb-host/5",
					"rdb-host/2",
					"rdb-host/1",
				}
				results, err := Read(span.Single(ctx), ingestionIDs)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(results), should.Equal(5))
				assert.Loosely(t, results[0], should.Match(entriesToCreate[2]))
				assert.Loosely(t, results[1], should.Match(entriesToCreate[3]))
				assert.Loosely(t, results[2], should.BeNil)
				assert.Loosely(t, results[3], should.Match(entriesToCreate[1]))
				assert.Loosely(t, results[4], should.Match(entriesToCreate[0]))
			})
		})
		t.Run(`InsertOrUpdate`, func(t *ftt.Test) {
			testInsertOrUpdate := func(e *Entry) (time.Time, error) {
				commitTime, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					return InsertOrUpdate(ctx, e)
				})
				return commitTime.In(time.UTC), err
			}

			entryToCreate := NewEntry(0).Build()

			_, err := SetEntriesForTesting(ctx, t, entryToCreate)
			assert.Loosely(t, err, should.BeNil)

			e := NewEntry(1).Build()

			t.Run(`Valid`, func(t *ftt.Test) {
				t.Run(`Insert`, func(t *ftt.Test) {
					commitTime, err := testInsertOrUpdate(e)
					assert.Loosely(t, err, should.BeNil)
					e.LastUpdated = commitTime

					result, err := Read(span.Single(ctx), []IngestionID{e.IngestionID})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(result), should.Equal(1))
					assert.Loosely(t, result[0], should.Match(e))
				})
				t.Run(`Update`, func(t *ftt.Test) {
					// Update the existing entry.
					e.IngestionID = entryToCreate.IngestionID

					commitTime, err := testInsertOrUpdate(e)
					assert.Loosely(t, err, should.BeNil)
					e.LastUpdated = commitTime

					result, err := Read(span.Single(ctx), []IngestionID{e.IngestionID})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(result), should.Equal(1))
					assert.Loosely(t, result[0], should.Match(e))
				})
			})
			t.Run(`With invalid Ingestion ID`, func(t *ftt.Test) {
				e.IngestionID = IngestionID("")
				_, err := testInsertOrUpdate(e)
				assert.Loosely(t, err, should.ErrLike("ingestionID must be set"))
			})
			t.Run(`With invalid Build Project`, func(t *ftt.Test) {
				t.Run(`Missing`, func(t *ftt.Test) {
					e.BuildProject = ""
					_, err := testInsertOrUpdate(e)
					assert.Loosely(t, err, should.ErrLike("build project: unspecified"))
				})
				t.Run(`Invalid`, func(t *ftt.Test) {
					e.BuildProject = "!"
					_, err := testInsertOrUpdate(e)
					assert.Loosely(t, err, should.ErrLike(`build project: must match ^[a-z0-9\-]{1,40}$`))
				})
			})
			t.Run(`With invalid Build Result`, func(t *ftt.Test) {
				t.Run(`Missing host`, func(t *ftt.Test) {
					e.BuildResult.Host = ""
					_, err := testInsertOrUpdate(e)
					assert.Loosely(t, err, should.ErrLike("host must be specified"))
				})
				t.Run(`Missing id`, func(t *ftt.Test) {
					e.BuildResult.Id = 0
					_, err := testInsertOrUpdate(e)
					assert.Loosely(t, err, should.ErrLike("id must be specified"))
				})
				t.Run(`Missing creation time`, func(t *ftt.Test) {
					e.BuildResult.CreationTime = nil
					_, err := testInsertOrUpdate(e)
					assert.Loosely(t, err, should.ErrLike("build result: creation time must be specified"))
				})
				t.Run(`Missing project`, func(t *ftt.Test) {
					e.BuildResult.Project = ""
					_, err := testInsertOrUpdate(e)
					assert.Loosely(t, err, should.ErrLike("build result: project must be specified"))
				})
				t.Run(`Missing resultdb_host`, func(t *ftt.Test) {
					e.BuildResult.HasInvocation = true
					e.BuildResult.ResultdbHost = ""
					_, err := testInsertOrUpdate(e)
					assert.Loosely(t, err, should.ErrLike("build result: resultdb_host must be specified if has_invocation set"))
				})
				t.Run(`Missing builder`, func(t *ftt.Test) {
					e.BuildResult.Builder = ""
					_, err := testInsertOrUpdate(e)
					assert.Loosely(t, err, should.ErrLike("build result: builder must be specified"))
				})
				t.Run(`Missing status`, func(t *ftt.Test) {
					e.BuildResult.Status = analysispb.BuildStatus_BUILD_STATUS_UNSPECIFIED
					_, err := testInsertOrUpdate(e)
					assert.Loosely(t, err, should.ErrLike("build result: build status must be specified"))
				})
			})
			t.Run(`With invalid Invocation Project`, func(t *ftt.Test) {
				t.Run(`Unspecified`, func(t *ftt.Test) {
					e.InvocationProject = ""
					_, err := testInsertOrUpdate(e)
					assert.Loosely(t, err, should.ErrLike(`invocation project: unspecified`))
				})
				t.Run(`Invalid`, func(t *ftt.Test) {
					e.InvocationProject = "!"
					_, err := testInsertOrUpdate(e)
					assert.Loosely(t, err, should.ErrLike(`invocation project: must match ^[a-z0-9\-]{1,40}$`))
				})
			})
			t.Run(`With invalid Invocation Result`, func(t *ftt.Test) {
				t.Run(`Set when HasInvocation = false`, func(t *ftt.Test) {
					assert.Loosely(t, e.InvocationResult, should.NotBeNil)
					e.HasInvocation = false
					_, err := testInsertOrUpdate(e)
					assert.Loosely(t, err, should.ErrLike("invocation result must not be set unless HasInvocation is set"))
				})
			})
			t.Run(`With invalid Presubmit Project`, func(t *ftt.Test) {
				t.Run(`Unspecified`, func(t *ftt.Test) {
					e.PresubmitProject = ""
					_, err := testInsertOrUpdate(e)
					assert.Loosely(t, err, should.ErrLike(`presubmit project: unspecified`))
				})
				t.Run(`Invalid`, func(t *ftt.Test) {
					e.PresubmitProject = "!"
					_, err := testInsertOrUpdate(e)
					assert.Loosely(t, err, should.ErrLike(`presubmit project: must match ^[a-z0-9\-]{1,40}$`))
				})
			})
			t.Run(`With invalid Presubmit Result`, func(t *ftt.Test) {
				t.Run(`Set when IsPresbumit = false`, func(t *ftt.Test) {
					assert.Loosely(t, e.PresubmitResult, should.NotBeNil)
					e.IsPresubmit = false
					_, err := testInsertOrUpdate(e)
					assert.Loosely(t, err, should.ErrLike("presubmit result must not be set unless IsPresubmit is set"))
				})
				t.Run(`Missing Presubmit run ID`, func(t *ftt.Test) {
					e.PresubmitResult.PresubmitRunId = nil
					_, err := testInsertOrUpdate(e)
					assert.Loosely(t, err, should.ErrLike("presubmit run ID must be specified"))
				})
				t.Run(`Invalid Presubmit run ID host`, func(t *ftt.Test) {
					e.PresubmitResult.PresubmitRunId.System = "!"
					_, err := testInsertOrUpdate(e)
					assert.Loosely(t, err, should.ErrLike("presubmit run system must be 'luci-cv'"))
				})
				t.Run(`Missing Presubmit run ID system-specific ID`, func(t *ftt.Test) {
					e.PresubmitResult.PresubmitRunId.Id = ""
					_, err := testInsertOrUpdate(e)
					assert.Loosely(t, err, should.ErrLike("presubmit run system-specific ID must be specified"))
				})
				t.Run(`Missing creation time`, func(t *ftt.Test) {
					e.PresubmitResult.CreationTime = nil
					_, err := testInsertOrUpdate(e)
					assert.Loosely(t, err, should.ErrLike("presubmit result: creation time must be specified"))
				})
				t.Run(`Missing mode`, func(t *ftt.Test) {
					e.PresubmitResult.Mode = analysispb.PresubmitRunMode_PRESUBMIT_RUN_MODE_UNSPECIFIED
					_, err := testInsertOrUpdate(e)
					assert.Loosely(t, err, should.ErrLike("presubmit result: mode must be specified"))
				})
				t.Run(`Missing status`, func(t *ftt.Test) {
					e.PresubmitResult.Status = analysispb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_UNSPECIFIED
					_, err := testInsertOrUpdate(e)
					assert.Loosely(t, err, should.ErrLike("presubmit result: status must be specified"))
				})
			})
		})
		t.Run(`ReadBuildToPresubmitRunJoinStatistics`, func(t *ftt.Test) {
			t.Run(`No data`, func(t *ftt.Test) {
				_, err := SetEntriesForTesting(ctx, t, nil...)
				assert.Loosely(t, err, should.BeNil)

				results, err := ReadBuildToPresubmitRunJoinStatistics(span.Single(ctx))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, results, should.Match(map[string]JoinStatistics{}))
			})
			t.Run(`Data`, func(t *ftt.Test) {
				reference := time.Now().Add(-1 * time.Minute)
				entriesToCreate := []*Entry{
					// Setup following data:
					// Project Alpha ("alpha") :=
					//  ]-1 hour, now]: 4 presubmit builds, 2 of which without
					//                  presubmit result, 1 of which without
					//                  build result.
					//                  1 non-presubmit build.
					//  ]-36 hours, -35 hours]: 1 presubmit build,
					//                          with all results.
					//  ]-37 hours, -36 hours]: 1 presubmit build,
					//                          with all results
					//                         (should be ignored as >36 hours old).
					// Project Beta ("beta") :=
					//  ]-37 hours, -36 hours]: 1 presubmit build,
					//                          without presubmit result.
					NewEntry(0).WithBuildProject("alpha").WithBuildJoinedTime(reference).Build(),
					NewEntry(1).WithBuildProject("alpha").WithBuildJoinedTime(reference).WithPresubmitResult(nil).Build(),
					NewEntry(2).WithBuildProject("alpha").WithBuildJoinedTime(reference).WithPresubmitResult(nil).Build(),
					NewEntry(3).WithPresubmitProject("alpha").WithPresubmitJoinedTime(reference).WithBuildResult(nil).Build(),
					NewEntry(4).WithBuildProject("alpha").WithBuildJoinedTime(reference).WithIsPresubmit(false).WithPresubmitResult(nil).Build(),
					NewEntry(5).WithBuildProject("alpha").WithBuildJoinedTime(reference.Add(-35 * time.Hour)).Build(),
					NewEntry(6).WithBuildProject("alpha").WithBuildJoinedTime(reference.Add(-36 * time.Hour)).Build(),
					NewEntry(7).WithBuildProject("beta").WithBuildJoinedTime(reference.Add(-36 * time.Hour)).WithPresubmitResult(nil).Build(),
				}
				_, err := SetEntriesForTesting(ctx, t, entriesToCreate...)
				assert.Loosely(t, err, should.BeNil)

				results, err := ReadBuildToPresubmitRunJoinStatistics(span.Single(ctx))
				assert.Loosely(t, err, should.BeNil)

				expectedAlpha := JoinStatistics{
					TotalByHour:  make([]int64, 36),
					JoinedByHour: make([]int64, 36),
				}
				expectedAlpha.TotalByHour[0] = 3
				expectedAlpha.JoinedByHour[0] = 1
				expectedAlpha.TotalByHour[35] = 1
				expectedAlpha.JoinedByHour[35] = 1
				// Only data in the last 36 hours is included, so the build
				// older than 36 hours is excluded.

				// Expect no entry to be returned for Project beta
				// as all data is older than 36 hours.

				assert.Loosely(t, results, should.Match(map[string]JoinStatistics{
					"alpha": expectedAlpha,
				}))
			})
		})
		t.Run(`ReadPresubmitToBuildJoinStatistics`, func(t *ftt.Test) {
			t.Run(`No data`, func(t *ftt.Test) {
				_, err := SetEntriesForTesting(ctx, t, nil...)
				assert.Loosely(t, err, should.BeNil)

				results, err := ReadPresubmitToBuildJoinStatistics(span.Single(ctx))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, results, should.Match(map[string]JoinStatistics{}))
			})
			t.Run(`Data`, func(t *ftt.Test) {
				reference := time.Now().Add(-1 * time.Minute)
				entriesToCreate := []*Entry{
					// Setup following data:
					// Project Alpha ("alpha") :=
					//  ]-1 hour, now]: 4 presubmit builds, 2 of which without
					//                  build result, 1 of which without
					//                  presubmit result.
					//                  1 non-presubmit build.
					//  ]-36 hours, -35 hours]: 1 presubmit build,
					//                          with all results.
					//  ]-37 hours, -36 hours]: 1 presubmit build,
					//                          with all results
					//                         (should be ignored as >36 hours old).
					// Project Beta ("beta") :=
					//  ]-37 hours, -36 hours]: 1 presubmit build,
					//                          without build result.
					NewEntry(0).WithPresubmitProject("alpha").WithPresubmitJoinedTime(reference).Build(),
					NewEntry(1).WithPresubmitProject("alpha").WithPresubmitJoinedTime(reference).WithBuildResult(nil).Build(),
					NewEntry(2).WithPresubmitProject("alpha").WithPresubmitJoinedTime(reference).WithBuildResult(nil).Build(),
					NewEntry(3).WithBuildProject("alpha").WithBuildJoinedTime(reference).WithPresubmitResult(nil).Build(),
					NewEntry(4).WithPresubmitProject("alpha").WithPresubmitJoinedTime(reference).WithIsPresubmit(false).WithBuildResult(nil).Build(),
					NewEntry(5).WithPresubmitProject("alpha").WithPresubmitJoinedTime(reference.Add(-35 * time.Hour)).Build(),
					NewEntry(6).WithPresubmitProject("alpha").WithPresubmitJoinedTime(reference.Add(-36 * time.Hour)).Build(),
					NewEntry(7).WithPresubmitProject("beta").WithPresubmitJoinedTime(reference.Add(-36 * time.Hour)).WithBuildResult(nil).Build(),
				}
				_, err := SetEntriesForTesting(ctx, t, entriesToCreate...)
				assert.Loosely(t, err, should.BeNil)

				results, err := ReadPresubmitToBuildJoinStatistics(span.Single(ctx))
				assert.Loosely(t, err, should.BeNil)

				expectedAlpha := JoinStatistics{
					TotalByHour:  make([]int64, 36),
					JoinedByHour: make([]int64, 36),
				}
				expectedAlpha.TotalByHour[0] = 3
				expectedAlpha.JoinedByHour[0] = 1
				expectedAlpha.TotalByHour[35] = 1
				expectedAlpha.JoinedByHour[35] = 1
				// Only data in the last 36 hours is included, so the build
				// older than 36 hours is excluded.

				// Expect no entry to be returned for Project beta
				// as all data is older than 36 hours.

				assert.Loosely(t, results, should.Match(map[string]JoinStatistics{
					"alpha": expectedAlpha,
				}))
			})
		})
		t.Run(`ReadBuildToInvocationJoinStatistics`, func(t *ftt.Test) {
			t.Run(`No data`, func(t *ftt.Test) {
				_, err := SetEntriesForTesting(ctx, t, nil...)
				assert.Loosely(t, err, should.BeNil)

				results, err := ReadBuildToInvocationJoinStatistics(span.Single(ctx))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, results, should.Match(map[string]JoinStatistics{}))
			})
			t.Run(`Data`, func(t *ftt.Test) {
				reference := time.Now().Add(-1 * time.Minute)
				entriesToCreate := []*Entry{
					// Setup following data:
					// Project Alpha ("alpha") :=
					//  ]-1 hour, now]: 4 builds /w invocation, 2 of which without
					//                  invocation result, 1 of which without
					//                  build result.
					//                  1 build w/o invocation.
					//  ]-36 hours, -35 hours]: 1 build /w invocation,
					//                          with all results.
					//  ]-37 hours, -36 hours]: 1 build /w invocation,
					//                          with all results
					//                         (should be ignored as >36 hours old).
					// Project Beta ("beta") :=
					//  ]-37 hours, -36 hours]: 1 build /w invocation,
					//                          without invocation result.
					NewEntry(0).WithBuildProject("alpha").WithBuildJoinedTime(reference).Build(),
					NewEntry(1).WithBuildProject("alpha").WithBuildJoinedTime(reference).WithInvocationResult(nil).Build(),
					NewEntry(2).WithBuildProject("alpha").WithBuildJoinedTime(reference).WithInvocationResult(nil).Build(),
					NewEntry(3).WithInvocationProject("alpha").WithInvocationJoinedTime(reference).WithBuildResult(nil).Build(),
					NewEntry(4).WithBuildProject("alpha").WithBuildJoinedTime(reference).WithHasInvocation(false).WithInvocationResult(nil).Build(),
					NewEntry(5).WithBuildProject("alpha").WithBuildJoinedTime(reference.Add(-35 * time.Hour)).Build(),
					NewEntry(6).WithBuildProject("alpha").WithBuildJoinedTime(reference.Add(-36 * time.Hour)).Build(),
					NewEntry(7).WithBuildProject("beta").WithBuildJoinedTime(reference.Add(-36 * time.Hour)).WithInvocationResult(nil).Build(),
				}
				_, err := SetEntriesForTesting(ctx, t, entriesToCreate...)
				assert.Loosely(t, err, should.BeNil)

				results, err := ReadBuildToInvocationJoinStatistics(span.Single(ctx))
				assert.Loosely(t, err, should.BeNil)

				expectedAlpha := JoinStatistics{
					TotalByHour:  make([]int64, 36),
					JoinedByHour: make([]int64, 36),
				}
				expectedAlpha.TotalByHour[0] = 3
				expectedAlpha.JoinedByHour[0] = 1
				expectedAlpha.TotalByHour[35] = 1
				expectedAlpha.JoinedByHour[35] = 1
				// Only data in the last 36 hours is included, so the build
				// older than 36 hours is excluded.

				// Expect no entry to be returned for Project beta
				// as all data is older than 36 hours.

				assert.Loosely(t, results, should.Match(map[string]JoinStatistics{
					"alpha": expectedAlpha,
				}))
			})
		})
		t.Run(`ReadInvocationToBuildJoinStatistics`, func(t *ftt.Test) {
			t.Run(`No data`, func(t *ftt.Test) {
				_, err := SetEntriesForTesting(ctx, t, nil...)
				assert.Loosely(t, err, should.BeNil)

				results, err := ReadInvocationToBuildJoinStatistics(span.Single(ctx))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, results, should.Match(map[string]JoinStatistics{}))
			})
			t.Run(`Data`, func(t *ftt.Test) {
				reference := time.Now().Add(-1 * time.Minute)
				entriesToCreate := []*Entry{
					// Setup following data:
					// Project Alpha ("alpha") :=
					//  ]-1 hour, now]: 4 builds /w invocation, 2 of which without
					//                  build result, 1 of which without
					//                  invocation result.
					//                  1 build w/o invocation.
					//  ]-36 hours, -35 hours]: 1 build /w invocation,
					//                          with all results.
					//  ]-37 hours, -36 hours]: 1 build /w invocation,
					//                          with all results
					//                         (should be ignored as >36 hours old).
					// Project Beta ("beta") :=
					//  ]-37 hours, -36 hours]: 1 build /w invocation,
					//                          without build result.
					NewEntry(0).WithInvocationProject("alpha").WithInvocationJoinedTime(reference).Build(),
					NewEntry(1).WithInvocationProject("alpha").WithInvocationJoinedTime(reference).WithBuildResult(nil).Build(),
					NewEntry(2).WithInvocationProject("alpha").WithInvocationJoinedTime(reference).WithBuildResult(nil).Build(),
					NewEntry(3).WithBuildProject("alpha").WithBuildJoinedTime(reference).WithInvocationResult(nil).Build(),
					NewEntry(4).WithInvocationProject("alpha").WithInvocationJoinedTime(reference).WithHasInvocation(false).WithBuildResult(nil).Build(),
					NewEntry(5).WithInvocationProject("alpha").WithInvocationJoinedTime(reference.Add(-35 * time.Hour)).Build(),
					NewEntry(6).WithInvocationProject("alpha").WithInvocationJoinedTime(reference.Add(-36 * time.Hour)).Build(),
					NewEntry(7).WithInvocationProject("beta").WithInvocationJoinedTime(reference.Add(-36 * time.Hour)).WithBuildResult(nil).Build(),
					NewEntry(8).WithInvocationProject("beta").WithInvocationJoinedTime(reference).WithBuildResult(nil).WithHasBuildbucketBuild(false).Build(),
				}
				_, err := SetEntriesForTesting(ctx, t, entriesToCreate...)
				assert.Loosely(t, err, should.BeNil)

				results, err := ReadInvocationToBuildJoinStatistics(span.Single(ctx))
				assert.Loosely(t, err, should.BeNil)

				expectedAlpha := JoinStatistics{
					TotalByHour:  make([]int64, 36),
					JoinedByHour: make([]int64, 36),
				}
				expectedAlpha.TotalByHour[0] = 3
				expectedAlpha.JoinedByHour[0] = 1
				expectedAlpha.TotalByHour[35] = 1
				expectedAlpha.JoinedByHour[35] = 1
				// Only data in the last 36 hours is included, so the build
				// older than 36 hours is excluded.

				// Expect no entry to be returned for Project beta
				// as all data is older than 36 hours.

				assert.Loosely(t, results, should.Match(map[string]JoinStatistics{
					"alpha": expectedAlpha,
				}))
			})
		})
	})
}
