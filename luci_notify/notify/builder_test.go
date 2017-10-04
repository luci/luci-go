// Copyright 2017 The LUCI Authors.
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

package notify

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/luci_notify/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBuilderLookup(t *testing.T) {
	t.Parallel()

	Convey(`Test Environment for Builder Lookups`, t, func() {
		c := memory.UseWithAppID(context.Background(), "dev~luci-notify")
		bucket := "bucket.bucket"
		oldestTime := time.Date(2015, 1, 1, 1, 1, 1, 0, time.UTC)
		middleTime := time.Date(2016, 6, 6, 6, 6, 6, 0, time.UTC)
		newestTime := time.Date(2016, 12, 12, 12, 12, 12, 0, time.UTC)

		checkLookup := func(build *buildbucket.Build, expectTime time.Time, expectStatus buildbucket.Status) {
			builderID := getBuilderID(build)
			builder, err := LookupBuilder(c, builderID, build)
			So(err, ShouldBeNil)
			So(builder.ID, ShouldResemble, builderID)
			So(builder.Status, ShouldResemble, expectStatus)
			So(builder.StatusTime, ShouldEqual, expectTime)
		}

		coldLookup := func(builder string, buildTime time.Time) *buildbucket.Build {
			build := testutil.TestBuild(buildTime, bucket, builder, buildbucket.StatusSuccess)
			checkLookup(build, time.Time{}, StatusUnknown)
			return build
		}

		Convey(`no builder found`, func() {
			coldLookup("not-there", oldestTime)
		})

		Convey(`builder found`, func() {
			build := testutil.TestBuild(oldestTime, bucket, "there", buildbucket.StatusSuccess)
			datastore.Put(c, NewBuilder(getBuilderID(build), build))
			checkLookup(build, oldestTime, buildbucket.StatusSuccess)
		})

		Convey(`repeated builder lookup`, func() {
			build := coldLookup("maybe-there", oldestTime)
			build.CreationTime = middleTime
			checkLookup(build, oldestTime, buildbucket.StatusSuccess)
		})

		Convey(`out-of-order`, func() {
			build := coldLookup("out-of-order", middleTime)
			build.CreationTime = oldestTime
			checkLookup(build, middleTime, buildbucket.StatusSuccess)
			build.CreationTime = newestTime
			checkLookup(build, middleTime, buildbucket.StatusSuccess)
		})
	})
}
