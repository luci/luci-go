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
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/user"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/appengine/tq/tqtesting"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging/memlogger"
	gitpb "go.chromium.org/luci/common/proto/git"

	apicfg "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/luci_notify/config"
	"go.chromium.org/luci/luci_notify/internal"
	"go.chromium.org/luci/luci_notify/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

var (
	rev1        = "deadbeef"
	rev2        = "badcoffe"
	testCommits = []*gitpb.Commit{
		{Id: rev1},
		{Id: rev2},
	}
)

func pubsubDummyBuild(builder string, status buildbucketpb.Status, creationTime time.Time, revision string, notifyEmails ...EmailNotify) *Build {
	ret := &Build{
		Build: buildbucketpb.Build{
			Builder: &buildbucketpb.BuilderID{
				Project: "chromium",
				Bucket:  "ci",
				Builder: builder,
			},
			Status: status,
			Input: &buildbucketpb.Build_Input{
				GitilesCommit: &buildbucketpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Id:      revision,
				},
			},
		},
		EmailNotify: notifyEmails,
	}
	ret.Build.CreateTime, _ = ptypes.TimestampProto(creationTime)
	return ret
}

func TestExtractEmailNotifyValues(t *testing.T) {
	Convey(`Test Environment for extractEmailNotifyValues`, t, func() {
		Convey(`empty parametersJson`, func() {
			results, err := extractEmailNotifyValues("")
			So(results, ShouldHaveLength, 0)
			So(err, ShouldBeNil)
		})

		Convey(`populated without email_notify`, func() {
			results, err := extractEmailNotifyValues(`{"foo": 1}`)
			So(results, ShouldHaveLength, 0)
			So(err, ShouldBeNil)
		})

		Convey(`single email_notify value`, func() {
			results, err := extractEmailNotifyValues(`{"email_notify": [{"email": "test@email"}]}`)
			So(results, ShouldResemble, []EmailNotify{
				{
					Email:    "test@email",
					Template: "",
				},
			})
			So(err, ShouldBeNil)
		})

		Convey(`single email_notify value_with_template`, func() {
			results, err := extractEmailNotifyValues(`{"email_notify": [{"email": "test@email", "template": "test-template"}]}`)
			So(results, ShouldResemble, []EmailNotify{
				{
					Email:    "test@email",
					Template: "test-template",
				},
			})
			So(err, ShouldBeNil)
		})

		Convey(`multiple email_notify values`, func() {
			results, err := extractEmailNotifyValues(`{"email_notify": [{"email": "test@email"}, {"email": "test2@email"}]}`)
			So(results, ShouldResemble, []EmailNotify{
				{
					Email:    "test@email",
					Template: "",
				},
				{
					Email:    "test2@email",
					Template: "",
				},
			})
			So(err, ShouldBeNil)
		})
	})
}

