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

package state

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSpanner(t *testing.T) {
	Convey(`With Spanner Test Database`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		Convey(`Create`, func() {
			testCreate := func(e *Entry) (time.Time, error) {
				commitTime, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					return Create(ctx, e)
				})
				return commitTime, err
			}
			e := NewEntry(100).Build()
			Convey(`Valid`, func() {
				commitTime, err := testCreate(e)
				So(err, ShouldBeNil)
				e.LastUpdated = commitTime.In(time.UTC)

				txn := span.Single(ctx)
				actual, err := Read(txn, e.Project, e.ChunkID)
				So(err, ShouldBeNil)
				So(actual, ShouldResemble, e)
			})
			Convey(`Invalid`, func() {
				Convey(`Project missing`, func() {
					e.Project = ""
					_, err := testCreate(e)
					So(err, ShouldErrLike, `project "" is not valid`)
				})
				Convey(`Chunk ID missing`, func() {
					e.ChunkID = ""
					_, err := testCreate(e)
					So(err, ShouldErrLike, `chunk ID "" is not valid`)
				})
				Convey(`Partition Time missing`, func() {
					var t time.Time
					e.PartitionTime = t
					_, err := testCreate(e)
					So(err, ShouldErrLike, "partition time must be specified")
				})
				Convey(`Object ID missing`, func() {
					e.ObjectID = ""
					_, err := testCreate(e)
					So(err, ShouldErrLike, "object ID must be specified")
				})
				Convey(`Config Version missing`, func() {
					var t time.Time
					e.Clustering.ConfigVersion = t
					_, err := testCreate(e)
					So(err, ShouldErrLike, "config version must be valid")
				})
				Convey(`Rules Version missing`, func() {
					var t time.Time
					e.Clustering.RulesVersion = t
					_, err := testCreate(e)
					So(err, ShouldErrLike, "rules version must be valid")
				})
				Convey(`Algorithms Version missing`, func() {
					e.Clustering.AlgorithmsVersion = 0
					_, err := testCreate(e)
					So(err, ShouldErrLike, "algorithms version must be specified")
				})
				Convey(`Clusters missing`, func() {
					e.Clustering.Clusters = nil
					_, err := testCreate(e)
					So(err, ShouldErrLike, "there must be clustered test results in the chunk")
				})
				Convey(`Algorithms invalid`, func() {
					Convey(`Empty algorithm`, func() {
						e.Clustering.Algorithms[""] = struct{}{}
						_, err := testCreate(e)
						So(err, ShouldErrLike, `algorithm "" is not valid`)
					})
					Convey("Algorithm invalid", func() {
						e.Clustering.Algorithms["!!!"] = struct{}{}
						_, err := testCreate(e)
						So(err, ShouldErrLike, `algorithm "!!!" is not valid`)
					})
				})
				Convey(`Clusters invalid`, func() {
					Convey("Algorithm not in algorithms set", func() {
						e.Clustering.Algorithms = map[string]struct{}{}
						_, err := testCreate(e)
						So(err, ShouldErrLike, `clusters: test result 0: cluster 0: algorithm not in algorithms list`)
					})
					Convey("ID missing", func() {
						e.Clustering.Clusters[1][1].ID = ""
						_, err := testCreate(e)
						So(err, ShouldErrLike, `clusters: test result 1: cluster 1: cluster ID is not valid: ID is empty`)
					})
				})
			})
		})
		Convey(`UpdateClustering`, func() {
			Convey(`Valid`, func() {
				entry := NewEntry(0).Build()
				entries := []*Entry{
					entry,
				}
				commitTime, err := CreateEntriesForTesting(ctx, entries)
				So(err, ShouldBeNil)
				entry.LastUpdated = commitTime.In(time.UTC)

				expected := NewEntry(0).Build()

				test := func(update clustering.ClusterResults, expected *Entry) {
					// Apply the update.
					commitTime, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
						err := UpdateClustering(ctx, entry, &update)
						return err
					})
					So(err, ShouldEqual, nil)
					expected.LastUpdated = commitTime.In(time.UTC)

					// Assert the update was applied.
					actual, err := Read(span.Single(ctx), expected.Project, expected.ChunkID)
					So(err, ShouldBeNil)
					So(actual, ShouldResemble, expected)
				}
				Convey(`Full update`, func() {
					// Prepare an update.
					newClustering := NewEntry(1).Build().Clustering
					expected.Clustering = newClustering

					So(clustering.AlgorithmsAndClustersEqual(&entry.Clustering, &newClustering), ShouldBeFalse)
					test(newClustering, expected)
				})
				Convey(`Minor update`, func() {
					// Update only algorithms + rules + config version, without changing clustering content.
					newClustering := NewEntry(0).
						WithAlgorithmsVersion(10).
						WithConfigVersion(time.Date(2024, time.July, 5, 4, 3, 2, 1, time.UTC)).
						WithRulesVersion(time.Date(2024, time.June, 5, 4, 3, 2, 1000, time.UTC)).
						Build().Clustering

					expected.Clustering = newClustering
					So(clustering.AlgorithmsAndClustersEqual(&entries[0].Clustering, &newClustering), ShouldBeTrue)
					test(newClustering, expected)
				})
				Convey(`No-op update`, func() {
					test(entry.Clustering, expected)
				})
			})
			Convey(`Invalid`, func() {
				originalEntry := NewEntry(0).Build()
				newClustering := &NewEntry(0).Build().Clustering

				// Try an invalid algorithm. We do not repeat all the same
				// validation test cases as create, as the underlying
				// implementation is the same.
				newClustering.Algorithms["!!!"] = struct{}{}

				_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					err := UpdateClustering(ctx, originalEntry, newClustering)
					return err
				})
				So(err, ShouldErrLike, `algorithm "!!!" is not valid`)
			})
		})
		Convey(`ReadLastUpdated`, func() {
			// Create two entries at different times to give them different LastUpdated times.
			entryOne := NewEntry(0).Build()
			lastUpdatedOne, err := CreateEntriesForTesting(ctx, []*Entry{entryOne})
			So(err, ShouldBeNil)

			entryTwo := NewEntry(1).Build()
			lastUpdatedTwo, err := CreateEntriesForTesting(ctx, []*Entry{entryTwo})
			So(err, ShouldBeNil)

			chunkKeys := []ChunkKey{
				{Project: testProject, ChunkID: entryOne.ChunkID},
				{Project: "otherproject", ChunkID: entryOne.ChunkID},
				{Project: testProject, ChunkID: "1234567890abcdef1234567890abcdef"},
				{Project: testProject, ChunkID: entryTwo.ChunkID},
			}

			actual, err := ReadLastUpdated(span.Single(ctx), chunkKeys)
			So(err, ShouldBeNil)
			So(len(actual), ShouldEqual, len(chunkKeys))
			So(actual[0], ShouldEqual, lastUpdatedOne)
			So(actual[1], ShouldEqual, time.Time{})
			So(actual[2], ShouldEqual, time.Time{})
			So(actual[3], ShouldEqual, lastUpdatedTwo)
		})
		Convey(`ReadNextN`, func() {
			targetRulesVersion := time.Date(2024, 1, 1, 1, 1, 1, 0, time.UTC)
			targetConfigVersion := time.Date(2024, 2, 1, 1, 1, 1, 0, time.UTC)
			targetAlgorithmsVersion := 10
			entries := []*Entry{
				// Should not be read.
				NewEntry(0).WithChunkIDPrefix("11").
					WithAlgorithmsVersion(10).
					WithConfigVersion(targetConfigVersion).
					WithRulesVersion(targetRulesVersion).Build(),

				// Should be read (rulesVersion < targetRulesVersion).
				NewEntry(1).WithChunkIDPrefix("11").
					WithAlgorithmsVersion(10).
					WithConfigVersion(targetConfigVersion).
					WithRulesVersion(targetRulesVersion.Add(-1 * time.Hour)).Build(),
				NewEntry(2).WithChunkIDPrefix("11").
					WithRulesVersion(targetRulesVersion.Add(-1 * time.Hour)).Build(),

				// Should be read (configVersion < targetConfigVersion).
				NewEntry(3).WithChunkIDPrefix("11").
					WithAlgorithmsVersion(10).
					WithConfigVersion(targetConfigVersion.Add(-1 * time.Hour)).
					WithRulesVersion(targetRulesVersion).Build(),
				NewEntry(4).WithChunkIDPrefix("11").
					WithConfigVersion(targetConfigVersion.Add(-1 * time.Hour)).Build(),

				// Should be read (algorithmsVersion < targetAlgorithmsVersion).
				NewEntry(5).WithChunkIDPrefix("11").
					WithAlgorithmsVersion(9).
					WithConfigVersion(targetConfigVersion).
					WithRulesVersion(targetRulesVersion).Build(),
				NewEntry(6).WithChunkIDPrefix("11").
					WithAlgorithmsVersion(2).Build(),

				// Should not be read (other project).
				NewEntry(7).WithChunkIDPrefix("11").
					WithAlgorithmsVersion(2).
					WithProject("other").Build(),

				// Check handling of EndChunkID as an inclusive upper-bound.
				NewEntry(8).WithChunkIDPrefix("11" + strings.Repeat("ff", 15)).WithAlgorithmsVersion(2).Build(), // Should be read.
				NewEntry(9).WithChunkIDPrefix("12" + strings.Repeat("00", 15)).WithAlgorithmsVersion(2).Build(), // Should not be read.
			}

			commitTime, err := CreateEntriesForTesting(ctx, entries)
			for _, e := range entries {
				e.LastUpdated = commitTime.In(time.UTC)
			}
			So(err, ShouldBeNil)

			expectedEntries := []*Entry{
				entries[1],
				entries[2],
				entries[3],
				entries[4],
				entries[5],
				entries[6],
				entries[8],
			}
			sort.Slice(expectedEntries, func(i, j int) bool {
				return expectedEntries[i].ChunkID < expectedEntries[j].ChunkID
			})

			readOpts := ReadNextOptions{
				StartChunkID:      "11" + strings.Repeat("00", 15),
				EndChunkID:        "11" + strings.Repeat("ff", 15),
				AlgorithmsVersion: int64(targetAlgorithmsVersion),
				ConfigVersion:     targetConfigVersion,
				RulesVersion:      targetRulesVersion,
			}
			// Reads first page.
			rows, err := ReadNextN(span.Single(ctx), testProject, readOpts, 4)
			So(err, ShouldBeNil)
			So(rows, ShouldResemble, expectedEntries[0:4])

			// Read second page.
			readOpts.StartChunkID = rows[3].ChunkID
			rows, err = ReadNextN(span.Single(ctx), testProject, readOpts, 4)
			So(err, ShouldBeNil)
			So(rows, ShouldResemble, expectedEntries[4:])

			// Read empty last page.
			readOpts.StartChunkID = rows[2].ChunkID
			rows, err = ReadNextN(span.Single(ctx), testProject, readOpts, 4)
			So(err, ShouldBeNil)
			So(rows, ShouldBeEmpty)
		})
		Convey(`EstimateChunks`, func() {
			Convey(`Less than 100 chunks`, func() {
				est, err := EstimateChunks(span.Single(ctx), testProject)
				So(err, ShouldBeNil)
				So(est, ShouldBeLessThan, 100)
			})
			Convey(`At least 100 chunks`, func() {
				var entries []*Entry
				for i := 0; i < 200; i++ {
					entries = append(entries, NewEntry(i).Build())
				}
				_, err := CreateEntriesForTesting(ctx, entries)
				So(err, ShouldBeNil)

				count, err := EstimateChunks(span.Single(ctx), testProject)
				So(err, ShouldBeNil)
				So(count, ShouldBeGreaterThan, 190)
				So(count, ShouldBeLessThan, 210)
			})
		})
	})
	Convey(`estimateChunksFromID`, t, func() {
		// Extremely full table. This is the minimum that the 100th ID
		// could be (considering 0x63 = 99).
		count, err := estimateChunksFromID("00000000000000000000000000000063")
		So(err, ShouldBeNil)
		// The maximum estimate.
		So(count, ShouldEqual, 1000*1000*1000)

		// The 100th ID is right in the middle of the keyspace.
		count, err = estimateChunksFromID("7fffffffffffffffffffffffffffffff")
		So(err, ShouldBeNil)
		So(count, ShouldEqual, 200)

		// The 100th ID is right at the end of the keyspace.
		count, err = estimateChunksFromID("ffffffffffffffffffffffffffffffff")
		So(err, ShouldBeNil)
		So(count, ShouldEqual, 100)
	})
}
