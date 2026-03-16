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

package updater

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/third_party/google.golang.org/genproto/googleapis/devtools/issuetracker/v1"

	"go.chromium.org/luci/analysis/internal/analysis"
	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	"go.chromium.org/luci/analysis/internal/bugs"
	"go.chromium.org/luci/analysis/internal/bugs/buganizer"
	bugspb "go.chromium.org/luci/analysis/internal/bugs/proto"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms"
	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/clustering/runs"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/config/compiledcfg"
	"go.chromium.org/luci/analysis/internal/testutil"
	configpb "go.chromium.org/luci/analysis/proto/config"
)

func TestUpdate(t *testing.T) {
	ftt.Run("With bug updater", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx = memory.Use(ctx)
		ctx = context.WithValue(ctx, &buganizer.BuganizerSelfEmailKey, "email@test.com")

		const project = "chromeos"

		// Has two policies:
		// exoneration-policy (P2):
		// - activation threshold: 100 in one day
		// - deactivation threshold: 10 in one day
		// cls-rejected-policy (P1):
		// - activation threshold: 10 in one week
		// - deactivation threshold: 1 in one week
		projectCfg := createProjectConfig()
		projectsCfg := map[string]*configpb.ProjectConfig{
			project: projectCfg,
		}
		err := config.SetTestProjectConfig(ctx, projectsCfg)
		assert.Loosely(t, err, should.BeNil)

		compiledCfg, err := compiledcfg.NewConfig(projectCfg)
		assert.Loosely(t, err, should.BeNil)

		suggestedClusters := []*analysis.Cluster{
			makeReasonCluster(t, compiledCfg, 0),
			makeReasonCluster(t, compiledCfg, 1),
			makeReasonCluster(t, compiledCfg, 2),
			makeReasonCluster(t, compiledCfg, 3),
			makeReasonCluster(t, compiledCfg, 4),
		}
		analysisClient := &fakeAnalysisClient{
			clusters: suggestedClusters,
		}

		buganizerClient := buganizer.NewFakeClient()
		buganizerStore := buganizerClient.FakeStore

		// Unless otherwise specified, assume re-clustering has caught up to
		// the latest version of algorithms and config.
		err = runs.SetRunsForTesting(ctx, t, []*runs.ReclusteringRun{
			runs.NewRun(0).
				WithProject(project).
				WithAlgorithmsVersion(algorithms.AlgorithmsVersion).
				WithConfigVersion(projectCfg.LastUpdated.AsTime()).
				WithRulesVersion(rules.StartingEpoch).
				WithCompletedProgress().Build(),
		})
		assert.Loosely(t, err, should.BeNil)

		progress, err := runs.ReadReclusteringProgress(ctx, project)
		assert.Loosely(t, err, should.BeNil)

		opts := UpdateOptions{
			UIBaseURL:            "https://luci-analysis-test.appspot.com",
			Project:              project,
			AnalysisClient:       analysisClient,
			BuganizerClient:      buganizerClient,
			MaxBugsFiledPerRun:   1,
			ReclusteringProgress: progress,
			RunTimestamp:         time.Date(2100, 2, 2, 2, 2, 2, 2, time.UTC),
		}

		// Mock current time. This is needed to control behaviours like
		// automatic archiving of rules after 30 days of bug being marked
		// Closed (Verified).
		now := time.Date(2055, time.May, 5, 5, 5, 5, 5, time.UTC)
		ctx, tc := testclock.UseTime(ctx, now)

		t.Run("configuration used for testing is valid", func(t *ftt.Test) {
			c := validation.Context{Context: context.Background()}
			config.ValidateProjectConfig(&c, project, projectCfg)
			assert.Loosely(t, c.Finalize(), should.BeNil)
		})
		t.Run("with a suggested cluster", func(t *ftt.Test) {
			// Create a suggested cluster we should consider filing a bug for.
			sourceClusterID := reasonClusterID(t, compiledCfg, "Failed to connect to 100.1.1.99.")
			suggestedClusters[1].ClusterID = sourceClusterID
			suggestedClusters[1].ExampleFailureReason = bigquery.NullString{StringVal: "Failed to connect to 100.1.1.105.", Valid: true}
			suggestedClusters[1].TopTestIDs = []analysis.TopCount{
				{Value: "network-test-1", Count: 10},
				{Value: "network-test-2", Count: 10},
			}
			// Meets failure dispersion thresholds.
			suggestedClusters[1].DistinctUserCLsWithFailures7d.Residual = 3

			expectedRule := &rules.Entry{
				Project:                 "chromeos",
				RuleDefinition:          `reason LIKE "Failed to connect to %.%.%.%."`,
				BugID:                   bugs.BugID{System: bugs.BuganizerSystem, ID: "1"},
				IsActive:                true,
				IsManagingBug:           true,
				IsManagingBugPriority:   true,
				SourceCluster:           sourceClusterID,
				CreateUser:              rules.LUCIAnalysisSystem,
				LastAuditableUpdateUser: rules.LUCIAnalysisSystem,
				BugManagementState: &bugspb.BugManagementState{
					RuleAssociationNotified: true,
					PolicyState: map[string]*bugspb.BugManagementState_PolicyState{
						"exoneration-policy": {
							IsActive:           true,
							LastActivationTime: timestamppb.New(opts.RunTimestamp),
							ActivationNotified: true,
						},
						"cls-rejected-policy": {},
					},
				},
			}
			expectedRules := []*rules.Entry{expectedRule}

			expectedBuganizerBug := buganizerBug{
				ID:            1,
				Component:     projectCfg.BugManagement.Buganizer.DefaultComponent.Id,
				ExpectedTitle: "Failed to connect to 100.1.1.105.",
				// Expect the bug description to contain the top tests.
				ExpectedContent: []string{
					"https://luci-analysis-test.appspot.com/p/chromeos/rules/", // Rule ID randomly generated.
					"network-test-1",
					"network-test-2",
				},
				ExpectedPolicyIDsActivated: []string{
					"exoneration-policy",
				},
			}

			issueCount := func() int {
				return len(buganizerStore.Issues)
			}

			// Bug-filing threshold met.
			suggestedClusters[1].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{
				OneDay: metrics.Counts{Residual: 100},
			}

			t.Run("bug filing threshold must be met to file a new bug", func(t *ftt.Test) {
				t.Run("Reason cluster", func(t *ftt.Test) {
					t.Run("Above threshold", func(t *ftt.Test) {
						suggestedClusters[1].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{OneDay: metrics.Counts{Residual: 100}}

						// Act
						err = UpdateBugsForProject(ctx, opts)

						// Verify
						assert.Loosely(t, err, should.BeNil)

						assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)
						assert.Loosely(t, expectBuganizerBug(buganizerStore, expectedBuganizerBug), should.BeNil)
						assert.Loosely(t, issueCount(), should.Equal(1))

						// Further updates do nothing.
						err = UpdateBugsForProject(ctx, opts)

						// Verify
						assert.Loosely(t, err, should.BeNil)

						assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)
						assert.Loosely(t, expectBuganizerBug(buganizerStore, expectedBuganizerBug), should.BeNil)
						assert.Loosely(t, issueCount(), should.Equal(1))
					})
					t.Run("Below threshold", func(t *ftt.Test) {
						suggestedClusters[1].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{OneDay: metrics.Counts{Residual: 99}}

						// Act
						err = UpdateBugsForProject(ctx, opts)

						// Verify
						assert.Loosely(t, err, should.BeNil)

						// No bug should be created.
						assert.Loosely(t, verifyRulesResemble(ctx, t, nil), should.BeNil)
						assert.Loosely(t, issueCount(), should.BeZero)
					})
				})
				t.Run("Test name cluster", func(t *ftt.Test) {
					suggestedClusters[1].ClusterID = testIDClusterID(t, compiledCfg, "ui-test-1")
					suggestedClusters[1].TopTestIDs = []analysis.TopCount{
						{Value: "ui-test-1", Count: 10},
					}
					expectedRule.RuleDefinition = `test = "ui-test-1"`
					expectedRule.SourceCluster = suggestedClusters[1].ClusterID
					expectedBuganizerBug.ExpectedTitle = "ui-test-1"
					expectedBuganizerBug.ExpectedContent = []string{"ui-test-1"}

					// 34% more impact is required for a test name cluster to
					// be filed, compared to a failure reason cluster.
					t.Run("Above threshold", func(t *ftt.Test) {
						suggestedClusters[1].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{OneDay: metrics.Counts{Residual: 134}}

						// Act
						err = UpdateBugsForProject(ctx, opts)

						// Verify
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)
						assert.Loosely(t, expectBuganizerBug(buganizerStore, expectedBuganizerBug), should.BeNil)
						assert.Loosely(t, issueCount(), should.Equal(1))

						// Further updates do nothing.
						err = UpdateBugsForProject(ctx, opts)

						// Verify
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)
						assert.Loosely(t, expectBuganizerBug(buganizerStore, expectedBuganizerBug), should.BeNil)
						assert.Loosely(t, issueCount(), should.Equal(1))
					})
					t.Run("Below threshold", func(t *ftt.Test) {
						suggestedClusters[1].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{OneDay: metrics.Counts{Residual: 133}}

						// Act
						err = UpdateBugsForProject(ctx, opts)

						// Verify
						assert.Loosely(t, err, should.BeNil)

						// No bug should be created.
						assert.Loosely(t, verifyRulesResemble(ctx, t, nil), should.BeNil)
						assert.Loosely(t, issueCount(), should.BeZero)
					})
				})
			})
			t.Run("policies are correctly activated when new bugs are filed", func(t *ftt.Test) {
				t.Run("other policy activation threshold not met", func(t *ftt.Test) {
					suggestedClusters[1].MetricValues[metrics.HumanClsFailedPresubmit.ID] = metrics.TimewiseCounts{SevenDay: metrics.Counts{Residual: 9}}

					// Act
					err = UpdateBugsForProject(ctx, opts)

					// Verify
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)
					assert.Loosely(t, expectBuganizerBug(buganizerStore, expectedBuganizerBug), should.BeNil)
					assert.Loosely(t, issueCount(), should.Equal(1))
				})
				t.Run("other policy activation threshold met", func(t *ftt.Test) {
					suggestedClusters[1].MetricValues[metrics.HumanClsFailedPresubmit.ID] = metrics.TimewiseCounts{SevenDay: metrics.Counts{Residual: 10}}
					expectedRule.BugManagementState.PolicyState["cls-rejected-policy"].IsActive = true
					expectedRule.BugManagementState.PolicyState["cls-rejected-policy"].LastActivationTime = timestamppb.New(opts.RunTimestamp)
					expectedRule.BugManagementState.PolicyState["cls-rejected-policy"].ActivationNotified = true
					expectedBuganizerBug.ExpectedPolicyIDsActivated = []string{
						"cls-rejected-policy",
						"exoneration-policy",
					}

					// Act
					err = UpdateBugsForProject(ctx, opts)

					// Verify
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)
					assert.Loosely(t, expectBuganizerBug(buganizerStore, expectedBuganizerBug), should.BeNil)
					assert.Loosely(t, issueCount(), should.Equal(1))
				})
			})
			t.Run("dispersion criteria must be met to file a new bug", func(t *ftt.Test) {
				t.Run("met via User CLs with failures", func(t *ftt.Test) {
					suggestedClusters[1].DistinctUserCLsWithFailures7d.Residual = 3
					suggestedClusters[1].PostsubmitBuildsWithFailures7d.Residual = 0

					// Act
					err = UpdateBugsForProject(ctx, opts)

					// Verify
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)
					assert.Loosely(t, expectBuganizerBug(buganizerStore, expectedBuganizerBug), should.BeNil)
					assert.Loosely(t, issueCount(), should.Equal(1))
				})
				t.Run("met via Postsubmit builds with failures", func(t *ftt.Test) {
					suggestedClusters[1].DistinctUserCLsWithFailures7d.Residual = 0
					suggestedClusters[1].PostsubmitBuildsWithFailures7d.Residual = 1

					// Act
					err = UpdateBugsForProject(ctx, opts)

					// Verify
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)
					assert.Loosely(t, expectBuganizerBug(buganizerStore, expectedBuganizerBug), should.BeNil)
					assert.Loosely(t, issueCount(), should.Equal(1))
				})
				t.Run("not met", func(t *ftt.Test) {
					suggestedClusters[1].DistinctUserCLsWithFailures7d.Residual = 0
					suggestedClusters[1].PostsubmitBuildsWithFailures7d.Residual = 0

					// Act
					err = UpdateBugsForProject(ctx, opts)

					// Verify
					assert.Loosely(t, err, should.BeNil)
					// No bug should be created.
					assert.Loosely(t, verifyRulesResemble(ctx, t, nil), should.BeNil)
					assert.Loosely(t, issueCount(), should.BeZero)
				})
			})
			t.Run("duplicate bugs are suppressed", func(t *ftt.Test) {
				t.Run("where a rule was recently filed for the same suggested cluster, and reclustering is pending", func(t *ftt.Test) {
					createTime := time.Date(2021, time.January, 5, 12, 30, 0, 0, time.UTC)
					buganizerStore.StoreIssue(ctx, buganizer.NewFakeIssue(1))
					existingRule := rules.NewRule(1).
						WithBugSystem(bugs.BuganizerSystem).
						WithProject(project).
						WithCreateTime(createTime).
						WithPredicateLastUpdateTime(createTime.Add(1 * time.Hour)).
						WithLastAuditableUpdateTime(createTime.Add(2 * time.Hour)).
						WithLastUpdateTime(createTime.Add(3 * time.Hour)).
						WithBugPriorityManaged(true).
						WithBugPriorityManagedLastUpdateTime(createTime.Add(1 * time.Hour)).
						WithSourceCluster(sourceClusterID).Build()
					err := rules.SetForTesting(ctx, t, []*rules.Entry{
						existingRule,
					})
					assert.Loosely(t, err, should.BeNil)

					// Initially do not expect a new bug to be filed.
					err = UpdateBugsForProject(ctx, opts)

					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, verifyRulesResemble(ctx, t, []*rules.Entry{existingRule}), should.BeNil)
					assert.Loosely(t, issueCount(), should.Equal(1))

					// Once re-clustering has incorporated the version of rules
					// that included this new rule, it is OK to file another bug
					// for the suggested cluster if sufficient impact remains.
					// This should only happen when the rule definition has been
					// manually narrowed in some way from the originally filed bug.
					err = runs.SetRunsForTesting(ctx, t, []*runs.ReclusteringRun{
						runs.NewRun(0).
							WithProject(project).
							WithAlgorithmsVersion(algorithms.AlgorithmsVersion).
							WithConfigVersion(projectCfg.LastUpdated.AsTime()).
							WithRulesVersion(createTime).
							WithCompletedProgress().Build(),
					})
					assert.Loosely(t, err, should.BeNil)
					progress, err := runs.ReadReclusteringProgress(ctx, project)
					assert.Loosely(t, err, should.BeNil)
					opts.ReclusteringProgress = progress

					// Act
					err = UpdateBugsForProject(ctx, opts)

					// Verify
					assert.Loosely(t, err, should.BeNil)
					expectedBuganizerBug.ID = 2 // Because we already created a bug with ID 1 above.
					expectedRule.BugID.ID = "2"
					assert.Loosely(t, verifyRulesResemble(ctx, t, []*rules.Entry{expectedRule, existingRule}), should.BeNil)
					assert.Loosely(t, expectBuganizerBug(buganizerStore, expectedBuganizerBug), should.BeNil)
					assert.Loosely(t, issueCount(), should.Equal(2))
				})
				t.Run("when re-clustering to new algorithms", func(t *ftt.Test) {
					err = runs.SetRunsForTesting(ctx, t, []*runs.ReclusteringRun{
						runs.NewRun(0).
							WithProject(project).
							WithAlgorithmsVersion(algorithms.AlgorithmsVersion - 1).
							WithConfigVersion(projectCfg.LastUpdated.AsTime()).
							WithRulesVersion(rules.StartingEpoch).
							WithCompletedProgress().Build(),
					})
					assert.Loosely(t, err, should.BeNil)
					progress, err := runs.ReadReclusteringProgress(ctx, project)
					assert.Loosely(t, err, should.BeNil)
					opts.ReclusteringProgress = progress

					// Act
					err = UpdateBugsForProject(ctx, opts)

					// Verify no bugs were filed.
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, verifyRulesResemble(ctx, t, nil), should.BeNil)
					assert.Loosely(t, issueCount(), should.BeZero)
				})
				t.Run("when re-clustering to new config", func(t *ftt.Test) {
					err = runs.SetRunsForTesting(ctx, t, []*runs.ReclusteringRun{
						runs.NewRun(0).
							WithProject(project).
							WithAlgorithmsVersion(algorithms.AlgorithmsVersion).
							WithConfigVersion(projectCfg.LastUpdated.AsTime().Add(-1 * time.Hour)).
							WithRulesVersion(rules.StartingEpoch).
							WithCompletedProgress().Build(),
					})
					assert.Loosely(t, err, should.BeNil)
					progress, err := runs.ReadReclusteringProgress(ctx, project)
					assert.Loosely(t, err, should.BeNil)
					opts.ReclusteringProgress = progress

					// Act
					err = UpdateBugsForProject(ctx, opts)

					// Verify no bugs were filed.
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, verifyRulesResemble(ctx, t, nil), should.BeNil)
					assert.Loosely(t, issueCount(), should.BeZero)
				})
			})
			t.Run("bugs are routed to the correct issue tracker and component", func(t *ftt.Test) {
				// As currently only Buganizer is supported, issues should always
				// be routed there.

				suggestedClusters[1].TopBuganizerComponents = []analysis.TopCount{
					{Value: "77777", Count: 20},
				}
				expectedBuganizerBug.Component = 77777

				suggestedClusters[1].TopMonorailComponents = []analysis.TopCount{
					{Value: "Blink>Layout", Count: 40},  // >30% of failures.
					{Value: "Blink>Network", Count: 31}, // >30% of failures.
					{Value: "Blink>Other", Count: 4},
				}
				expectedRule.BugID = bugs.BugID{
					System: "buganizer",
					ID:     "1",
				}

				// Act
				err = UpdateBugsForProject(ctx, opts)

				// Verify
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)
				assert.Loosely(t, expectBuganizerBug(buganizerStore, expectedBuganizerBug), should.BeNil)
				assert.Loosely(t, issueCount(), should.Equal(1))
			})
			t.Run("partial success creating bugs is correctly handled", func(t *ftt.Test) {
				// Inject an error updating the bug after creation.
				buganizerClient.CreateCommentError = status.Errorf(codes.Internal, "internal error creating comment")

				// Act
				err = UpdateBugsForProject(ctx, opts)

				// Do not expect policy activations to have been notified.
				expectedBuganizerBug.ExpectedPolicyIDsActivated = []string{}
				expectedRule.BugManagementState.PolicyState["exoneration-policy"].ActivationNotified = false

				// Verify the rule was still created.
				assert.Loosely(t, err, should.ErrLike("internal error creating comment"))
				assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)
				assert.Loosely(t, expectBuganizerBug(buganizerStore, expectedBuganizerBug), should.BeNil)
				assert.Loosely(t, issueCount(), should.Equal(1))
			})
		})
		t.Run("With both failure reason and test name clusters above bug-filing threshold", func(t *ftt.Test) {
			// Reason cluster above the 1-day exoneration threshold.
			suggestedClusters[2] = makeReasonCluster(t, compiledCfg, 2)
			suggestedClusters[2].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{
				OneDay:   metrics.Counts{Residual: 100},
				ThreeDay: metrics.Counts{Residual: 100},
				SevenDay: metrics.Counts{Residual: 100},
			}
			suggestedClusters[2].PostsubmitBuildsWithFailures7d.Residual = 1

			// Test name cluster with 33% more impact.
			suggestedClusters[1] = makeTestNameCluster(t, compiledCfg, 3)
			suggestedClusters[1].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{
				OneDay:   metrics.Counts{Residual: 133},
				ThreeDay: metrics.Counts{Residual: 133},
				SevenDay: metrics.Counts{Residual: 133},
			}
			suggestedClusters[1].PostsubmitBuildsWithFailures7d.Residual = 1

			// Limit to one bug filed each time, so that
			// we test change throttling.
			opts.MaxBugsFiledPerRun = 1

			t.Run("reason clusters preferred over test name clusters", func(t *ftt.Test) {
				// Test name cluster has <34% more impact than the reason
				// cluster.

				// Act
				err = UpdateBugsForProject(ctx, opts)

				// Verify reason cluster filed.
				rs, err := rules.ReadAllForTesting(span.Single(ctx))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(rs), should.Equal(1))
				assert.Loosely(t, rs[0].SourceCluster, should.Match(suggestedClusters[2].ClusterID))
				assert.Loosely(t, rs[0].SourceCluster.IsFailureReasonCluster(), should.BeTrue)
			})
			t.Run("test name clusters can be filed if significantly more impact", func(t *ftt.Test) {
				// Increase impact of the test name cluster so that the
				// test name cluster has >34% more impact than the reason
				// cluster.
				suggestedClusters[1].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{
					OneDay:   metrics.Counts{Residual: 135},
					ThreeDay: metrics.Counts{Residual: 135},
					SevenDay: metrics.Counts{Residual: 135},
				}

				// Act
				err = UpdateBugsForProject(ctx, opts)

				// Verify test name cluster filed.
				rs, err := rules.ReadAllForTesting(span.Single(ctx))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(rs), should.Equal(1))
				assert.Loosely(t, rs[0].SourceCluster, should.Match(suggestedClusters[1].ClusterID))
				assert.Loosely(t, rs[0].SourceCluster.IsTestNameCluster(), should.BeTrue)
			})
		})
		t.Run("With multiple rules / bugs on file", func(t *ftt.Test) {
			// Use a mix of test name and failure reason clusters for
			// code path coverage.
			suggestedClusters[0] = makeTestNameCluster(t, compiledCfg, 0)
			suggestedClusters[0].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{
				OneDay:   metrics.Counts{Residual: 940},
				ThreeDay: metrics.Counts{Residual: 940},
				SevenDay: metrics.Counts{Residual: 940},
			}
			suggestedClusters[0].PostsubmitBuildsWithFailures7d.Residual = 1

			suggestedClusters[1] = makeReasonCluster(t, compiledCfg, 1)
			suggestedClusters[1].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{
				OneDay:   metrics.Counts{Residual: 300},
				ThreeDay: metrics.Counts{Residual: 300},
				SevenDay: metrics.Counts{Residual: 300},
			}
			suggestedClusters[1].PostsubmitBuildsWithFailures7d.Residual = 1

			suggestedClusters[2] = makeReasonCluster(t, compiledCfg, 2)
			suggestedClusters[2].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{
				OneDay:   metrics.Counts{Residual: 250},
				ThreeDay: metrics.Counts{Residual: 250},
				SevenDay: metrics.Counts{Residual: 250},
			}
			suggestedClusters[2].PostsubmitBuildsWithFailures7d.Residual = 1
			suggestedClusters[2].TopMonorailComponents = []analysis.TopCount{
				{Value: "Monorail", Count: 250},
			}

			suggestedClusters[3] = makeReasonCluster(t, compiledCfg, 3)
			suggestedClusters[3].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{
				OneDay:   metrics.Counts{Residual: 200},
				ThreeDay: metrics.Counts{Residual: 200},
				SevenDay: metrics.Counts{Residual: 200},
			}
			suggestedClusters[3].PostsubmitBuildsWithFailures7d.Residual = 1
			suggestedClusters[3].TopMonorailComponents = []analysis.TopCount{
				{Value: "Monorail", Count: 200},
			}

			expectedRules := []*rules.Entry{
				{
					Project:                 "chromeos",
					RuleDefinition:          `test = "testname-0"`,
					BugID:                   bugs.BugID{System: bugs.BuganizerSystem, ID: "1"},
					SourceCluster:           suggestedClusters[0].ClusterID,
					IsActive:                true,
					IsManagingBug:           true,
					IsManagingBugPriority:   true,
					CreateUser:              rules.LUCIAnalysisSystem,
					LastAuditableUpdateUser: rules.LUCIAnalysisSystem,
					BugManagementState: &bugspb.BugManagementState{
						RuleAssociationNotified: true,
						PolicyState: map[string]*bugspb.BugManagementState_PolicyState{
							"exoneration-policy": {
								IsActive:           true,
								LastActivationTime: timestamppb.New(opts.RunTimestamp),
								ActivationNotified: true,
							},
							"cls-rejected-policy": {},
						},
					},
				},
				{
					Project:                 "chromeos",
					RuleDefinition:          `reason LIKE "want foo, got bar"`,
					BugID:                   bugs.BugID{System: bugs.BuganizerSystem, ID: "2"},
					SourceCluster:           suggestedClusters[1].ClusterID,
					IsActive:                true,
					IsManagingBug:           true,
					IsManagingBugPriority:   true,
					CreateUser:              rules.LUCIAnalysisSystem,
					LastAuditableUpdateUser: rules.LUCIAnalysisSystem,
					BugManagementState: &bugspb.BugManagementState{
						RuleAssociationNotified: true,
						PolicyState: map[string]*bugspb.BugManagementState_PolicyState{
							"exoneration-policy": {
								IsActive:           true,
								LastActivationTime: timestamppb.New(opts.RunTimestamp),
								ActivationNotified: true,
							},
							"cls-rejected-policy": {},
						},
					},
				},
				{
					Project:                 "chromeos",
					RuleDefinition:          `reason LIKE "want foofoo, got bar"`,
					BugID:                   bugs.BugID{System: bugs.BuganizerSystem, ID: "3"},
					SourceCluster:           suggestedClusters[2].ClusterID,
					IsActive:                true,
					IsManagingBug:           true,
					IsManagingBugPriority:   true,
					CreateUser:              rules.LUCIAnalysisSystem,
					LastAuditableUpdateUser: rules.LUCIAnalysisSystem,
					BugManagementState: &bugspb.BugManagementState{
						RuleAssociationNotified: true,
						PolicyState: map[string]*bugspb.BugManagementState_PolicyState{
							"exoneration-policy": {
								IsActive:           true,
								LastActivationTime: timestamppb.New(opts.RunTimestamp),
								ActivationNotified: true,
							},
							"cls-rejected-policy": {},
						},
					},
				},
				{
					Project:                 "chromeos",
					RuleDefinition:          `reason LIKE "want foofoofoo, got bar"`,
					BugID:                   bugs.BugID{System: bugs.BuganizerSystem, ID: "4"},
					SourceCluster:           suggestedClusters[3].ClusterID,
					IsActive:                true,
					IsManagingBug:           true,
					IsManagingBugPriority:   true,
					CreateUser:              rules.LUCIAnalysisSystem,
					LastAuditableUpdateUser: rules.LUCIAnalysisSystem,
					BugManagementState: &bugspb.BugManagementState{
						RuleAssociationNotified: true,
						PolicyState: map[string]*bugspb.BugManagementState_PolicyState{
							"exoneration-policy": {
								IsActive:           true,
								LastActivationTime: timestamppb.New(opts.RunTimestamp),
								ActivationNotified: true,
							},
							"cls-rejected-policy": {},
						},
					},
				},
			}

			// The offset of the first monorail rule in the rules slice.
			// (Rules read by rules.Read...() are sorted by bug system and bug ID,
			// so monorail always appears after Buganizer.)
			const firstMonorailRuleIndex = 2

			// Limit to one bug filed each time, so that
			// we test change throttling.
			opts.MaxBugsFiledPerRun = 1

			// Verify one bug is filed at a time.
			for i := 0; i < len(expectedRules); i++ {
				// Act
				err = UpdateBugsForProject(ctx, opts)

				// Verify
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules[:i+1]), should.BeNil)
			}

			// Further updates do nothing.
			err = UpdateBugsForProject(ctx, opts)

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)

			rs, err := rules.ReadAllForTesting(span.Single(ctx))
			assert.Loosely(t, err, should.BeNil)

			bugClusters := []*analysis.Cluster{
				makeBugCluster(rs[0].RuleID),
				makeBugCluster(rs[1].RuleID),
				makeBugCluster(rs[2].RuleID),
				makeBugCluster(rs[3].RuleID),
			}

			t.Run("if re-clustering in progress", func(t *ftt.Test) {
				analysisClient.clusters = append(suggestedClusters, bugClusters...)

				t.Run("negligable cluster metrics does not affect issue priority, status or active policies", func(t *ftt.Test) {
					// The policy should already be active from previous setup.
					assert.Loosely(t, expectedRules[0].BugManagementState.PolicyState["exoneration-policy"].IsActive, should.BeTrue)

					issue := buganizerStore.Issues[1]
					originalPriority := issue.Issue.IssueState.Priority
					originalStatus := issue.Issue.IssueState.Status
					assert.Loosely(t, originalStatus, should.NotEqual(issuetracker.Issue_VERIFIED))

					SetResidualMetrics(bugClusters[1], bugs.ClusterMetrics{
						metrics.CriticalFailuresExonerated.ID: bugs.MetricValues{},
					})

					// Act
					err = UpdateBugsForProject(ctx, opts)

					// Verify.
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, issue.Issue.IssueState.Priority, should.Equal(originalPriority))
					assert.Loosely(t, issue.Issue.IssueState.Status, should.Equal(originalStatus))
					assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)
				})
			})
			t.Run("with re-clustering complete", func(t *ftt.Test) {
				analysisClient.clusters = append(suggestedClusters, bugClusters...)

				// Move residual impact from suggested clusters to new bug clusters.
				bugClusters[0].MetricValues = suggestedClusters[0].MetricValues
				bugClusters[1].MetricValues = suggestedClusters[1].MetricValues
				bugClusters[2].MetricValues = suggestedClusters[2].MetricValues
				bugClusters[3].MetricValues = suggestedClusters[3].MetricValues

				// Clear residual impact on suggested clusters to inhibit
				// further bug filing.
				suggestedClusters[0].MetricValues = emptyMetricValues()
				suggestedClusters[1].MetricValues = emptyMetricValues()
				suggestedClusters[2].MetricValues = emptyMetricValues()
				suggestedClusters[3].MetricValues = emptyMetricValues()

				// Mark reclustering complete.
				err := runs.SetRunsForTesting(ctx, t, []*runs.ReclusteringRun{
					runs.NewRun(0).
						WithProject(project).
						WithAlgorithmsVersion(algorithms.AlgorithmsVersion).
						WithConfigVersion(projectCfg.LastUpdated.AsTime()).
						WithRulesVersion(rs[3].PredicateLastUpdateTime).
						WithCompletedProgress().Build(),
				})
				assert.Loosely(t, err, should.BeNil)

				progress, err := runs.ReadReclusteringProgress(ctx, project)
				assert.Loosely(t, err, should.BeNil)
				opts.ReclusteringProgress = progress

				opts.RunTimestamp = opts.RunTimestamp.Add(10 * time.Minute)

				t.Run("policy activation", func(t *ftt.Test) {
					// Verify updates work, even when rules are in later batches.
					opts.UpdateRuleBatchSize = 1

					t.Run("policy remains inactive if activation threshold unmet", func(t *ftt.Test) {
						// The policy should be inactive from previous setup.
						expectedPolicyState := expectedRules[1].BugManagementState.PolicyState["cls-rejected-policy"]
						assert.Loosely(t, expectedPolicyState.IsActive, should.BeFalse)

						// Set metrics just below the policy activation threshold.
						bugClusters[1].MetricValues[metrics.HumanClsFailedPresubmit.ID] = metrics.TimewiseCounts{
							OneDay:   metrics.Counts{Residual: 9},
							ThreeDay: metrics.Counts{Residual: 9},
							SevenDay: metrics.Counts{Residual: 9},
						}

						// Act
						err = UpdateBugsForProject(ctx, opts)

						// Verify policy activation unchanged.
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)
					})
					t.Run("policy activates if activation threshold met", func(t *ftt.Test) {
						// The policy should be inactive from previous setup.
						expectedPolicyState := expectedRules[1].BugManagementState.PolicyState["cls-rejected-policy"]
						assert.Loosely(t, expectedPolicyState.IsActive, should.BeFalse)
						assert.Loosely(t, expectedPolicyState.ActivationNotified, should.BeFalse)

						// Update metrics so that policy should activate.
						bugClusters[1].MetricValues[metrics.HumanClsFailedPresubmit.ID] = metrics.TimewiseCounts{
							OneDay:   metrics.Counts{Residual: 0},
							ThreeDay: metrics.Counts{Residual: 0},
							SevenDay: metrics.Counts{Residual: 10},
						}

						issue := buganizerStore.Issues[2]
						assert.Loosely(t, expectedRules[1].BugID, should.Match(bugs.BugID{System: bugs.BuganizerSystem, ID: "2"}))
						existingCommentCount := len(issue.Comments)

						// Act
						err = UpdateBugsForProject(ctx, opts)

						// Verify policy activates.
						assert.Loosely(t, err, should.BeNil)
						expectedPolicyState.IsActive = true
						expectedPolicyState.LastActivationTime = timestamppb.New(opts.RunTimestamp)
						expectedPolicyState.ActivationNotified = true
						assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)

						// Expect comments to be posted.
						assert.Loosely(t, issue.Comments, should.HaveLength(existingCommentCount+2))
						assert.Loosely(t, issue.Comments[2].Comment, should.ContainSubstring(
							"Why LUCI Analysis posted this comment: https://luci-analysis-test.appspot.com/help#policy-activated (Policy ID: cls-rejected-policy)"))
						assert.Loosely(t, issue.Comments[3].Comment, should.ContainSubstring(
							"The bug priority has been increased from P2 to P1."))
					})
					t.Run("policy remains active if deactivation threshold unmet", func(t *ftt.Test) {
						// The policy should already be active from previous setup.
						expectedPolicyState := expectedRules[0].BugManagementState.PolicyState["exoneration-policy"]
						assert.Loosely(t, expectedPolicyState.IsActive, should.BeTrue)

						// Metrics still meet/exceed the deactivation threshold, so deactivation is inhibited.
						bugClusters[0].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{
							OneDay:   metrics.Counts{Residual: 10},
							ThreeDay: metrics.Counts{Residual: 10},
							SevenDay: metrics.Counts{Residual: 10},
						}

						// Act
						err = UpdateBugsForProject(ctx, opts)

						// Verify policy activation should be unchanged.
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)
					})
					t.Run("policy deactivates if deactivation threshold met", func(t *ftt.Test) {
						// The policy should already be active from previous setup.
						expectedPolicyState := expectedRules[0].BugManagementState.PolicyState["exoneration-policy"]
						assert.Loosely(t, expectedPolicyState.IsActive, should.BeTrue)

						// Update metrics so that policy should de-activate.
						bugClusters[0].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{
							OneDay:   metrics.Counts{Residual: 9},
							ThreeDay: metrics.Counts{Residual: 9},
							SevenDay: metrics.Counts{Residual: 9},
						}

						// Act
						err = UpdateBugsForProject(ctx, opts)

						// Verify policy deactivated.
						assert.Loosely(t, err, should.BeNil)
						expectedPolicyState.IsActive = false
						expectedPolicyState.LastDeactivationTime = timestamppb.New(opts.RunTimestamp)
						assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)
					})
					t.Run("policy configuration changes are handled", func(t *ftt.Test) {
						// Delete the existing policy named "exoneration-policy", and replace it with a new policy,
						// "new-exoneration-policy". Activation and de-activation criteria remain the same.
						projectCfg.BugManagement.Policies[0].Id = "new-exoneration-policy"

						// Act
						err = UpdateBugsForProject(ctx, opts)

						// Verify state for the old policy is deleted, and state for the new policy is added.
						assert.Loosely(t, err, should.BeNil)
						expectedRules[0].BugManagementState.PolicyState = map[string]*bugspb.BugManagementState_PolicyState{
							"new-exoneration-policy": {
								// The new policy should activate, because the metrics justify its activation.
								IsActive:           true,
								LastActivationTime: timestamppb.New(opts.RunTimestamp),
							},
							"cls-rejected-policy": {},
						}
						expectedRules[1].BugManagementState.PolicyState = map[string]*bugspb.BugManagementState_PolicyState{
							"new-exoneration-policy": {},
							"cls-rejected-policy":    {},
						}
						expectedRules[2].BugManagementState.PolicyState = map[string]*bugspb.BugManagementState_PolicyState{
							"new-exoneration-policy": {},
							"cls-rejected-policy":    {},
						}
					})
				})
				t.Run("rule associated notification", func(t *ftt.Test) {
					t.Run("buganizer", func(t *ftt.Test) {
						// Select a Buganizer issue.
						issue := buganizerStore.Issues[1]

						// Get the corresponding rule, confirming we got the right one.
						rule := rs[0]
						assert.Loosely(t, rule.BugID.ID, should.Equal(fmt.Sprintf("%v", issue.Issue.IssueId)))

						// Reset RuleAssociationNotified on the rule.
						rule.BugManagementState.RuleAssociationNotified = false
						assert.Loosely(t, rules.SetForTesting(ctx, t, rs), should.BeNil)

						originalCommentCount := len(issue.Comments)

						// Act
						err = UpdateBugsForProject(ctx, opts)

						// Verify
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)
						assert.Loosely(t, issue.Comments, should.HaveLength(originalCommentCount+1))
						assert.Loosely(t, issue.Comments[originalCommentCount].Comment, should.Equal(
							"This bug has been associated with failures in LUCI Analysis."+
								" To view failure examples or update the association, go to LUCI Analysis at: https://luci-analysis-test.appspot.com/p/chromeos/rules/"+rule.RuleID))

						// Further runs should not lead to repeated posting of the comment.
						err = UpdateBugsForProject(ctx, opts)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)
						assert.Loosely(t, issue.Comments, should.HaveLength(originalCommentCount+1))
					})
				})
				t.Run("invalidated user bug closures", func(t *ftt.Test) {
					t.Run("buganizer", func(t *ftt.Test) {
						t.Run("create new bug if bug closure is invalidated", func(t *ftt.Test) {
							assert.Loosely(t, len(buganizerStore.Issues), should.Equal(4))
							buganizerStore.Issues[1].Issue.IssueState.Status = issuetracker.Issue_FIXED
							now := time.Now()
							twentyFiveHoursAgo := now.Add(-25 * time.Hour)
							buganizerStore.Issues[1].Issue.ResolvedTime = timestamppb.New(twentyFiveHoursAgo)
							expectedRules[0].IsActive = true
							expectedRules[0].BugID = bugs.BugID{System: bugs.BuganizerSystem, ID: "5"}
							expectedBuganizerBug := buganizerBug{
								ID:            5,
								Component:     projectCfg.BugManagement.Buganizer.DefaultComponent.Id,
								ExpectedTitle: buganizerStore.Issues[1].Issue.IssueState.Title,
								ExpectedContent: []string{
									"This bug re-raises b/1 for all test failures matching:",   // Old bug is mentioned.
									"https://luci-analysis-test.appspot.com/p/chromeos/rules/", // Rule ID randomly generated.
									"testname-0", // Test name is included in rule definition.
								},
								ExpectedPolicyIDsActivated: []string{
									"exoneration-policy",
								},
							}

							err = UpdateBugsForProject(ctx, opts)

							assert.Loosely(t, err, should.BeNil)
							assert.Loosely(t, len(buganizerStore.Issues), should.Equal(5))
							assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)
							assert.Loosely(t, expectBuganizerBug(buganizerStore, expectedBuganizerBug), should.BeNil)

							// Further updates do nothing.
							err = UpdateBugsForProject(ctx, opts)

							assert.Loosely(t, err, should.BeNil)
							assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)
							assert.Loosely(t, len(buganizerStore.Issues), should.Equal(5))
						})
					})
				})
				t.Run("priority updates and auto-closure", func(t *ftt.Test) {
					t.Run("buganizer", func(t *ftt.Test) {
						// Select a Buganizer issue.
						issue := buganizerStore.Issues[1]
						originalPriority := issue.Issue.IssueState.Priority
						originalStatus := issue.Issue.IssueState.Status
						assert.Loosely(t, originalStatus, should.Equal(issuetracker.Issue_NEW))

						// Get the corresponding rule, confirming we got the right one.
						rule := rs[0]
						assert.Loosely(t, rule.BugID.ID, should.Equal(fmt.Sprintf("%v", issue.Issue.IssueId)))

						// Activate the cls-rejected-policy, which should raise the priority to P1.
						assert.Loosely(t, originalPriority, should.NotEqual(issuetracker.Issue_P1))
						SetResidualMetrics(bugClusters[0], bugs.ClusterMetrics{
							metrics.CriticalFailuresExonerated.ID: bugs.MetricValues{OneDay: 100},
							metrics.HumanClsFailedPresubmit.ID:    bugs.MetricValues{SevenDay: 10},
						})

						t.Run("priority updates to reflect active policies", func(t *ftt.Test) {
							expectedRules[0].BugManagementState.PolicyState["cls-rejected-policy"].IsActive = true
							expectedRules[0].BugManagementState.PolicyState["cls-rejected-policy"].LastActivationTime = timestamppb.New(opts.RunTimestamp)
							expectedRules[0].BugManagementState.PolicyState["cls-rejected-policy"].ActivationNotified = true
							assert.Loosely(t, originalPriority, should.NotEqual(issuetracker.Issue_P1))

							// Act
							err = UpdateBugsForProject(ctx, opts)

							// Verify
							assert.Loosely(t, err, should.BeNil)
							assert.Loosely(t, issue.Issue.IssueState.Priority, should.Equal(issuetracker.Issue_P1))
							assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)
						})
						t.Run("disabling IsManagingBugPriority prevents priority updates", func(t *ftt.Test) {
							expectedRules[0].BugManagementState.PolicyState["cls-rejected-policy"].IsActive = true
							expectedRules[0].BugManagementState.PolicyState["cls-rejected-policy"].LastActivationTime = timestamppb.New(opts.RunTimestamp)
							expectedRules[0].BugManagementState.PolicyState["cls-rejected-policy"].ActivationNotified = true

							// Set IsManagingBugPriority to false on the rule.
							rule.IsManagingBugPriority = false
							assert.Loosely(t, rules.SetForTesting(ctx, t, rs), should.BeNil)

							// Act
							err = UpdateBugsForProject(ctx, opts)

							// Verify
							assert.Loosely(t, err, should.BeNil)

							// Check that the bug priority and status has not changed.
							assert.Loosely(t, issue.Issue.IssueState.Status, should.Equal(originalStatus))
							assert.Loosely(t, issue.Issue.IssueState.Priority, should.Equal(originalPriority))

							// Check the rules have not changed except for the IsManagingBugPriority change.
							expectedRules[0].IsManagingBugPriority = false
							assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)
						})
						t.Run("manually setting a priority prevents bug updates", func(t *ftt.Test) {
							expectedRules[0].BugManagementState.PolicyState["cls-rejected-policy"].IsActive = true
							expectedRules[0].BugManagementState.PolicyState["cls-rejected-policy"].LastActivationTime = timestamppb.New(opts.RunTimestamp)
							expectedRules[0].BugManagementState.PolicyState["cls-rejected-policy"].ActivationNotified = true

							issue.IssueUpdates = append(issue.IssueUpdates, &issuetracker.IssueUpdate{
								Author: &issuetracker.User{
									EmailAddress: "testuser@google.com",
								},
								Timestamp: timestamppb.New(clock.Now(ctx).Add(time.Minute * 4)),
								FieldUpdates: []*issuetracker.FieldUpdate{
									{
										Field: "priority",
									},
								},
							})

							t.Run("happy path", func(t *ftt.Test) {
								// Act
								err = UpdateBugsForProject(ctx, opts)

								// Verify
								assert.Loosely(t, err, should.BeNil)
								assert.Loosely(t, issue.Issue.IssueState.Status, should.Equal(originalStatus))
								assert.Loosely(t, issue.Issue.IssueState.Priority, should.Equal(originalPriority))
								expectedRules[0].IsManagingBugPriority = false
								assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)

								t.Run("further updates leave no comments", func(t *ftt.Test) {
									initialComments := len(issue.Comments)

									// Act
									err = UpdateBugsForProject(ctx, opts)

									// Verify
									assert.Loosely(t, err, should.BeNil)
									assert.Loosely(t, len(issue.Comments), should.Equal(initialComments))
									assert.Loosely(t, issue.Issue.IssueState.Status, should.Equal(originalStatus))
									assert.Loosely(t, issue.Issue.IssueState.Priority, should.Equal(originalPriority))
									assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)
								})
							})

							t.Run("errors updating other bugs", func(t *ftt.Test) {
								// Check we handle partial success correctly:
								// Even if there is an error updating another bug, if we comment on a bug
								// to say the user took manual priority control, we must commit
								// the rule update setting IsManagingBugPriority to false. Otherwise
								// we may get stuck in a loop where we comment on the bug every
								// time bug filing runs.

								// Trigger a priority update for bug 2 in addition to the
								// manual priority update.
								SetResidualMetrics(bugClusters[1], bugs.ClusterMetrics{
									metrics.CriticalFailuresExonerated.ID: bugs.MetricValues{OneDay: 100},
									metrics.HumanClsFailedPresubmit.ID:    bugs.MetricValues{SevenDay: 10},
								})

								// But prevent LUCI Analysis from applying that priority update, due to an error.
								modifyError := errors.New("this issue may not be modified")
								buganizerStore.Issues[2].UpdateError = modifyError

								// Act
								err = UpdateBugsForProject(ctx, opts)

								// Verify

								// The error modifying bug 2 is bubbled up.
								assert.Loosely(t, err, should.NotBeNil)
								assert.Loosely(t, errors.Is(err, modifyError), should.BeTrue)

								// The policy on the bug 2 was activated, and we notified
								// bug 2 of the policy activation, even if we did
								// not succeed then updating its priority.
								// Furthermore, we record that we notified the policy
								// activation, so repeated notifications do not occur.
								expectedRules[1].BugManagementState.PolicyState["cls-rejected-policy"].IsActive = true
								expectedRules[1].BugManagementState.PolicyState["cls-rejected-policy"].LastActivationTime = timestamppb.New(opts.RunTimestamp)
								expectedRules[1].BugManagementState.PolicyState["cls-rejected-policy"].ActivationNotified = true

								otherIssue := buganizerStore.Issues[2]
								assert.Loosely(t, otherIssue.Comments[len(otherIssue.Comments)-1].Comment, should.ContainSubstring(
									"Why LUCI Analysis posted this comment: https://luci-analysis-test.appspot.com/help#policy-activated (Policy ID: cls-rejected-policy)"))

								// Despite the issue with bug 2, bug 1 was commented on updated and
								// IsManagingBugPriority was set to false.
								expectedRules[0].IsManagingBugPriority = false
								assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)
								assert.Loosely(t, issue.Comments[len(issue.Comments)-1].Comment, should.ContainSubstring(
									"The bug priority has been manually set."))
								assert.Loosely(t, issue.Issue.IssueState.Status, should.Equal(originalStatus))
								assert.Loosely(t, issue.Issue.IssueState.Priority, should.Equal(originalPriority))
							})
						})
						t.Run("if all policies de-activate, bug is auto-closed", func(t *ftt.Test) {
							SetResidualMetrics(bugClusters[0], bugs.ClusterMetrics{
								metrics.CriticalFailuresExonerated.ID: bugs.MetricValues{OneDay: 9},
								metrics.HumanClsFailedPresubmit.ID:    bugs.MetricValues{},
							})

							// Act
							err = UpdateBugsForProject(ctx, opts)

							// Verify
							assert.Loosely(t, err, should.BeNil)
							expectedRules[0].BugManagementState.PolicyState["exoneration-policy"].IsActive = false
							expectedRules[0].BugManagementState.PolicyState["exoneration-policy"].LastDeactivationTime = timestamppb.New(opts.RunTimestamp)
							assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)
							assert.Loosely(t, issue.Issue.IssueState.Status, should.Equal(issuetracker.Issue_VERIFIED))
							assert.Loosely(t, issue.Issue.IssueState.Priority, should.Equal(issuetracker.Issue_P2))
						})
						t.Run("disabling IsManagingBug prevents bug closure", func(t *ftt.Test) {
							SetResidualMetrics(bugClusters[0], bugs.ClusterMetrics{
								metrics.CriticalFailuresExonerated.ID: bugs.MetricValues{OneDay: 9},
								metrics.HumanClsFailedPresubmit.ID:    bugs.MetricValues{},
							})
							expectedRules[0].BugManagementState.PolicyState["exoneration-policy"].IsActive = false
							expectedRules[0].BugManagementState.PolicyState["exoneration-policy"].LastDeactivationTime = timestamppb.New(opts.RunTimestamp)

							// Set IsManagingBug to false on the rule.
							rule.IsManagingBug = false
							assert.Loosely(t, rules.SetForTesting(ctx, t, rs), should.BeNil)

							// Act
							err = UpdateBugsForProject(ctx, opts)

							// Verify
							assert.Loosely(t, err, should.BeNil)

							// Check the rules have not changed except for the IsManagingBug change.
							expectedRules[0].IsManagingBug = false
							assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)

							// Check that the bug priority and status has not changed.
							assert.Loosely(t, issue.Issue.IssueState.Status, should.Equal(originalStatus))
							assert.Loosely(t, issue.Issue.IssueState.Priority, should.Equal(originalPriority))
						})
						t.Run("cluster disappearing closes issue", func(t *ftt.Test) {
							// Drop the corresponding bug cluster. This is consistent with
							// no more failures in the cluster occuring.
							bugClusters = bugClusters[1:]
							analysisClient.clusters = append(suggestedClusters, bugClusters...)

							// Act
							err = UpdateBugsForProject(ctx, opts)

							// Verify
							assert.Loosely(t, err, should.BeNil)
							expectedRules[0].BugManagementState.PolicyState["exoneration-policy"].IsActive = false
							expectedRules[0].BugManagementState.PolicyState["exoneration-policy"].LastDeactivationTime = timestamppb.New(opts.RunTimestamp)
							assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)
							assert.Loosely(t, issue.Issue.IssueState.Status, should.Equal(issuetracker.Issue_VERIFIED))

							t.Run("rule automatically archived after 30 days", func(t *ftt.Test) {
								tc.Add(time.Hour * 24 * 30)

								// Act
								err = UpdateBugsForProject(ctx, opts)

								// Verify
								assert.Loosely(t, err, should.BeNil)
								expectedRules[0].IsActive = false
								assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)
								assert.Loosely(t, issue.Issue.IssueState.Status, should.Equal(issuetracker.Issue_VERIFIED))
							})
						})
						t.Run("if all policies are removed, bug is auto-closed", func(t *ftt.Test) {
							projectCfg.BugManagement.Policies = nil
							err := config.SetTestProjectConfig(ctx, projectsCfg)
							assert.Loosely(t, err, should.BeNil)

							for _, expectedRule := range expectedRules {
								expectedRule.BugManagementState.PolicyState = nil
							}

							// Act
							err = UpdateBugsForProject(ctx, opts)

							// Verify
							assert.Loosely(t, err, should.BeNil)
							assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)
							assert.Loosely(t, issue.Issue.IssueState.Status, should.Equal(issuetracker.Issue_VERIFIED))
							assert.Loosely(t, issue.Issue.IssueState.Priority, should.Equal(issuetracker.Issue_P2))
						})
					})
				})
				t.Run("duplicate handling", func(t *ftt.Test) {
					t.Run("buganizer to buganizer", func(t *ftt.Test) {
						// Setup
						issueOne := buganizerStore.Issues[1]
						issueTwo := buganizerStore.Issues[2]
						issueOne.Issue.IssueState.Status = issuetracker.Issue_DUPLICATE
						issueOne.Issue.IssueState.CanonicalIssueId = issueTwo.Issue.IssueId

						issueOneOriginalCommentCount := len(issueOne.Comments)
						issueTwoOriginalCommentCount := len(issueTwo.Comments)

						// Ensure rule association and policy activation notified, so we
						// can confirm whether notifications are correctly reset.
						rs[0].BugManagementState.RuleAssociationNotified = true
						for _, policyState := range rs[0].BugManagementState.PolicyState {
							policyState.ActivationNotified = true
						}
						assert.Loosely(t, rules.SetForTesting(ctx, t, rs), should.BeNil)

						expectedRules[0].BugManagementState.RuleAssociationNotified = true
						for _, policyState := range expectedRules[0].BugManagementState.PolicyState {
							policyState.ActivationNotified = true
						}

						t.Run("happy path", func(t *ftt.Test) {
							// Act
							err = UpdateBugsForProject(ctx, opts)

							// Verify
							assert.Loosely(t, err, should.BeNil)
							expectedRules[0].IsActive = false
							expectedRules[1].RuleDefinition = "reason LIKE \"want foo, got bar\" OR\ntest = \"testname-0\""
							assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)

							assert.Loosely(t, issueOne.Comments, should.HaveLength(issueOneOriginalCommentCount+1))
							assert.Loosely(t, issueOne.Comments[issueOneOriginalCommentCount].Comment, should.ContainSubstring("LUCI Analysis has merged the failure association rule for this bug into the rule for the canonical bug."))
							assert.Loosely(t, issueOne.Comments[issueOneOriginalCommentCount].Comment, should.ContainSubstring(expectedRules[2].RuleID))

							assert.Loosely(t, issueTwo.Comments, should.HaveLength(issueTwoOriginalCommentCount))
						})
						t.Run("happy path, with comments for duplicate bugs disabled", func(t *ftt.Test) {
							// Setup
							projectCfg.BugManagement.DisableDuplicateBugComments = true
							projectsCfg := map[string]*configpb.ProjectConfig{
								project: projectCfg,
							}
							err = config.SetTestProjectConfig(ctx, projectsCfg)
							assert.Loosely(t, err, should.BeNil)

							// Act
							err = UpdateBugsForProject(ctx, opts)

							// Verify
							assert.Loosely(t, err, should.BeNil)
							expectedRules[0].IsActive = false
							expectedRules[1].RuleDefinition = "reason LIKE \"want foo, got bar\" OR\ntest = \"testname-0\""
							assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)

							assert.Loosely(t, issueOne.Comments, should.HaveLength(issueOneOriginalCommentCount))
							assert.Loosely(t, issueTwo.Comments, should.HaveLength(issueTwoOriginalCommentCount))
						})
						t.Run("happy path, bug marked as duplicate of bug without a rule in this project", func(t *ftt.Test) {
							// Setup
							issueOne.Issue.IssueState.Status = issuetracker.Issue_DUPLICATE
							issueOne.Issue.IssueState.CanonicalIssueId = 1234

							buganizerStore.StoreIssue(ctx, buganizer.NewFakeIssue(1234))

							extraRule := &rules.Entry{
								Project:                 "otherproject",
								RuleDefinition:          `reason LIKE "blah"`,
								RuleID:                  "1234567890abcdef1234567890abcdef",
								BugID:                   bugs.BugID{System: bugs.BuganizerSystem, ID: "1234"},
								IsActive:                true,
								IsManagingBug:           true,
								IsManagingBugPriority:   true,
								BugManagementState:      &bugspb.BugManagementState{},
								CreateUser:              "user@chromium.org",
								LastAuditableUpdateUser: "user@chromium.org",
							}
							_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
								ms, err := rules.Create(extraRule, "user@chromium.org")
								if err != nil {
									return err
								}
								span.BufferWrite(ctx, ms)
								return nil
							})
							assert.Loosely(t, err, should.BeNil)

							// Act
							err = UpdateBugsForProject(ctx, opts)

							// Verify
							assert.Loosely(t, err, should.BeNil)
							expectedRules[0].BugID = bugs.BugID{System: bugs.BuganizerSystem, ID: "1234"}
							// Should reset to false as we didn't create the destination bug.
							expectedRules[0].IsManagingBug = false
							// Should reset because of the change in associated bug.
							expectedRules[0].BugManagementState.RuleAssociationNotified = false
							for _, policyState := range expectedRules[0].BugManagementState.PolicyState {
								policyState.ActivationNotified = false
							}
							expectedRules = append(expectedRules, extraRule)
							assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)

							assert.Loosely(t, issueOne.Comments, should.HaveLength(issueOneOriginalCommentCount+1))
							assert.Loosely(t, issueOne.Comments[issueOneOriginalCommentCount].Comment, should.ContainSubstring("LUCI Analysis has merged the failure association rule for this bug into the rule for the canonical bug."))
							assert.Loosely(t, issueOne.Comments[issueOneOriginalCommentCount].Comment, should.ContainSubstring(expectedRules[0].RuleID))
						})
						t.Run("error cases", func(t *ftt.Test) {
							t.Run("bugs are in a duplicate bug cycle", func(t *ftt.Test) {
								// Note that this is a simple cycle with only two bugs.
								// The implementation allows for larger cycles, however.
								issueTwo.Issue.IssueState.Status = issuetracker.Issue_DUPLICATE
								issueTwo.Issue.IssueState.CanonicalIssueId = issueOne.Issue.IssueId

								// Act
								err = UpdateBugsForProject(ctx, opts)

								// Verify
								assert.Loosely(t, err, should.BeNil)

								// Issue one kicked out of duplicate status.
								assert.Loosely(t, issueOne.Issue.IssueState.Status, should.NotEqual(issuetracker.Issue_DUPLICATE))

								// As the cycle is now broken, issue two is merged into
								// issue one.
								expectedRules[0].RuleDefinition = "reason LIKE \"want foo, got bar\" OR\ntest = \"testname-0\""
								expectedRules[1].IsActive = false
								assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)

								assert.Loosely(t, issueOne.Comments, should.HaveLength(issueOneOriginalCommentCount+1))
								assert.Loosely(t, issueOne.Comments[issueOneOriginalCommentCount].Comment, should.ContainSubstring("a cycle was detected in the bug merged-into graph"))
							})
							t.Run("merged rule would be too long", func(t *ftt.Test) {
								// Setup
								// Make one of the rules we will be merging very close
								// to the rule length limit.
								longRule := fmt.Sprintf("test = \"%s\"", strings.Repeat("a", rules.MaxRuleDefinitionLength-10))

								_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
									issueOneRule, err := rules.ReadByBug(ctx, bugs.BugID{System: bugs.BuganizerSystem, ID: "1"})
									if err != nil {
										return err
									}
									issueOneRule[0].RuleDefinition = longRule

									ms, err := rules.Update(issueOneRule[0], rules.UpdateOptions{
										IsAuditableUpdate: true,
										PredicateUpdated:  true,
									}, rules.LUCIAnalysisSystem)
									if err != nil {
										return err
									}
									span.BufferWrite(ctx, ms)
									return nil
								})
								assert.Loosely(t, err, should.BeNil)

								// Act
								err = UpdateBugsForProject(ctx, opts)

								// Verify
								assert.Loosely(t, err, should.BeNil)

								// Rules should not have changed (except for the update we made).
								expectedRules[0].RuleDefinition = longRule
								assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)

								// Issue one kicked out of duplicate status.
								assert.Loosely(t, issueOne.Issue.IssueState.Status, should.NotEqual(issuetracker.Issue_DUPLICATE))

								// Comment should appear on the bug.
								assert.Loosely(t, issueOne.Comments, should.HaveLength(issueOneOriginalCommentCount+1))
								assert.Loosely(t, issueOne.Comments[issueOneOriginalCommentCount].Comment, should.ContainSubstring("the merged failure association rule would be too long"))
							})
							t.Run("bug marked as duplicate of bug we cannot access", func(t *ftt.Test) {
								issueTwo.ShouldReturnAccessPermissionError = true

								// Act
								err = UpdateBugsForProject(ctx, opts)

								// Verify issue one kicked out of duplicate status.
								assert.Loosely(t, err, should.BeNil)
								assert.Loosely(t, issueOne.Issue.IssueState.Status, should.NotEqual(issuetracker.Issue_DUPLICATE))
								assert.Loosely(t, issueOne.Comments, should.HaveLength(issueOneOriginalCommentCount+1))
								assert.Loosely(t, issueOne.Comments[issueOneOriginalCommentCount].Comment, should.ContainSubstring("LUCI Analysis cannot merge the association rule for this bug into the rule"))
							})
							t.Run("failed to handle duplicate bug - bug has an assignee", func(t *ftt.Test) {
								issueTwo.ShouldReturnAccessPermissionError = true

								// Has an assignee.
								issueOne.Issue.IssueState.Assignee = &issuetracker.User{
									EmailAddress: "user@google.com",
								}
								// Act
								err = UpdateBugsForProject(ctx, opts)

								// Verify issue is put back to assigned status, instead of New.
								assert.Loosely(t, err, should.BeNil)
								assert.Loosely(t, issueOne.Issue.IssueState.Status, should.Equal(issuetracker.Issue_ASSIGNED))
							})
							t.Run("failed to handle duplicate bug - bug has no assignee", func(t *ftt.Test) {
								issueTwo.ShouldReturnAccessPermissionError = true

								// Has no assignee.
								issueOne.Issue.IssueState.Assignee = nil

								// Act
								err = UpdateBugsForProject(ctx, opts)

								// Verify issue is put back to New status, instead of Assigned.
								assert.Loosely(t, err, should.BeNil)
								assert.Loosely(t, issueOne.Issue.IssueState.Status, should.Equal(issuetracker.Issue_NEW))
							})
						})
					})
				})
				t.Run("bug marked as archived should archive rule", func(t *ftt.Test) {
					t.Run("buganizer", func(t *ftt.Test) {
						issueOne := buganizerStore.Issues[1].Issue
						issueOne.IsArchived = true

						// Act
						err = UpdateBugsForProject(ctx, opts)
						assert.Loosely(t, err, should.BeNil)

						// Verify
						expectedRules[0].IsActive = false
						assert.Loosely(t, verifyRulesResemble(ctx, t, expectedRules), should.BeNil)
					})
				})
			})
		})
	})
}

