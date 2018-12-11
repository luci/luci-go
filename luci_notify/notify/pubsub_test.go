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
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/ptypes"

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
	. "go.chromium.org/luci/common/testing/assertions"
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
					Host:    defaultGitilesHost,
					Project: defaultGitilesProject,
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
		extract := func(buildJSONPB string) ([]EmailNotify, error) {
			build := &buildbucketpb.Build{}
			err := jsonpb.UnmarshalString(buildJSONPB, build)
			So(err, ShouldBeNil)
			return extractEmailNotifyValues(build, "")
		}

		Convey(`empty`, func() {
			results, err := extract(`{}`)
			So(err, ShouldBeNil)
			So(results, ShouldHaveLength, 0)
		})

		Convey(`populated without email_notify`, func() {
			results, err := extract(`{
				"input": {
					"properties": {
						"foo": 1
					}
				}
			}`)
			So(err, ShouldBeNil)
			So(results, ShouldHaveLength, 0)
		})

		Convey(`single email_notify value in input`, func() {
			results, err := extract(`{
				"input": {
					"properties": {
						"email_notify": [{"email": "test@email"}]
					}
				}
			}`)
			So(err, ShouldBeNil)
			So(results, ShouldResemble, []EmailNotify{
				{
					Email:    "test@email",
					Template: "",
				},
			})
		})

		Convey(`single email_notify value_with_template`, func() {
			results, err := extract(`{
				"input": {
					"properties": {
						"email_notify": [{
							"email": "test@email",
							"template": "test-template"
						}]
					}
				}
			}`)
			So(err, ShouldBeNil)
			So(results, ShouldResemble, []EmailNotify{
				{
					Email:    "test@email",
					Template: "test-template",
				},
			})
		})

		Convey(`multiple email_notify values`, func() {
			results, err := extract(`{
				"input": {
					"properties": {
						"email_notify": [
							{"email": "test@email"},
							{"email": "test2@email"}
						]
					}
				}
			}`)
			So(err, ShouldBeNil)
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
		})

		Convey(`output takes precedence`, func() {
			results, err := extract(`{
				"input": {
					"properties": {
						"email_notify": [
							{"email": "test@email"}
						]
					}
				},
				"output": {
					"properties": {
						"email_notify": [
							{"email": "test2@email"}
						]
					}
				}
			}`)
			So(err, ShouldBeNil)
			So(results, ShouldResemble, []EmailNotify{
				{
					Email:    "test2@email",
					Template: "",
				},
			})
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
		assertTasks := func(build *Build, checkout Checkout, expectedRecipients ...EmailNotify) {
			history := mockHistoryFunc(map[string][]*gitpb.Commit{
				"chromium/src":      testCommits,
				"third_party/hello": revTestCommits,
			})
			// Test handleBuild.
			err := handleBuild(c, dispatcher, build, mockCheckoutFunc(checkout), history)
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

		verifyBuilder := func(build *Build, revision string, checkout Checkout) {
			datastore.GetTestable(c).CatchupIndexes()
			id := getBuilderID(build)
			builder := config.Builder{
				ProjectKey: datastore.KeyForObj(c, project),
				ID:         id,
			}
			So(datastore.Get(c, &builder), ShouldBeNil)
			So(builder.Revision, ShouldResemble, revision)
			So(builder.Status, ShouldEqual, build.Status)
			expectCommits := checkout.ToGitilesCommits()
			So(&builder.GitilesCommits, ShouldResembleProto, &expectCommits)
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
		commit1Email := EmailNotify{
			Email: commitEmail1,
		}
		commit2Email := EmailNotify{
			Email: commitEmail2,
		}

		grepLog := func(substring string) {
			buf := new(bytes.Buffer)
			_, err := memlogger.Dump(c, buf)
			So(err, ShouldBeNil)
			So(buf.String(), ShouldContainSubstring, substring)
		}

		Convey(`no config`, func() {
			build := pubsubDummyBuild("not-a-builder", buildbucketpb.Status_FAILURE, oldTime, rev1)
			assertTasks(build, nil)
			grepLog("No builder")
		})

		Convey(`no config w/property`, func() {
			build := pubsubDummyBuild("not-a-builder", buildbucketpb.Status_FAILURE, oldTime, rev1, propEmail)
			assertTasks(build, nil, propEmail)
		})

		Convey(`no repository in-order`, func() {
			build := pubsubDummyBuild("test-builder-no-repo", buildbucketpb.Status_FAILURE, oldTime, rev1)
			assertTasks(build, nil, failEmail)
		})

		Convey(`no repository out-of-order`, func() {
			build := pubsubDummyBuild("test-builder-no-repo", buildbucketpb.Status_FAILURE, newTime, rev1)
			assertTasks(build, nil, failEmail)

			newBuild := pubsubDummyBuild("test-builder-no-repo", buildbucketpb.Status_SUCCESS, oldTime, rev2)
			assertTasks(newBuild, nil, failEmail, successEmail)
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
			assertTasks(build, nil)
			grepLog("revision")
		})

		Convey(`init builder`, func() {
			build := pubsubDummyBuild("test-builder-1", buildbucketpb.Status_FAILURE, oldTime, rev1)
			assertTasks(build, nil, failEmail)
			verifyBuilder(build, rev1, nil)
		})

		Convey(`init builder w/property`, func() {
			build := pubsubDummyBuild("test-builder-1", buildbucketpb.Status_FAILURE, oldTime, rev1, propEmail)
			assertTasks(build, nil, failEmail, propEmail)
			verifyBuilder(build, rev1, nil)
		})

		Convey(`repository mismatch`, func() {
			build := pubsubDummyBuild("test-builder-1", buildbucketpb.Status_FAILURE, oldTime, rev1, propEmail)
			assertTasks(build, nil, failEmail, propEmail)
			verifyBuilder(build, rev1, nil)

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
							Host:    defaultGitilesHost,
							Project: "example/src",
							Id:      rev2,
						},
					},
				},
			}
			assertTasks(newBuild, nil, failEmail, propEmail)
			grepLog("triggered by commit")
		})

		Convey(`out-of-order revision`, func() {
			build := pubsubDummyBuild("test-builder-2", buildbucketpb.Status_SUCCESS, oldTime, rev2)
			assertTasks(build, nil, successEmail)
			verifyBuilder(build, rev2, nil)

			oldRevBuild := pubsubDummyBuild("test-builder-2", buildbucketpb.Status_FAILURE, newTime, rev1)
			assertTasks(oldRevBuild, nil, successEmail, failEmail) //no changeEmail
			grepLog("old commit")
		})

		Convey(`revision update`, func() {
			build := pubsubDummyBuild("test-builder-3", buildbucketpb.Status_SUCCESS, oldTime, rev1)
			assertTasks(build, nil, successEmail)
			verifyBuilder(build, rev1, nil)

			newBuild := pubsubDummyBuild("test-builder-3", buildbucketpb.Status_FAILURE, newTime, rev2)
			newBuild.Id++
			assertTasks(newBuild, nil, successEmail, failEmail, changeEmail)
			verifyBuilder(newBuild, rev2, nil)
		})

		Convey(`revision update w/property`, func() {
			build := pubsubDummyBuild("test-builder-3", buildbucketpb.Status_SUCCESS, oldTime, rev1, propEmail)
			assertTasks(build, nil, successEmail, propEmail)
			verifyBuilder(build, rev1, nil)

			newBuild := pubsubDummyBuild("test-builder-3", buildbucketpb.Status_FAILURE, newTime, rev2, propEmail)
			newBuild.Id++
			assertTasks(newBuild, nil, successEmail, propEmail, failEmail, changeEmail, propEmail)
			verifyBuilder(newBuild, rev2, nil)
		})

		Convey(`out-of-order creation time`, func() {
			build := pubsubDummyBuild("test-builder-4", buildbucketpb.Status_SUCCESS, newTime, rev1)
			build.Id = 2
			assertTasks(build, nil, successEmail)
			verifyBuilder(build, rev1, nil)

			oldBuild := pubsubDummyBuild("test-builder-4", buildbucketpb.Status_FAILURE, oldTime, rev1)
			oldBuild.Id = 1
			assertTasks(oldBuild, nil, successEmail, failEmail) // no changeEmail
			grepLog("old time")
		})

		checkoutOld := Checkout{
			"https://chromium.googlesource.com/chromium/src":      rev1,
			"https://chromium.googlesource.com/third_party/hello": rev1,
		}
		checkoutNew := Checkout{
			"https://chromium.googlesource.com/chromium/src":      rev2,
			"https://chromium.googlesource.com/third_party/hello": rev2,
		}

		testBlamelistConfig := func(builderID string, emails ...EmailNotify) {
			build := pubsubDummyBuild(builderID, buildbucketpb.Status_SUCCESS, oldTime, rev1)
			assertTasks(build, checkoutOld)
			verifyBuilder(build, rev1, checkoutOld)

			newBuild := pubsubDummyBuild(builderID, buildbucketpb.Status_FAILURE, newTime, rev2)
			newBuild.Id++
			assertTasks(newBuild, checkoutNew, emails...)
			verifyBuilder(newBuild, rev2, checkoutNew)
		}

		Convey(`blamelist no whitelist`, func() {
			testBlamelistConfig("test-builder-blamelist-1", changeEmail, commit2Email)
		})

		Convey(`blamelist with whitelist`, func() {
			testBlamelistConfig("test-builder-blamelist-2", changeEmail, commit1Email)
		})

		Convey(`blamelist against last non-empty checkout`, func() {
			build := pubsubDummyBuild("test-builder-blamelist-2", buildbucketpb.Status_SUCCESS, oldTime, rev1)
			assertTasks(build, checkoutOld)
			verifyBuilder(build, rev1, checkoutOld)

			newBuild := pubsubDummyBuild("test-builder-blamelist-2", buildbucketpb.Status_FAILURE, newTime, rev2)
			newBuild.Id++
			assertTasks(newBuild, nil, changeEmail)
			verifyBuilder(newBuild, rev2, checkoutOld)

			newestTime := time.Date(2017, 2, 3, 12, 59, 9, 0, time.UTC)
			newestBuild := pubsubDummyBuild("test-builder-blamelist-2", buildbucketpb.Status_SUCCESS, newestTime, rev2)
			newestBuild.Id++
			assertTasks(newestBuild, checkoutNew, changeEmail, commit1Email)
			verifyBuilder(newestBuild, rev2, checkoutNew)
		})

		Convey(`blamelist mixed`, func() {
			testBlamelistConfig("test-builder-blamelist-3", commit1Email, commit2Email)
		})

		Convey(`blamelist duplicate`, func() {
			testBlamelistConfig("test-builder-blamelist-4", commit2Email, commit2Email, commit2Email)
		})
	})
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

func mockCheckoutFunc(c Checkout) CheckoutFunc {
	return func(_ context.Context, _ *Build) (Checkout, error) {
		return c, nil
	}
}

func createMockTaskQueue(c context.Context) (*tq.Dispatcher, tqtesting.Testable) {
	dispatcher := &tq.Dispatcher{}
	InitDispatcher(dispatcher)
	taskqueue := tqtesting.GetTestable(c, dispatcher)
	taskqueue.CreateQueues()
	return dispatcher, taskqueue
}
