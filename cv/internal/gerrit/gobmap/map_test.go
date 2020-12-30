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
	"strings"
	"testing"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	pb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap/internal"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGobMap(t *testing.T) {
	t.Parallel()

	Convey("Update and Lookup", t, func() {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		tc := config.TestController{}
		tc.Create(ctx, "chromium", &pb.Config{
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
					// This is the fallback group, so "refs/heads/main" should
					// be handled by the other group but not this one, even
					// though it matches the include regexp list.
					Name: "group_other",
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
		})

		Convey("Update nonexistent project does not store anything", func() {
			So(Update(ctx, "bogus"), ShouldBeNil)
			mps := []*mapPart{}
			q := datastore.NewQuery(mapKind)
			So(datastore.GetAll(ctx, q, &mps), ShouldBeNil)
			So(mps, ShouldBeEmpty)
		})

		// Helper function to return just the projects and config group
		// names returned by Lookup.
		lookup := func(ctx context.Context, host, repo, ref string) map[string][]string {
			ret := map[string][]string{}
			ac, err := Lookup(ctx, host, repo, ref)
			So(err, ShouldBeNil)
			for _, p := range ac.Projects {
				var names []string
				for _, id := range p.ConfigGroupIds {
					parts := strings.Split(id, "/")
					So(len(parts), ShouldEqual, 2)
					names = append(names, parts[1])
				}
				ret[p.Name] = names
			}
			return ret
		}

		Convey("Lookup nonexistent project returns empty ApplicableConfig", func() {
			So(
				lookup(ctx, "foo-review.googlesource.com",
					"repo", "refs/heads/main"),
				ShouldBeEmpty)
		})

		Convey("Update and Lookup existing project", func() {
			So(Update(ctx, "chromium"), ShouldBeNil)

			Convey("Lookup main ref", func() {
				// Note that even though the other config group also matches,
				// only the main config group is applicable since the other one
				// is the fallback config group.
				So(
					lookup(ctx, "chromium-review.googlesource.com",
						"chromium/src", "refs/heads/main"),
					ShouldResemble,
					map[string][]string{
						"chromium": []string{"group_main"},
					})
			})

			Convey("Lookup other ref", func() {
				// refs/heads/something matches other group, but
				// not main group.
				So(
					lookup(ctx, "chromium-review.googlesource.com",
						"chromium/src", "refs/heads/something"),
					ShouldResemble,
					map[string][]string{
						"chromium": []string{"group_other"},
					})
			})

			Convey("Lookup excluded ref", func() {
				// refs/heads/123 is specifically excluded from other group,
				// and also not included in main group.
				So(
					lookup(ctx, "chromium-review.googlesource.com",
						"chromium/src", "refs/heads/123"),
					ShouldBeEmpty)
			})

			Convey("Lookup ref with no matches", func() {
				// If a ref doesn't match any include patterns
				// then no groups match.
				So(
					lookup(ctx, "chromium-review.googlesource.com",
						"chromium/src", "refs/branch-heads/beta"),
					ShouldBeEmpty)
			})

		})

		Convey("Lookup again returns nothing for deleted project", func() {
			// Simulate deleting project.
			tc.Disable(ctx, "chromium")
			// Update gobmap;
			So(Update(ctx, "chromium"), ShouldBeNil)
			So(
				lookup(ctx, "chromium-review.googlesource.com",
					"chromium/src", "refs/heads/main"),
				ShouldBeEmpty)
		})

		// Simulate the project being updated so that the "other" group is no
		// longer a fallback group. No some refs will match both groups.
		tc.Enable(ctx, "chromium")
		tc.Update(ctx, "chromium", &pb.Config{
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
					Fallback: pb.Toggle_NO,
				},
			},
		})

		Convey("Lookup main ref matching two refs", func() {
			// This adds coverage for matching two groups.
			So(Update(ctx, "chromium"), ShouldBeNil)

			So(
				lookup(ctx, "chromium-review.googlesource.com",
					"chromium/src", "refs/heads/main"),
				ShouldResemble,
				map[string][]string{"chromium": []string{"group_main", "group_other"}})
		})

		//  Add a new host/repo to the main group and delete the "other" group.
		//  This adds both deletes & puts in an update, and also tests multiple hosts.
		tc.Update(ctx, "chromium", &pb.Config{
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
						{
							Url: "https://cr2-review.googlesource.com/",
							Projects: []*pb.ConfigGroup_Gerrit_Project{
								{
									Name:      "cr2/src",
									RefRegexp: []string{"refs/heads/main"},
								},
							},
						},
					},
				},
			},
		})

		// Below another project is created that watches the same repo
		// and ref. This tests multiple projects matching...
		// XXX finish this part
		tc.Create(ctx, "foo", &pb.Config{
			ConfigGroups: []*pb.ConfigGroup{
				{
					Name: "group_foo",
					Gerrit: []*pb.ConfigGroup_Gerrit{
						{
							Url: "https://foo-review.googlesource.com/",
							Projects: []*pb.ConfigGroup_Gerrit_Project{
								{
									Name:      "chromium/src",
									RefRegexp: []string{"refs/heads/main"},
								},
							},
						},
					},
				},
			},
		})
		//tc.Create(ctx, "foo", cfg)

	})

	// TODO(qyearsley): Instead of testing listUpdates, add more cases
	// using Update and Lookup above, for example:
	// Update/Lookup, e.g.:
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