func createProjectConfig() *configpb.ProjectConfig {
	return &configpb.ProjectConfig{
		BugManagement: &configpb.BugManagement{
			DefaultBugSystem: configpb.BugSystem_BUGANIZER,
			Buganizer:        buganizer.ChromeOSTestConfig(),
			Policies: []*configpb.BugManagementPolicy{
				createExonerationPolicy(),
				createCLsRejectedPolicy(),
			},
			BugClosureInvalidationAction: &configpb.BugManagement_FileNewBugs{},
		},
		LastUpdated: timestamppb.New(time.Date(2000, 1, 2, 3, 4, 5, 6, time.UTC)),
	}
}

func createExonerationPolicy() *configpb.BugManagementPolicy {
	return &configpb.BugManagementPolicy{
		Id:                "exoneration-policy",
		Owners:            []string{"username@google.com"},
		HumanReadableName: "test variant(s) are being exonerated in presubmit",
		Priority:          configpb.BuganizerPriority_P2,
		Metrics: []*configpb.BugManagementPolicy_Metric{
			{
				MetricId: metrics.CriticalFailuresExonerated.ID.String(),
				ActivationThreshold: &configpb.MetricThreshold{
					OneDay: proto.Int64(100),
				},
				DeactivationThreshold: &configpb.MetricThreshold{
					OneDay: proto.Int64(10),
				},
			},
		},
		Explanation: &configpb.BugManagementPolicy_Explanation{
			ProblemHtml: "problem",
			ActionHtml:  "action",
		},
		BugTemplate: &configpb.BugManagementPolicy_BugTemplate{
			CommentTemplate: `{{if .BugID.IsBuganizer }}Buganizer Bug ID: {{ .BugID.BuganizerBugID }}{{end}}` +
				`{{if .BugID.IsMonorail }}Monorail Project: {{ .BugID.MonorailProject }}; ID: {{ .BugID.MonorailBugID }}{{end}}` +
				`Rule URL: {{.RuleURL}}`,
			Monorail: &configpb.BugManagementPolicy_BugTemplate_Monorail{
				Labels: []string{"Test-Exonerated"},
			},
			Buganizer: &configpb.BugManagementPolicy_BugTemplate_Buganizer{
				Hotlists: []int64{1234},
			},
		},
	}
}

