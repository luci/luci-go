// Copyright 2022 The LUCI Authors.
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

package resultingester

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/mock/gomock"
	. "github.com/smartystreets/goconvey/convey"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/gae/impl/memory"
	rdbpbutil "go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"
	_ "go.chromium.org/luci/server/tq/txn/spanner"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/analysis/internal/analysis"
	"go.chromium.org/luci/analysis/internal/analysis/clusteredfailures"
	"go.chromium.org/luci/analysis/internal/buildbucket"
	"go.chromium.org/luci/analysis/internal/clustering/chunkstore"
	"go.chromium.org/luci/analysis/internal/clustering/ingestion"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/ingestion/control"
	ctrlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	"go.chromium.org/luci/analysis/internal/resultdb"
	"go.chromium.org/luci/analysis/internal/services/resultcollector"
	"go.chromium.org/luci/analysis/internal/services/testvariantupdator"
	spanutil "go.chromium.org/luci/analysis/internal/span"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testresults"
	"go.chromium.org/luci/analysis/internal/testresults/gitreferences"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/internal/testutil/insert"
	"go.chromium.org/luci/analysis/pbutil"
	atvpb "go.chromium.org/luci/analysis/proto/analyzedtestvariant"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestSchedule(t *testing.T) {
	Convey(`TestSchedule`, t, func() {
		ctx := testutil.IntegrationTestContext(t)
		ctx, skdr := tq.TestingContext(ctx, nil)

		task := &taskspb.IngestTestResults{
			Build:         &ctrlpb.BuildResult{},
			PartitionTime: timestamppb.New(time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC)),
		}
		expected := proto.Clone(task).(*taskspb.IngestTestResults)

		_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			Schedule(ctx, task)
			return nil
		})
		So(err, ShouldBeNil)
		So(skdr.Tasks().Payloads()[0], ShouldResembleProto, expected)
	})
}

func TestShouldIngestForTestVariants(t *testing.T) {
	t.Parallel()
	Convey(`With realm config`, t, func() {
		realm := &configpb.RealmConfig{
			Name: "ci",
			TestVariantAnalysis: &configpb.TestVariantAnalysisConfig{
				UpdateTestVariantTask: &configpb.UpdateTestVariantTask{
					UpdateTestVariantTaskInterval:   durationpb.New(time.Hour),
					TestVariantStatusUpdateDuration: durationpb.New(24 * time.Hour),
				},
			},
		}
		payload := &taskspb.IngestTestResults{
			Build: &ctrlpb.BuildResult{
				Host: "host",
				Id:   int64(1),
			},
			PartitionTime: timestamppb.New(time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC)),
		}
		Convey(`CI`, func() {
			So(shouldIngestForTestVariants(realm, payload), ShouldBeTrue)
		})
		Convey(`CQ run`, func() {
			payload.PresubmitRun = &ctrlpb.PresubmitResult{
				PresubmitRunId: &pb.PresubmitRunId{
					System: "luci-cv",
					Id:     "chromium/1111111111111-1-1111111111111111",
				},
				Status: pb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_SUCCEEDED,
				Mode:   pb.PresubmitRunMode_FULL_RUN,
			}
			Convey(`Successful full run`, func() {
				So(shouldIngestForTestVariants(realm, payload), ShouldBeTrue)
			})
			Convey(`Successful dry run`, func() {
				payload.PresubmitRun.Mode = pb.PresubmitRunMode_DRY_RUN
				So(shouldIngestForTestVariants(realm, payload), ShouldBeFalse)
			})
			Convey(`Successful quick dry run`, func() {
				payload.PresubmitRun.Mode = pb.PresubmitRunMode_QUICK_DRY_RUN
				So(shouldIngestForTestVariants(realm, payload), ShouldBeFalse)
			})
			Convey(`Successful new patchset run`, func() {
				payload.PresubmitRun.Mode = pb.PresubmitRunMode_NEW_PATCHSET_RUN
				So(shouldIngestForTestVariants(realm, payload), ShouldBeFalse)
			})
			Convey(`Failed run`, func() {
				payload.PresubmitRun.Status = pb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_FAILED
				So(shouldIngestForTestVariants(realm, payload), ShouldBeFalse)
			})
		})
		Convey(`Test Variant analysis not configured`, func() {
			realm.TestVariantAnalysis = nil
			So(shouldIngestForTestVariants(realm, payload), ShouldBeFalse)
		})
	})
}

func createProjectsConfig() map[string]*configpb.ProjectConfig {
	return map[string]*configpb.ProjectConfig{
		"project": {
			Realms: []*configpb.RealmConfig{
				{
					Name: "ci",
					TestVariantAnalysis: &configpb.TestVariantAnalysisConfig{
						UpdateTestVariantTask: &configpb.UpdateTestVariantTask{
							UpdateTestVariantTaskInterval:   durationpb.New(time.Hour),
							TestVariantStatusUpdateDuration: durationpb.New(24 * time.Hour),
						},
					},
				},
			},
		},
	}
}

