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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/testutil"
)

func TestSpanner(t *testing.T) {
	ftt.Run(`With Spanner Test Database`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		t.Run(`Create`, func(t *ftt.Test) {
			testCreate := func(e *Entry) (time.Time, error) {
				commitTime, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					return Create(ctx, e)
				})
				return commitTime, err
			}
			e := NewEntry(100).Build()
			t.Run(`Valid`, func(t *ftt.Test) {
				commitTime, err := testCreate(e)
				assert.Loosely(t, err, should.BeNil)
				e.LastUpdated = commitTime.In(time.UTC)

				txn := span.Single(ctx)
				actual, err := Read(txn, e.Project, e.ChunkID)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, actual, should.Resemble(e))
			})
			t.Run(`Invalid`, func(t *ftt.Test) {
				t.Run(`Project missing`, func(t *ftt.Test) {
					e.Project = ""
					_, err := testCreate(e)
					assert.Loosely(t, err, should.ErrLike(`project: unspecified`))
				})
				t.Run(`Chunk ID missing`, func(t *ftt.Test) {
					e.ChunkID = ""
					_, err := testCreate(e)
					assert.Loosely(t, err, should.ErrLike(`chunk ID "" is not valid`))
				})
				t.Run(`Partition Time missing`, func(t *ftt.Test) {
					var ts time.Time
					e.PartitionTime = ts
					_, err := testCreate(e)
					assert.Loosely(t, err, should.ErrLike("partition time must be specified"))
				})
				t.Run(`Object ID missing`, func(t *ftt.Test) {
					e.ObjectID = ""
					_, err := testCreate(e)
					assert.Loosely(t, err, should.ErrLike("object ID must be specified"))
				})
				t.Run(`Config Version missing`, func(t *ftt.Test) {
					var ts time.Time
					e.Clustering.ConfigVersion = ts
					_, err := testCreate(e)
					assert.Loosely(t, err, should.ErrLike("config version must be valid"))
				})
				t.Run(`Rules Version missing`, func(t *ftt.Test) {
					var ts time.Time
					e.Clustering.RulesVersion = ts
					_, err := testCreate(e)
					assert.Loosely(t, err, should.ErrLike("rules version must be valid"))
				})
				t.Run(`Algorithms Version missing`, func(t *ftt.Test) {
					e.Clustering.AlgorithmsVersion = 0
					_, err := testCreate(e)
					assert.Loosely(t, err, should.ErrLike("algorithms version must be specified"))
				})
				t.Run(`Clusters missing`, func(t *ftt.Test) {
					e.Clustering.Clusters = nil
					_, err := testCreate(e)
					assert.Loosely(t, err, should.ErrLike("there must be clustered test results in the chunk"))
				})
				t.Run(`Algorithms invalid`, func(t *ftt.Test) {
					t.Run(`Empty algorithm`, func(t *ftt.Test) {
						e.Clustering.Algorithms[""] = struct{}{}
						_, err := testCreate(e)
						assert.Loosely(t, err, should.ErrLike(`algorithm "" is not valid`))
					})
					t.Run("Algorithm invalid", func(t *ftt.Test) {
						e.Clustering.Algorithms["!!!"] = struct{}{}
						_, err := testCreate(e)
						assert.Loosely(t, err, should.ErrLike(`algorithm "!!!" is not valid`))
					})
				})
				t.Run(`Clusters invalid`, func(t *ftt.Test) {
					t.Run("Algorithm not in algorithms set", func(t *ftt.Test) {
						e.Clustering.Algorithms = map[string]struct{}{}
						_, err := testCreate(e)
						assert.Loosely(t, err, should.ErrLike(`clusters: test result 0: cluster 0: algorithm not in algorithms list`))
					})
					t.Run("ID missing", func(t *ftt.Test) {
						e.Clustering.Clusters[1][1].ID = ""
						_, err := testCreate(e)
						assert.Loosely(t, err, should.ErrLike(`clusters: test result 1: cluster 1: cluster ID is not valid: ID is empty`))
					})
				})
			})
		})
		t.Run(`UpdateClustering`, func(t *ftt.Test) {
			t.Run(`Valid`, func(t *ftt.Test) {
				entry := NewEntry(0).Build()
				entries := []*Entry{
					entry,
				}
				commitTime, err := CreateEntriesForTesting(ctx, entries)
				assert.Loosely(t, err, should.BeNil)
				entry.LastUpdated = commitTime.In(time.UTC)

				expected := NewEntry(0).Build()

				test := func(update clustering.ClusterResults, expected *Entry) {
					// Apply the update.
					commitTime, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
						err := UpdateClustering(ctx, entry, &update)
						return err
					})
					assert.Loosely(t, err, should.BeNil)
					expected.LastUpdated = commitTime.In(time.UTC)

					// Assert the update was applied.
					actual, err := Read(span.Single(ctx), expected.Project, expected.ChunkID)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, actual, should.Resemble(expected))
				}
				t.Run(`Full update`, func(t *ftt.Test) {
					// Prepare an update.
					newClustering := NewEntry(1).Build().Clustering
					expected.Clustering = newClustering

					assert.Loosely(t, clustering.AlgorithmsAndClustersEqual(&entry.Clustering, &newClustering), should.BeFalse)
					test(newClustering, expected)
				})
				t.Run(`Minor update`, func(t *ftt.Test) {
					// Update only algorithms + rules + config version, without changing clustering content.
					newClustering := NewEntry(0).
						WithAlgorithmsVersion(10).
						WithConfigVersion(time.Date(2024, time.July, 5, 4, 3, 2, 1, time.UTC)).
						WithRulesVersion(time.Date(2024, time.June, 5, 4, 3, 2, 1000, time.UTC)).
						Build().Clustering

					expected.Clustering = newClustering
					assert.Loosely(t, clustering.AlgorithmsAndClustersEqual(&entries[0].Clustering, &newClustering), should.BeTrue)
					test(newClustering, expected)
				})
				t.Run(`No-op update`, func(t *ftt.Test) {
					test(entry.Clustering, expected)
				})
			})
			t.Run(`Invalid`, func(t *ftt.Test) {
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
				assert.Loosely(t, err, should.ErrLike(`algorithm "!!!" is not valid`))
			})
		})
		t.Run(`ReadLastUpdated`, func(t *ftt.Test) {
			// Create two entries at different times to give them different LastUpdated times.
			entryOne := NewEntry(0).Build()
			lastUpdatedOne, err := CreateEntriesForTesting(ctx, []*Entry{entryOne})
			assert.Loosely(t, err, should.BeNil)

			entryTwo := NewEntry(1).Build()
			lastUpdatedTwo, err := CreateEntriesForTesting(ctx, []*Entry{entryTwo})
			assert.Loosely(t, err, should.BeNil)

			chunkKeys := []ChunkKey{
				{Project: testProject, ChunkID: entryOne.ChunkID},
				{Project: "otherproject", ChunkID: entryOne.ChunkID},
				{Project: testProject, ChunkID: "1234567890abcdef1234567890abcdef"},
				{Project: testProject, ChunkID: entryTwo.ChunkID},
			}

			actual, err := ReadLastUpdated(span.Single(ctx), chunkKeys)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(actual), should.Equal(len(chunkKeys)))
			assert.That(t, actual[0], should.Match(lastUpdatedOne))
			assert.That(t, actual[1], should.Match(time.Time{}))
			assert.That(t, actual[2], should.Match(time.Time{}))
			assert.That(t, actual[3], should.Match(lastUpdatedTwo))
		})
		t.Run(`ReadNextN`, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)

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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rows, should.Resemble(expectedEntries[0:4]))

			// Read second page.
			readOpts.StartChunkID = rows[3].ChunkID
			rows, err = ReadNextN(span.Single(ctx), testProject, readOpts, 4)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rows, should.Resemble(expectedEntries[4:]))

			// Read empty last page.
			readOpts.StartChunkID = rows[2].ChunkID
			rows, err = ReadNextN(span.Single(ctx), testProject, readOpts, 4)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rows, should.BeEmpty)
		})
		t.Run(`EstimateChunks`, func(t *ftt.Test) {
			t.Run(`Less than 100 chunks`, func(t *ftt.Test) {
				est, err := EstimateChunks(span.Single(ctx), testProject)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, est, should.BeLessThan(100))
			})
			t.Run(`At least 100 chunks`, func(t *ftt.Test) {
				var entries []*Entry
				for i := 0; i < 200; i++ {
					entries = append(entries, NewEntry(i).Build())
				}
				_, err := CreateEntriesForTesting(ctx, entries)
				assert.Loosely(t, err, should.BeNil)

				count, err := EstimateChunks(span.Single(ctx), testProject)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, count, should.BeGreaterThan(190))
				assert.Loosely(t, count, should.BeLessThan(210))
			})
		})
	})
	ftt.Run(`estimateChunksFromID`, t, func(t *ftt.Test) {
		// Extremely full table. This is the minimum that the 100th ID
		// could be (considering 0x63 = 99).
		count, err := estimateChunksFromID("00000000000000000000000000000063")
		assert.Loosely(t, err, should.BeNil)
		// The maximum estimate.
		assert.Loosely(t, count, should.Equal(1000*1000*1000))

		// The 100th ID is right in the middle of the keyspace.
		count, err = estimateChunksFromID("7fffffffffffffffffffffffffffffff")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, count, should.Equal(200))

		// The 100th ID is right at the end of the keyspace.
		count, err = estimateChunksFromID("ffffffffffffffffffffffffffffffff")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, count, should.Equal(100))
	})
}