func TestHandleBuild(t *testing.T) {
	t.Parallel()

	Convey(`Test Environment for handleBuild`, t, func() {
		cfgName := "basic"
		cfg, err := testutil.LoadProjectConfig(cfgName)
		So(err, ShouldBeNil)

		c := gaetesting.TestingContextWithAppID("luci-notify-test")
		c = clock.Set(c, testclock.New(time.Now()))
		c = memlogger.Use(c)
		user.GetTestable(c).Login("noreply@luci-notify-test.appspotmail.com", "", false)

		// Add Project and Notifiers to datastore and update indexes.
		project := &config.Project{Name: "chromium"}
		builders := makeBuilders(c, "chromium", cfg)
		So(datastore.Put(c, project, builders), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		oldTime := time.Date(2015, 2, 3, 12, 54, 3, 0, time.UTC)
		newTime := time.Date(2015, 2, 3, 12, 58, 7, 0, time.UTC)

		dispatcher, tqTestable := createMockTaskQueue(c)
		assertTasks := func(build *Build, expectedRecipients ...EmailNotify) {
			// Test handleBuild.
			err := handleBuild(c, dispatcher, build, mockHistoryFunc(testCommits))
			So(err, ShouldBeNil)

			// Verify tasks were scheduled.
			var actualEmails []string
			for _, t := range tqTestable.GetScheduledTasks() {
				actualEmails = append(actualEmails, t.Payload.(*internal.EmailTask).Recipients...)
			}
			var expectedEmails []string
			for _, r := range expectedRecipients {
				expectedEmails = append(expectedEmails, r.Email)
			}
			sort.Strings(actualEmails)
			sort.Strings(expectedEmails)
			So(actualEmails, ShouldResemble, expectedEmails)
		}

		verifyBuilder := func(build *Build, revision string) {
			datastore.GetTestable(c).CatchupIndexes()
			id := getBuilderID(build)
			builder := config.Builder{
				ProjectKey: datastore.KeyForObj(c, project),
				ID:         id,
			}
			So(datastore.Get(c, &builder), ShouldBeNil)
			So(builder.StatusRevision, ShouldResemble, revision)
			So(builder.Status, ShouldEqual, build.Status)
		}

		propEmail := EmailNotify{
			Email: "property@google.com",
		}
		successEmail := EmailNotify{
			Email: "test-example-success@google.com",
		}
		failEmail := EmailNotify{
			Email: "test-example-failure@google.com",
		}
		changeEmail := EmailNotify{
			Email: "test-example-change@google.com",
		}

		grepLog := func(substring string) {
			buf := new(bytes.Buffer)
			_, err := memlogger.Dump(c, buf)
			So(err, ShouldBeNil)
			So(buf.String(), ShouldContainSubstring, substring)
		}

		Convey(`no config`, func() {
			build := pubsubDummyBuild("not-a-builder", buildbucketpb.Status_FAILURE, oldTime, rev1)
			assertTasks(build)
			grepLog("No builder")
		})

		Convey(`no config w/property`, func() {
			build := pubsubDummyBuild("not-a-builder", buildbucketpb.Status_FAILURE, oldTime, rev1, propEmail)
			assertTasks(build, propEmail)
		})

		Convey(`no repository in-order`, func() {
			build := pubsubDummyBuild("test-builder-no-repo", buildbucketpb.Status_FAILURE, oldTime, rev1)
			assertTasks(build, failEmail)
		})

		Convey(`no repository out-of-order`, func() {
			build := pubsubDummyBuild("test-builder-no-repo", buildbucketpb.Status_FAILURE, newTime, rev1)
			assertTasks(build, failEmail)

			newBuild := pubsubDummyBuild("test-builder-no-repo", buildbucketpb.Status_SUCCESS, oldTime, rev2)
			assertTasks(newBuild, failEmail, successEmail)
			grepLog("old time")
		})

		Convey(`no revision`, func() {
			build := &Build{
				Build: buildbucketpb.Build{
					Builder: &buildbucketpb.BuilderID{
						Project: "chromium",
						Bucket:  "ci",
						Builder: "test-builder-1",
					},
					Status: buildbucketpb.Status_SUCCESS,
				},
			}
			assertTasks(build)
			grepLog("revision")
		})

		Convey(`init builder`, func() {
			build := pubsubDummyBuild("test-builder-1", buildbucketpb.Status_FAILURE, oldTime, rev1)
			assertTasks(build, failEmail)
			verifyBuilder(build, rev1)
		})

		Convey(`init builder w/property`, func() {
			build := pubsubDummyBuild("test-builder-1", buildbucketpb.Status_FAILURE, oldTime, rev1, propEmail)
			assertTasks(build, failEmail, propEmail)
			verifyBuilder(build, rev1)
		})

		Convey(`repository mismatch`, func() {
			build := pubsubDummyBuild("test-builder-1", buildbucketpb.Status_FAILURE, oldTime, rev1, propEmail)
			assertTasks(build, failEmail, propEmail)
			verifyBuilder(build, rev1)

			newBuild := &Build{
				Build: buildbucketpb.Build{
					Builder: &buildbucketpb.BuilderID{
						Project: "chromium",
						Bucket:  "ci",
						Builder: "test-builder-1",
					},
					Status: buildbucketpb.Status_SUCCESS,
					Input: &buildbucketpb.Build_Input{
						GitilesCommit: &buildbucketpb.GitilesCommit{
							Host:    "chromium.googlesource.com",
							Project: "example/src",
							Id:      rev2,
						},
					},
				},
			}
			assertTasks(newBuild, failEmail, propEmail)
			grepLog("triggered by commit")
		})

		Convey(`out-of-order revision`, func() {
			build := pubsubDummyBuild("test-builder-2", buildbucketpb.Status_SUCCESS, oldTime, rev2)
			assertTasks(build, successEmail)
			verifyBuilder(build, rev2)

			oldRevBuild := pubsubDummyBuild("test-builder-2", buildbucketpb.Status_FAILURE, newTime, rev1)
			assertTasks(oldRevBuild, successEmail, failEmail) //no changeEmail
			grepLog("old commit")
		})

		Convey(`revision update`, func() {
			build := pubsubDummyBuild("test-builder-3", buildbucketpb.Status_SUCCESS, oldTime, rev1)
			assertTasks(build, successEmail)
			verifyBuilder(build, rev1)

			newBuild := pubsubDummyBuild("test-builder-3", buildbucketpb.Status_FAILURE, newTime, rev2)
			newBuild.Id++
			assertTasks(newBuild, successEmail, failEmail, changeEmail)
			verifyBuilder(newBuild, rev2)
		})

		Convey(`revision update w/property`, func() {
			build := pubsubDummyBuild("test-builder-3", buildbucketpb.Status_SUCCESS, oldTime, rev1, propEmail)
			assertTasks(build, successEmail, propEmail)
			verifyBuilder(build, rev1)

			newBuild := pubsubDummyBuild("test-builder-3", buildbucketpb.Status_FAILURE, newTime, rev2, propEmail)
			newBuild.Id++
			assertTasks(newBuild, successEmail, propEmail, failEmail, changeEmail, propEmail)
			verifyBuilder(newBuild, rev2)
		})

		Convey(`out-of-order creation time`, func() {
			build := pubsubDummyBuild("test-builder-4", buildbucketpb.Status_SUCCESS, newTime, rev1)
			build.Id = 2
			assertTasks(build, successEmail)
			verifyBuilder(build, rev1)

			oldBuild := pubsubDummyBuild("test-builder-4", buildbucketpb.Status_FAILURE, oldTime, rev1)
			oldBuild.Id = 1
			assertTasks(oldBuild, successEmail, failEmail) // no changeEmail
			grepLog("old time")
		})
	})
}

// mockHistoryFunc returns a mock HistoryFunc that gets its history from
// a given list of gitpb.Commit.
func mockHistoryFunc(mockCommits []*gitpb.Commit) HistoryFunc {
	return func(_ context.Context, _, _, oldRevision, newRevision string) ([]*gitpb.Commit, error) {
		oldCommit := -1
		newCommit := -1
		for i, c := range mockCommits {
			if c.Id == oldRevision {
				oldCommit = i
			}
			if c.Id == newRevision {
				newCommit = i
			}
		}
		if oldCommit == -1 || newCommit == -1 || newCommit < oldCommit {
			return nil, nil
		}
		commits := make([]*gitpb.Commit, newCommit-oldCommit+1)
		for i, j := newCommit, 0; i >= oldCommit; i, j = i-1, j+1 {
			commits[j] = mockCommits[i]
		}
		return commits, nil
	}
}

func makeBuilders(c context.Context, projectID string, cfg *apicfg.ProjectConfig) []*config.Builder {
	var builders []*config.Builder
	parentKey := datastore.MakeKey(c, "Project", projectID)
	for _, cfgNotifier := range cfg.Notifiers {
		for _, cfgBuilder := range cfgNotifier.Builders {
			builders = append(builders, &config.Builder{
				ProjectKey: parentKey,
				ID:         fmt.Sprintf("%s/%s", cfgBuilder.Bucket, cfgBuilder.Name),
				Repository: cfgBuilder.Repository,
				Notifications: apicfg.Notifications{
					Notifications: cfgNotifier.Notifications,
				},
			})
		}
	}
	return builders
}

func createMockTaskQueue(c context.Context) (*tq.Dispatcher, tqtesting.Testable) {
	dispatcher := &tq.Dispatcher{}
	InitDispatcher(dispatcher)
	taskqueue := tqtesting.GetTestable(c, dispatcher)
	taskqueue.CreateQueues()
	return dispatcher, taskqueue
}
