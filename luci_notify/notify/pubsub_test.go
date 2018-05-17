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

	"github.com/golang/protobuf/ptypes"
	"golang.org/x/net/context"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/user"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging/memlogger"
	gitpb "go.chromium.org/luci/common/proto/git"

	config "go.chromium.org/luci/config"
	memImpl "go.chromium.org/luci/config/impl/memory"
	notifyConfig "go.chromium.org/luci/luci_notify/config"
	"go.chromium.org/luci/luci_notify/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

var (
	rev1        = testutil.TestRevision("some random data here")
	rev2        = testutil.TestRevision("other random string")
	testCommits = []*gitpb.Commit{
		{Id: rev1},
		{Id: rev2},
	}
)

func pubsubDummyBuild(builder string, status buildbucketpb.Status, creationTime time.Time, revision string, notifyEmails ...EmailNotify) *Build {
	var build Build
	build.Build = *testutil.TestBuild("test", "hello", builder, status)
	build.Input = &buildbucketpb.Build_Input{
		GitilesCommits: []*buildbucketpb.GitilesCommit{
			{
				Host:    "test.googlesource.com",
				Project: "test",
				Id:      revision,
			},
		},
	}
	build.CreateTime, _ = ptypes.TimestampProto(creationTime)
	for _, en := range notifyEmails {
		build.EmailNotify = append(build.EmailNotify, en)
	}
	return &build
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
	//t.Parallel()

	Convey(`Test Environment for handleBuild`, t, func() {
		cfgName := "basic"
		cfg, err := testutil.LoadProjectConfig(cfgName)
		So(err, ShouldBeNil)

		c := memory.UseWithAppID(context.Background(), "luci-notify-test")
		c = clock.Set(c, testclock.New(time.Now()))
		c = memlogger.Use(c)
		user.GetTestable(c).Login("noreply@luci-notify-test.appspotmail.com", "", false)
		impl := memImpl.New(map[config.Set]memImpl.Files{
			"projects/proj1": {
				"file": "file content",
				"email_templates/minimal.template": "Subject: Test Subject\nTest Content",
			},
		})
		c = notifyConfig.WithConfigService(c, impl)

		// Add Notifiers to datastore and update indexes.
		notifiers := extractNotifiers(c, "test", cfg)
		for _, n := range notifiers {
			datastore.Put(c, n)
		}
		datastore.GetTestable(c).CatchupIndexes()

		oldTime := time.Date(2015, 2, 3, 12, 54, 3, 0, time.UTC)
		newTime := time.Date(2015, 2, 3, 12, 58, 7, 0, time.UTC)

		dispatcher, taskqueue := createMockTaskQueue(c)
		testSuccess := func(build *Build, emailExpect ...EmailNotify) {
			// Test handleBuild.
			err := handleBuild(c, dispatcher, build, mockHistoryFunc(testCommits))
			So(err, ShouldBeNil)

			// Flatten email notify array of arrays
			notifyEmail := []EmailNotify{}
			for _, en := range emailExpect {
				notifyEmail = append(notifyEmail, en)
			}

			// Verify sent messages.
			verifyTasksAndMessages(c, taskqueue, notifyEmail)
		}

		verifyBuilder := func(build *Build, revision string) {
			datastore.GetTestable(c).CatchupIndexes()
			id := getBuilderID(build)
			builder := Builder{ID: id}
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
			So(strings.Contains(buf.String(), substring), ShouldEqual, true)
		}

		Convey(`no config`, func() {
			build := pubsubDummyBuild("not-a-builder", buildbucketpb.Status_FAILURE, oldTime, rev1)
			testSuccess(build)
			grepLog("Nobody to notify")
		})

		Convey(`no config w/property`, func() {
			build := pubsubDummyBuild("not-a-builder", buildbucketpb.Status_FAILURE, oldTime, rev1, propEmail)
			testSuccess(build, propEmail)
		})

		Convey(`no revision`, func() {
			build := &Build{Build: *testutil.TestBuild("test", "hello", "test-builder-1", buildbucketpb.Status_SUCCESS)}
			testSuccess(build)
			grepLog("revision")
		})

		Convey(`init builder`, func() {
			build := pubsubDummyBuild("test-builder-1", buildbucketpb.Status_FAILURE, oldTime, rev1)
			testSuccess(build, failEmail)
			verifyBuilder(build, rev1)
		})

		Convey(`init builder w/property`, func() {
			build := pubsubDummyBuild("test-builder-1", buildbucketpb.Status_FAILURE, oldTime, rev1, propEmail)
			testSuccess(build, failEmail, propEmail)
			verifyBuilder(build, rev1)
		})

		Convey(`out-of-order revision`, func() {
			build := pubsubDummyBuild("test-builder-2", buildbucketpb.Status_SUCCESS, oldTime, rev2)
			testSuccess(build, successEmail)
			verifyBuilder(build, rev2)

			oldRevBuild := pubsubDummyBuild("test-builder-2", buildbucketpb.Status_FAILURE, newTime, rev1)
			testSuccess(oldRevBuild, failEmail)
			grepLog("old commit")
		})

		Convey(`revision update`, func() {
			build := pubsubDummyBuild("test-builder-3", buildbucketpb.Status_SUCCESS, oldTime, rev1)
			testSuccess(build, successEmail)
			verifyBuilder(build, rev1)

			newBuild := pubsubDummyBuild("test-builder-3", buildbucketpb.Status_FAILURE, newTime, rev2)
			testSuccess(newBuild, failEmail, changeEmail)
			verifyBuilder(newBuild, rev2)
		})

		Convey(`revision update w/property`, func() {
			build := pubsubDummyBuild("test-builder-3", buildbucketpb.Status_SUCCESS, oldTime, rev1, propEmail)
			testSuccess(build, successEmail, propEmail)
			verifyBuilder(build, rev1)

			newBuild := pubsubDummyBuild("test-builder-3", buildbucketpb.Status_FAILURE, newTime, rev2, propEmail)
			testSuccess(newBuild, failEmail, changeEmail, propEmail)
			verifyBuilder(newBuild, rev2)
		})

		Convey(`out-of-order creation time`, func() {
			build := pubsubDummyBuild("test-builder-4", buildbucketpb.Status_SUCCESS, newTime, rev1)
			testSuccess(build, successEmail)
			verifyBuilder(build, rev1)

			oldBuild := pubsubDummyBuild("test-builder-4", buildbucketpb.Status_FAILURE, oldTime, rev1)
			testSuccess(oldBuild, failEmail)
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
