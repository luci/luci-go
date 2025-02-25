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

package gerritchangelists

import (
	"testing"
	"time"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/gerrit"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestPopulateOwnerKinds(t *testing.T) {
	ftt.Run("PopulateOwnerKinds", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)

		clsByHost := make(map[string][]*gerritpb.ChangeInfo)
		clsByHost["chromium-review.googlesource.com"] = []*gerritpb.ChangeInfo{
			{
				Number: 111,
				Owner: &gerritpb.AccountInfo{
					Email: "username@chromium.org",
				},
				Project: "chromium/src",
			},
		}

		ctx = gerrit.UseFakeClient(ctx, clsByHost)

		sources := make(map[string]*rdbpb.Sources)
		sources["sources-1"] = &rdbpb.Sources{
			GitilesCommit: &rdbpb.GitilesCommit{
				Host:       "chromium.googlesource.com",
				Project:    "chromium/src",
				Ref:        "refs/heads/main",
				CommitHash: "aa11bb22cc33dd44ee55aa11bb22cc33dd44ee55",
				Position:   123,
			},
			Changelists: []*rdbpb.GerritChange{
				{
					Host:     "chromium-review.googlesource.com",
					Project:  "chromium/src",
					Change:   111,
					Patchset: 3,
				},
			},
		}

		expectedSources := make(map[string]*pb.Sources)
		expectedSources["sources-1"] = pbutil.SourcesFromResultDB(sources["sources-1"])
		expectedSources["sources-1"].Changelists[0].OwnerKind = pb.ChangelistOwnerKind_HUMAN

		expectedChangelists := []*GerritChangelist{
			{
				Project:   "testproject",
				Host:      "chromium-review.googlesource.com",
				Change:    111,
				OwnerKind: pb.ChangelistOwnerKind_HUMAN,
			},
		}

		t.Run("With human CL author", func(t *ftt.Test) {
			augmentedSources, err := PopulateOwnerKindsBatch(ctx, "testproject", sources)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, augmentedSources, should.Match(expectedSources))

			// Verify cache record was created.
			cls, err := ReadAll(span.Single(ctx))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, removeCreationTimestamps(cls), should.Match(expectedChangelists))
		})
		t.Run("With automation CL author", func(t *ftt.Test) {
			clsByHost["chromium-review.googlesource.com"][0].Owner.Email = "robot@chromium.gserviceaccount.com"

			augmentedSources, err := PopulateOwnerKindsBatch(ctx, "testproject", sources)
			assert.Loosely(t, err, should.BeNil)
			expectedSources["sources-1"].Changelists[0].OwnerKind = pb.ChangelistOwnerKind_AUTOMATION
			assert.Loosely(t, augmentedSources, should.Match(expectedSources))

			// Verify cache record was created.
			cls, err := ReadAll(span.Single(ctx))
			assert.Loosely(t, err, should.BeNil)
			expectedChangelists[0].OwnerKind = pb.ChangelistOwnerKind_AUTOMATION
			assert.Loosely(t, removeCreationTimestamps(cls), should.Match(expectedChangelists))
		})
		t.Run("With CL not existing", func(t *ftt.Test) {
			clsByHost["chromium-review.googlesource.com"] = []*gerritpb.ChangeInfo{}

			augmentedSources, err := PopulateOwnerKindsBatch(ctx, "testproject", sources)
			assert.Loosely(t, err, should.BeNil)
			expectedSources["sources-1"].Changelists[0].OwnerKind = pb.ChangelistOwnerKind_CHANGELIST_OWNER_UNSPECIFIED
			assert.Loosely(t, augmentedSources, should.Match(expectedSources))

			// Verify cache record was created.
			cls, err := ReadAll(span.Single(ctx))
			assert.Loosely(t, err, should.BeNil)
			expectedChangelists[0].OwnerKind = pb.ChangelistOwnerKind_CHANGELIST_OWNER_UNSPECIFIED
			assert.Loosely(t, removeCreationTimestamps(cls), should.Match(expectedChangelists))
		})
		t.Run("With cache record", func(t *ftt.Test) {
			// Create a cache record that says the CL was created by automation.
			expectedChangelists[0].OwnerKind = pb.ChangelistOwnerKind_AUTOMATION
			assert.Loosely(t, SetGerritChangelistsForTesting(ctx, t, expectedChangelists), should.BeNil)

			augmentedSources, err := PopulateOwnerKindsBatch(ctx, "testproject", sources)
			assert.Loosely(t, err, should.BeNil)
			expectedSources["sources-1"].Changelists[0].OwnerKind = pb.ChangelistOwnerKind_AUTOMATION
			assert.Loosely(t, augmentedSources, should.Match(expectedSources))

			// Verify cache remains in tact.
			cls, err := ReadAll(span.Single(ctx))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, removeCreationTimestamps(cls), should.Match(expectedChangelists))
		})
		t.Run("With no changelists", func(t *ftt.Test) {
			sources["sources-1"].Changelists = nil

			augmentedSources, err := PopulateOwnerKindsBatch(ctx, "testproject", sources)
			assert.Loosely(t, err, should.BeNil)
			expectedSources["sources-1"].Changelists = nil
			assert.Loosely(t, augmentedSources, should.Match(expectedSources))

			// Verify no cache entries created.
			cls, err := ReadAll(span.Single(ctx))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cls, should.BeNil)
		})
		t.Run("With multiple changelists", func(t *ftt.Test) {
			sources := make(map[string]*rdbpb.Sources)
			sources["sources-1"] = &rdbpb.Sources{
				GitilesCommit: &rdbpb.GitilesCommit{
					Host:       "chromium.googlesource.com",
					Project:    "chromium/src",
					Ref:        "refs/heads/main",
					CommitHash: "aa11bb22cc33dd44ee55aa11bb22cc33dd44ee55",
					Position:   123,
				},
				Changelists: []*rdbpb.GerritChange{
					{
						Host:     "chromium-review.googlesource.com",
						Project:  "chromium/src",
						Change:   111,
						Patchset: 3,
					},
					{
						Host:     "chromium-review.googlesource.com",
						Project:  "chromium/src",
						Change:   222,
						Patchset: 3,
					},
				},
			}
			sources["sources-2"] = &rdbpb.Sources{
				GitilesCommit: &rdbpb.GitilesCommit{
					Host:       "chromium.googlesource.com",
					Project:    "chromium/src",
					Ref:        "refs/heads/main",
					CommitHash: "bb22cc33dd44dd44ee55aa11bb22cc33dd44ee55",
					Position:   456,
				},
				Changelists: []*rdbpb.GerritChange{
					{
						Host:     "chromium-review.googlesource.com",
						Project:  "chromium/src",
						Change:   111,
						Patchset: 3,
					},
					{
						Host:     "chromium-review.googlesource.com",
						Project:  "chromium/src",
						Change:   333,
						Patchset: 3,
					},
					{
						Host:     "chromium-review.googlesource.com",
						Project:  "chromium/special",
						Change:   444,
						Patchset: 3,
					},
				},
			}

			expectedSources := make(map[string]*pb.Sources)
			expectedSources["sources-1"] = pbutil.SourcesFromResultDB(sources["sources-1"])
			expectedSources["sources-1"].Changelists[0].OwnerKind = pb.ChangelistOwnerKind_HUMAN
			expectedSources["sources-1"].Changelists[1].OwnerKind = pb.ChangelistOwnerKind_AUTOMATION
			expectedSources["sources-2"] = pbutil.SourcesFromResultDB(sources["sources-2"])
			expectedSources["sources-2"].Changelists[0].OwnerKind = pb.ChangelistOwnerKind_HUMAN
			expectedSources["sources-2"].Changelists[1].OwnerKind = pb.ChangelistOwnerKind_CHANGELIST_OWNER_UNSPECIFIED
			expectedSources["sources-2"].Changelists[2].OwnerKind = pb.ChangelistOwnerKind_HUMAN

			clsByHost["chromium-review.googlesource.com"] = []*gerritpb.ChangeInfo{
				{
					Number: 111,
					Owner: &gerritpb.AccountInfo{
						Email: "username@chromium.org",
					},
					Project: "chromium/src",
				},
				{
					Number: 222,
					Owner: &gerritpb.AccountInfo{
						Email: "robot@chromium.gserviceaccount.com",
					},
					Project: "chromium/src",
				},
				// 333 does not exist.
				{
					Number: 444,
					Owner: &gerritpb.AccountInfo{
						Email: "username@chromium.org",
					},
					Project: "chromium/special",
				},
			}

			expectedChangelists := []*GerritChangelist{
				{
					Project:   "testproject",
					Host:      "chromium-review.googlesource.com",
					Change:    111,
					OwnerKind: pb.ChangelistOwnerKind_HUMAN,
				},
				{
					Project:   "testproject",
					Host:      "chromium-review.googlesource.com",
					Change:    222,
					OwnerKind: pb.ChangelistOwnerKind_AUTOMATION,
				},
				{
					Project:   "testproject",
					Host:      "chromium-review.googlesource.com",
					Change:    333,
					OwnerKind: pb.ChangelistOwnerKind_CHANGELIST_OWNER_UNSPECIFIED,
				},
				{
					Project:   "testproject",
					Host:      "chromium-review.googlesource.com",
					Change:    444,
					OwnerKind: pb.ChangelistOwnerKind_HUMAN,
				},
			}

			t.Run("Without cache entries", func(t *ftt.Test) {
				augmentedSources, err := PopulateOwnerKindsBatch(ctx, "testproject", sources)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, augmentedSources, should.Match(expectedSources))

				// Verify cache record was created.
				cls, err := ReadAll(span.Single(ctx))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, removeCreationTimestamps(cls), should.Match(expectedChangelists))
			})
			t.Run("With cache entries", func(t *ftt.Test) {
				assert.Loosely(t, SetGerritChangelistsForTesting(ctx, t, expectedChangelists), should.BeNil)
				// Remove CLs from gerrit to verify we did infact use the cache.
				clsByHost["chromium-review.googlesource.com"] = nil

				augmentedSources, err := PopulateOwnerKindsBatch(ctx, "testproject", sources)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, augmentedSources, should.Match(expectedSources))

				// Verify cache records remain in tact.
				cls, err := ReadAll(span.Single(ctx))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, removeCreationTimestamps(cls), should.Match(expectedChangelists))
			})
		})
	})
}

func removeCreationTimestamps(cls []*GerritChangelist) []*GerritChangelist {
	var results []*GerritChangelist
	for _, cl := range cls {
		copy := *cl
		copy.CreationTime = time.Time{}
		results = append(results, &copy)
	}
	return results
}