func createCLsRejectedPolicy() *configpb.BugManagementPolicy {
	return &configpb.BugManagementPolicy{
		Id:                "cls-rejected-policy",
		Owners:            []string{"username@google.com"},
		HumanReadableName: "many CL(s) are being falsely rejected in presubmit",
		Priority:          configpb.BuganizerPriority_P1,
		Metrics: []*configpb.BugManagementPolicy_Metric{
			{
				MetricId: metrics.HumanClsFailedPresubmit.ID.String(),
				ActivationThreshold: &configpb.MetricThreshold{
					SevenDay: proto.Int64(10),
				},
				DeactivationThreshold: &configpb.MetricThreshold{
					SevenDay: proto.Int64(1),
				},
			},
		},
		Explanation: &configpb.BugManagementPolicy_Explanation{
			ProblemHtml: "problem",
			ActionHtml:  "action",
		},
		BugTemplate: &configpb.BugManagementPolicy_BugTemplate{
			CommentTemplate: `Many CLs are failing presubmit. Policy text goes here.`,
			Monorail: &configpb.BugManagementPolicy_BugTemplate_Monorail{
				Labels: []string{"Test-Exonerated"},
			},
			Buganizer: &configpb.BugManagementPolicy_BugTemplate_Buganizer{
				Hotlists: []int64{1234},
			},
		},
	}
}

