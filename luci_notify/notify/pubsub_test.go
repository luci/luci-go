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
	"compress/zlib"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging/memlogger"
	gitpb "go.chromium.org/luci/common/proto/git"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/tq"

	apicfg "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/luci_notify/common"
	"go.chromium.org/luci/luci_notify/config"
	"go.chromium.org/luci/luci_notify/internal"
	"go.chromium.org/luci/luci_notify/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func dummyBuildWithEmails(builder string, status buildbucketpb.Status, creationTime time.Time, revision string, notifyEmails ...EmailNotify) *Build {
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
	ret.Build.CreateTime = timestamppb.New(creationTime)
	return ret
}

func dummyBuildWithFailingSteps(status buildbucketpb.Status, failingSteps []string) *Build {
	build := &Build{
		Build: buildbucketpb.Build{
			Builder: &buildbucketpb.BuilderID{
				Project: "chromium",
				Bucket:  "ci",
				Builder: "test-builder-tree-closer",
			},
			Status: status,
			Input: &buildbucketpb.Build_Input{
				GitilesCommit: &buildbucketpb.GitilesCommit{
					Host:    defaultGitilesHost,
					Project: defaultGitilesProject,
					Id:      "deadbeef",
				},
			},
			EndTime: timestamppb.Now(),
		},
	}

	for _, stepName := range failingSteps {
		build.Build.Steps = append(build.Build.Steps, &buildbucketpb.Step{
			Name:   stepName,
			Status: buildbucketpb.Status_FAILURE,
		})
	}

	return build
}

