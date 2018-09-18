// Copyright 2018 The LUCI Authors.
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
	"strings"
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/grpc/grpcutil"

	api "go.chromium.org/luci/cipd/api/cipd/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDeletePackage(t *testing.T) {
	t.Parallel()

	// No other test is hiting DeletePackage, so its fine to change the global
	// variable here in this parallel test.
	deletionBatchSize = 3

	Convey("Works", t, func() {
		ctx := gaetesting.TestingContext()

		// Returns number of entities in an entity group.
		entitiesCount := func(root *datastore.Key) (count int64) {
			q := datastore.NewQuery("").Ancestor(root).KeysOnly(true)
			So(datastore.Run(ctx, q, func(k *datastore.Key) {
				// Skip magical __entity_group__ entities created by the datastore.
				// This is roughly same as using GetTestable(ctx).DisableSpecialEntities
				// except we can't disable them, since we need them for transactions to
				// work.
				if !strings.HasPrefix(k.Kind(), "__") {
					count += 1
				}
			}), ShouldBeNil)
			return
		}

		// This will create 20 entities to be deleted. For deletionBatchSize == 3
		// set above, it means we'll have 6 full batches and one incomplete final
		// batch, thus covering all important code paths.
		for _, chr := range []string{"a", "b", "c", "d"} {
			reg, inst, _ := RegisterInstance(ctx, &Instance{
				InstanceID: strings.Repeat(chr, 40),
				Package:    PackageKey(ctx, "pkg"),
			}, nil)
			So(reg, ShouldBeTrue)

			So(datastore.Put(ctx, &ProcessingResult{
				ProcID:   "zzz",
				Instance: datastore.KeyForObj(ctx, inst),
			}), ShouldBeNil)

			So(SetRef(ctx, chr+"-ref", inst, ""), ShouldBeNil)
			So(AttachTags(ctx, inst, []*api.Tag{
				{Key: "k1", Value: chr},
				{Key: "k2", Value: chr},
			}, ""), ShouldBeNil)
		}

		// Some unrelated instance in a different package to be left alone.
		reg, _, _ := RegisterInstance(ctx, &Instance{
			InstanceID: strings.Repeat("a", 40),
			Package:    PackageKey(ctx, "another-pkg"),
		}, nil)
		So(reg, ShouldBeTrue)

		// Before the deletion.
		So(entitiesCount(PackageKey(ctx, "pkg")), ShouldEqual, 21)
		So(entitiesCount(PackageKey(ctx, "another-pkg")), ShouldEqual, 2)

		So(DeletePackage(ctx, "pkg"), ShouldBeNil)

		// After the deletion we are left only with another-pkg.
		So(entitiesCount(PackageKey(ctx, "pkg")), ShouldEqual, 0)
		So(entitiesCount(PackageKey(ctx, "another-pkg")), ShouldEqual, 2)

		// And DeletePackage now complains that the package is gone.
		err := DeletePackage(ctx, "pkg")
		So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
	})
}
