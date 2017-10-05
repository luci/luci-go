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

func notifyDummyBuild(status buildbucket.Status) *buildbucket.Build {
	return testutil.TestBuild("hello", "test-builder", status)
}

func TestNotify(t *testing.T) {
	t.Parallel()

	Convey(`Test Environment for Notify`, t, func() {
		cfgName := "basic"
		cfg, err := testutil.LoadProjectConfig(cfgName)
		So(err, ShouldBeNil)
		c := memory.UseWithAppID(context.Background(), "dev~luci-notify")
		notifiers := extractNotifiers(c, cfgName, cfg)
		goodBuild := notifyDummyBuild(buildbucket.StatusSuccess)
		badBuild := notifyDummyBuild(buildbucket.StatusFailure)
		goodBuilder := &Builder{
			StatusBuildTime: time.Date(2015, 2, 3, 12, 54, 3, 0, time.UTC),
			Status:          buildbucket.StatusSuccess,
		}
		badBuilder := &Builder{
			StatusBuildTime: time.Date(2015, 2, 3, 12, 54, 3, 0, time.UTC),
			Status:          buildbucket.StatusFailure,
		}

		test := func(build *buildbucket.Build, builder *Builder, emailExpect ...string) {
			// Login and test notifying.
			user.GetTestable(c).Login("noreply@luci-notify-dev.appspotmail.com", "", false)
			err := Notify(c, notifiers, build, builder)
			So(err, ShouldBeNil)

			messages := mail.GetTestable(c).SentMessages()
			So(len(messages), ShouldEqual, 1)

			// Put the recipients into sets so prevent flakiness.
			actualRecipients := stringset.NewFromSlice(messages[0].To...)
			expectRecipients := stringset.NewFromSlice(emailExpect...)
			So(actualRecipients, ShouldResemble, expectRecipients)
		}

		Convey(`empty`, func() {
			testNone := func(build *buildbucket.Build, builder *Builder) {
				err := Notify(c, []*config.Notifier{}, build, builder)
				So(err, ShouldBeNil)
				So(len(mail.GetTestable(c).SentMessages()), ShouldEqual, 0)
			}
			testNone(goodBuild, goodBuilder)
			testNone(goodBuild, badBuilder)
			testNone(badBuild, goodBuilder)
			testNone(badBuild, badBuilder)
		})

		Convey(`on success`, func() {
			test(
				goodBuild,
				goodBuilder,
				"test-example-success@google.com",
			)
		})

		Convey(`on failure`, func() {
			test(
				badBuild,
				badBuilder,
				"test-example-failure@google.com",
			)
		})

		Convey(`on change to failure`, func() {
			test(
				badBuild,
				goodBuilder,
				"test-example-failure@google.com",
				"test-example-change@google.com",
			)
		})

		Convey(`on change to success`, func() {
			test(
				goodBuild,
				badBuilder,
				"test-example-success@google.com",
				"test-example-change@google.com",
			)
		})
	})
}