func TestIngestTestResults(t *testing.T) {
	resultcollector.RegisterTaskClass()
	testvariantupdator.RegisterTaskClass()

	Convey(`TestIngestTestResults`, t, func() {
		ctx := testutil.IntegrationTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx) // For failure association rules cache.
		ctx, skdr := tq.TestingContext(ctx, nil)
		ctx = memory.Use(ctx)

		chunkStore := chunkstore.NewFakeClient()
		clusteredFailures := clusteredfailures.NewFakeClient()
		analysis := analysis.NewClusteringHandler(clusteredFailures)
		ri := &resultIngester{
			clustering: ingestion.New(chunkStore, analysis),
		}

		Convey(`partition time`, func() {
			payload := &taskspb.IngestTestResults{
				Build: &ctrlpb.BuildResult{
					Host:    "host",
					Id:      13131313,
					Project: "project",
				},
				PartitionTime: timestamppb.New(clock.Now(ctx).Add(-1 * time.Hour)),
			}
			Convey(`too early`, func() {
				payload.PartitionTime = timestamppb.New(clock.Now(ctx).Add(25 * time.Hour))
				err := ri.ingestTestResults(ctx, payload)
				So(err, ShouldErrLike, "too far in the future")
			})
			Convey(`too late`, func() {
				payload.PartitionTime = timestamppb.New(clock.Now(ctx).Add(-91 * 24 * time.Hour))
				err := ri.ingestTestResults(ctx, payload)
				So(err, ShouldErrLike, "too long ago")
			})
		})

		Convey(`valid payload`, func() {
			config.SetTestProjectConfig(ctx, createProjectsConfig())

			ctl := gomock.NewController(t)
			defer ctl.Finish()

			mrc := resultdb.NewMockedClient(ctx, ctl)
			mbc := buildbucket.NewMockedClient(mrc.Ctx, ctl)
			ctx = mbc.Ctx

			bHost := "host"
			bID := int64(87654321)
			inv := "invocations/build-87654321"
			realm := "project:ci"
			partitionTime := clock.Now(ctx).Add(-1 * time.Hour)

			expectedGitReference := &gitreferences.GitReference{
				Project:          "project",
				GitReferenceHash: gitreferences.GitReferenceHash("myproject.googlesource.com", "someproject/src", "refs/heads/mybranch"),
				Hostname:         "myproject.googlesource.com",
				Repository:       "someproject/src",
				Reference:        "refs/heads/mybranch",
			}

			expectedInvocation := &testresults.IngestedInvocation{
				Project:              "project",
				IngestedInvocationID: "build-87654321",
				SubRealm:             "ci",
				PartitionTime:        timestamppb.New(partitionTime).AsTime(),
				BuildStatus:          pb.BuildStatus_BUILD_STATUS_FAILURE,
				PresubmitRun: &testresults.PresubmitRun{
					Owner: "automation",
					Mode:  pb.PresubmitRunMode_FULL_RUN,
				},
				GitReferenceHash: expectedGitReference.GitReferenceHash,
				CommitPosition:   111888,
				CommitHash:       strings.Repeat("0a", 20),
				Changelists: []testresults.Changelist{
					{
						Host:     "anothergerrit.gerrit.instance",
						Change:   77788,
						Patchset: 19,
					},
					{
						Host:     "mygerrit-review.googlesource.com",
						Change:   12345,
						Patchset: 5,
					},
				},
			}

			verifyIngestedInvocation := func(expected *testresults.IngestedInvocation) {
				var invs []*testresults.IngestedInvocation
				// Validate IngestedInvocations table is populated.
				err := testresults.ReadIngestedInvocations(span.Single(ctx), spanner.AllKeys(), func(inv *testresults.IngestedInvocation) error {
					invs = append(invs, inv)
					return nil
				})
				So(err, ShouldBeNil)
				if expected != nil {
					So(invs, ShouldHaveLength, 1)
					So(invs[0], ShouldResemble, expected)
				} else {
					So(invs, ShouldHaveLength, 0)
				}
			}

			verifyGitReference := func(expected *gitreferences.GitReference) {
				refs, err := gitreferences.ReadAll(span.Single(ctx))
				So(err, ShouldBeNil)
				if expected != nil {
					So(refs, ShouldHaveLength, 1)
					actual := refs[0]
					// LastIngestionTime is a commit timestamp in the
					// control of the implementation. We check it is
					// populated and assert nothing beyond that.
					So(actual.LastIngestionTime, ShouldNotBeEmpty)
					actual.LastIngestionTime = time.Time{}

					So(actual, ShouldResemble, expected)
				} else {
					So(refs, ShouldHaveLength, 0)
				}
			}

			verifyTestResults := func(expectCommitPosition bool) {
				trBuilder := testresults.NewTestResult().
					WithProject("project").
					WithPartitionTime(timestamppb.New(partitionTime).AsTime()).
					WithIngestedInvocationID("build-87654321").
					WithSubRealm("ci").
					WithBuildStatus(pb.BuildStatus_BUILD_STATUS_FAILURE).
					WithChangelists([]testresults.Changelist{
						{
							Host:     "anothergerrit.gerrit.instance",
							Change:   77788,
							Patchset: 19,
						},
						{
							Host:     "mygerrit-review.googlesource.com",
							Change:   12345,
							Patchset: 5,
						},
					}).
					WithPresubmitRun(&testresults.PresubmitRun{
						Owner: "automation",
						Mode:  pb.PresubmitRunMode_FULL_RUN,
					})
				if expectCommitPosition {
					trBuilder = trBuilder.WithCommitPosition(expectedInvocation.GitReferenceHash, expectedInvocation.CommitPosition)
				} else {
					trBuilder = trBuilder.WithoutCommitPosition()
				}

				expectedTRs := []*testresults.TestResult{
					trBuilder.WithTestID("ninja://test_consistent_failure").
						WithVariantHash("hash").
						WithRunIndex(0).
						WithResultIndex(0).
						WithIsUnexpected(true).
						WithStatus(pb.TestResultStatus_FAIL).
						WithRunDuration(3*time.Second).
						WithExonerationReasons(pb.ExonerationReason_OCCURS_ON_OTHER_CLS, pb.ExonerationReason_NOT_CRITICAL, pb.ExonerationReason_OCCURS_ON_MAINLINE).
						Build(),
					trBuilder.WithTestID("ninja://test_expected").
						WithVariantHash("hash").
						WithRunIndex(0).
						WithResultIndex(0).
						WithIsUnexpected(false).
						WithStatus(pb.TestResultStatus_PASS).
						WithRunDuration(5 * time.Second).
						WithoutExoneration().
						Build(),
					trBuilder.WithTestID("ninja://test_has_unexpected").
						WithVariantHash("hash").
						WithRunIndex(0).
						WithResultIndex(0).
						WithIsUnexpected(true).
						WithStatus(pb.TestResultStatus_FAIL).
						WithoutRunDuration().
						WithoutExoneration().
						Build(),
					trBuilder.WithTestID("ninja://test_has_unexpected").
						WithVariantHash("hash").
						WithRunIndex(1).
						WithResultIndex(0).
						WithIsUnexpected(false).
						WithStatus(pb.TestResultStatus_PASS).
						WithoutRunDuration().
						WithoutExoneration().
						Build(),
					trBuilder.WithTestID("ninja://test_known_flake").
						WithVariantHash("hash_2").
						WithRunIndex(0).
						WithResultIndex(0).
						WithIsUnexpected(true).
						WithStatus(pb.TestResultStatus_FAIL).
						WithRunDuration(2 * time.Second).
						WithoutExoneration().
						Build(),
					trBuilder.WithTestID("ninja://test_new_failure").
						WithVariantHash("hash_1").
						WithRunIndex(0).
						WithResultIndex(0).
						WithIsUnexpected(true).
						WithStatus(pb.TestResultStatus_FAIL).
						WithRunDuration(1 * time.Second).
						WithoutExoneration().
						Build(),
					trBuilder.WithTestID("ninja://test_new_flake").
						WithVariantHash("hash").
						WithRunIndex(0).
						WithResultIndex(0).
						WithIsUnexpected(true).
						WithStatus(pb.TestResultStatus_FAIL).
						WithRunDuration(10 * time.Second).
						WithoutExoneration().
						Build(),
					trBuilder.WithTestID("ninja://test_new_flake").
						WithVariantHash("hash").
						WithRunIndex(0).
						WithResultIndex(1).
						WithIsUnexpected(true).
						WithStatus(pb.TestResultStatus_FAIL).
						WithRunDuration(11 * time.Second).
						WithoutExoneration().
						Build(),
					trBuilder.WithTestID("ninja://test_new_flake").
						WithVariantHash("hash").
						WithRunIndex(1).
						WithResultIndex(0).
						WithIsUnexpected(false).
						WithStatus(pb.TestResultStatus_PASS).
						WithRunDuration(12 * time.Second).
						WithoutExoneration().
						Build(),
					trBuilder.WithTestID("ninja://test_no_new_results").
						WithVariantHash("hash").
						WithRunIndex(0).
						WithResultIndex(0).
						WithIsUnexpected(true).
						WithStatus(pb.TestResultStatus_FAIL).
						WithRunDuration(4 * time.Second).
						WithoutExoneration().
						Build(),
					trBuilder.WithTestID("ninja://test_skip").
						WithVariantHash("hash").
						WithRunIndex(0).
						WithResultIndex(0).
						WithIsUnexpected(true).
						WithStatus(pb.TestResultStatus_SKIP).
						WithoutRunDuration().
						WithoutExoneration().
						Build(),
					trBuilder.WithTestID("ninja://test_unexpected_pass").
						WithVariantHash("hash").
						WithRunIndex(0).
						WithResultIndex(0).
						WithIsUnexpected(true).
						WithStatus(pb.TestResultStatus_PASS).
						WithoutRunDuration().
						WithoutExoneration().
						Build(),
				}

				// Validate TestResults table is populated.
				var actualTRs []*testresults.TestResult
				err := testresults.ReadTestResults(span.Single(ctx), spanner.AllKeys(), func(tr *testresults.TestResult) error {
					actualTRs = append(actualTRs, tr)
					return nil
				})
				So(err, ShouldBeNil)
				So(actualTRs, ShouldResemble, expectedTRs)

				// Validate TestVariantRealms table is populated.
				tvrs := make([]*testresults.TestVariantRealm, 0)
				err = testresults.ReadTestVariantRealms(span.Single(ctx), spanner.AllKeys(), func(tvr *testresults.TestVariantRealm) error {
					tvrs = append(tvrs, tvr)
					return nil
				})
				So(err, ShouldBeNil)

				expectedRealms := []*testresults.TestVariantRealm{
					{
						Project:     "project",
						TestID:      "ninja://test_consistent_failure",
						VariantHash: "hash",
						SubRealm:    "ci",
						Variant:     nil,
					},
					{
						Project:     "project",
						TestID:      "ninja://test_expected",
						VariantHash: "hash",
						SubRealm:    "ci",
						Variant:     nil,
					},
					{
						Project:     "project",
						TestID:      "ninja://test_has_unexpected",
						VariantHash: "hash",
						SubRealm:    "ci",
						Variant:     nil,
					},
					{
						Project:     "project",
						TestID:      "ninja://test_known_flake",
						VariantHash: "hash_2",
						SubRealm:    "ci",
						Variant:     pbutil.VariantFromResultDB(rdbpbutil.Variant("k1", "v2")),
					},
					{
						Project:     "project",
						TestID:      "ninja://test_new_failure",
						VariantHash: "hash_1",
						SubRealm:    "ci",
						Variant:     pbutil.VariantFromResultDB(rdbpbutil.Variant("k1", "v1")),
					},
					{
						Project:     "project",
						TestID:      "ninja://test_new_flake",
						VariantHash: "hash",
						SubRealm:    "ci",
						Variant:     nil,
					},
					{
						Project:     "project",
						TestID:      "ninja://test_no_new_results",
						VariantHash: "hash",
						SubRealm:    "ci",
						Variant:     nil,
					},
					{
						Project:     "project",
						TestID:      "ninja://test_skip",
						VariantHash: "hash",
						SubRealm:    "ci",
						Variant:     nil,
					},
					{
						Project:     "project",
						TestID:      "ninja://test_unexpected_pass",
						VariantHash: "hash",
						SubRealm:    "ci",
						Variant:     nil,
					},
				}

				So(tvrs, ShouldHaveLength, len(expectedRealms))
				for i, tvr := range tvrs {
					expectedTVR := expectedRealms[i]
					So(tvr.LastIngestionTime, ShouldNotBeZeroValue)
					expectedTVR.LastIngestionTime = tvr.LastIngestionTime
					So(tvr, ShouldResemble, expectedTVR)
				}

				// Validate TestRealms table is populated.
				testRealms := make([]*testresults.TestRealm, 0)
				err = testresults.ReadTestRealms(span.Single(ctx), spanner.AllKeys(), func(tvr *testresults.TestRealm) error {
					testRealms = append(testRealms, tvr)
					return nil
				})
				So(err, ShouldBeNil)

				expectedTestRealms := []*testresults.TestRealm{
					{
						Project:  "project",
						TestID:   "ninja://test_consistent_failure",
						SubRealm: "ci",
					},
					{
						Project:  "project",
						TestID:   "ninja://test_expected",
						SubRealm: "ci",
					},
					{
						Project:  "project",
						TestID:   "ninja://test_has_unexpected",
						SubRealm: "ci",
					},
					{
						Project:  "project",
						TestID:   "ninja://test_known_flake",
						SubRealm: "ci",
					},
					{
						Project:  "project",
						TestID:   "ninja://test_new_failure",
						SubRealm: "ci",
					},
					{
						Project:  "project",
						TestID:   "ninja://test_new_flake",
						SubRealm: "ci",
					},
					{
						Project:  "project",
						TestID:   "ninja://test_no_new_results",
						SubRealm: "ci",
					},
					{
						Project:  "project",
						TestID:   "ninja://test_skip",
						SubRealm: "ci",
					},
					{
						Project:  "project",
						TestID:   "ninja://test_unexpected_pass",
						SubRealm: "ci",
					},
				}

				So(testRealms, ShouldHaveLength, len(expectedTestRealms))
				for i, tr := range testRealms {
					expectedTR := expectedTestRealms[i]
					So(tr.LastIngestionTime, ShouldNotBeZeroValue)
					expectedTR.LastIngestionTime = tr.LastIngestionTime
					So(tr, ShouldResemble, expectedTR)
				}
			}

			verifyClustering := func() {
				// Confirm chunks have been written to GCS.
				So(len(chunkStore.Contents), ShouldEqual, 1)

				// Confirm clustering has occurred, with each test result in at
				// least one cluster.
				actualClusteredFailures := make(map[string]int)
				for project, insertions := range clusteredFailures.InsertionsByProject {
					So(project, ShouldEqual, "project")
					for _, f := range insertions {
						actualClusteredFailures[f.TestId] += 1
					}
				}
				expectedClusteredFailures := map[string]int{
					"ninja://test_new_failure":        1,
					"ninja://test_known_flake":        1,
					"ninja://test_consistent_failure": 1,
					"ninja://test_no_new_results":     1,
					"ninja://test_new_flake":          2,
					"ninja://test_has_unexpected":     1,
				}
				So(actualClusteredFailures, ShouldResemble, expectedClusteredFailures)
			}

			verifyAnalyzedTestVariants := func() {
				// Read rows from Spanner to confirm the analyzed test variants are saved.
				ctx, cancel := span.ReadOnlyTransaction(ctx)
				defer cancel()

				exp := map[string]atvpb.Status{
					"ninja://test_new_failure":        atvpb.Status_HAS_UNEXPECTED_RESULTS,
					"ninja://test_known_flake":        atvpb.Status_FLAKY,
					"ninja://test_consistent_failure": atvpb.Status_CONSISTENTLY_UNEXPECTED,
					"ninja://test_no_new_results":     atvpb.Status_HAS_UNEXPECTED_RESULTS,
					"ninja://test_new_flake":          atvpb.Status_FLAKY,
					"ninja://test_has_unexpected":     atvpb.Status_FLAKY,
				}
				act := make(map[string]atvpb.Status)
				expProtos := map[string]*atvpb.AnalyzedTestVariant{
					"ninja://test_new_failure": {
						Realm:       realm,
						TestId:      "ninja://test_new_failure",
						VariantHash: "hash_1",
						Status:      atvpb.Status_HAS_UNEXPECTED_RESULTS,
						Variant:     pbutil.VariantFromResultDB(sampleVar),
						Tags:        pbutil.StringPairs("monorail_component", "Monorail>Component"),
						// Mock ResultDB data specifies test metadata for the
						// new test failure. Make sure it is used.
						TestMetadata: pbutil.TestMetadataFromResultDB(updatedTmd),
					},
					"ninja://test_known_flake": {
						Realm:       realm,
						TestId:      "ninja://test_known_flake",
						VariantHash: "hash_2",
						Status:      atvpb.Status_FLAKY,
						Tags:        pbutil.StringPairs("monorail_component", "Monorail>Component", "os", "Mac", "test_name", "test_known_flake"),
						// Mock ResultDB data specifies test metadata for the
						// test result. Make sure it is used.
						TestMetadata: pbutil.TestMetadataFromResultDB(updatedTmd),
					},
					"ninja://test_consistent_failure": {
						Realm:       realm,
						TestId:      "ninja://test_consistent_failure",
						VariantHash: "hash",
						Status:      atvpb.Status_CONSISTENTLY_UNEXPECTED,
						Tags:        pbutil.StringPairs(),
						// Mock ResultDB data does not specify test metadata for the
						// ingested test result. Keep the same test metadata.
						TestMetadata: pbutil.TestMetadataFromResultDB(originalTmd),
					},
				}

				var testIDsWithNextTask []string
				fields := []string{"Realm", "TestId", "VariantHash", "Status", "Variant", "Tags", "TestMetadata", "NextUpdateTaskEnqueueTime"}
				actProtos := make(map[string]*atvpb.AnalyzedTestVariant, len(expProtos))
				var b spanutil.Buffer
				err := span.Read(ctx, "AnalyzedTestVariants", spanner.AllKeys(), fields).Do(
					func(row *spanner.Row) error {
						tv := &atvpb.AnalyzedTestVariant{}
						var tmd spanutil.Compressed
						var enqTime spanner.NullTime
						err := b.FromSpanner(row, &tv.Realm, &tv.TestId, &tv.VariantHash, &tv.Status, &tv.Variant, &tv.Tags, &tmd, &enqTime)
						So(err, ShouldBeNil)
						So(tv.Realm, ShouldEqual, realm)

						if len(tmd) > 0 {
							tv.TestMetadata = &pb.TestMetadata{}
							err = proto.Unmarshal(tmd, tv.TestMetadata)
							So(err, ShouldBeNil)
						}

						act[tv.TestId] = tv.Status
						if _, ok := expProtos[tv.TestId]; ok {
							actProtos[tv.TestId] = tv
						}

						if !enqTime.IsNull() {
							testIDsWithNextTask = append(testIDsWithNextTask, tv.TestId)
						}
						return nil
					},
				)
				So(err, ShouldBeNil)
				So(act, ShouldResemble, exp)
				for k, actProto := range actProtos {
					v, ok := expProtos[k]
					So(ok, ShouldBeTrue)
					So(actProto, ShouldResembleProto, v)
				}
				sort.Strings(testIDsWithNextTask)

				var actTestIDsWithTasks []string
				for _, pl := range skdr.Tasks().Payloads() {
					switch pl.(type) {
					case *taskspb.UpdateTestVariant:
						plp := pl.(*taskspb.UpdateTestVariant)
						actTestIDsWithTasks = append(actTestIDsWithTasks, plp.TestVariantKey.TestId)
					default:
					}
				}
				sort.Strings(actTestIDsWithTasks)
				So(len(actTestIDsWithTasks), ShouldEqual, 3)
				So(actTestIDsWithTasks, ShouldResemble, testIDsWithNextTask)
			}

			verifyCollectTask := func(expectExists bool) {
				expColTask := &taskspb.CollectTestResults{
					Resultdb: &taskspb.ResultDB{
						Invocation: &rdbpb.Invocation{
							Name:  inv,
							Realm: realm,
						},
						Host: "results.api.cr.dev",
					},
					Builder:                   "builder",
					Project:                   "project",
					IsPreSubmit:               true,
					ContributedToClSubmission: true,
				}
				collectTaskCount := 0
				for _, pl := range skdr.Tasks().Payloads() {
					switch pl.(type) {
					case *taskspb.CollectTestResults:
						plp := pl.(*taskspb.CollectTestResults)
						So(plp, ShouldResembleProto, expColTask)
						collectTaskCount++
					default:
					}
				}
				if expectExists {
					So(collectTaskCount, ShouldEqual, 1)
				} else {
					So(collectTaskCount, ShouldEqual, 0)
				}
			}

			verifyContinuationTask := func(expectedContinuation *taskspb.IngestTestResults) {
				count := 0
				for _, pl := range skdr.Tasks().Payloads() {
					switch pl := pl.(type) {
					case *taskspb.IngestTestResults:
						So(pl, ShouldResembleProto, expectedContinuation)
						count++
					default:
					}
				}
				if expectedContinuation != nil {
					So(count, ShouldEqual, 1)
				} else {
					So(count, ShouldEqual, 0)
				}
			}

			verifyIngestionControl := func(expected *control.Entry) {
				actual, err := control.Read(span.Single(ctx), []string{expected.BuildID})
				So(err, ShouldBeNil)
				So(actual, ShouldHaveLength, 1)
				a := *actual[0]
				e := *expected

				// Compare protos separately, as they are not compared
				// correctly by ShouldResemble.
				So(a.PresubmitResult, ShouldResembleProto, e.PresubmitResult)
				a.PresubmitResult = nil
				e.PresubmitResult = nil

				So(a.BuildResult, ShouldResembleProto, e.BuildResult)
				a.BuildResult = nil
				e.BuildResult = nil

				So(a.InvocationResult, ShouldResembleProto, e.InvocationResult)
				a.InvocationResult = nil
				e.InvocationResult = nil

				// Do not compare last updated time, as it is determined
				// by commit timestamp.
				So(a.LastUpdated, ShouldNotBeEmpty)
				e.LastUpdated = a.LastUpdated

				So(a, ShouldResemble, e)
			}

			setupGetInvocationMock := func() {
				invReq := &rdbpb.GetInvocationRequest{
					Name: inv,
				}
				invRes := &rdbpb.Invocation{
					Name:  inv,
					Realm: realm,
				}
				mrc.GetInvocation(invReq, invRes)
			}

			setupQueryTestVariantsMock := func(modifiers ...func(*rdbpb.QueryTestVariantsResponse)) {
				tvReq := &rdbpb.QueryTestVariantsRequest{
					Invocations: []string{inv},
					PageSize:    10000,
					ReadMask:    testVariantReadMask,
					PageToken:   "expected_token",
				}
				tvRsp := mockedQueryTestVariantsRsp()
				tvRsp.NextPageToken = "continuation_token"
				for _, modifier := range modifiers {
					modifier(tvRsp)
				}
				mrc.QueryTestVariants(tvReq, tvRsp)
			}

			// Prepare some existing analyzed test variants to update.
			tmdBytes, _ := proto.Marshal(originalTmd)
			ms := []*spanner.Mutation{
				// Known flake's status should remain unchanged.
				insert.AnalyzedTestVariant(realm, "ninja://test_known_flake", "hash_2", atvpb.Status_FLAKY, map[string]interface{}{
					"Tags":         pbutil.StringPairs("test_name", "test_known_flake", "monorail_component", "Monorail>OldComponent"),
					"TestMetadata": spanutil.Compressed(tmdBytes),
				}),
				// Non-flake test variant's status will change when see a flaky occurrence.
				insert.AnalyzedTestVariant(realm, "ninja://test_has_unexpected", "hash", atvpb.Status_HAS_UNEXPECTED_RESULTS, nil),
				// Consistently failed test variant.
				insert.AnalyzedTestVariant(realm, "ninja://test_consistent_failure", "hash", atvpb.Status_CONSISTENTLY_UNEXPECTED, map[string]interface{}{
					"TestMetadata": spanutil.Compressed(tmdBytes),
				}),
				// Stale test variant has new failure.
				insert.AnalyzedTestVariant(realm, "ninja://test_no_new_results", "hash", atvpb.Status_NO_NEW_RESULTS, nil),
			}
			testutil.MustApply(ctx, ms...)

			payload := &taskspb.IngestTestResults{
				Build: &ctrlpb.BuildResult{
					Host:         bHost,
					Id:           bID,
					CreationTime: timestamppb.New(time.Date(2020, time.April, 1, 2, 3, 4, 5, time.UTC)),
					Project:      "project",
					Builder:      "builder",
					Status:       pb.BuildStatus_BUILD_STATUS_FAILURE,
					Changelists: []*pb.Changelist{
						{
							Host:     "mygerrit-review.googlesource.com",
							Change:   12345,
							Patchset: 5,
						},
						{
							Host:     "anothergerrit.gerrit.instance",
							Change:   77788,
							Patchset: 19,
						},
					},
					Commit: &bbpb.GitilesCommit{
						Host:     "myproject.googlesource.com",
						Project:  "someproject/src",
						Id:       strings.Repeat("0a", 20),
						Ref:      "refs/heads/mybranch",
						Position: 111888,
					},
					HasInvocation:        true,
					ResultdbHost:         "results.api.cr.dev",
					IsIncludedByAncestor: false,
				},
				PartitionTime: timestamppb.New(partitionTime),
				PresubmitRun: &ctrlpb.PresubmitResult{
					PresubmitRunId: &pb.PresubmitRunId{
						System: "luci-cv",
						Id:     "infra/12345",
					},
					Status:       pb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_SUCCEEDED,
					Mode:         pb.PresubmitRunMode_FULL_RUN,
					Owner:        "automation",
					CreationTime: timestamppb.New(time.Date(2021, time.April, 1, 2, 3, 4, 5, time.UTC)),
				},
				PageToken: "expected_token",
				TaskIndex: 0,
			}
			expectedContinuation := proto.Clone(payload).(*taskspb.IngestTestResults)
			expectedContinuation.PageToken = "continuation_token"
			expectedContinuation.TaskIndex = 1

			ingestionCtl :=
				control.NewEntry(0).
					WithBuildID(control.BuildID(bHost, bID)).
					WithBuildResult(proto.Clone(payload.Build).(*ctrlpb.BuildResult)).
					WithPresubmitResult(proto.Clone(payload.PresubmitRun).(*ctrlpb.PresubmitResult)).
					WithTaskCount(1).
					Build()

			Convey("First task", func() {
				setupGetInvocationMock()
				setupQueryTestVariantsMock()
				_, err := control.SetEntriesForTesting(ctx, ingestionCtl)
				So(err, ShouldBeNil)

				// Act
				err = ri.ingestTestResults(ctx, payload)
				So(err, ShouldBeNil)

				// Verify
				verifyIngestedInvocation(expectedInvocation)
				verifyGitReference(expectedGitReference)

				// Expect a continuation task to be created.
				verifyContinuationTask(expectedContinuation)
				ingestionCtl.TaskCount = ingestionCtl.TaskCount + 1 // Expect to have been incremented.
				verifyIngestionControl(ingestionCtl)
				expectCommitPosition := true
				verifyTestResults(expectCommitPosition)
				verifyClustering()
				verifyAnalyzedTestVariants()
				expectCollectTaskExists := false
				verifyCollectTask(expectCollectTaskExists)
			})
			Convey("Last task", func() {
				payload.TaskIndex = 10
				ingestionCtl.TaskCount = 11

				setupGetInvocationMock()
				setupQueryTestVariantsMock(func(rsp *rdbpb.QueryTestVariantsResponse) {
					rsp.NextPageToken = ""
				})

				_, err := control.SetEntriesForTesting(ctx, ingestionCtl)
				So(err, ShouldBeNil)

				// Act
				err = ri.ingestTestResults(ctx, payload)
				So(err, ShouldBeNil)

				// Verify
				// Only the first task should create the ingested
				// invocation record and git reference record (if any).
				verifyIngestedInvocation(nil)
				verifyGitReference(nil)

				// As this is the last task, do not expect a continuation
				// task to be created.
				verifyContinuationTask(nil)
				verifyIngestionControl(ingestionCtl)
				expectCommitPosition := true
				verifyTestResults(expectCommitPosition)
				verifyClustering()
				verifyAnalyzedTestVariants()

				// Expect a collect task to be created.
				expectCollectTaskExists := true
				verifyCollectTask(expectCollectTaskExists)
			})
			Convey("Retry task after continuation task already created", func() {
				// Scenario: First task fails after it has already scheduled
				// its continuation.
				ingestionCtl.TaskCount = 2

				setupGetInvocationMock()
				setupQueryTestVariantsMock()
				_, err := control.SetEntriesForTesting(ctx, ingestionCtl)
				So(err, ShouldBeNil)

				// Act
				err = ri.ingestTestResults(ctx, payload)
				So(err, ShouldBeNil)

				// Verify
				verifyIngestedInvocation(expectedInvocation)
				verifyGitReference(expectedGitReference)

				// Do not expect a continuation task to be created,
				// as it was already scheduled.
				verifyContinuationTask(nil)
				verifyIngestionControl(ingestionCtl)
				expectCommitPosition := true
				verifyTestResults(expectCommitPosition)
				verifyClustering()
				verifyAnalyzedTestVariants()

				expectCollectTaskExists := false
				verifyCollectTask(expectCollectTaskExists)
			})
			Convey("No commit position", func() {
				// Scenario: The build which completed did not include commit
				// position data in its output or input.
				payload.Build.Commit = nil

				setupGetInvocationMock()
				setupQueryTestVariantsMock()
				_, err := control.SetEntriesForTesting(ctx, ingestionCtl)
				So(err, ShouldBeNil)

				// Act
				err = ri.ingestTestResults(ctx, payload)
				So(err, ShouldBeNil)

				// Verify
				// The ingested invocation record should not record
				// the commit position.
				expectedInvocation.CommitHash = ""
				expectedInvocation.CommitPosition = 0
				expectedInvocation.GitReferenceHash = nil
				verifyIngestedInvocation(expectedInvocation)

				// No git reference record should be created.
				verifyGitReference(nil)

				// Test results should not have a commit position.
				expectCommitPosition := false
				verifyTestResults(expectCommitPosition)
			})
			Convey("No project config", func() {
				// If no project config exists, results should be ingested into
				// TestResults, but not clustered or used for test variant
				// analysis.
				config.SetTestProjectConfig(ctx, map[string]*configpb.ProjectConfig{})

				setupGetInvocationMock()
				setupQueryTestVariantsMock()
				_, err := control.SetEntriesForTesting(ctx, ingestionCtl)
				So(err, ShouldBeNil)

				// Act
				err = ri.ingestTestResults(ctx, payload)
				So(err, ShouldBeNil)

				// Verify
				// Test results still ingested.
				expectCommitPosition := true
				verifyTestResults(expectCommitPosition)

				// Confirm no chunks have been written to GCS.
				So(len(chunkStore.Contents), ShouldEqual, 0)
				// Confirm no clustering has occurred.
				So(clusteredFailures.InsertionsByProject, ShouldHaveLength, 0)
			})
			Convey(`build included by ancestor`, func() {
				payload.Build.IsIncludedByAncestor = true

				// Act
				err := ri.ingestTestResults(ctx, payload)
				So(err, ShouldBeNil)

				// Verify no test results ingested into test history.
				var actualTRs []*testresults.TestResult
				err = testresults.ReadTestResults(span.Single(ctx), spanner.AllKeys(), func(tr *testresults.TestResult) error {
					actualTRs = append(actualTRs, tr)
					return nil
				})
				So(err, ShouldBeNil)
				So(actualTRs, ShouldHaveLength, 0)
			})
		})
	})
}
