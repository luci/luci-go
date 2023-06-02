// Copyright 2023 The LUCI Authors.
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

package model

import (
	"context"
	"testing"

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestModel(t *testing.T) {
	t.Parallel()

	Convey("GetLatestConfigFile", t, func() {
		ctx := memory.UseWithAppID(context.Background(), "dev~app-id")
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		cs := &ConfigSet{
			ID:             config.MustServiceSet("service"),
			LatestRevision: RevisionInfo{ID: "latest"},
		}
		So(datastore.Put(ctx, cs), ShouldBeNil)

		Convey("ok", func() {
			latest := &File{
				Path:     "file",
				Revision: datastore.MakeKey(ctx, ConfigSetKind, "services/service", RevisionKind, "latest"),
				Content:  []byte("latest"),
			}
			stale := &File{
				Path:     "file",
				Revision: datastore.MakeKey(ctx, ConfigSetKind, "services/service", RevisionKind, "stale"),
				Content:  []byte("stale"),
			}
			So(datastore.Put(ctx, latest, stale), ShouldBeNil)
			actual, err := GetLatestConfigFile(ctx, config.MustServiceSet("service"), "file")
			So(err, ShouldBeNil)
			So(actual, ShouldResemble, latest)
		})

		Convey("error", func() {
			Convey("configset not exist", func() {
				_, err := GetLatestConfigFile(ctx, config.MustServiceSet("nonexist"), "file")
				So(err, ShouldErrLike, `failed to fetch ConfigSet "services/nonexist"`)
			})

			Convey("file not exist", func() {
				_, err := GetLatestConfigFile(ctx, config.MustServiceSet("service"), "file")
				So(err, ShouldErrLike, `failed to fetch file "services/service" for config set "file"`)
			})
		})
	})
}
