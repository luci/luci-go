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

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/luci_notify/buildbucket"
	"go.chromium.org/luci/luci_notify/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBuilderLookup(t *testing.T) {
	Convey(`Test Environment for Builder Lookups`, t, func() {
		c := memory.UseWithAppID(context.Background(), "dev~luci-notify")
		bucket := "bucket.bucket"
		oldestTime := testutil.Timestamp(2015, 1, 1, 1, 1, 1)
		middleTime := testutil.Timestamp(2016, 6, 6, 6, 6, 6)
		newestTime := testutil.Timestamp(2016, 12, 12, 12, 12, 12)

		checkLookup := func(build *buildbucket.BuildInfo, expectTime int64, expectResult string) {
			builder, err := LookupBuilder(c, build)
			So(err, ShouldBeNil)
			So(builder.ID, ShouldResemble, build.BuilderID())
			if expectTime != 0 {
				So(builder.LastBuildTime, ShouldEqual, expectTime)
			}
			So(builder.LastBuildResult, ShouldResemble, expectResult)
		}

		coldLookup := func(name string, time int64) *buildbucket.BuildInfo {
			build := testutil.CreateTestBuild(time, bucket, name, "SUCCESS")
			checkLookup(build, 0, "UNKNOWN")
			return build
		}

		Convey(`no builder found`, func() {
			coldLookup("not-there", oldestTime)
		})

		Convey(`builder found`, func() {
			build := testutil.CreateTestBuild(oldestTime, bucket, "there", "SUCCESS")
			datastore.Put(c, NewBuilder(build.BuilderID(), build))
			checkLookup(build, oldestTime, "SUCCESS")
		})

		Convey(`repeated builder lookup`, func() {
			build := coldLookup("maybe-there", oldestTime)
			build.Build.CreatedTs = middleTime
			checkLookup(build, oldestTime, "SUCCESS")
		})

		Convey(`out-of-order`, func() {
			build := coldLookup("out-of-order", middleTime)
			build.Build.CreatedTs = oldestTime
			checkLookup(build, middleTime, "SUCCESS")
			build.Build.CreatedTs = newestTime
			checkLookup(build, middleTime, "SUCCESS")
		})
	})
}
