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
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/gerrit"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestPopulateOwnerKinds(t *testing.T) {
	Convey("PopulateOwnerKinds", t, func() {
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

		Convey("With human CL author", func() {
			augmentedSources, err := PopulateOwnerKinds(ctx, "testproject", sources)
			So(err, ShouldBeNil)
			So(augmentedSources, ShouldResembleProto, expectedSources)

			// Verify cache record was created.
			cls, err := ReadAll(span.Single(ctx))
			So(err, ShouldBeNil)
			So(removeCreationTimestamps(cls), ShouldResemble, expectedChangelists)
		})
		Convey("With automation CL author", func() {
			clsByHost["chromium-review.googlesource.com"][0].Owner.Email = "robot@chromium.gserviceaccount.com"

			augmentedSources, err := PopulateOwnerKinds(ctx, "testproject", sources)
			So(err, ShouldBeNil)
			expectedSources["sources-1"].Changelists[0].OwnerKind = pb.ChangelistOwnerKind_AUTOMATION
			So(augmentedSources, ShouldResembleProto, expectedSources)

			// Verify cache record was created.
			cls, err := ReadAll(span.Single(ctx))
			So(err, ShouldBeNil)
			expectedChangelists[0].OwnerKind = pb.ChangelistOwnerKind_AUTOMATION
			So(removeCreationTimestamps(cls), ShouldResemble, expectedChangelists)
		})
		Convey("With CL not existing", func() {
			clsByHost["chromium-review.googlesource.com"] = []*gerritpb.ChangeInfo{}

			augmentedSources, err := PopulateOwnerKinds(ctx, "testproject", sources)
			So(err, ShouldBeNil)
			expectedSources["sources-1"].Changelists[0].OwnerKind = pb.ChangelistOwnerKind_CHANGELIST_OWNER_UNSPECIFIED
			So(augmentedSources, ShouldResembleProto, expectedSources)

			// Verify cache record was created.
			cls, err := ReadAll(span.Single(ctx))
			So(err, ShouldBeNil)
			expectedChangelists[0].OwnerKind = pb.ChangelistOwnerKind_CHANGELIST_OWNER_UNSPECIFIED
			So(removeCreationTimestamps(cls), ShouldResemble, expectedChangelists)
		})
		Convey("With cache record", func() {
			// Create a cache record that says the CL was created by automation.
			expectedChangelists[0].OwnerKind = pb.ChangelistOwnerKind_AUTOMATION
			So(SetGerritChangelistsForTesting(ctx, expectedChangelists), ShouldBeNil)

			augmentedSources, err := PopulateOwnerKinds(ctx, "testproject", sources)
			So(err, ShouldBeNil)
			expectedSources["sources-1"].Changelists[0].OwnerKind = pb.ChangelistOwnerKind_AUTOMATION
			So(augmentedSources, ShouldResembleProto, expectedSources)

			// Verify cache remains in tact.
			cls, err := ReadAll(span.Single(ctx))
			So(err, ShouldBeNil)
			So(removeCreationTimestamps(cls), ShouldResemble, expectedChangelists)
		})
		Convey("With no changelists", func() {
			sources["sources-1"].Changelists = nil

			augmentedSources, err := PopulateOwnerKinds(ctx, "testproject", sources)
			So(err, ShouldBeNil)
			expectedSources["sources-1"].Changelists = nil
			So(augmentedSources, ShouldResembleProto, expectedSources)

			// Verify no cache entries created.
			cls, err := ReadAll(span.Single(ctx))
			So(err, ShouldBeNil)
			So(cls, ShouldBeNil)
		})
		Convey("With multiple changelists", func() {
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

			Convey("Without cache entries", func() {
				augmentedSources, err := PopulateOwnerKinds(ctx, "testproject", sources)
				So(err, ShouldBeNil)
				So(augmentedSources, ShouldResembleProto, expectedSources)

				// Verify cache record was created.
				cls, err := ReadAll(span.Single(ctx))
				So(err, ShouldBeNil)
				So(removeCreationTimestamps(cls), ShouldResemble, expectedChangelists)
			})
			Convey("With cache entries", func() {
				So(SetGerritChangelistsForTesting(ctx, expectedChangelists), ShouldBeNil)
				// Remove CLs from gerrit to verify we did infact use the cache.
				clsByHost["chromium-review.googlesource.com"] = nil

				augmentedSources, err := PopulateOwnerKinds(ctx, "testproject", sources)
				So(err, ShouldBeNil)
				So(augmentedSources, ShouldResembleProto, expectedSources)

				// Verify cache records remain in tact.
				cls, err := ReadAll(span.Single(ctx))
				So(err, ShouldBeNil)
				So(removeCreationTimestamps(cls), ShouldResemble, expectedChangelists)
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