func TestExtractEmailNotifyValues(t *testing.T) {
	Convey(`Test Environment for extractEmailNotifyValues`, t, func() {
		extract := func(buildJSONPB string) ([]EmailNotify, error) {
			build := &buildbucketpb.Build{}
			err := protojson.Unmarshal([]byte(buildJSONPB), build)
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

func init() {
	InitDispatcher(&tq.Default)
}

func TestHandleBuild(t *testing.T) {
	t.Parallel()

	Convey(`Test Environment for handleBuild`, t, func() {
		cfgName := "basic"
		cfg, err := testutil.LoadProjectConfig(cfgName)
		So(err, ShouldBeNil)

		c := memory.Use(context.Background())
		c = common.SetAppIDForTest(c, "luci-notify-test")
		c = caching.WithEmptyProcessCache(c)
		c = clock.Set(c, testclock.New(time.Now()))
		c = memlogger.Use(c)
		c, sched := tq.TestingContext(c, nil)

		// Add entities to datastore and update indexes.
		project := &config.Project{Name: "chromium"}
		builders := makeBuilders(c, "chromium", cfg)
		template := &config.EmailTemplate{
			ProjectKey:          datastore.KeyForObj(c, project),
			Name:                "template",
			SubjectTextTemplate: "Builder {{.Build.Builder.Builder}} failed on steps {{stepNames .MatchingFailedSteps}}",
		}
		So(datastore.Put(c, project, builders, template), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		oldTime := time.Date(2015, 2, 3, 12, 54, 3, 0, time.UTC)
		newTime := time.Date(2015, 2, 3, 12, 58, 7, 0, time.UTC)
		newTime2 := time.Date(2015, 2, 3, 12, 59, 8, 0, time.UTC)

		assertTasks := func(build *Build, checkoutFunc CheckoutFunc, expectedRecipients ...EmailNotify) {
			history := mockHistoryFunc(map[string][]*gitpb.Commit{
				"chromium/src":      testCommits,
				"third_party/hello": revTestCommits,
			})

			// Test handleBuild.
			err := handleBuild(c, build, checkoutFunc, history)
			So(err, ShouldBeNil)

			// Verify tasks were scheduled.
			var actualEmails []string
			for _, t := range sched.Tasks() {
				et := t.Payload.(*internal.EmailTask)
				actualEmails = append(actualEmails, et.Recipients...)
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
			id := getBuilderID(&build.Build)
			builder := config.Builder{
				ProjectKey: datastore.KeyForObj(c, project),
				ID:         id,
			}
			So(datastore.Get(c, &builder), ShouldBeNil)
			So(builder.Revision, ShouldResemble, revision)
			So(builder.Status, ShouldEqual, build.Status)
			expectCommits := checkout.ToGitilesCommits()
			So(builder.GitilesCommits, ShouldResembleProto, expectCommits)
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
		infraFailEmail := EmailNotify{
			Email: "test-example-infra-failure@google.com",
		}
		failAndInfraFailEmail := EmailNotify{
			Email: "test-example-failure-and-infra-failure@google.com",
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
			build := dummyBuildWithEmails("not-a-builder", buildbucketpb.Status_FAILURE, oldTime, rev1)
			assertTasks(build, mockCheckoutFunc(nil))
			grepLog("No builder")
		})

		Convey(`no config w/property`, func() {
			build := dummyBuildWithEmails("not-a-builder", buildbucketpb.Status_FAILURE, oldTime, rev1, propEmail)
			assertTasks(build, mockCheckoutFunc(nil), propEmail)
		})

		Convey(`no repository in-order`, func() {
			build := dummyBuildWithEmails("test-builder-no-repo", buildbucketpb.Status_FAILURE, oldTime, rev1)
			assertTasks(build, mockCheckoutFunc(nil), failEmail)
		})

		Convey(`no repository out-of-order`, func() {
			build := dummyBuildWithEmails("test-builder-no-repo", buildbucketpb.Status_FAILURE, newTime, rev1)
			assertTasks(build, mockCheckoutFunc(nil), failEmail)

			newBuild := dummyBuildWithEmails("test-builder-no-repo", buildbucketpb.Status_SUCCESS, oldTime, rev2)
			assertTasks(newBuild, mockCheckoutFunc(nil), failEmail, successEmail)
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
			assertTasks(build, mockCheckoutFunc(nil), successEmail)
			grepLog("revision")
		})

		Convey(`init builder`, func() {
			build := dummyBuildWithEmails("test-builder-1", buildbucketpb.Status_FAILURE, oldTime, rev1)
			assertTasks(build, mockCheckoutFunc(nil), failEmail)
			verifyBuilder(build, rev1, nil)
		})

		Convey(`init builder w/property`, func() {
			build := dummyBuildWithEmails("test-builder-1", buildbucketpb.Status_FAILURE, oldTime, rev1, propEmail)
			assertTasks(build, mockCheckoutFunc(nil), failEmail, propEmail)
			verifyBuilder(build, rev1, nil)
		})

		Convey(`source manifest return error`, func() {
			build := dummyBuildWithEmails("test-builder-1", buildbucketpb.Status_FAILURE, oldTime, rev1, propEmail)
			assertTasks(build, mockCheckoutReturnsErrorFunc(), failEmail, propEmail)
			verifyBuilder(build, rev1, nil)
			grepLog("Got error when getting source manifest for build")
		})

		Convey(`repository mismatch`, func() {
			build := dummyBuildWithEmails("test-builder-1", buildbucketpb.Status_FAILURE, oldTime, rev1, propEmail)
			assertTasks(build, mockCheckoutFunc(nil), failEmail, propEmail)
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
			assertTasks(newBuild, mockCheckoutFunc(nil), failEmail, propEmail, successEmail)
			grepLog("triggered by commit")
		})

		Convey(`out-of-order revision`, func() {
			build := dummyBuildWithEmails("test-builder-2", buildbucketpb.Status_SUCCESS, oldTime, rev2)
			assertTasks(build, mockCheckoutFunc(nil), successEmail)
			verifyBuilder(build, rev2, nil)

			oldRevBuild := dummyBuildWithEmails("test-builder-2", buildbucketpb.Status_FAILURE, newTime, rev1)
			assertTasks(oldRevBuild, mockCheckoutFunc(nil), successEmail, failEmail)
			grepLog("old commit")
		})

		Convey(`revision update`, func() {
			build := dummyBuildWithEmails("test-builder-3", buildbucketpb.Status_SUCCESS, oldTime, rev1)
			assertTasks(build, mockCheckoutFunc(nil), successEmail)
			verifyBuilder(build, rev1, nil)

			newBuild := dummyBuildWithEmails("test-builder-3", buildbucketpb.Status_FAILURE, newTime, rev2)
			newBuild.Id++
			assertTasks(newBuild, mockCheckoutFunc(nil), successEmail, failEmail, changeEmail)
			verifyBuilder(newBuild, rev2, nil)
		})

		Convey(`revision update w/property`, func() {
			build := dummyBuildWithEmails("test-builder-3", buildbucketpb.Status_SUCCESS, oldTime, rev1, propEmail)
			assertTasks(build, mockCheckoutFunc(nil), successEmail, propEmail)
			verifyBuilder(build, rev1, nil)

			newBuild := dummyBuildWithEmails("test-builder-3", buildbucketpb.Status_FAILURE, newTime, rev2, propEmail)
			newBuild.Id++
			assertTasks(newBuild, mockCheckoutFunc(nil), successEmail, propEmail, failEmail, changeEmail, propEmail)
			verifyBuilder(newBuild, rev2, nil)
		})

		Convey(`out-of-order creation time`, func() {
			build := dummyBuildWithEmails("test-builder-4", buildbucketpb.Status_SUCCESS, newTime, rev1)
			build.Id = 2
			assertTasks(build, mockCheckoutFunc(nil), successEmail)
			verifyBuilder(build, rev1, nil)

			oldBuild := dummyBuildWithEmails("test-builder-4", buildbucketpb.Status_FAILURE, oldTime, rev1)
			oldBuild.Id = 1
			assertTasks(oldBuild, mockCheckoutFunc(nil), successEmail, failEmail)
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
			build := dummyBuildWithEmails(builderID, buildbucketpb.Status_SUCCESS, oldTime, rev1)
			assertTasks(build, mockCheckoutFunc(checkoutOld))
			verifyBuilder(build, rev1, checkoutOld)

			newBuild := dummyBuildWithEmails(builderID, buildbucketpb.Status_FAILURE, newTime, rev2)
			newBuild.Id++
			assertTasks(newBuild, mockCheckoutFunc(checkoutNew), emails...)
			verifyBuilder(newBuild, rev2, checkoutNew)
		}

		Convey(`blamelist no allowlist`, func() {
			testBlamelistConfig("test-builder-blamelist-1", changeEmail, commit2Email)
		})

		Convey(`blamelist with allowlist`, func() {
			testBlamelistConfig("test-builder-blamelist-2", changeEmail, commit1Email)
		})

		Convey(`blamelist against last non-empty checkout`, func() {
			build := dummyBuildWithEmails("test-builder-blamelist-2", buildbucketpb.Status_SUCCESS, oldTime, rev1)
			assertTasks(build, mockCheckoutFunc(checkoutOld))
			verifyBuilder(build, rev1, checkoutOld)

			newBuild := dummyBuildWithEmails("test-builder-blamelist-2", buildbucketpb.Status_FAILURE, newTime, rev2)
			newBuild.Id++
			assertTasks(newBuild, mockCheckoutFunc(nil), changeEmail)
			verifyBuilder(newBuild, rev2, checkoutOld)

			newestTime := time.Date(2017, 2, 3, 12, 59, 9, 0, time.UTC)
			newestBuild := dummyBuildWithEmails("test-builder-blamelist-2", buildbucketpb.Status_SUCCESS, newestTime, rev2)
			newestBuild.Id++
			assertTasks(newestBuild, mockCheckoutFunc(checkoutNew), changeEmail, commit1Email)
			verifyBuilder(newestBuild, rev2, checkoutNew)
		})

		Convey(`blamelist mixed`, func() {
			testBlamelistConfig("test-builder-blamelist-3", commit1Email, commit2Email)
		})

		Convey(`blamelist duplicate`, func() {
			testBlamelistConfig("test-builder-blamelist-4", commit2Email, commit2Email, commit2Email)
		})

		Convey(`failure type infra`, func() {
			infra_failure_build := dummyBuildWithEmails("test-builder-infra-1", buildbucketpb.Status_SUCCESS, oldTime, rev2)
			assertTasks(infra_failure_build, mockCheckoutFunc(nil))

			infra_failure_build = dummyBuildWithEmails("test-builder-infra-1", buildbucketpb.Status_FAILURE, newTime, rev2)
			assertTasks(infra_failure_build, mockCheckoutFunc(nil))

			infra_failure_build = dummyBuildWithEmails("test-builder-infra-1", buildbucketpb.Status_INFRA_FAILURE, newTime2, rev2)
			assertTasks(infra_failure_build, mockCheckoutFunc(nil), infraFailEmail)
		})

		Convey(`failure type mixed`, func() {
			failure_and_infra_failure_build := dummyBuildWithEmails("test-builder-failure-and-infra-failures-1", buildbucketpb.Status_SUCCESS, oldTime, rev2)
			assertTasks(failure_and_infra_failure_build, mockCheckoutFunc(nil))

			failure_and_infra_failure_build = dummyBuildWithEmails("test-builder-failure-and-infra-failures-1", buildbucketpb.Status_FAILURE, newTime, rev2)
			assertTasks(failure_and_infra_failure_build, mockCheckoutFunc(nil), failAndInfraFailEmail)

			failure_and_infra_failure_build = dummyBuildWithEmails("test-builder-failure-and-infra-failures-1", buildbucketpb.Status_INFRA_FAILURE, newTime2, rev2)
			assertTasks(failure_and_infra_failure_build, mockCheckoutFunc(nil), failAndInfraFailEmail)
		})

		// Some arbitrary time guaranteed to be less than time.Now() when called from handleBuild.
		µs, _ := time.ParseDuration("1µs")
		initialTimestamp := time.Now().AddDate(-1, 0, 0).UTC().Round(µs)

		runHandleBuild := func(buildStatus buildbucketpb.Status, initialStatus config.TreeCloserStatus, failingSteps []string) *config.TreeCloser {
			// Insert the tree closer to test into datastore.
			builderKey := datastore.KeyForObj(c, &config.Builder{
				ProjectKey: datastore.KeyForObj(c, &config.Project{Name: "chromium"}),
				ID:         "ci/test-builder-tree-closer",
			})

			tc := &config.TreeCloser{
				BuilderKey: builderKey,
				TreeName:   "chromium-status.appspot.com",
				TreeCloser: apicfg.TreeCloser{
					FailedStepRegexp:        "include",
					FailedStepRegexpExclude: "exclude",
					Template:                "template",
				},
				Status:    initialStatus,
				Timestamp: initialTimestamp,
			}
			So(datastore.Put(c, tc), ShouldBeNil)

			// Handle a new build.
			build := dummyBuildWithFailingSteps(buildStatus, failingSteps)
			history := mockHistoryFunc(map[string][]*gitpb.Commit{})
			So(handleBuild(c, build, mockCheckoutFunc(nil), history), ShouldBeNil)

			// Fetch the new tree closer.
			So(datastore.Get(c, tc), ShouldBeNil)
			return tc
		}

		testStatus := func(buildStatus buildbucketpb.Status, initialStatus, expectedNewStatus config.TreeCloserStatus, expectingUpdatedTimestamp bool, failingSteps []string) {
			tc := runHandleBuild(buildStatus, initialStatus, failingSteps)

			// Assert the resulting state of the tree closer.
			So(tc.Status, ShouldEqual, expectedNewStatus)
			So(tc.Timestamp.After(initialTimestamp), ShouldEqual, expectingUpdatedTimestamp)
		}

		// We want to exhaustively test all combinations of the following:
		//   * Did the build succeed?
		//   * If not, do the filters (if any) match?
		//   * Is the resulting status the same as the old status?
		// All possibilities are explored in the tests below.

		Convey(`Build passed, Closed -> Open`, func() {
			testStatus(buildbucketpb.Status_SUCCESS, config.Closed, config.Open, true, []string{})
		})

		Convey(`Build passed, Open -> Open`, func() {
			testStatus(buildbucketpb.Status_SUCCESS, config.Open, config.Open, true, []string{})
		})

		Convey(`Build failed, filters don't match, Closed -> Open`, func() {
			testStatus(buildbucketpb.Status_FAILURE, config.Closed, config.Open, true, []string{"exclude"})
		})

		Convey(`Build failed, filters don't match, Open -> Open`, func() {
			testStatus(buildbucketpb.Status_FAILURE, config.Open, config.Open, true, []string{"exclude"})
		})

		Convey(`Build failed, filters match, Open -> Closed`, func() {
			testStatus(buildbucketpb.Status_FAILURE, config.Open, config.Closed, true, []string{"include"})
		})

		Convey(`Build failed, filters match, Closed -> Closed`, func() {
			testStatus(buildbucketpb.Status_FAILURE, config.Closed, config.Closed, true, []string{"include"})
		})

		// In addition, we want to test that statuses other than SUCCESS and FAILURE don't
		// cause any updates, regardless of the initial state.

		Convey(`Infra failure, stays Open`, func() {
			testStatus(buildbucketpb.Status_INFRA_FAILURE, config.Open, config.Open, false, []string{"include"})
		})

		Convey(`Infra failure, stays Closed`, func() {
			testStatus(buildbucketpb.Status_INFRA_FAILURE, config.Closed, config.Closed, false, []string{"include"})
		})

		// Test that the correct status message is generated.
		Convey(`Status message`, func() {
			tc := runHandleBuild(buildbucketpb.Status_FAILURE, config.Open, []string{"include"})

			So(tc.Message, ShouldEqual, `Builder test-builder-tree-closer failed on steps "include"`)
		})

		Convey(`All failed steps listed if no filter`, func() {
			// Insert the tree closer to test into datastore.
			builderKey := datastore.KeyForObj(c, &config.Builder{
				ProjectKey: datastore.KeyForObj(c, &config.Project{Name: "chromium"}),
				ID:         "ci/test-builder-tree-closer",
			})

			tc := &config.TreeCloser{
				BuilderKey: builderKey,
				TreeName:   "chromium-status.appspot.com",
				TreeCloser: apicfg.TreeCloser{Template: "template"},
				Status:     config.Open,
				Timestamp:  initialTimestamp,
			}
			So(datastore.Put(c, tc), ShouldBeNil)

			// Handle a new build.
			build := dummyBuildWithFailingSteps(buildbucketpb.Status_FAILURE, []string{"step1", "step2"})
			history := mockHistoryFunc(map[string][]*gitpb.Commit{})
			So(handleBuild(c, build, mockCheckoutFunc(nil), history), ShouldBeNil)

			// Fetch the new tree closer.
			So(datastore.Get(c, tc), ShouldBeNil)

			So(tc.Message, ShouldEqual, `Builder test-builder-tree-closer failed on steps "step1", "step2"`)
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

func mockCheckoutReturnsErrorFunc() CheckoutFunc {
	return func(_ context.Context, _ *Build) (Checkout, error) {
		return nil, errors.New("Some error")
	}
}

func TestExtractBuild(t *testing.T) {
	t.Parallel()

	Convey("builds_v2 pubsub message", t, func() {
		Convey("success", func() {
			ctx := memory.Use(context.Background())
			props := &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"email_notify": structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{
						structpb.NewStructValue(&structpb.Struct{
							Fields: map[string]*structpb.Value{
								"email": {
									Kind: &structpb.Value_StringValue{
										StringValue: "abc@gmail.com",
									},
								},
							},
						}),
					}}),
				},
			}
			originalBuild := &buildbucketpb.Build{
				Id: 123,
				Builder: &buildbucketpb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status: buildbucketpb.Status_SUCCESS,
				Infra: &buildbucketpb.BuildInfra{
					Buildbucket: &buildbucketpb.BuildInfra_Buildbucket{
						Hostname: "buildbuckt.com",
					},
				},
				Input: &buildbucketpb.Build_Input{},
				Output: &buildbucketpb.Build_Output{
					Properties: props,
				},
				Steps: []*buildbucketpb.Step{{Name: "step1"}},
			}
			pubsubMsg, err := makeBuildsV2PubsubMsg(originalBuild)
			So(err, ShouldBeNil)
			b, err := extractBuild(ctx, &http.Request{Body: pubsubMsg})
			So(err, ShouldBeNil)
			So(b.Id, ShouldEqual, originalBuild.Id)
			So(b.Builder, ShouldResembleProto, originalBuild.Builder)
			So(b.Status, ShouldEqual, buildbucketpb.Status_SUCCESS)
			So(b.Infra, ShouldResembleProto, originalBuild.Infra)
			So(b.Input, ShouldResembleProto, originalBuild.Input)
			So(b.Output, ShouldResembleProto, originalBuild.Output)
			So(b.Steps, ShouldResembleProto, originalBuild.Steps)
			So(b.BuildbucketHostname, ShouldEqual, originalBuild.Infra.Buildbucket.Hostname)
			So(b.EmailNotify, ShouldResemble, []EmailNotify{{Email: "abc@gmail.com"}})
		})

		Convey("success with no email_notify field", func() {
			ctx := memory.Use(context.Background())
			props := &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"other": structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{
						structpb.NewStructValue(&structpb.Struct{
							Fields: map[string]*structpb.Value{
								"other": {
									Kind: &structpb.Value_StringValue{
										StringValue: "other",
									},
								},
							},
						}),
					}}),
				},
			}
			originalBuild := &buildbucketpb.Build{
				Id: 123,
				Builder: &buildbucketpb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status: buildbucketpb.Status_CANCELED,
				Infra: &buildbucketpb.BuildInfra{
					Buildbucket: &buildbucketpb.BuildInfra_Buildbucket{
						Hostname: "buildbuckt.com",
					},
				},
				Input: &buildbucketpb.Build_Input{},
				Output: &buildbucketpb.Build_Output{
					Properties: props,
				},
				Steps: []*buildbucketpb.Step{{Name: "step1"}},
			}
			pubsubMsg, err := makeBuildsV2PubsubMsg(originalBuild)
			So(err, ShouldBeNil)
			b, err := extractBuild(ctx, &http.Request{Body: pubsubMsg})
			So(err, ShouldBeNil)
			So(b.Id, ShouldEqual, originalBuild.Id)
			So(b.Builder, ShouldResembleProto, originalBuild.Builder)
			So(b.Status, ShouldEqual, buildbucketpb.Status_CANCELED)
			So(b.Infra, ShouldResembleProto, originalBuild.Infra)
			So(b.Input, ShouldResembleProto, originalBuild.Input)
			So(b.Output, ShouldResembleProto, originalBuild.Output)
			So(b.Steps, ShouldResembleProto, originalBuild.Steps)
			So(b.BuildbucketHostname, ShouldEqual, originalBuild.Infra.Buildbucket.Hostname)
			So(b.EmailNotify, ShouldBeNil)
		})

		Convey("incompleted build", func() {
			ctx := memory.Use(context.Background())
			originalBuild := &buildbucketpb.Build{
				Id: 123,
				Builder: &buildbucketpb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status: buildbucketpb.Status_SCHEDULED,
				Infra: &buildbucketpb.BuildInfra{
					Buildbucket: &buildbucketpb.BuildInfra_Buildbucket{
						Hostname: "buildbuckt.com",
					},
				},
				Input:  &buildbucketpb.Build_Input{},
				Output: &buildbucketpb.Build_Output{},
				Steps:  []*buildbucketpb.Step{{Name: "step1"}},
			}
			pubsubMsg, err := makeBuildsV2PubsubMsg(originalBuild)
			So(err, ShouldBeNil)
			b, err := extractBuild(ctx, &http.Request{Body: pubsubMsg})
			So(err, ShouldBeNil)
			So(b, ShouldBeNil)
		})

	})
}

func makeBuildsV2PubsubMsg(b *buildbucketpb.Build) (io.ReadCloser, error) {
	copyB := proto.Clone(b).(*buildbucketpb.Build)
	large := &buildbucketpb.Build{
		Input: &buildbucketpb.Build_Input{
			Properties: copyB.GetInput().GetProperties(),
		},
		Output: &buildbucketpb.Build_Output{
			Properties: copyB.GetOutput().GetProperties(),
		},
		Steps: copyB.GetSteps(),
	}
	if copyB.Input != nil {
		copyB.Input.Properties = nil
	}
	if copyB.Output != nil {
		copyB.Output.Properties = nil
	}
	copyB.Steps = nil
	compress := func(data []byte) ([]byte, error) {
		buf := &bytes.Buffer{}
		zw := zlib.NewWriter(buf)
		if _, err := zw.Write(data); err != nil {
			return nil, errors.Annotate(err, "failed to compress").Err()
		}
		if err := zw.Close(); err != nil {
			return nil, errors.Annotate(err, "error closing zlib writer").Err()
		}
		return buf.Bytes(), nil
	}
	largeBytes, err := proto.Marshal(large)
	if err != nil {
		return nil, errors.Annotate(err, "failed to marshal large").Err()
	}
	compressedLarge, err := compress(largeBytes)
	if err != nil {
		return nil, err
	}
	data, _ := protojson.Marshal(&buildbucketpb.BuildsV2PubSub{
		Build:            copyB,
		BuildLargeFields: compressedLarge,
	})
	isCompleted := copyB.Status&buildbucketpb.Status_ENDED_MASK == buildbucketpb.Status_ENDED_MASK
	attrs := map[string]any{
		"project":      copyB.Builder.GetProject(),
		"bucket":       copyB.Builder.GetBucket(),
		"builder":      copyB.Builder.GetBuilder(),
		"is_completed": strconv.FormatBool(isCompleted),
		"version":      "v2",
	}
	msg := struct {
		Message struct {
			Data       string
			Attributes map[string]any
		}
	}{struct {
		Data       string
		Attributes map[string]any
	}{Data: base64.StdEncoding.EncodeToString(data), Attributes: attrs}}
	jmsg, _ := json.Marshal(msg)
	return io.NopCloser(bytes.NewReader(jmsg)), nil
}
