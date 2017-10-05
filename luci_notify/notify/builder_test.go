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
	"go.chromium.org/luci/common/api/gitiles"

	"go.chromium.org/luci/luci_notify/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBuilderLookup(t *testing.T) {
	t.Parallel()

	Convey(`Test Environment for Builder Lookups`, t, func() {
		c := memory.UseWithAppID(context.Background(), "dev~luci-notify")
		bucket := "bucket.bucket"
		oldestTime := time.Date(2015, 1, 1, 1, 1, 1, 0, time.UTC)
		newestTime := time.Date(2016, 12, 12, 12, 12, 12, 0, time.UTC)
		noCommit := testutil.TestCommit(time.Time{}, "")
		firstCommit := testutil.TestCommit(oldestTime, testutil.TestRevision())
		lastCommit := testutil.TestCommit(newestTime, testutil.TestRevision())

		checkLookup := func(build *buildbucket.Build, commit *gitiles.Commit, expectCommit *gitiles.Commit, expectStatus buildbucket.Status) {
			builderID := getBuilderID(build)
			builder, err := LookupBuilder(c, builderID, build, commit)
			So(err, ShouldBeNil)
			So(builder.ID, ShouldResemble, builderID)
			So(builder.Status, ShouldResemble, expectStatus)
			So(builder.StatusTime, ShouldResemble, expectCommit.Committer.Time.Time)
			So(builder.StatusRevision, ShouldEqual, expectCommit.Commit)
		}

		coldLookup := func(builder string, commit *gitiles.Commit) *buildbucket.Build {
			build := testutil.TestBuild(bucket, builder, buildbucket.StatusSuccess)
			checkLookup(build, commit, noCommit, StatusUnknown)
			return build
		}

		Convey(`no builder found`, func() {
			coldLookup("not-there", firstCommit)
		})

		Convey(`builder found`, func() {
			build := testutil.TestBuild(bucket, "there", buildbucket.StatusSuccess)
			datastore.Put(c, NewBuilder(getBuilderID(build), build.Status, firstCommit))
			checkLookup(build, firstCommit, firstCommit, buildbucket.StatusSuccess)
		})

		Convey(`repeated builder lookup`, func() {
			build := coldLookup("maybe-there", firstCommit)
			build.Status = buildbucket.StatusFailure
			checkLookup(build, lastCommit, firstCommit, buildbucket.StatusSuccess)
			checkLookup(build, firstCommit, lastCommit, buildbucket.StatusFailure)
		})

		Convey(`out-of-order`, func() {
			build := coldLookup("out-of-order", lastCommit)
			checkLookup(build, firstCommit, lastCommit, buildbucket.StatusSuccess)
		})

		Convey(`same status`, func() {
			build := coldLookup("same", firstCommit)
			checkLookup(build, lastCommit, firstCommit, buildbucket.StatusSuccess)
			checkLookup(build, lastCommit, firstCommit, buildbucket.StatusSuccess)
		})
	})
}
