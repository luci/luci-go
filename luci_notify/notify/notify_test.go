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
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/stringset"

	config "go.chromium.org/luci/config"
	memImpl "go.chromium.org/luci/config/impl/memory"
	apiConfig "go.chromium.org/luci/luci_notify/api/config"
	notifyConfig "go.chromium.org/luci/luci_notify/config"
	"go.chromium.org/luci/luci_notify/internal"
	"go.chromium.org/luci/luci_notify/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func extractNotifiers(c context.Context, projectID string, cfg *apiConfig.ProjectConfig) []*notifyConfig.Notifier {
	var notifiers []*notifyConfig.Notifier
	parentKey := datastore.MakeKey(c, "Project", projectID)
	for _, n := range cfg.Notifiers {
		notifiers = append(notifiers, notifyConfig.NewNotifier(parentKey, n))
	}
	return notifiers
}

func notifyDummyBuild(status buildbucketpb.Status, notifyEmails ...[]emailNotify) *Build {
	var build Build
	build.Build = *testutil.TestBuild("test", "hello", "test-builder", status)
	for _, en := range notifyEmails {
		build.EmailNotify = append(build.EmailNotify, en...)
	}
	return &build
}

func verifyStringSliceResembles(actual []string, expect []emailNotify) {
	// Put the strings into sets so prevent flakiness.
	actualSet := stringset.NewFromSlice(actual...)
	var addresses []string
	for _, en := range expect {
		addresses = append(addresses, en.Email)
	}
	expectSet := stringset.NewFromSlice(addresses...)
	So(actualSet, ShouldResemble, expectSet)
}

func verifyEmailContent(recipients []emailNotify, task tqtesting.Task) {
	tr, ok := task.Payload.(*internal.EmailTask)
	So(ok, ShouldEqual, true)
	for _, email := range tr.Recipients {
		for _, en := range recipients {
			if en.Email == email {
				if en.Template == "minimal" {
					So(tr.Subject, ShouldEqual, "Test Subject")
					So(tr.Body, ShouldEqual, "Test Content")
				}
			}
		}
	}
}

func createMockTaskQueue(c context.Context) (*tq.Dispatcher, tqtesting.Testable) {
	dispatcher := &tq.Dispatcher{}
	InitDispatcher(dispatcher)
	taskqueue := tqtesting.GetTestable(c, dispatcher)
	taskqueue.CreateQueues()
	return dispatcher, taskqueue
}

func verifyTasksAndMessages(c context.Context, taskqueue tqtesting.Testable, emailExpect []emailNotify) {
	// Make sure a task either was or wasn't scheduled.
	tasks := taskqueue.GetScheduledTasks()
	if len(emailExpect) == 0 {
		So(len(tasks), ShouldEqual, 0)
		return
	}

	var actual []string
	// Extract and check the task.
	for _, task := range tasks {
		t, ok := task.Payload.(*internal.EmailTask)
		So(ok, ShouldEqual, true)
		actual = append(actual, t.Recipients...)
		verifyEmailContent(emailExpect, task)
	}
	verifyStringSliceResembles(actual, emailExpect)

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
		So(len(messages), ShouldEqual, len(emailExpect))
	}

	// Reset messages sent for other tasks.
	mail.GetTestable(c).Reset()
}

