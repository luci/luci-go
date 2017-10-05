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
	"bytes"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/user"
	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging/memlogger"

	"go.chromium.org/luci/luci_notify/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

var (
	rev1        = testutil.TestRevision("some random data here")
	rev2        = testutil.TestRevision("other random string")
	testCommits = []gitiles.Commit{
		{Commit: rev1},
		{Commit: rev2},
	}
)

func pubsubDummyBuild(builder string, status buildbucket.Status, creationTime time.Time, revision string) *buildbucket.Build {
	build := testutil.TestBuild("test", "hello", builder, status)
	build.BuildSets = []buildbucket.BuildSet{buildbucket.BuildSet(&buildbucket.GitilesCommit{
		Host:     "test.googlesource.com",
		Project:  "test",
		Revision: revision,
	})}
	build.CreationTime = creationTime
	return build
}

func TestHandleBuild(t *testing.T) {
	t.Parallel()

	Convey(`Test Environment for handleBuild`, t, func() {
		cfgName := "basic"
		cfg, err := testutil.LoadProjectConfig(cfgName)
		So(err, ShouldBeNil)

		c := memory.UseWithAppID(context.Background(), "dev~luci-notify")
		c = clock.Set(c, testclock.New(time.Now()))
		c = memlogger.Use(c)
		user.GetTestable(c).Login("noreply@luci-notify-dev.appspotmail.com", "", false)

		// Add Notifiers to datastore and update indexes.
		notifiers := extractNotifiers(c, "test", cfg)
		for _, n := range notifiers {
			datastore.Put(c, n)
		}
		datastore.GetTestable(c).CatchupIndexes()

		oldTime := time.Date(2015, 2, 3, 12, 54, 3, 0, time.UTC)
		newTime := time.Date(2015, 2, 3, 12, 58, 7, 0, time.UTC)

		history := testutil.NewMockHistoryFunc(testCommits)

		dispatcher, taskqueue := createMockTaskQueue(c)

		testSuccess := func(build *buildbucket.Build, emailExpect ...string) {
			// Test handleBuild.
			err := handleBuild(c, dispatcher, build, history)
			So(err, ShouldBeNil)

			// Verify sent messages.
			verifyTasksAndMessages(c, taskqueue, emailExpect)
		}

		verifyBuilder := func(build *buildbucket.Build, revision string) {
			datastore.GetTestable(c).CatchupIndexes()
			id := getBuilderID(build)
			builder := Builder{ID: id}
			So(datastore.Get(c, &builder), ShouldBeNil)
			So(builder.StatusRevision, ShouldResemble, revision)
			So(builder.Status, ShouldEqual, build.Status)
		}

		grepLog := func(substring string) {
			buf := new(bytes.Buffer)
			_, err := memlogger.Dump(c, buf)
			So(err, ShouldBeNil)
			So(strings.Contains(buf.String(), substring), ShouldEqual, true)
		}

		Convey(`no config`, func() {
			build := pubsubDummyBuild("not-a-builder", buildbucket.StatusFailure, oldTime, rev1)
			testSuccess(build)
			grepLog("configuration")
		})

		Convey(`no revision`, func() {
			build := testutil.TestBuild("test", "hello", "test-builder-1", buildbucket.StatusSuccess)
			testSuccess(build)
			grepLog("revision")
		})

		Convey(`init builder`, func() {
			build := pubsubDummyBuild("test-builder-1", buildbucket.StatusFailure, oldTime, rev1)
			testSuccess(build, "test-example-failure@google.com")
			verifyBuilder(build, rev1)
		})

		Convey(`out-of-order revision`, func() {
			build := pubsubDummyBuild("test-builder-2", buildbucket.StatusSuccess, oldTime, rev2)
			testSuccess(build, "test-example-success@google.com")
			verifyBuilder(build, rev2)

			oldRevBuild := pubsubDummyBuild("test-builder-2", buildbucket.StatusFailure, newTime, rev1)
			testSuccess(oldRevBuild, "test-example-failure@google.com")
			grepLog("old commit")
		})

		Convey(`revision update`, func() {
			build := pubsubDummyBuild("test-builder-3", buildbucket.StatusSuccess, oldTime, rev1)
			testSuccess(build, "test-example-success@google.com")
			verifyBuilder(build, rev1)

			newBuild := pubsubDummyBuild("test-builder-3", buildbucket.StatusFailure, newTime, rev2)
			testSuccess(newBuild, "test-example-failure@google.com", "test-example-change@google.com")
			verifyBuilder(newBuild, rev2)
		})

		Convey(`out-of-order creation time`, func() {
			build := pubsubDummyBuild("test-builder-4", buildbucket.StatusSuccess, newTime, rev1)
			testSuccess(build, "test-example-success@google.com")
			verifyBuilder(build, rev1)

			oldBuild := pubsubDummyBuild("test-builder-4", buildbucket.StatusFailure, oldTime, rev1)
			testSuccess(oldBuild, "test-example-failure@google.com")
			grepLog("old time")
		})
	})
}