// verifyRulesResemble verifies rules stored in Spanner resemble
// the passed expectations, modulo assigned RuleIDs and
// audit timestamps.
func verifyRulesResemble(ctx context.Context, t testing.TB, expectedRules []*rules.Entry) error {
	t.Helper()
	// Read all rules. Sorted by BugSystem, BugId, Project.
	rs, err := rules.ReadAllForTesting(span.Single(ctx))
	if err != nil {
		return err
	}

	// Sort expectations in the same order as rules.
	sortedExpected := make([]*rules.Entry, len(expectedRules))
	copy(sortedExpected, expectedRules)
	sort.Slice(sortedExpected, func(i, j int) bool {
		ruleI := sortedExpected[i]
		ruleJ := sortedExpected[j]
		if ruleI.BugID.System != ruleJ.BugID.System {
			return ruleI.BugID.System < ruleJ.BugID.System
		}
		if ruleI.BugID.ID != ruleJ.BugID.ID {
			return ruleI.BugID.ID < ruleJ.BugID.ID
		}
		return ruleI.Project < ruleJ.Project
	})

	for _, r := range rs {
		// Accept whatever values the implementation has set
		// (these values are assigned non-deterministically).
		r.RuleID = ""
		r.CreateTime = time.Time{}
		r.LastAuditableUpdateTime = time.Time{}
		r.LastUpdateTime = time.Time{}
		r.PredicateLastUpdateTime = time.Time{}
		r.IsManagingBugPriorityLastUpdateTime = time.Time{}
	}
	for i, rule := range sortedExpected {
		expectationCopy := rule.Clone()
		// Clear the fields on the expectations as well.
		expectationCopy.RuleID = ""
		expectationCopy.CreateTime = time.Time{}
		expectationCopy.LastAuditableUpdateTime = time.Time{}
		expectationCopy.LastUpdateTime = time.Time{}
		expectationCopy.PredicateLastUpdateTime = time.Time{}
		expectationCopy.IsManagingBugPriorityLastUpdateTime = time.Time{}
		sortedExpected[i] = expectationCopy
	}

	assert.That(t, rs, should.Match(sortedExpected), truth.LineContext())
	return nil
}

