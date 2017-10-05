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
	"go.chromium.org/gae/service/mail"
	"go.chromium.org/gae/service/user"
	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/data/stringset"

	notifyConfig "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/luci_notify/config"
	"go.chromium.org/luci/luci_notify/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func extractNotifiers(c context.Context, cfgName string, cfg *notifyConfig.ProjectConfig) []*config.Notifier {
	var notifiers []*config.Notifier
	parentKey := datastore.MakeKey(c, "Project", cfgName)
	for _, n := range cfg.Notifiers {
		notifiers = append(notifiers, config.NewNotifier(parentKey, n))
	}
	return notifiers
}

func dummyBuildInfo(status buildbucket.Status) *buildbucket.Build {
	return testutil.TestBuild("test.bucket", "test-builder", status)
}

func TestNotification(t *testing.T) {
	t.Parallel()

	Convey(`Test Environment for Notification`, t, func() {
		cfgName := "basic"
		cfg, err := testutil.LoadProjectConfig(cfgName)
		So(err, ShouldBeNil)
		c := memory.UseWithAppID(context.Background(), "dev~luci-notify")
		notifiers := extractNotifiers(c, cfgName, cfg)
		goodBuild := dummyBuildInfo(buildbucket.StatusSuccess)
		badBuild := dummyBuildInfo(buildbucket.StatusFailure)
		goodCommit := testutil.TestCommit(time.Date(2015, 2, 3, 12, 55, 2, 0, time.UTC), testutil.TestRevision())
		badCommit := testutil.TestCommit(time.Date(2015, 2, 3, 12, 56, 2, 0, time.UTC), testutil.TestRevision())
		goodBuilder := &Builder{
			StatusTime: time.Date(2015, 2, 3, 12, 54, 3, 0, time.UTC),
			Status:     buildbucket.StatusSuccess,
		}
		badBuilder := &Builder{
			StatusTime: time.Date(2015, 2, 3, 12, 54, 3, 0, time.UTC),
			Status:     buildbucket.StatusFailure,
		}

		test := func(build *buildbucket.Build, commit *gitiles.Commit, builder *Builder, emailExpect ...string) {
			// Test creating the notification.
			n := CreateNotification(c, notifiers, build, commit, builder)
			So(n, ShouldNotBeNil)

			// Put the recipients into sets so prevent flakiness.
			actualRecipients := stringset.NewFromSlice(n.EmailRecipients...)
			expectRecipients := stringset.NewFromSlice(emailExpect...)
			So(actualRecipients, ShouldResemble, expectRecipients)
			So(n.Build, ShouldEqual, build)
			So(n.Builder, ShouldEqual, builder)

			// Login and send email.
			user.GetTestable(c).Login("noreply@luci-notify-dev.appspotmail.com", "", false)
			So(n.Dispatch(c), ShouldBeNil)

			// Make sure an email was sent, but don't test its exact contents.
			So(len(mail.GetTestable(c).SentMessages()), ShouldEqual, 1)
		}

		Convey(`empty`, func() {
			creationNoneTest := func(build *buildbucket.Build, commit *gitiles.Commit, builder *Builder) {
				n := CreateNotification(c, []*config.Notifier{}, build, commit, builder)
				So(n, ShouldBeNil)
			}
			creationNoneTest(goodBuild, goodCommit, goodBuilder)
			creationNoneTest(goodBuild, goodCommit, badBuilder)
			creationNoneTest(badBuild, badCommit, goodBuilder)
			creationNoneTest(badBuild, badCommit, badBuilder)
		})

		Convey(`out-of-order`, func() {
			oldBuild := dummyBuildInfo(buildbucket.StatusSuccess)
			oldCommit := testutil.TestCommit(time.Date(2013, 5, 6, 4, 12, 55, 0, time.UTC), testutil.TestRevision())
			n := CreateNotification(c, notifiers, oldBuild, oldCommit, goodBuilder)
			So(n, ShouldBeNil)
		})

		Convey(`on success`, func() {
			test(
				goodBuild,
				goodCommit,
				goodBuilder,
				"test-example-success@google.com",
			)
		})

		Convey(`on failure`, func() {
			test(
				badBuild,
				badCommit,
				badBuilder,
				"test-example-failure@google.com",
			)
		})

		Convey(`on change to failure`, func() {
			test(
				badBuild,
				badCommit,
				goodBuilder,
				"test-example-failure@google.com",
				"test-example-change@google.com",
			)
		})

		Convey(`on change to success`, func() {
			test(
				goodBuild,
				goodCommit,
				badBuilder,
				"test-example-success@google.com",
				"test-example-change@google.com",
			)
		})
	})
}
