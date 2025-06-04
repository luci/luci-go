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
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/prpc/prpcpb"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/tq"

	apicfg "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/luci_notify/common"
	"go.chromium.org/luci/luci_notify/config"
	"go.chromium.org/luci/luci_notify/internal"
	"go.chromium.org/luci/luci_notify/testutil"
)

func dummyBuildWithEmails(builder string, status buildbucketpb.Status, creationTime time.Time, revision string, notifyEmails ...EmailNotify) *Build {
	ret := &Build{
		Build: &buildbucketpb.Build{
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
		Build: &buildbucketpb.Build{
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
	ftt.Run(`Test Environment for extractEmailNotifyValues`, t, func(t *ftt.Test) {
		extract := func(buildJSONPB string) ([]EmailNotify, error) {
			build := &buildbucketpb.Build{}
			err := protojson.Unmarshal([]byte(buildJSONPB), build)
			assert.Loosely(t, err, should.BeNil)
			return extractEmailNotifyValues(build, "")
		}

		t.Run(`empty`, func(t *ftt.Test) {
			results, err := extract(`{}`)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, results, should.HaveLength(0))
		})

		t.Run(`populated without email_notify`, func(t *ftt.Test) {
			results, err := extract(`{
				"input": {
					"properties": {
						"foo": 1
					}
				}
			}`)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, results, should.HaveLength(0))
		})

		t.Run(`single email_notify value in input`, func(t *ftt.Test) {
			results, err := extract(`{
				"input": {
					"properties": {
						"email_notify": [{"email": "test@email"}]
					}
				}
			}`)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, results, should.Match([]EmailNotify{
				{
					Email:    "test@email",
					Template: "",
				},
			}))
		})

		t.Run(`single email_notify value_with_template`, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, results, should.Match([]EmailNotify{
				{
					Email:    "test@email",
					Template: "test-template",
				},
			}))
		})

		t.Run(`multiple email_notify values`, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, results, should.Match([]EmailNotify{
				{
					Email:    "test@email",
					Template: "",
				},
				{
					Email:    "test2@email",
					Template: "",
				},
			}))
		})

		t.Run(`output takes precedence`, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, results, should.Match([]EmailNotify{
				{
					Email:    "test2@email",
					Template: "",
				},
			}))
		})
	})
}

func init() {
	InitDispatcher(&tq.Default)
}

