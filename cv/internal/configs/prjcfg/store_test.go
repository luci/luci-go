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

package prjcfg

import (
	"context"
	"strings"
	"testing"

	gaememory "go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"

	. "github.com/smartystreets/goconvey/convey"
)

func TestComputeHash(t *testing.T) {
	t.Parallel()
	testCfg := &cfgpb.Config{
		CqStatusHost: "chromium-cq-status.appspot.com",
		ConfigGroups: []*cfgpb.ConfigGroup{
			{
				Name: "group_foo",
				Gerrit: []*cfgpb.ConfigGroup_Gerrit{
					{
						Url: "https://chromium-review.googlesource.com/",
						Projects: []*cfgpb.ConfigGroup_Gerrit_Project{
							{
								Name:      "chromium/src",
								RefRegexp: []string{"refs/heads/main"},
							},
						},
					},
				},
			},
		},
	}

	Convey("Compute Hash", t, func() {
		tokens := strings.Split(ComputeHash(testCfg), ":")
		So(tokens, ShouldHaveLength, 2)
		So(tokens[0], ShouldEqual, "sha256")
		So(tokens[1], ShouldHaveLength, 16)
	})
}

func TestGetAllProjectIDs(t *testing.T) {
	t.Parallel()
	Convey("Get Project IDs", t, func() {
		ctx := gaememory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		enabledPC := ProjectConfig{
			Project: "enabledProject",
			Enabled: true,
		}
		disabledPC := ProjectConfig{
			Project: "disabledProject",
			Enabled: false,
		}
		err := datastore.Put(ctx, &enabledPC, &disabledPC)
		So(err, ShouldBeNil)

		Convey("All", func() {
			ret, err := GetAllProjectIDs(ctx, false)
			So(err, ShouldBeNil)
			So(ret, ShouldResemble, []string{"disabledProject", "enabledProject"})
		})

		Convey("Enabled", func() {
			ret, err := GetAllProjectIDs(ctx, true)
			So(err, ShouldBeNil)
			So(ret, ShouldResemble, []string{"enabledProject"})
		})
	})
}

func TestMakeConfigGroupID(t *testing.T) {
	t.Parallel()
	Convey("Make ConfigGroupID", t, func() {
		id := MakeConfigGroupID("sha256:deadbeefdeadbeef", "foo")
		So(id, ShouldEqual, "sha256:deadbeefdeadbeef/foo")
	})
}

func TestConfigGroupProjectString(t *testing.T) {
	t.Parallel()

	Convey("ConfigGroup.ProjectString works", t, func() {
		ctx := gaememory.Use(context.Background())
		c := ConfigGroup{
			Project: ProjectConfigKey(ctx, "chromium"),
		}
		So(c.ProjectString(), ShouldEqual, "chromium")
	})
}
