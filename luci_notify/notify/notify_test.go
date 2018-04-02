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
	"go.chromium.org/luci/buildbucket/proto"
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

func notifyDummyBuild(status buildbucketpb.Status, notifyEmails ...EmailNotify) *Build {
	var build Build
	build.Build = *testutil.TestBuild("test", "hello", "test-builder", status)
	build.EmailNotify = notifyEmails

	return &build
}

func IsStringSliceEqualSet(left []string, right stringset.Set) bool {
	if len(left) != right.Len() {
		return false
	}

	for _, l := range left {
		if !right.Has(l) {
			return false
		}
	}
	return true
}

func verifyStringSliceResembles(actual, expect []string) {
	// Put the strings into sets so prevent flakiness.
	actualSet := stringset.NewFromSlice(actual...)
	expectSet := stringset.NewFromSlice(expect...)
	So(actualSet, ShouldResemble, expectSet)
	// To ensure the conversion to set didn't de-dup content.
	So(len(actual), ShouldEqual, len(expect))
}

func createMockTaskQueue(c context.Context) (*tq.Dispatcher, tqtesting.Testable) {
	dispatcher := &tq.Dispatcher{}
	InitDispatcher(dispatcher)
	taskqueue := tqtesting.GetTestable(c, dispatcher)
	taskqueue.CreateQueues()
	return dispatcher, taskqueue
}

// verifySendListsMatchEmailMap verifies that each list of addresses has a
// matching expectation in the templateEmailMap, and that all templates have a
// matching sent list.
func verifySendListsMatchEmailMap(sendLists [][]string, emailExpect templateEmailMap) {
	// Make list of template addresses not yet verified to be sent.
	notfound := stringset.New(len(emailExpect))
	for k := range emailExpect {
		notfound.Add(k)
	}

	// Remove each sent address list, from not found expectations, until all match.
	// We know sizes match from assertions above.
	for _, sendList := range sendLists {
		for _, nf := range notfound.ToSlice() {
			if IsStringSliceEqualSet(sendList, emailExpect[nf]) {
				notfound.Del(nf)
				break
			}
			// sendList doesn't match any expectation, show failure with all data,
			// even though mismatched types will show in error.
			So(sendList, ShouldResemble, emailExpect)
		}
	}

	// send_list doesn't match any expectation, show failure with all data,
	// even though mismatched types will show in error.
	if notfound.Len() > 0 {
		So(sendLists, ShouldResemble, emailExpect)
	}
}

func verifyTasksAndMessages(c context.Context, taskqueue tqtesting.Testable, emailExpect templateEmailMap) {
	// Make sure a task either was or wasn't scheduled.
	tasks := taskqueue.GetScheduledTasks()

	// Collect lists of email addresses to send to.
	taskSendLists := make([][]string, len(tasks))
	for i, task := range tasks {
		taskPayload, ok := task.Payload.(*internal.EmailTask)
		So(ok, ShouldEqual, true)
		taskSendLists[i] = taskPayload.Recipients
	}

	// Simulate running the tasks.
	done, pending, err := taskqueue.RunSimulation(c, nil)
	So(len(done), ShouldEqual, len(tasks))
	So(len(pending), ShouldEqual, 0)
	So(err, ShouldBeNil)

	messages := mail.GetTestable(c).SentMessages()

	// Assume messages are sent in task order, and verify they match task send_lists.
	for i, message := range messages {
		verifyStringSliceResembles(message.To, taskSendLists[i])
	}

	// Verify task send_lists match per-template expectations.
	verifySendListsMatchEmailMap(taskSendLists, emailExpect)

	// Reset messages sent for other tasks.
	mail.GetTestable(c).Reset()
}

