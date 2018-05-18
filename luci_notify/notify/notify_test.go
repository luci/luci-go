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

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/appengine/tq/tqtesting"
	"go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"

	apicfg "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/luci_notify/config"
	"go.chromium.org/luci/luci_notify/internal"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNotify(t *testing.T) {
	Convey("Notify", t, func() {
		c := gaetesting.TestingContextWithAppID("luci-notify")
		c = clock.Set(c, testclock.New(testclock.TestRecentTimeUTC))
		c = gologger.StdConfig.Use(c)
		c = logging.SetLevel(c, logging.Debug)

		build := &Build{
			Build: buildbucketpb.Build{
				Id: 54,
				Builder: &buildbucketpb.Builder_ID{
					Project: "chromium",
					Bucket:  "ci",
					Builder: "linux-rel",
				},
				Status: buildbucketpb.Status_SUCCESS,
			},
		}

		// Put Project and EmailTemplate entities.
		project := &config.Project{Name: "chromium", Revision: "deadbeef"}
		templates := []*config.EmailTemplate{
			{
				ProjectKey:          datastore.KeyForObj(c, project),
				Name:                "default",
				SubjectTextTemplate: "Build {{.Build.Id}} completed",
				BodyHTMLTemplate:    "Build {{.Build.Id}} completed with status {{.Build.Status}}",
			},
			{
				ProjectKey:          datastore.KeyForObj(c, project),
				Name:                "non-default",
				SubjectTextTemplate: "Build {{.Build.Id}} completed from non-default template",
				BodyHTMLTemplate:    "Build {{.Build.Id}} completed with status {{.Build.Status}} from non-default template",
			},
		}
		So(datastore.Put(c, project, templates), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		Convey("createEmailTasks", func() {
			emailNotify := []EmailNotify{
				{
					Email: "jane@example.com",
				},
				{
					Email: "john@example.com",
				},
				{
					Email:    "don@example.com",
					Template: "non-default",
				},
			}
			tasks, err := createEmailTasks(c, emailNotify, buildbucketpb.Status_SUCCESS, build)
			So(err, ShouldBeNil)
			So(tasks, ShouldResemble, []*tq.Task{
				{
					Payload: &internal.EmailTask{
						Recipients: []string{"jane@example.com"},
						Subject:    "Build 54 completed",
						Body:       "Build 54 completed with status SUCCESS",
					},
				},
				{
					Payload: &internal.EmailTask{
						Recipients: []string{"john@example.com"},
						Subject:    "Build 54 completed",
						Body:       "Build 54 completed with status SUCCESS",
					},
				},
				{
					Payload: &internal.EmailTask{
						Recipients: []string{"don@example.com"},
						Subject:    "Build 54 completed from non-default template",
						Body:       "Build 54 completed with status SUCCESS from non-default template",
					},
				},
			})
		})
	})
}

func makeNotifiers(c context.Context, projectID string, cfg *apicfg.ProjectConfig) []*config.Notifier {
	var notifiers []*config.Notifier
	parentKey := datastore.MakeKey(c, "Project", projectID)
	for _, n := range cfg.Notifiers {
		notifiers = append(notifiers, config.NewNotifier(parentKey, n))
	}
	return notifiers
}

// func notifyDummyBuild(status buildbucketpb.Status, notifyEmails ...EmailNotify) *Build {
// 	return &Build{
// 		Build: buildbucketpb.Build{
// 			Id: 54,
// 			Builder: &buildbucketpb.Builder_ID{
// 				Project: "chromium",
// 				Bucket: "ci",
// 				Builder: "test-builder",
// 			},
// 			Status: status,
// 		},
// 		EmailNotify: notifyEmails,
// 	}
// }

// func verifyStringSliceResembles(actual []string, expect []EmailNotify) {
// 	// Put the strings into sets so prevent flakiness.
// 	actualSet := stringset.NewFromSlice(actual...)
// 	expectSet := stringset.New(len(expect))
// 	for _, en := range expect {
// 		expectSet.Add(en.Email)
// 	}
// 	So(actualSet, ShouldResemble, expectSet)
// }

// func verifyEmailContent(recipients []EmailNotify, task tqtesting.Task) {
// 	tr, ok := task.Payload.(*internal.EmailTask)
// 	So(ok, ShouldEqual, true)
// 	for _, email := range tr.Recipients {
// 		for _, en := range recipients {
// 			if en.Email == email {
// 				So(tr.Body, ShouldStartWith, "Build 54 completed with status SUCCESS")
// 				So(tr.Subject, ShouldEqual, "Build 54 completed")
// 			}
// 		}
// 	}
// }

func createMockTaskQueue(c context.Context) (*tq.Dispatcher, tqtesting.Testable) {
	dispatcher := &tq.Dispatcher{}
	InitDispatcher(dispatcher)
	taskqueue := tqtesting.GetTestable(c, dispatcher)
	taskqueue.CreateQueues()
	return dispatcher, taskqueue
}

// func verifyTasksAndMessages(c context.Context, taskqueue tqtesting.Testable, emailExpect []EmailNotify) {
// 	// Make sure a task either was or wasn't scheduled.
// 	tasks := taskqueue.GetScheduledTasks()
// 	if len(emailExpect) == 0 {
// 		So(len(tasks), ShouldEqual, 0)
// 		return
// 	}

// 	var actual []string
// 	// Extract and check the task.
// 	for _, task := range tasks {
// 		t, ok := task.Payload.(*internal.EmailTask)
// 		So(ok, ShouldEqual, true)
// 		actual = append(actual, t.Recipients...)
// 		verifyEmailContent(emailExpect, task)
// 	}
// 	verifyStringSliceResembles(actual, emailExpect)

// 	// Simulate running the tasks.
// 	done, pending, err := taskqueue.RunSimulation(c, nil)
// 	So(err, ShouldBeNil)

// 	// Check to see if any messages were sent.
// 	messages := mail.GetTestable(c).SentMessages()
// 	if len(emailExpect) == 0 {
// 		So(len(done), ShouldEqual, 1)
// 		So(len(pending), ShouldEqual, 0)
// 		So(len(messages), ShouldEqual, 0)
// 	} else {
// 		So(len(messages), ShouldEqual, len(emailExpect))
// 	}

// 	// Reset messages sent for other tasks.
// 	mail.GetTestable(c).Reset()
// }

// func TestNotify(t *testing.T) {
// 	t.Parallel()

// 	Convey(`Test Environment for Notify`, t, func() {
// 		cfgName := "basic"
// 		cfg, err := testutil.LoadProjectConfig(cfgName)
// 		So(err, ShouldBeNil)

// 		c := gaetesting.TestingContextWithAppID("luci-notify-test")
// 		c = clock.Set(c, testclock.New(testclock.TestRecentTimeUTC))
// 		user.GetTestable(c).Login("noreply@luci-notify-test.appspotmail.com", "", false)

// 		// Put Project and EmailTemplate entities.
// 		project := &notifyConfig.Project{Name: "chromium", Revision: "deadbeef"}
// 		templates := []*notifyConfig.EmailTemplate{
// 			{
// 				Name:                "default",
// 				SubjectTextTemplate: "Build {{.Build.Id}} completed",
// 				BodyHTMLTemplate:    "Build {{.Build.Id}} completed with status {{.Build.Status}}",
// 			},
// 			{
// 				Name:                "non-default",
// 				SubjectTextTemplate: "Build {{.Build.Id}} completed from non-default template",
// 				BodyHTMLTemplate:    "Build {{.Build.Id}} completed with status {{.Build.Status}} from non-default template",
// 			},
// 		}
// 		So(datastore.Put(c, project, templates), ShouldBeNil)
// 		datastore.GetTestable(c).CatchupIndexes()

// 		// Get notifiers from test config.
// 		notifiers := makeNotifiers(c, cfgName, cfg)

// 		// Re-usable builds and builders for running Notify.
// 		dummyPropEmail := EmailNotify{
// 			Email:    "property@google.com",
// 			Template: "default",
// 		}
// 		dummyBogusEmail := EmailNotify{
// 			Email: "bogus@gmail.com",
// 		}
// 		goodBuild := notifyDummyBuild(buildbucketpb.Status_SUCCESS)
// 		goodEmailBuild := notifyDummyBuild(buildbucketpb.Status_SUCCESS, dummyPropEmail, dummyBogusEmail)
// 		badBuild := notifyDummyBuild(buildbucketpb.Status_FAILURE)
// 		badEmailBuild := notifyDummyBuild(buildbucketpb.Status_FAILURE, dummyPropEmail, dummyBogusEmail)
// 		goodBuilder := &Builder{
// 			StatusBuildTime: time.Date(2015, 2, 3, 12, 54, 3, 0, time.UTC),
// 			Status:          buildbucketpb.Status_SUCCESS,
// 		}
// 		badBuilder := &Builder{
// 			StatusBuildTime: time.Date(2015, 2, 3, 12, 54, 3, 0, time.UTC),
// 			Status:          buildbucketpb.Status_FAILURE,
// 		}

// 		dispatcher, taskqueue := createMockTaskQueue(c)

// 	})
// }