func TestNotify(t *testing.T) {
	//t.Parallel()

	Convey(`Test Environment for Notify`, t, func() {
		cfgName := "basic"
		cfg, err := testutil.LoadProjectConfig(cfgName)
		So(err, ShouldBeNil)

		c := memory.UseWithAppID(context.Background(), "luci-notify-test")
		c = clock.Set(c, testclock.New(time.Now()))
		user.GetTestable(c).Login("noreply@luci-notify-test.appspotmail.com", "", false)
		impl := memImpl.New(map[config.Set]memImpl.Files{
			"projects/test": {
				"file": "file content",
				"luci-notify/email_templates/minimal.template": `Test Subject

				Test Content`,
				"minimal": `Invalid Template Name

				Will be rejected`,
			},
		})
		c = context.WithValue(c, configInterfaceKey, impl)

		// Get notifiers from test config.
		notifiers := extractNotifiers(c, cfgName, cfg)

		// Re-usable builds and builders for running Notify.
		emails := []emailNotify{
			{
				Email:    "property@google.com",
				Template: "minimal",
			},
			{
				Email: "bogus@gmail.com",
			},
		}
		goodBuild := notifyDummyBuild(buildbucketpb.Status_SUCCESS)
		goodEmailBuild := notifyDummyBuild(buildbucketpb.Status_SUCCESS, emails)
		badBuild := notifyDummyBuild(buildbucketpb.Status_FAILURE)
		badEmailBuild := notifyDummyBuild(buildbucketpb.Status_FAILURE, emails)
		goodBuilder := &Builder{
			StatusBuildTime: time.Date(2015, 2, 3, 12, 54, 3, 0, time.UTC),
			Status:          buildbucketpb.Status_SUCCESS,
		}
		badBuilder := &Builder{
			StatusBuildTime: time.Date(2015, 2, 3, 12, 54, 3, 0, time.UTC),
			Status:          buildbucketpb.Status_FAILURE,
		}

		dispatcher, taskqueue := createMockTaskQueue(c)

		test := func(notifiers []*notifyConfig.Notifier, build *Build, builder *Builder, emailExpect ...[]emailNotify) {
			// Test Notify.
			err := Notify(c, dispatcher, notifiers, builder.Status, build)
			So(err, ShouldBeNil)

			// Flatten email notify array of arrays
			notifyEmail := []emailNotify{}
			for _, ena := range emailExpect {
				notifyEmail = append(notifyEmail, ena...)
			}

			// Verify sent messages.
			verifyTasksAndMessages(c, taskqueue, notifyEmail)
		}

		propEmail := []emailNotify{
			{
				Email:    "property@google.com",
				Template: "minimal",
			},
		}
		successEmail := []emailNotify{
			{
				Email: "test-example-success@google.com",
			},
		}
		failEmail := []emailNotify{
			{
				Email: "test-example-failure@google.com",
			},
		}
		changeEmail := []emailNotify{
			{
				Email: "test-example-change@google.com",
			},
		}

		Convey(`empty`, func() {
			var noNotifiers []*notifyConfig.Notifier
			test(noNotifiers, goodBuild, goodBuilder)
			test(noNotifiers, goodBuild, badBuilder)
			test(noNotifiers, badBuild, goodBuilder)
			test(noNotifiers, badBuild, badBuilder)
			test(noNotifiers, goodEmailBuild, goodBuilder, propEmail)
			test(noNotifiers, badEmailBuild, goodBuilder, propEmail)
		})

		Convey(`on success`, func() {
			test(
				notifiers,
				goodBuild,
				goodBuilder,
				successEmail,
			)
			test(
				notifiers,
				goodEmailBuild,
				goodBuilder,
				successEmail,
				propEmail,
			)
		})

		Convey(`on failure`, func() {
			test(
				notifiers,
				badBuild,
				badBuilder,
				failEmail,
			)
			test(
				notifiers,
				badEmailBuild,
				badBuilder,
				failEmail,
				propEmail,
			)
		})

		Convey(`on change to failure`, func() {
			test(
				notifiers,
				badBuild,
				goodBuilder,
				failEmail,
				changeEmail,
			)
			test(
				notifiers,
				badEmailBuild,
				goodBuilder,
				failEmail,
				changeEmail,
				propEmail,
			)
		})

		Convey(`on change to success`, func() {
			test(
				notifiers,
				goodBuild,
				badBuilder,
				successEmail,
				changeEmail,
			)
			test(
				notifiers,
				goodEmailBuild,
				badBuilder,
				successEmail,
				changeEmail,
				propEmail,
			)
		})
	})
}
