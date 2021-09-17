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
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	gaememory "go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	_ "go.chromium.org/luci/gae/service/datastore/crbug1242998safeget"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var testCfg = &cfgpb.Config{
	DrainingStartTime: "2014-05-11T14:37:57Z",
	SubmitOptions: &cfgpb.SubmitOptions{
		MaxBurst:   50,
		BurstDelay: durationpb.New(2 * time.Second),
	},
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

func TestComputeHash(t *testing.T) {
	t.Parallel()
	Convey("Compute Hash", t, func() {
		tokens := strings.Split(computeHash(testCfg), ":")
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
		Convey("Name specified", func() {
			id := makeConfigGroupID("sha256:deadbeefdeadbeef", "foo", 123)
			So(id, ShouldEqual, "sha256:deadbeefdeadbeef/foo")
		})
		Convey("Name missing", func() {
			id := makeConfigGroupID("sha256:deadbeefdeadbeef", "", 123)
			So(id, ShouldEqual, "sha256:deadbeefdeadbeef/index#123")
		})
	})
}

func TestConfigGroupProjectString(t *testing.T) {
	t.Parallel()

	Convey("ConfigGroup.ProjectString works", t, func() {
		ctx := gaememory.Use(context.Background())
		c := ConfigGroup{
			Project: datastore.MakeKey(ctx, projectConfigKind, "chromium"),
		}
		So(c.ProjectString(), ShouldEqual, "chromium")
	})
}

type readOnlyFilter struct{ datastore.RawInterface }

func (f readOnlyFilter) PutMulti(keys []*datastore.Key, vals []datastore.PropertyMap, cb datastore.NewKeyCB) error {
	panic("write is not supported")
}

func TestPutConfigGroups(t *testing.T) {
	t.Parallel()
	Convey("PutConfigGroups", t, func() {
		ctx := gaememory.Use(context.Background())
		Convey("New Configs", func() {
			hash := computeHash(testCfg)
			err := putConfigGroups(ctx, testCfg, "chromium", hash)
			So(err, ShouldBeNil)
			stored := ConfigGroup{
				ID:      makeConfigGroupID(hash, "group_foo", 0),
				Project: datastore.MakeKey(ctx, projectConfigKind, "chromium"),
			}
			err = datastore.Get(ctx, &stored)
			So(err, ShouldBeNil)
			So(stored.DrainingStartTime, ShouldEqual, testCfg.GetDrainingStartTime())
			So(stored.SubmitOptions, ShouldResembleProto, testCfg.GetSubmitOptions())
			So(stored.Content, ShouldResembleProto, testCfg.GetConfigGroups()[0])

			Convey("Skip if already exists", func() {
				ctx := datastore.AddRawFilters(ctx, func(_ context.Context, rds datastore.RawInterface) datastore.RawInterface {
					return readOnlyFilter{rds}
				})
				err := putConfigGroups(ctx, testCfg, "chromium", computeHash(testCfg))
				So(err, ShouldBeNil)
			})
		})
	})
}