func defaultTemplate(emails ...string) templateEmailMap {
	result := templateEmailMap{}
	result.Add("default", emails...)
	return result
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
		goodBuild := notifyDummyBuild(buildbucketpb.Status_SUCCESS)
		goodEmailBuild := notifyDummyBuild(
			buildbucketpb.Status_SUCCESS,
			EmailNotify{Email: "bogus@gmail.com"},
			EmailNotify{Email: "property@google.com"},
			EmailNotify{Email: "property@google.com"}, // Dedup test.
			// EmailNotify{Email: "property@google.com", Template: "custom"}, // Second Template Test
			// EmailNotify{Email: "custom@google.com", Template: "custom"},
		)
		badBuild := notifyDummyBuild(buildbucketpb.Status_FAILURE)
		badEmailBuild := notifyDummyBuild(
			buildbucketpb.Status_FAILURE,
			EmailNotify{Email: "property@google.com"},
			EmailNotify{Email: "bogus@gmail.com"},
		)
		goodBuilder := &Builder{
			StatusBuildTime: time.Date(2015, 2, 3, 12, 54, 3, 0, time.UTC),
			Status:          buildbucketpb.Status_SUCCESS,
		}
		badBuilder := &Builder{
			StatusBuildTime: time.Date(2015, 2, 3, 12, 54, 3, 0, time.UTC),
			Status:          buildbucketpb.Status_FAILURE,
		}

		dispatcher, taskqueue := createMockTaskQueue(c)

		test := func(notifiers []*config.Notifier, build *Build, builder *Builder, emailExpect templateEmailMap) {
			// Test Notify.
			err := Notify(c, dispatcher, notifiers, builder.Status, build)
			So(err, ShouldBeNil)

			// Verify sent messages.
			verifyTasksAndMessages(c, taskqueue, emailExpect)
		}

		goodBuilderExpected := templateEmailMap{}
		goodBuilderExpected.Add("default", "property@google.com")
		// goodBuilderExpected.Add("custom", "property@google.com", "custom@google.com")

		Convey(`empty`, func() {
			var noNotifiers []*config.Notifier
			noEmail := templateEmailMap{}
			Convey(`no properties`, func() {
				test(noNotifiers, goodBuild, goodBuilder, noEmail)
				test(noNotifiers, goodBuild, badBuilder, noEmail)
				test(noNotifiers, badBuild, goodBuilder, noEmail)
				test(noNotifiers, badBuild, badBuilder, noEmail)
			})
			Convey(`properties`, func() {
				test(noNotifiers, goodEmailBuild, goodBuilder, goodBuilderExpected)
				test(noNotifiers, badEmailBuild, goodBuilder, goodBuilderExpected)
			})
		})

		Convey(`on success`, func() {
			test(
				notifiers,
				goodBuild,
				goodBuilder,
				defaultTemplate("test-example-success@google.com"),
			)
			test(
				notifiers,
				goodEmailBuild,
				goodBuilder,
				defaultTemplate(
					"test-example-success@google.com",
					"property@google.com"),
			)
		})

		Convey(`on failure`, func() {
			test(
				notifiers,
				badBuild,
				badBuilder,
				defaultTemplate("test-example-failure@google.com"),
			)
			test(
				notifiers,
				badEmailBuild,
				badBuilder,
				defaultTemplate(
					"test-example-failure@google.com",
					"property@google.com"),
			)
		})

		Convey(`on change to failure`, func() {
			test(
				notifiers,
				badBuild,
				goodBuilder,
				defaultTemplate(
					"test-example-failure@google.com",
					"test-example-change@google.com"),
			)
			test(
				notifiers,
				badEmailBuild,
				goodBuilder,
				defaultTemplate(
					"test-example-failure@google.com",
					"test-example-change@google.com",
					"property@google.com"),
			)
		})

		Convey(`on change to success`, func() {
			test(
				notifiers,
				goodBuild,
				badBuilder,
				defaultTemplate(
					"test-example-success@google.com",
					"test-example-change@google.com"),
			)
			test(
				notifiers,
				goodEmailBuild,
				badBuilder,
				defaultTemplate(
					"test-example-success@google.com",
					"test-example-change@google.com",
					"property@google.com"),
			)
		})
	})
}
