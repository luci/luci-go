// Copyright 2020 The LUCI Authors.
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

package gobmap

import (
	"context"
	"testing"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	pb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap/internal"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGobMap(t *testing.T) {
	t.Parallel()

	Convey("Update and Lookup", t, func() {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		// Example config used in tests below.
		var cfg = &pb.Config{
			// Note: Config also has other fields which are excluded here
			// because only ConfiGroups matter for this functionality.
			ConfigGroups: []*pb.ConfigGroup{
				{
					Name: "group_main",
					Gerrit: []*pb.ConfigGroup_Gerrit{
						{
							Url: "https://chromium-review.googlesource.com/",
							Projects: []*pb.ConfigGroup_Gerrit_Project{
								{
									Name:      "chromium/src",
									RefRegexp: []string{"refs/heads/main"},
								},
							},
						},
					},
				},
				{
					Name: "group_other",
					// This is the fallback group, so "refs/heads/main" should
					// be handled by the other group but not this one, even
					// though it matches the include regexp list.
					Gerrit: []*pb.ConfigGroup_Gerrit{
						{
							Url: "https://chromium-review.googlesource.com/",
							Projects: []*pb.ConfigGroup_Gerrit_Project{
								{
									Name:             "chromium/src",
									RefRegexp:        []string{"refs/heads/.*"},
									RefRegexpExclude: []string{"refs/heads/123"},
								},
							},
						},
					},
					Fallback: pb.Toggle_YES,
				},
			},
		}

		tc := config.TestController{}
		tc.Create(ctx, "chromium", cfg)

		Convey("Update nonexistent project does not store anything", func() {
			err := Update(ctx, "bogus")
			So(err, ShouldBeNil)
			mps := []*mapPart{}
			q := datastore.NewQuery(mapKind)
			So(datastore.GetAll(ctx, q, &mps), ShouldBeNil)
			So(mps, ShouldBeEmpty)
		})

		Convey("Lookup nonexistent project returns empty ApplicableConfig", func() {
			ac, err := Lookup(ctx, "foo-review.googlesource.com", "repo", "refs/heads/main")
			So(err, ShouldBeNil)
			So(ac.Projects, ShouldBeEmpty)
		})

		Convey("Update and Lookup existing project", func() {
			err := Update(ctx, "chromium")
			So(err, ShouldBeNil)

			// TODO(qyearsley): Consider adding a helper function to reduce
			// boilerplate. Also make a helper function to wrap Lookup
			// so that we don't need to assert the hashes.

			Convey("Lookup main ref", func() {
				ac, err := Lookup(ctx, "chromium-review.googlesource.com", "chromium/src", "refs/heads/main")
				So(err, ShouldBeNil)
				// Note that even though the other config group also matches,
				// only the main ref is applicable since the other one is the
				// fallback config group.
				So(ac.Projects, ShouldHaveLength, 1)
				So(ac.Projects[0], ShouldResembleProto,
					&changelist.ApplicableConfig_Project{
						Name:           "chromium",
						ConfigGroupIds: []string{"sha256:a5bf9aeca30c0177/group_main"},
					})
			})

			Convey("Lookup other ref", func() {
				ac, err := Lookup(ctx, "chromium-review.googlesource.com", "chromium/src", "refs/heads/something")
				So(err, ShouldBeNil)
				So(ac.Projects, ShouldHaveLength, 1)
				So(ac.Projects[0], ShouldResembleProto,
					&changelist.ApplicableConfig_Project{
						Name:           "chromium",
						ConfigGroupIds: []string{"sha256:a5bf9aeca30c0177/group_other"},
					})
			})

			Convey("Lookup excluded ref", func() {
				ac, err := Lookup(ctx, "chromium-review.googlesource.com", "chromium/src", "refs/heads/123")
				So(err, ShouldBeNil)
				So(ac.Projects, ShouldBeEmpty)
			})

			Convey("Lookup ref with no matches", func() {
				ac, err := Lookup(ctx, "chromium-review.googlesource.com", "chromium/src", "refs/branch-heads/beta")
				So(err, ShouldBeNil)
				So(ac.Projects, ShouldBeEmpty)
			})

			tc.Disable(ctx, "chromium")
			err = Update(ctx, "chromium")
			So(err, ShouldBeNil)

			Convey("Lookup again returns nothing for deleted project", func() {
				ac, err := Lookup(ctx, "chromium-review.googlesource.com", "chromium/src", "refs/heads/main")
				So(err, ShouldBeNil)
				So(ac.Projects, ShouldBeEmpty)
			})
		})
	})

	// TODO(qyearsley): Instead of testing listUpdates, add more cases
	// using Update and Lookup above, for example:
	// Update/Lookup, e.g.:
	//  1. Re-enable the project by updating fallback group name to no longer be
	//  fallback. This also adds coverage for matching 2 groups, which I think
	//  is missing right now.
	//  2. Add new host/repo to the main group and delete ex-fallback group.
	//  This adds both deletes & puts in an update, and also tests multiple hosts.
	//  3. Add new LUCI project watching the previously deleted fallback group;
	//  we need coverage for >1 LUCI project.
	Convey("listUpdates", t, func() {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		// Sample MapPart entities and ConfigGroups to use in the test below.
		mps := []*mapPart{
			&mapPart{
				ID:      "foo",
				Project: "foo",
				Parent:  datastore.MakeKey(ctx, parentKind, "foo-review.googlesource.com/repo/one"),
				Groups: &internal.Groups{
					Groups: []*internal.Group{
						&internal.Group{
							Id:      "sha256:c0ffee/main",
							Include: []string{"refs/heads/main"},
						},
						&internal.Group{
							Id:       "sha256:c0ffee/other",
							Include:  []string{"refs/heads/.*"},
							Fallback: true,
						},
					},
				},
				ConfigHash: "c0ffee",
			},
			&mapPart{
				ID:      "foo",
				Project: "foo",
				Parent:  datastore.MakeKey(ctx, parentKind, "foo-review.googlesource.com/repo/two"),
				Groups: &internal.Groups{
					Groups: []*internal.Group{
						&internal.Group{
							Id:      "sha256:c0ffee/main",
							Include: []string{"refs/heads/main"},
						},
					},
				},
				ConfigHash: "c0ffee",
			},
		}

		cgs := []*config.ConfigGroup{
			&config.ConfigGroup{
				ID: "sha256:c0ffee/main",
				Content: &pb.ConfigGroup{
					Name: "main",
					Gerrit: []*pb.ConfigGroup_Gerrit{
						&pb.ConfigGroup_Gerrit{
							Url: "https://foo-review.googlesource.com/",
							Projects: []*pb.ConfigGroup_Gerrit_Project{
								&pb.ConfigGroup_Gerrit_Project{
									Name:      "repo/one",
									RefRegexp: []string{"refs/heads/main"},
								},
							},
						},
					},
				},
			},
			&config.ConfigGroup{
				ID: "sha256:c0ffee/other",
				Content: &pb.ConfigGroup{
					Name: "other",
					Gerrit: []*pb.ConfigGroup_Gerrit{
						&pb.ConfigGroup_Gerrit{
							Url: "https://foo-review.googlesource.com/",
							Projects: []*pb.ConfigGroup_Gerrit_Project{
								&pb.ConfigGroup_Gerrit_Project{
									Name:      "repo/one",
									RefRegexp: []string{"refs/heads/.*"},
								},
							},
						},
					},
					Fallback: pb.Toggle_YES,
				},
			},
			&config.ConfigGroup{
				ID: "sha256:c0ffee/maintwo",
				Content: &pb.ConfigGroup{
					Name: "maintwo",
					Gerrit: []*pb.ConfigGroup_Gerrit{
						&pb.ConfigGroup_Gerrit{
							Url: "https://foo-review.googlesource.com/",
							Projects: []*pb.ConfigGroup_Gerrit_Project{
								&pb.ConfigGroup_Gerrit_Project{
									Name:      "repo/two",
									RefRegexp: []string{"refs/heads/main"},
								},
							},
						},
					},
				},
			},
		}

		Convey("deletes all 2 GWMs if latest config is empty", func() {
			put, delete := listUpdates(ctx, mps, nil, "c0ffee", "foo")
			So(put, ShouldHaveLength, 0)
			So(delete, ShouldHaveLength, 2)
		})

		Convey("puts all if stored GWMs is empty", func() {
			put, delete := listUpdates(ctx, nil, cgs, "c0ffee", "foo")
			So(put, ShouldHaveLength, 2)
			So(delete, ShouldHaveLength, 0)
		})

		Convey("Changes nothing if hash matches", func() {
			put, delete := listUpdates(ctx, mps, cgs, "c0ffee", "foo")
			So(put, ShouldHaveLength, 0)
			So(delete, ShouldHaveLength, 0)
		})

		Convey("Updates if groups exist but hash is different", func() {
			put, delete := listUpdates(ctx, mps, cgs, "newhash", "foo")
			So(put, ShouldHaveLength, 2)
			So(delete, ShouldHaveLength, 0)
		})
	})
}
