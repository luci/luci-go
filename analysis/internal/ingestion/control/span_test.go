// Copyright 2022 The LUCI Authors.
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
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/testutil"
	analysispb "go.chromium.org/luci/analysis/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSpan(t *testing.T) {
	Convey(`With Spanner Test Database`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		Convey(`Read`, func() {
			entriesToCreate := []*Entry{
				NewEntry(0).WithBuildID("buildbucket-instance/1").Build(),
				NewEntry(2).WithBuildID("buildbucket-instance/2").WithBuildResult(nil).Build(),
				NewEntry(3).WithBuildID("buildbucket-instance/3").WithPresubmitResult(nil).Build(),
				NewEntry(4).WithBuildID("buildbucket-instance/4").WithInvocationResult(nil).Build(),
			}
			_, err := SetEntriesForTesting(ctx, entriesToCreate...)
			So(err, ShouldBeNil)

			Convey(`None exist`, func() {
				buildIDs := []string{"buildbucket-instance/5"}
				results, err := Read(span.Single(ctx), buildIDs)
				So(err, ShouldBeNil)
				So(len(results), ShouldEqual, 1)
				So(results[0], ShouldResembleEntry, nil)
			})
			Convey(`Some exist`, func() {
				buildIDs := []string{
					"buildbucket-instance/3",
					"buildbucket-instance/4",
					"buildbucket-instance/5",
					"buildbucket-instance/2",
					"buildbucket-instance/1",
				}
				results, err := Read(span.Single(ctx), buildIDs)
				So(err, ShouldBeNil)
				So(len(results), ShouldEqual, 5)
				So(results[0], ShouldResembleEntry, entriesToCreate[2])
				So(results[1], ShouldResembleEntry, entriesToCreate[3])
				So(results[2], ShouldResembleEntry, nil)
				So(results[3], ShouldResembleEntry, entriesToCreate[1])
				So(results[4], ShouldResembleEntry, entriesToCreate[0])
			})
		})
		Convey(`InsertOrUpdate`, func() {
			testInsertOrUpdate := func(e *Entry) (time.Time, error) {
				commitTime, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					return InsertOrUpdate(ctx, e)
				})
				return commitTime.In(time.UTC), err
			}

			entryToCreate := NewEntry(0).Build()

			_, err := SetEntriesForTesting(ctx, entryToCreate)
			So(err, ShouldBeNil)

			e := NewEntry(1).Build()

			Convey(`Valid`, func() {
				Convey(`Insert`, func() {
					commitTime, err := testInsertOrUpdate(e)
					So(err, ShouldBeNil)
					e.LastUpdated = commitTime

					result, err := Read(span.Single(ctx), []string{e.BuildID})
					So(err, ShouldBeNil)
					So(len(result), ShouldEqual, 1)
					So(result[0], ShouldResembleEntry, e)
				})
				Convey(`Update`, func() {
					// Update the existing entry.
					e.BuildID = entryToCreate.BuildID

					commitTime, err := testInsertOrUpdate(e)
					So(err, ShouldBeNil)
					e.LastUpdated = commitTime

					result, err := Read(span.Single(ctx), []string{e.BuildID})
					So(err, ShouldBeNil)
					So(len(result), ShouldEqual, 1)
					So(result[0], ShouldResembleEntry, e)
				})
			})
			Convey(`With missing Build ID`, func() {
				e.BuildID = ""
				_, err := testInsertOrUpdate(e)
				So(err, ShouldErrLike, "build ID must be specified")
			})
			Convey(`With invalid Build Project`, func() {
				Convey(`Missing`, func() {
					e.BuildProject = ""
					_, err := testInsertOrUpdate(e)
					So(err, ShouldErrLike, "build project: unspecified")
				})
				Convey(`Invalid`, func() {
					e.BuildProject = "!"
					_, err := testInsertOrUpdate(e)
					So(err, ShouldErrLike, `build project: must match ^[a-z0-9\-]{1,40}$`)
				})
			})
			Convey(`With invalid Build Result`, func() {
				Convey(`Missing host`, func() {
					e.BuildResult.Host = ""
					_, err := testInsertOrUpdate(e)
					So(err, ShouldErrLike, "host must be specified")
				})
				Convey(`Missing id`, func() {
					e.BuildResult.Id = 0
					_, err := testInsertOrUpdate(e)
					So(err, ShouldErrLike, "id must be specified")
				})
				Convey(`Missing creation time`, func() {
					e.BuildResult.CreationTime = nil
					_, err := testInsertOrUpdate(e)
					So(err, ShouldErrLike, "build result: creation time must be specified")
				})
				Convey(`Missing project`, func() {
					e.BuildResult.Project = ""
					_, err := testInsertOrUpdate(e)
					So(err, ShouldErrLike, "build result: project must be specified")
				})
				Convey(`Missing resultdb_host`, func() {
					e.BuildResult.HasInvocation = true
					e.BuildResult.ResultdbHost = ""
					_, err := testInsertOrUpdate(e)
					So(err, ShouldErrLike, "build result: resultdb_host must be specified if has_invocation set")
				})
				Convey(`Missing builder`, func() {
					e.BuildResult.Builder = ""
					_, err := testInsertOrUpdate(e)
					So(err, ShouldErrLike, "build result: builder must be specified")
				})
				Convey(`Missing status`, func() {
					e.BuildResult.Status = analysispb.BuildStatus_BUILD_STATUS_UNSPECIFIED
					_, err := testInsertOrUpdate(e)
					So(err, ShouldErrLike, "build result: build status must be specified")
				})
			})
			Convey(`With invalid Invocation Project`, func() {
				Convey(`Unspecified`, func() {
					e.InvocationProject = ""
					_, err := testInsertOrUpdate(e)
					So(err, ShouldErrLike, `invocation project: unspecified`)
				})
				Convey(`Invalid`, func() {
					e.InvocationProject = "!"
					_, err := testInsertOrUpdate(e)
					So(err, ShouldErrLike, `invocation project: must match ^[a-z0-9\-]{1,40}$`)
				})
			})
			Convey(`With invalid Invocation Result`, func() {
				Convey(`Set when HasInvocation = false`, func() {
					So(e.InvocationResult, ShouldNotBeNil)
					e.HasInvocation = false
					_, err := testInsertOrUpdate(e)
					So(err, ShouldErrLike, "invocation result must not be set unless HasInvocation is set")
				})
			})
			Convey(`With invalid Presubmit Project`, func() {
				Convey(`Unspecified`, func() {
					e.PresubmitProject = ""
					_, err := testInsertOrUpdate(e)
					So(err, ShouldErrLike, `presubmit project: unspecified`)
				})
				Convey(`Invalid`, func() {
					e.PresubmitProject = "!"
					_, err := testInsertOrUpdate(e)
					So(err, ShouldErrLike, `presubmit project: must match ^[a-z0-9\-]{1,40}$`)
				})
			})
			Convey(`With invalid Presubmit Result`, func() {
				Convey(`Set when IsPresbumit = false`, func() {
					So(e.PresubmitResult, ShouldNotBeNil)
					e.IsPresubmit = false
					_, err := testInsertOrUpdate(e)
					So(err, ShouldErrLike, "presubmit result must not be set unless IsPresubmit is set")
				})
				Convey(`Missing Presubmit run ID`, func() {
					e.PresubmitResult.PresubmitRunId = nil
					_, err := testInsertOrUpdate(e)
					So(err, ShouldErrLike, "presubmit run ID must be specified")
				})
				Convey(`Invalid Presubmit run ID host`, func() {
					e.PresubmitResult.PresubmitRunId.System = "!"
					_, err := testInsertOrUpdate(e)
					So(err, ShouldErrLike, "presubmit run system must be 'luci-cv'")
				})
				Convey(`Missing Presubmit run ID system-specific ID`, func() {
					e.PresubmitResult.PresubmitRunId.Id = ""
					_, err := testInsertOrUpdate(e)
					So(err, ShouldErrLike, "presubmit run system-specific ID must be specified")
				})
				Convey(`Missing creation time`, func() {
					e.PresubmitResult.CreationTime = nil
					_, err := testInsertOrUpdate(e)
					So(err, ShouldErrLike, "presubmit result: creation time must be specified")
				})
				Convey(`Missing mode`, func() {
					e.PresubmitResult.Mode = analysispb.PresubmitRunMode_PRESUBMIT_RUN_MODE_UNSPECIFIED
					_, err := testInsertOrUpdate(e)
					So(err, ShouldErrLike, "presubmit result: mode must be specified")
				})
				Convey(`Missing status`, func() {
					e.PresubmitResult.Status = analysispb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_UNSPECIFIED
					_, err := testInsertOrUpdate(e)
					So(err, ShouldErrLike, "presubmit result: status must be specified")
				})
			})
		})
		Convey(`ReadBuildToPresubmitRunJoinStatistics`, func() {
			Convey(`No data`, func() {
				_, err := SetEntriesForTesting(ctx, nil...)
				So(err, ShouldBeNil)

				results, err := ReadBuildToPresubmitRunJoinStatistics(span.Single(ctx))
				So(err, ShouldBeNil)
				So(results, ShouldResemble, map[string]JoinStatistics{})
			})
			Convey(`Data`, func() {
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
				_, err := SetEntriesForTesting(ctx, entriesToCreate...)
				So(err, ShouldBeNil)

				results, err := ReadBuildToPresubmitRunJoinStatistics(span.Single(ctx))
				So(err, ShouldBeNil)

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

				So(results, ShouldResemble, map[string]JoinStatistics{
					"alpha": expectedAlpha,
				})
			})
		})
		Convey(`ReadPresubmitToBuildJoinStatistics`, func() {
			Convey(`No data`, func() {
				_, err := SetEntriesForTesting(ctx, nil...)
				So(err, ShouldBeNil)

				results, err := ReadPresubmitToBuildJoinStatistics(span.Single(ctx))
				So(err, ShouldBeNil)
				So(results, ShouldResemble, map[string]JoinStatistics{})
			})
			Convey(`Data`, func() {
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
				_, err := SetEntriesForTesting(ctx, entriesToCreate...)
				So(err, ShouldBeNil)

				results, err := ReadPresubmitToBuildJoinStatistics(span.Single(ctx))
				So(err, ShouldBeNil)

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

				So(results, ShouldResemble, map[string]JoinStatistics{
					"alpha": expectedAlpha,
				})
			})
		})
		Convey(`ReadBuildToInvocationJoinStatistics`, func() {
			Convey(`No data`, func() {
				_, err := SetEntriesForTesting(ctx, nil...)
				So(err, ShouldBeNil)

				results, err := ReadBuildToInvocationJoinStatistics(span.Single(ctx))
				So(err, ShouldBeNil)
				So(results, ShouldResemble, map[string]JoinStatistics{})
			})
			Convey(`Data`, func() {
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
				_, err := SetEntriesForTesting(ctx, entriesToCreate...)
				So(err, ShouldBeNil)

				results, err := ReadBuildToInvocationJoinStatistics(span.Single(ctx))
				So(err, ShouldBeNil)

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

				So(results, ShouldResemble, map[string]JoinStatistics{
					"alpha": expectedAlpha,
				})
			})
		})
		Convey(`ReadInvocationToBuildJoinStatistics`, func() {
			Convey(`No data`, func() {
				_, err := SetEntriesForTesting(ctx, nil...)
				So(err, ShouldBeNil)

				results, err := ReadInvocationToBuildJoinStatistics(span.Single(ctx))
				So(err, ShouldBeNil)
				So(results, ShouldResemble, map[string]JoinStatistics{})
			})
			Convey(`Data`, func() {
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
				}
				_, err := SetEntriesForTesting(ctx, entriesToCreate...)
				So(err, ShouldBeNil)

				results, err := ReadInvocationToBuildJoinStatistics(span.Single(ctx))
				So(err, ShouldBeNil)

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

				So(results, ShouldResemble, map[string]JoinStatistics{
					"alpha": expectedAlpha,
				})
			})
		})
	})
}

func ShouldResembleEntry(actual any, expected ...any) string {
	if len(expected) != 1 {
		return fmt.Sprintf("ShouldResembleEntry expects 1 value, got %d", len(expected))
	}
	exp := expected[0]
	if exp == nil {
		return ShouldBeNil(actual)
	}

	a, ok := actual.(*Entry)
	if !ok {
		return "actual should be of type *Entry"
	}
	e, ok := exp.(*Entry)
	if !ok {
		return "expected value should be of type *Entry"
	}

	// Check equality of non-proto fields.
	a.BuildResult = nil
	a.PresubmitResult = nil
	e.BuildResult = nil
	e.PresubmitResult = nil
	if msg := ShouldResemble(a, e); msg != "" {
		return msg
	}

	// Check equality of proto fields.
	if msg := ShouldResembleProto(a.BuildResult, e.BuildResult); msg != "" {
		return msg
	}
	if msg := ShouldResembleProto(a.PresubmitResult, e.PresubmitResult); msg != "" {
		return msg
	}
	return ""
}
