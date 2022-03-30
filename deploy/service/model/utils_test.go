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

package model

import (
	"context"
	"testing"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/deploy/api/modelpb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestFetchAssets(t *testing.T) {
	t.Parallel()

	Convey("With entities", t, func() {
		ctx := memory.Use(context.Background())

		datastore.Put(ctx, []*Asset{
			{ID: "1", Asset: &modelpb.Asset{Id: "1"}},
			{ID: "3", Asset: &modelpb.Asset{Id: "3"}},
			{ID: "5", Asset: &modelpb.Asset{Id: "5"}},
		})

		Convey("shouldExist = false", func() {
			assets, err := fetchAssets(ctx, []string{"1", "2", "3", "4", "5"}, false)
			So(err, ShouldBeNil)
			So(assets, ShouldHaveLength, 5)
			for assetID, ent := range assets {
				So(ent.ID, ShouldEqual, assetID)
				So(ent.Asset.Id, ShouldEqual, assetID)
			}
		})

		Convey("shouldExist = true", func() {
			_, err := fetchAssets(ctx, []string{"1", "2", "3", "4", "5"}, true)
			So(err, ShouldErrLike, "assets entities unexpectedly missing: 2, 4")
		})

		Convey("One missing", func() {
			Convey("shouldExist = false", func() {
				assets, err := fetchAssets(ctx, []string{"missing"}, false)
				So(err, ShouldBeNil)
				So(assets, ShouldHaveLength, 1)
				So(assets["missing"].ID, ShouldEqual, "missing")
				So(assets["missing"].Asset.Id, ShouldEqual, "missing")
			})

			Convey("shouldExist = true", func() {
				_, err := fetchAssets(ctx, []string{"missing"}, true)
				So(err, ShouldErrLike, "assets entities unexpectedly missing: missing")
			})
		})
	})
}