func TestHandleBuild(t *testing.T) {
	t.Parallel()

	ftt.Run(`Test Environment for handleBuild`, t, func(t *ftt.Test) {
		cfgName := "basic"
		cfg, err := testutil.LoadProjectConfig(cfgName)
		assert.Loosely(t, err, should.BeNil)

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
		assert.Loosely(t, datastore.Put(c, project, builders, template), should.BeNil)
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
			assert.Loosely(t, err, should.BeNil)

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
			assert.Loosely(t, actualEmails, should.Match(expectedEmails))
		}

		verifyBuilder := func(build *Build, revision string, checkout Checkout) {
			datastore.GetTestable(c).CatchupIndexes()
			id := getBuilderID(build.Build)
			builder := config.Builder{
				ProjectKey: datastore.KeyForObj(c, project),
				ID:         id,
			}
			assert.Loosely(t, datastore.Get(c, &builder), should.BeNil)
			assert.Loosely(t, builder.Revision, should.Match(revision))
			assert.Loosely(t, builder.Status, should.Equal(build.Status))
			expectCommits := checkout.ToGitilesCommits()
			assert.Loosely(t, builder.GitilesCommits, should.Match(expectCommits))
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf.String(), should.ContainSubstring(substring))
		}

		t.Run(`no config`, func(t *ftt.Test) {
			build := dummyBuildWithEmails("not-a-builder", buildbucketpb.Status_FAILURE, oldTime, rev1)
			assertTasks(build, mockCheckoutFunc(nil))
			grepLog("No builder")
		})

		t.Run(`no config w/property`, func(t *ftt.Test) {
			build := dummyBuildWithEmails("not-a-builder", buildbucketpb.Status_FAILURE, oldTime, rev1, propEmail)
			assertTasks(build, mockCheckoutFunc(nil), propEmail)
		})

		t.Run(`no repository in-order`, func(t *ftt.Test) {
			build := dummyBuildWithEmails("test-builder-no-repo", buildbucketpb.Status_FAILURE, oldTime, rev1)
			assertTasks(build, mockCheckoutFunc(nil), failEmail)
		})

		t.Run(`no repository out-of-order`, func(t *ftt.Test) {
			build := dummyBuildWithEmails("test-builder-no-repo", buildbucketpb.Status_FAILURE, newTime, rev1)
			assertTasks(build, mockCheckoutFunc(nil), failEmail)

			newBuild := dummyBuildWithEmails("test-builder-no-repo", buildbucketpb.Status_SUCCESS, oldTime, rev2)
			assertTasks(newBuild, mockCheckoutFunc(nil), failEmail, successEmail)
			grepLog("old time")
		})

		t.Run(`no revision`, func(t *ftt.Test) {
			build := &Build{
				Build: &buildbucketpb.Build{
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

		t.Run(`init builder`, func(t *ftt.Test) {
			build := dummyBuildWithEmails("test-builder-1", buildbucketpb.Status_FAILURE, oldTime, rev1)
			assertTasks(build, mockCheckoutFunc(nil), failEmail)
			verifyBuilder(build, rev1, nil)
		})

		t.Run(`init builder w/property`, func(t *ftt.Test) {
			build := dummyBuildWithEmails("test-builder-1", buildbucketpb.Status_FAILURE, oldTime, rev1, propEmail)
			assertTasks(build, mockCheckoutFunc(nil), failEmail, propEmail)
			verifyBuilder(build, rev1, nil)
		})

		t.Run(`source manifest return error`, func(t *ftt.Test) {
			build := dummyBuildWithEmails("test-builder-1", buildbucketpb.Status_FAILURE, oldTime, rev1, propEmail)
			assertTasks(build, mockCheckoutReturnsErrorFunc(), failEmail, propEmail)
			verifyBuilder(build, rev1, nil)
			grepLog("Got error when getting source manifest for build")
		})

		t.Run(`repository mismatch`, func(t *ftt.Test) {
			build := dummyBuildWithEmails("test-builder-1", buildbucketpb.Status_FAILURE, oldTime, rev1, propEmail)
			assertTasks(build, mockCheckoutFunc(nil), failEmail, propEmail)
			verifyBuilder(build, rev1, nil)

			newBuild := &Build{
				Build: &buildbucketpb.Build{
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

		t.Run(`out-of-order revision`, func(t *ftt.Test) {
			build := dummyBuildWithEmails("test-builder-2", buildbucketpb.Status_SUCCESS, oldTime, rev2)
			assertTasks(build, mockCheckoutFunc(nil), successEmail)
			verifyBuilder(build, rev2, nil)

			oldRevBuild := dummyBuildWithEmails("test-builder-2", buildbucketpb.Status_FAILURE, newTime, rev1)
			assertTasks(oldRevBuild, mockCheckoutFunc(nil), successEmail, failEmail)
			grepLog("old commit")
		})

		t.Run(`revision update`, func(t *ftt.Test) {
			build := dummyBuildWithEmails("test-builder-3", buildbucketpb.Status_SUCCESS, oldTime, rev1)
			assertTasks(build, mockCheckoutFunc(nil), successEmail)
			verifyBuilder(build, rev1, nil)

			newBuild := dummyBuildWithEmails("test-builder-3", buildbucketpb.Status_FAILURE, newTime, rev2)
			newBuild.Id++
			assertTasks(newBuild, mockCheckoutFunc(nil), successEmail, failEmail, changeEmail)
			verifyBuilder(newBuild, rev2, nil)
		})

		t.Run(`revision update w/property`, func(t *ftt.Test) {
			build := dummyBuildWithEmails("test-builder-3", buildbucketpb.Status_SUCCESS, oldTime, rev1, propEmail)
			assertTasks(build, mockCheckoutFunc(nil), successEmail, propEmail)
			verifyBuilder(build, rev1, nil)

			newBuild := dummyBuildWithEmails("test-builder-3", buildbucketpb.Status_FAILURE, newTime, rev2, propEmail)
			newBuild.Id++
			assertTasks(newBuild, mockCheckoutFunc(nil), successEmail, propEmail, failEmail, changeEmail, propEmail)
			verifyBuilder(newBuild, rev2, nil)
		})

		t.Run(`out-of-order creation time`, func(t *ftt.Test) {
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

		t.Run(`blamelist no allowlist`, func(t *ftt.Test) {
			testBlamelistConfig("test-builder-blamelist-1", changeEmail, commit2Email)
		})

		t.Run(`blamelist with allowlist`, func(t *ftt.Test) {
			testBlamelistConfig("test-builder-blamelist-2", changeEmail, commit1Email)
		})

		t.Run(`blamelist against last non-empty checkout`, func(t *ftt.Test) {
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

		t.Run(`blamelist mixed`, func(t *ftt.Test) {
			testBlamelistConfig("test-builder-blamelist-3", commit1Email, commit2Email)
		})

		t.Run(`blamelist duplicate`, func(t *ftt.Test) {
			testBlamelistConfig("test-builder-blamelist-4", commit2Email, commit2Email, commit2Email)
		})

		t.Run(`failure type infra`, func(t *ftt.Test) {
			infra_failure_build := dummyBuildWithEmails("test-builder-infra-1", buildbucketpb.Status_SUCCESS, oldTime, rev2)
			assertTasks(infra_failure_build, mockCheckoutFunc(nil))

			infra_failure_build = dummyBuildWithEmails("test-builder-infra-1", buildbucketpb.Status_FAILURE, newTime, rev2)
			assertTasks(infra_failure_build, mockCheckoutFunc(nil))

			infra_failure_build = dummyBuildWithEmails("test-builder-infra-1", buildbucketpb.Status_INFRA_FAILURE, newTime2, rev2)
			assertTasks(infra_failure_build, mockCheckoutFunc(nil), infraFailEmail)
		})

		t.Run(`failure type mixed`, func(t *ftt.Test) {
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
			assert.Loosely(t, datastore.Put(c, tc), should.BeNil)

			// Handle a new build.
			build := dummyBuildWithFailingSteps(buildStatus, failingSteps)
			history := mockHistoryFunc(map[string][]*gitpb.Commit{})
			assert.Loosely(t, handleBuild(c, build, mockCheckoutFunc(nil), history), should.BeNil)

			// Fetch the new tree closer.
			assert.Loosely(t, datastore.Get(c, tc), should.BeNil)
			return tc
		}

		testStatus := func(buildStatus buildbucketpb.Status, initialStatus, expectedNewStatus config.TreeCloserStatus, expectingUpdatedTimestamp bool, failingSteps []string) {
			tc := runHandleBuild(buildStatus, initialStatus, failingSteps)

			// Assert the resulting state of the tree closer.
			assert.Loosely(t, tc.Status, should.Equal(expectedNewStatus))
			assert.Loosely(t, tc.Timestamp.After(initialTimestamp), should.Equal(expectingUpdatedTimestamp))
		}

		// We want to exhaustively test all combinations of the following:
		//   * Did the build succeed?
		//   * If not, do the filters (if any) match?
		//   * Is the resulting status the same as the old status?
		// All possibilities are explored in the tests below.

		t.Run(`Build passed, Closed -> Open`, func(t *ftt.Test) {
			testStatus(buildbucketpb.Status_SUCCESS, config.Closed, config.Open, true, []string{})
		})

		t.Run(`Build passed, Open -> Open`, func(t *ftt.Test) {
			testStatus(buildbucketpb.Status_SUCCESS, config.Open, config.Open, true, []string{})
		})

		t.Run(`Build failed, filters don't match, Closed -> Open`, func(t *ftt.Test) {
			testStatus(buildbucketpb.Status_FAILURE, config.Closed, config.Open, true, []string{"exclude"})
		})

		t.Run(`Build failed, filters don't match, Open -> Open`, func(t *ftt.Test) {
			testStatus(buildbucketpb.Status_FAILURE, config.Open, config.Open, true, []string{"exclude"})
		})

		t.Run(`Build failed, filters match, Open -> Closed`, func(t *ftt.Test) {
			testStatus(buildbucketpb.Status_FAILURE, config.Open, config.Closed, true, []string{"include"})
		})

		t.Run(`Build failed, filters match, Closed -> Closed`, func(t *ftt.Test) {
			testStatus(buildbucketpb.Status_FAILURE, config.Closed, config.Closed, true, []string{"include"})
		})

		// In addition, we want to test that statuses other than SUCCESS and FAILURE don't
		// cause any updates, regardless of the initial state.

		t.Run(`Infra failure, stays Open`, func(t *ftt.Test) {
			testStatus(buildbucketpb.Status_INFRA_FAILURE, config.Open, config.Open, false, []string{"include"})
		})

		t.Run(`Infra failure, stays Closed`, func(t *ftt.Test) {
			testStatus(buildbucketpb.Status_INFRA_FAILURE, config.Closed, config.Closed, false, []string{"include"})
		})

		// Test that the correct status message is generated.
		t.Run(`Status message`, func(t *ftt.Test) {
			tc := runHandleBuild(buildbucketpb.Status_FAILURE, config.Open, []string{"include"})

			assert.Loosely(t, tc.Message, should.Equal(`Builder test-builder-tree-closer failed on steps "include"`))
		})

		t.Run(`All failed steps listed if no filter`, func(t *ftt.Test) {
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
			assert.Loosely(t, datastore.Put(c, tc), should.BeNil)

			// Handle a new build.
			build := dummyBuildWithFailingSteps(buildbucketpb.Status_FAILURE, []string{"step1", "step2"})
			history := mockHistoryFunc(map[string][]*gitpb.Commit{})
			assert.Loosely(t, handleBuild(c, build, mockCheckoutFunc(nil), history), should.BeNil)

			// Fetch the new tree closer.
			assert.Loosely(t, datastore.Get(c, tc), should.BeNil)

			assert.Loosely(t, tc.Message, should.Equal(`Builder test-builder-tree-closer failed on steps "step1", "step2"`))
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
				Notifications: &apicfg.Notifications{
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

	ftt.Run("builds_v2 pubsub message", t, func(t *ftt.Test) {
		t.Run("success", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
			b, err := extractBuild(ctx, pubsubMsg)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, b.Id, should.Equal(originalBuild.Id))
			assert.Loosely(t, b.Builder, should.Match(originalBuild.Builder))
			assert.Loosely(t, b.Status, should.Equal(buildbucketpb.Status_SUCCESS))
			assert.Loosely(t, b.Infra, should.Match(originalBuild.Infra))
			assert.Loosely(t, b.Input, should.Match(originalBuild.Input))
			assert.Loosely(t, b.Output, should.Match(originalBuild.Output))
			assert.Loosely(t, b.Steps, should.Match(originalBuild.Steps))
			assert.Loosely(t, b.BuildbucketHostname, should.Equal(originalBuild.Infra.Buildbucket.Hostname))
			assert.Loosely(t, b.EmailNotify, should.Match([]EmailNotify{{Email: "abc@gmail.com"}}))
		})

		t.Run("success with no email_notify field", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
			b, err := extractBuild(ctx, pubsubMsg)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, b.Id, should.Equal(originalBuild.Id))
			assert.Loosely(t, b.Builder, should.Match(originalBuild.Builder))
			assert.Loosely(t, b.Status, should.Equal(buildbucketpb.Status_CANCELED))
			assert.Loosely(t, b.Infra, should.Match(originalBuild.Infra))
			assert.Loosely(t, b.Input, should.Match(originalBuild.Input))
			assert.Loosely(t, b.Output, should.Match(originalBuild.Output))
			assert.Loosely(t, b.Steps, should.Match(originalBuild.Steps))
			assert.Loosely(t, b.BuildbucketHostname, should.Equal(originalBuild.Infra.Buildbucket.Hostname))
			assert.Loosely(t, b.EmailNotify, should.BeNil)
		})

		t.Run("incompleted build", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
			b, err := extractBuild(ctx, pubsubMsg)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, b, should.BeNil)
		})

		t.Run("with need for additional GetBuild call", func(t *ftt.Test) {
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
			steps := []*buildbucketpb.Step{{Name: "step1"}}
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
				Steps: steps,
			}
			pubsubMsg, err := makeBuildsV2PubsubMsg(originalBuild)
			assert.Loosely(t, err, should.BeNil)
			pubsubMsg.BuildLargeFields = nil
			pubsubMsg.BuildLargeFieldsDropped = true

			ctl := gomock.NewController(t)
			defer ctl.Finish()
			mc := buildbucketpb.NewMockBuildsClient(ctl)
			ctx = context.WithValue(ctx, &mockedBBClientKey, mc)
			largeFields := &buildbucketpb.Build{
				Input: &buildbucketpb.Build_Input{},
				Output: &buildbucketpb.Build_Output{
					Properties: props,
				},
				Steps: steps,
			}

			t.Run("success", func(t *ftt.Test) {
				mc.EXPECT().GetBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(largeFields, nil).AnyTimes()

				b, err := extractBuild(ctx, pubsubMsg)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, b.Id, should.Equal(originalBuild.Id))
				assert.Loosely(t, b.Builder, should.Match(originalBuild.Builder))
				assert.Loosely(t, b.Status, should.Equal(buildbucketpb.Status_SUCCESS))
				assert.Loosely(t, b.Infra, should.Match(originalBuild.Infra))
				assert.Loosely(t, b.Input, should.Match(originalBuild.Input))
				assert.Loosely(t, b.Output, should.Match(originalBuild.Output))
				assert.Loosely(t, b.Steps, should.Match(originalBuild.Steps))
				assert.Loosely(t, b.BuildbucketHostname, should.Equal(originalBuild.Infra.Buildbucket.Hostname))
				assert.Loosely(t, b.EmailNotify, should.Match([]EmailNotify{{Email: "abc@gmail.com"}}))
			})
			t.Run("response too large", func(t *ftt.Test) {
				st := status.New(codes.Unavailable, "the response size exceeds the client limit")
				st, err := st.WithDetails(&prpcpb.ErrorDetails{
					Error: &prpcpb.ErrorDetails_ResponseTooBig{
						ResponseTooBig: &prpcpb.ResponseTooBig{
							ResponseSize:  222222,
							ResponseLimit: 111111,
						},
					},
				})
				assert.NoErr(t, err)

				mc.EXPECT().GetBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, st.Err()).AnyTimes()

				_, err = extractBuild(ctx, pubsubMsg)
				assert.Loosely(t, err, should.ErrLike("exceeds the client limit"))
				assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
			})
		})
	})
}

func makeBuildsV2PubsubMsg(b *buildbucketpb.Build) (*buildbucketpb.BuildsV2PubSub, error) {
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
			return nil, errors.Fmt("failed to compress: %w", err)
		}
		if err := zw.Close(); err != nil {
			return nil, errors.Fmt("error closing zlib writer: %w", err)
		}
		return buf.Bytes(), nil
	}
	largeBytes, err := proto.Marshal(large)
	if err != nil {
		return nil, errors.Fmt("failed to marshal large: %w", err)
	}
	compressedLarge, err := compress(largeBytes)
	if err != nil {
		return nil, err
	}
	return &buildbucketpb.BuildsV2PubSub{
		Build:            copyB,
		BuildLargeFields: compressedLarge,
	}, nil
}
