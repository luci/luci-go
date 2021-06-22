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
	"go.chromium.org/luci/cv/internal/configs/prjcfg"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGobMapUpdateAndLookup(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)

	// First set up an example project with two config groups to show basic
	// regular usage; there is a "main" group which matches a main ref, and
	// another fallback group that matches many other refs, but not all.
	tc := prjcfg.TestController{}
	tc.Create(ctx, "chromium", &pb.Config{
		ConfigGroups: []*pb.ConfigGroup{
			{
				Name: "group_main",
				Gerrit: []*pb.ConfigGroup_Gerrit{
					{
						Url: "https://cr-review.gs.com/",
						Projects: []*pb.ConfigGroup_Gerrit_Project{
							{
								Name:      "cr/src",
								RefRegexp: []string{"refs/heads/main"},
							},
						},
					},
				},
			},
			{
				// This is the fallback group, so "refs/heads/main" should be
				// handled by the main group but not this one, even though it
				// matches the include regexp list.
				Name:     "group_other",
				Fallback: pb.Toggle_YES,
				Gerrit: []*pb.ConfigGroup_Gerrit{
					{
						Url: "https://cr-review.gs.com/",
						Projects: []*pb.ConfigGroup_Gerrit_Project{
							{
								Name:             "cr/src",
								RefRegexp:        []string{"refs/heads/.*"},
								RefRegexpExclude: []string{"refs/heads/123"},
							},
						},
					},
				},
			},
		},
	})

	update := func(lProject string) error {
		meta := tc.MustExist(ctx, lProject)
		cgs, err := meta.GetConfigGroups(ctx)
		if err != nil {
			panic(err)
		}
		return Update(ctx, &meta, cgs)
	}

	Convey("Update with nonexistent project stores nothing", t, func() {
		So(Update(ctx, &prjcfg.Meta{Project: "bogus", Status: prjcfg.StatusNotExists}, nil), ShouldBeNil)
		mps := []*mapPart{}
		q := datastore.NewQuery(mapKind)
		So(datastore.GetAll(ctx, q, &mps), ShouldBeNil)
		So(mps, ShouldBeEmpty)
	})

	Convey("Lookup nonexistent project returns empty result", t, func() {
		So(
			lookup(ctx, "foo-review.gs.com", "repo", "refs/heads/main"),
			ShouldBeEmpty)
	})

	Convey("Basic behavior with one project", t, func() {
		So(update("chromium"), ShouldBeNil)

		Convey("Lookup with main ref returns main group", func() {
			// Note that even though the other config group also matches,
			// only the main config group is applicable since the other one
			// is the fallback config group.
			So(
				lookup(ctx, "cr-review.gs.com", "cr/src", "refs/heads/main"),
				ShouldResemble,
				map[string][]string{
					"chromium": {"group_main"},
				})
		})

		Convey("Lookup with other ref returns other group", func() {
			// refs/heads/something matches other group, but not main group.
			So(
				lookup(ctx, "cr-review.gs.com", "cr/src", "refs/heads/something"),
				ShouldResemble,
				map[string][]string{
					"chromium": {"group_other"},
				})
		})

		Convey("Lookup excluded ref returns nothing", func() {
			// refs/heads/123 is specifically excluded from the "other" group,
			// and also not included in main group.
			So(
				lookup(ctx, "cr-review.gs.com", "cr/src", "refs/heads/123"),
				ShouldBeEmpty)
		})

		Convey("For a ref with no matching groups the result is empty", func() {
			// If a ref doesn't match any include patterns then no groups
			// match.
			So(
				lookup(ctx, "cr-review.gs.com", "cr/src", "refs/branch-heads/beta"),
				ShouldBeEmpty)
		})
	})

	Convey("Lookup again returns nothing for disabled project", t, func() {
		// Simulate deleting project. Projects that are deleted are first
		// disabled in practice.
		tc.Disable(ctx, "chromium")
		So(update("chromium"), ShouldBeNil)
		So(
			lookup(ctx, "cr-review.gs.com", "cr/src", "refs/heads/main"),
			ShouldBeEmpty)
	})

	Convey("With two matches and no fallback...", t, func() {
		// Simulate the project being updated so that the "other" group is no
		// longer a fallback group. Now some refs will match both groups.
		tc.Enable(ctx, "chromium")
		tc.Update(ctx, "chromium", &pb.Config{
			ConfigGroups: []*pb.ConfigGroup{
				{
					Name: "group_main",
					Gerrit: []*pb.ConfigGroup_Gerrit{
						{
							Url: "https://cr-review.gs.com/",
							Projects: []*pb.ConfigGroup_Gerrit_Project{
								{
									Name:      "cr/src",
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
							Url: "https://cr-review.gs.com/",
							Projects: []*pb.ConfigGroup_Gerrit_Project{
								{
									Name:             "cr/src",
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
			So(update("chromium"), ShouldBeNil)
			So(
				lookup(ctx, "cr-review.gs.com", "cr/src", "refs/heads/main"),
				ShouldResemble,
				map[string][]string{"chromium": {"group_main", "group_other"}})
		})
	})

	Convey("With two repos in main group and no other group...", t, func() {
		// This update includes both additions and removals,
		// and also tests multiple hosts.
		tc.Update(ctx, "chromium", &pb.Config{
			ConfigGroups: []*pb.ConfigGroup{
				{
					Name: "group_main",
					Gerrit: []*pb.ConfigGroup_Gerrit{
						{
							Url: "https://cr-review.gs.com/",
							Projects: []*pb.ConfigGroup_Gerrit_Project{
								{
									Name:      "cr/src",
									RefRegexp: []string{"refs/heads/main"},
								},
							},
						},
						{
							Url: "https://cr2-review.gs.com/",
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
		So(update("chromium"), ShouldBeNil)

		Convey("main group matches two different hosts", func() {

			So(
				lookup(ctx, "cr-review.gs.com", "cr/src", "refs/heads/main"),
				ShouldResemble,
				map[string][]string{"chromium": {"group_main"}})
			So(
				lookup(ctx, "cr2-review.gs.com", "cr2/src", "refs/heads/main"),
				ShouldResemble,
				map[string][]string{"chromium": {"group_main"}})
		})

		Convey("other group no longer exists", func() {
			So(
				lookup(ctx, "cr-review.gs.com", "cr/src", "refs/heads/something"),
				ShouldBeEmpty)
		})
	})

	Convey("With another project matching the same ref...", t, func() {
		// Below another project is created that watches the same repo and ref.
		// This tests multiple projects matching for one Lookup.
		tc.Create(ctx, "foo", &pb.Config{
			ConfigGroups: []*pb.ConfigGroup{
				{
					Name: "group_foo",
					Gerrit: []*pb.ConfigGroup_Gerrit{
						{
							Url: "https://cr-review.gs.com/",
							Projects: []*pb.ConfigGroup_Gerrit_Project{
								{
									Name:      "cr/src",
									RefRegexp: []string{"refs/heads/main"},
								},
							},
						},
					},
				},
			},
		})
		So(update("foo"), ShouldBeNil)

		Convey("main group matches two different projects", func() {
			So(
				lookup(ctx, "cr-review.gs.com", "cr/src", "refs/heads/main"),
				ShouldResemble,
				map[string][]string{
					"chromium": {"group_main"},
					"foo":      {"group_foo"},
				})
		})
	})
}

// lookup is a test helper function to return just the projects and config
// group names returned by Lookup.
func lookup(ctx context.Context, host, repo, ref string) map[string][]string {
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