type buganizerBug struct {
	// Bug ID.
	ID int64
	// Expected buganizer component ID.
	Component int64
	// Content that is expected to appear in the bug title.
	ExpectedTitle string
	// Content that is expected to appear in the bug description.
	ExpectedContent []string
	// The policies which were expected to have activated, in the
	// order they should have reported activation.
	ExpectedPolicyIDsActivated []string
}

func expectBuganizerBug(buganizerStore *buganizer.FakeIssueStore, bug buganizerBug) error {
	issue := buganizerStore.Issues[bug.ID].Issue
	if issue == nil {
		return errors.Reason("buganizer issue %v not found", bug.ID).Err()
	}
	if issue.IssueId != bug.ID {
		return errors.Reason("issue ID: got %v, want %v", issue.IssueId, bug.ID).Err()
	}
	if !strings.Contains(issue.IssueState.Title, bug.ExpectedTitle) {
		return errors.Reason("issue title: got %q, expected it to contain %q", issue.IssueState.Title, bug.ExpectedTitle).Err()
	}
	if issue.IssueState.ComponentId != bug.Component {
		return errors.Reason("component: got %v; want %v", issue.IssueState.ComponentId, bug.Component).Err()
	}

	for _, expectedContent := range bug.ExpectedContent {
		if !strings.Contains(issue.Description.Comment, expectedContent) {
			return errors.Reason("issue description: got %q, expected it to contain %q", issue.Description.Comment, expectedContent).Err()
		}
	}
	comments := buganizerStore.Issues[bug.ID].Comments
	if len(comments) != 1+len(bug.ExpectedPolicyIDsActivated) {
		return errors.Reason("issue comments: got %v want %v", len(comments), 1+len(bug.ExpectedPolicyIDsActivated)).Err()
	}
	for i, activatedPolicyID := range bug.ExpectedPolicyIDsActivated {
		expectedContent := fmt.Sprintf("(Policy ID: %s)", activatedPolicyID)
		if !strings.Contains(comments[1+i].Comment, expectedContent) {
			return errors.Reason("issue comment %v: got %q, expected it to contain %q", i+1, comments[i+1].Comment, expectedContent).Err()
		}
	}
	return nil
}
