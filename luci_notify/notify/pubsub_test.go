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
	"testing"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/mail"
	"go.chromium.org/gae/service/user"
	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/data/stringset"

	"go.chromium.org/luci/luci_notify/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

var testCommits = []gitiles.Commit{

}

func pubsubDummyBuild(status buildbucket.Status, creationTime time.Time, revision string) *buildbucket.Build {
	build := testutil.TestBuild("test.bucket", "test-builder", status)
	build.BuildSets = []buildbucket.BuildSet{buildbucket.BuildSet(&buildbucket.GitilesCommit{
		Host: "test.googlesource.com",
		Project: "test",
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
		c = WithGitilesClientFactory(c, func(c context.Context, _ string) (GitilesClient, error) {
			 return GitilesClient(&testutil.GitilesMockClient{Commits: testCommits}), nil
		})

		notifiers := extractNotifiers(c, cfgName, cfg)
		for _, n := range notifiers {
			So(datastore.Put(c, n), ShouldBeNil)
		}

		/*
		goodBuild := pubsubDummyBuild(
			buildbucket.StatusSuccess,
			time.Date(2015, 2, 3, 12, 55, 2, 0, time.UTC),
			testutil.TestRevision(),
		)
		badBuild := pubsubDummyBuild(
			buildbucket.StatusFailure,
			time.Date(2015, 2, 3, 12, 55, 2, 0, time.UTC),
			testutil.TestRevision(),
		)

		goodBuilder := &Builder{
			StatusBuildTime: time.Date(2015, 2, 3, 12, 54, 3, 0, time.UTC),
			Status:     buildbucket.StatusSuccess,
		}
		badBuilder := &Builder{
			StatusBuildTime: time.Date(2015, 2, 3, 12, 54, 3, 0, time.UTC),
			Status:     buildbucket.StatusFailure,
		}*/

		testSuccess := func(build *buildbucket.Build, emailExpect ...string) {
			// Login and test notifying.
			user.GetTestable(c).Login("noreply@luci-notify-dev.appspotmail.com", "", false)
			err := handleBuild(c, build)
			So(err, ShouldBeNil)

			messages := mail.GetTestable(c).SentMessages()
			if len(emailExpect) == 0 {
				So(len(messages), ShouldEqual, 0)
				return
			}

			// Put the recipients into sets so prevent flakiness.
			actualRecipients := stringset.NewFromSlice(messages[0].To...)
			expectRecipients := stringset.NewFromSlice(emailExpect...)
			So(actualRecipients, ShouldResemble, expectRecipients)
		}

		Convey(`no revision`, func() {
			testSuccess(testutil.TestBuild("bleh", "blah", buildbucket.StatusFailure))
		})
	})

	Convey(`Empty handleBuild Environment`, t, func() {
		c := memory.UseWithAppID(context.Background(), "dev~luci-notify")
		c = WithGitilesClientFactory(c, func(_ context.Context, _ string) (GitilesClient, error) {
			return GitilesClient(&testutil.GitilesMockClient{}), nil
		})
		build := pubsubDummyBuild(
			buildbucket.StatusFailure,
			time.Date(2015, 2, 3, 12, 55, 2, 0, time.UTC),
			testutil.TestRevision(),
		)
		err := handleBuild(c, build)
		So(err, ShouldBeNil)
		So(len(mail.GetTestable(c).SentMessages()), ShouldEqual, 0)
	})
}
