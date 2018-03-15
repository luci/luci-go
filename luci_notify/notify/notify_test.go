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
	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/appengine/tq/tqtesting"
	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/stringset"

	notifyConfig "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/luci_notify/config"
	"go.chromium.org/luci/luci_notify/internal"
	"go.chromium.org/luci/luci_notify/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func extractNotifiers(c context.Context, projectID string, cfg *notifyConfig.ProjectConfig) []*config.Notifier {
	var notifiers []*config.Notifier
	parentKey := datastore.MakeKey(c, "Project", projectID)
	for _, n := range cfg.Notifiers {
		notifiers = append(notifiers, config.NewNotifier(parentKey, n))
	}
	return notifiers
}

func notifyDummyBuild(status buildbucket.Status, notifyEmails ...string) *Build {
	var build Build
	build.Build = *testutil.TestBuild("test", "hello", "test-builder", status)
	build.EmailNotify = notifyEmails

	return &build
}

func verifyStringSliceResembles(actual, expect []string) {
	// Put the strings into sets so prevent flakiness.
	actualSet := stringset.NewFromSlice(actual...)
	expectSet := stringset.NewFromSlice(expect...)
	So(actualSet, ShouldResemble, expectSet)
}

func createMockTaskQueue(c context.Context) (*tq.Dispatcher, tqtesting.Testable) {
	dispatcher := &tq.Dispatcher{}
	InitDispatcher(dispatcher)
	taskqueue := tqtesting.GetTestable(c, dispatcher)
	taskqueue.CreateQueues()
	return dispatcher, taskqueue
}

func verifyTasksAndMessages(c context.Context, taskqueue tqtesting.Testable, emailExpect []string) {
	// Make sure a task either was or wasn't scheduled.
	tasks := taskqueue.GetScheduledTasks()
	if len(emailExpect) == 0 {
		So(len(tasks), ShouldEqual, 0)
		return
	}
	So(len(tasks), ShouldEqual, 1)

	// Extract and check the task.
	task, ok := tasks[0].Payload.(*internal.EmailTask)
	So(ok, ShouldEqual, true)
	verifyStringSliceResembles(task.Recipients, emailExpect)

	// Simulate running the tasks.
	done, pending, err := taskqueue.RunSimulation(c, nil)
	So(err, ShouldBeNil)

	// Check to see if any messages were sent.
	messages := mail.GetTestable(c).SentMessages()
	if len(emailExpect) == 0 {
		So(len(done), ShouldEqual, 1)
		So(len(pending), ShouldEqual, 0)
		So(len(messages), ShouldEqual, 0)
	} else {
		verifyStringSliceResembles(messages[0].To, emailExpect)
	}

	// Reset messages sent for other tasks.
	mail.GetTestable(c).Reset()
}

func TestNotify(t *testing.T) {
	t.Parallel()

	Convey(`Test Environment for Notify`, t, func() {
		cfgName := "basic"
		cfg, err := testutil.LoadProjectConfig(cfgName)
		So(err, ShouldBeNil)

		c := memory.UseWithAppID(context.Background(), "luci-notify-test")
		c = clock.Set(c, testclock.New(time.Now()))
		user.GetTestable(c).Login("noreply@luci-notify-test.appspotmail.com", "", false)

		// Get notifiers from test config.
		notifiers := extractNotifiers(c, cfgName, cfg)

		// Re-usable builds and builders for running Notify.
		goodBuild := notifyDummyBuild(buildbucket.StatusSuccess)
		goodEmailBuild := notifyDummyBuild(buildbucket.StatusSuccess, "property@google.com", "bogus@gmail.com")
		badBuild := notifyDummyBuild(buildbucket.StatusFailure)
		badEmailBuild := notifyDummyBuild(buildbucket.StatusFailure, "property@google.com", "bogus@gmail.com")
		goodBuilder := &Builder{
			StatusBuildTime: time.Date(2015, 2, 3, 12, 54, 3, 0, time.UTC),
			Status:          buildbucket.StatusSuccess,
		}
		badBuilder := &Builder{
			StatusBuildTime: time.Date(2015, 2, 3, 12, 54, 3, 0, time.UTC),
			Status:          buildbucket.StatusFailure,
		}

		dispatcher, taskqueue := createMockTaskQueue(c)

		test := func(notifiers []*config.Notifier, build *Build, builder *Builder, emailExpect ...string) {
			// Test Notify.
			err := Notify(c, dispatcher, notifiers, builder.Status, build)
			So(err, ShouldBeNil)

			// Verify sent messages.
			verifyTasksAndMessages(c, taskqueue, emailExpect)
		}

		Convey(`empty`, func() {
			var noNotifiers []*config.Notifier
			test(noNotifiers, goodBuild, goodBuilder)
			test(noNotifiers, goodBuild, badBuilder)
			test(noNotifiers, badBuild, goodBuilder)
			test(noNotifiers, badBuild, badBuilder)
			test(noNotifiers, goodEmailBuild, goodBuilder, "property@google.com")
			test(noNotifiers, badEmailBuild, goodBuilder, "property@google.com")
		})

		Convey(`on success`, func() {
			test(
				notifiers,
				goodBuild,
				goodBuilder,
				"test-example-success@google.com",
			)
			test(
				notifiers,
				goodEmailBuild,
				goodBuilder,
				"test-example-success@google.com",
				"property@google.com",
			)
		})

		Convey(`on failure`, func() {
			test(
				notifiers,
				badBuild,
				badBuilder,
				"test-example-failure@google.com",
			)
			test(
				notifiers,
				badEmailBuild,
				badBuilder,
				"test-example-failure@google.com",
				"property@google.com",
			)
		})

		Convey(`on change to failure`, func() {
			test(
				notifiers,
				badBuild,
				goodBuilder,
				"test-example-failure@google.com",
				"test-example-change@google.com",
			)
			test(
				notifiers,
				badEmailBuild,
				goodBuilder,
				"test-example-failure@google.com",
				"test-example-change@google.com",
				"property@google.com",
			)
		})

		Convey(`on change to success`, func() {
			test(
				notifiers,
				goodBuild,
				badBuilder,
				"test-example-success@google.com",
				"test-example-change@google.com",
			)
			test(
				notifiers,
				goodEmailBuild,
				badBuilder,
				"test-example-success@google.com",
				"test-example-change@google.com",
				"property@google.com",
			)
		})
	})
}
