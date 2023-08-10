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
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

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
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/third_party/google.golang.org/genproto/googleapis/devtools/issuetracker/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestBuganizerUpdate(t *testing.T) {
	Convey("Run bug updates", t, func() {
		ctx := testutil.IntegrationTestContext(t)
		ctx = memory.Use(ctx)
		ctx = context.WithValue(ctx, &buganizer.BuganizerSelfEmailKey, "email@test.com")

		const project = "chromeos"
		buganizerCfg := buganizer.ChromeOSTestConfig()
		thres := []*configpb.ImpactMetricThreshold{
			{
				MetricId: "failures",
				// Should be more onerous than the "keep-open" thresholds
				// configured for each individual bug manager.
				Threshold: &configpb.MetricThreshold{
					OneDay:   proto.Int64(100),
					ThreeDay: proto.Int64(300),
					SevenDay: proto.Int64(700),
				},
			},
		}
		projectCfg := &configpb.ProjectConfig{
			BugSystem:           configpb.ProjectConfig_BUGANIZER,
			Buganizer:           buganizerCfg,
			BugFilingThresholds: thres,
			LastUpdated:         timestamppb.New(clock.Now(ctx)),
		}
		projectsCfg := map[string]*configpb.ProjectConfig{
			project: projectCfg,
		}
		err := config.SetTestProjectConfig(ctx, projectsCfg)
		So(err, ShouldBeNil)

		compiledCfg, err := compiledcfg.NewConfig(projectCfg)
		So(err, ShouldBeNil)

		suggestedClusters := []*analysis.Cluster{
			makeReasonCluster(compiledCfg, 0),
			makeReasonCluster(compiledCfg, 1),
			makeReasonCluster(compiledCfg, 2),
			makeReasonCluster(compiledCfg, 3),
		}
		analysisClient := &fakeAnalysisClient{
			clusters: suggestedClusters,
		}

		buganizerClient := buganizer.NewFakeClient()
		fakeStore := buganizerClient.FakeStore

		// Unless otherwise specified, assume re-clustering has caught up to
		// the latest version of algorithms and config.
		err = runs.SetRunsForTesting(ctx, []*runs.ReclusteringRun{
			runs.NewRun(0).
				WithProject(project).
				WithAlgorithmsVersion(algorithms.AlgorithmsVersion).
				WithConfigVersion(projectCfg.LastUpdated.AsTime()).
				WithRulesVersion(rules.StartingEpoch).
				WithCompletedProgress().Build(),
		})
		So(err, ShouldBeNil)

		progress, err := runs.ReadReclusteringProgress(ctx, project)
		So(err, ShouldBeNil)

		opts := updateOptions{
			appID:                "luci-analysis-test",
			project:              project,
			analysisClient:       analysisClient,
			buganizerClient:      buganizerClient,
			maxBugsFiledPerRun:   1,
			reclusteringProgress: progress,
		}

		// Mock current time. This is needed to control behaviours like
		// automatic archiving of rules after 30 days of bug being marked
		// Closed (Verified).
		now := time.Date(2055, time.May, 5, 5, 5, 5, 5, time.UTC)
		ctx, tc := testclock.UseTime(ctx, now)

		Convey("Configuration used for testing is valid", func() {
			c := validation.Context{Context: context.Background()}

			config.ValidateProjectConfig(&c, project, projectCfg)
			So(c.Finalize(), ShouldBeNil)
		})
		Convey("With context paramteter set", func() {
			Convey("No value doesn't create any buganizer client", func() {
				client, err := createBuganizerClient(ctx)
				So(err, ShouldBeNil)
				So(client, ShouldBeNil)
			})

			Convey("`disable` mode doesn't create any client", func() {
				ctx = context.WithValue(ctx, &buganizer.BuganizerClientModeKey, buganizer.ModeDisable)
				client, err := createBuganizerClient(ctx)
				So(err, ShouldBeNil)
				So(client, ShouldBeNil)
			})
		})
		Convey("With no impactful clusters", func() {
			err = updateBugsForProject(ctx, opts)
			So(err, ShouldBeNil)

			// No failure association rules.
			rs, err := rules.ReadActive(span.Single(ctx), project)
			So(err, ShouldBeNil)
			So(rs, ShouldResembleProto, []*rules.Entry{})

			// No Buganizer issues.
			So(fakeStore.Issues, ShouldHaveLength, 0)
		})
		Convey("With a suggested cluster", func() {
			sourceClusterID := reasonClusterID(compiledCfg, "Failed to connect to 100.1.1.99.")
			suggestedClusters[1].ClusterID = sourceClusterID
			suggestedClusters[1].ExampleFailureReason = bigquery.NullString{StringVal: "Failed to connect to 100.1.1.105.", Valid: true}
			suggestedClusters[1].TopTestIDs = []analysis.TopCount{
				{Value: "network-test-1", Count: 10},
				{Value: "network-test-2", Count: 10},
			}
			// Meets failure dispersion thresholds.
			suggestedClusters[1].DistinctUserCLsWithFailures7d.Residual = 3

			ignoreRuleID := ""
			expectCreate := true

			expectedRule := &rules.Entry{
				Project:               "chromeos",
				RuleDefinition:        `reason LIKE "Failed to connect to %.%.%.%."`,
				BugID:                 bugs.BugID{System: bugs.BuganizerSystem, ID: "1"},
				IsActive:              true,
				IsManagingBug:         true,
				IsManagingBugPriority: true,
				SourceCluster:         sourceClusterID,
				CreationUser:          rules.LUCIAnalysisSystem,
				LastUpdatedUser:       rules.LUCIAnalysisSystem,
				BugManagementState:    &bugspb.BugManagementState{},
			}

			expectedBugSummary := "Failed to connect to 100.1.1.105."

			// Expect the bug description to contain the top tests.
			expectedBugContents := []string{
				"network-test-1",
				"network-test-2",
			}

			testWithIssueId := func(id int64) {
				err = updateBugsForProject(ctx, opts)
				So(err, ShouldBeNil)

				rs, err := rules.ReadActive(span.Single(ctx), project)
				So(err, ShouldBeNil)

				var cleanedRules []*rules.Entry
				for _, r := range rs {
					if r.RuleID != ignoreRuleID {
						cleanedRules = append(cleanedRules, r)
					}
				}

				if !expectCreate {
					So(len(cleanedRules), ShouldEqual, 0)
					return
				}

				So(len(cleanedRules), ShouldEqual, 1)
				rule := cleanedRules[0]

				// Accept whatever bug cluster ID has been generated.
				So(rule.RuleID, ShouldNotBeEmpty)
				expectedRule.RuleID = rule.RuleID

				// Accept creation and last updated times, as set by Spanner.
				So(rule.CreationTime, ShouldNotBeZeroValue)
				expectedRule.CreationTime = rule.CreationTime
				So(rule.LastUpdated, ShouldNotBeZeroValue)
				expectedRule.LastUpdated = rule.LastUpdated
				So(rule.PredicateLastUpdated, ShouldNotBeZeroValue)
				expectedRule.PredicateLastUpdated = rule.PredicateLastUpdated
				So(rule.IsManagingBugPriorityLastUpdated, ShouldNotBeZeroValue)
				expectedRule.IsManagingBugPriorityLastUpdated = rule.IsManagingBugPriorityLastUpdated
				So(rule, ShouldResembleProto, expectedRule)

				So(len(fakeStore.Issues), ShouldEqual, id)
				So(fakeStore.Issues[id].Issue.IssueId, ShouldEqual, id)
				So(fakeStore.Issues[id].Issue.Description.Comment, ShouldContainSubstring, expectedBugSummary)
				So(fakeStore.Issues[id].Issue.IssueState.ComponentId, ShouldEqual, projectCfg.Buganizer.DefaultComponent.Id)
				So(len(fakeStore.Issues[id].Comments), ShouldEqual, 1)
				for _, expectedContent := range expectedBugContents {
					So(fakeStore.Issues[id].Issue.Description.Comment, ShouldContainSubstring, expectedContent)
				}
				// Expect a link to the bug and the rule.
				So(fakeStore.Issues[id].Comments[0].Comment, ShouldContainSubstring, fmt.Sprintf("https://luci-analysis-test.appspot.com/b/%d", id))
			}

			test := func() {
				testWithIssueId(1)
			}

			Convey("Dispersion threshold", func() {
				// Meets impact threshold.
				suggestedClusters[1].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{
					OneDay: metrics.Counts{Residual: 100},
				}

				Convey("Met via User CLs with failures", func() {
					suggestedClusters[1].DistinctUserCLsWithFailures7d.Residual = 3
					suggestedClusters[1].PostsubmitBuildsWithFailures7d.Residual = 0
					test()
				})
				Convey("Met via Postsubmit builds with failures", func() {
					suggestedClusters[1].DistinctUserCLsWithFailures7d.Residual = 0
					suggestedClusters[1].PostsubmitBuildsWithFailures7d.Residual = 1
					test()
				})
				Convey("Not met", func() {
					suggestedClusters[1].DistinctUserCLsWithFailures7d.Residual = 0
					suggestedClusters[1].PostsubmitBuildsWithFailures7d.Residual = 0
					expectCreate = false
					test()
				})
			})
			Convey("1d unexpected failures", func() {
				Convey("Reason cluster", func() {
					Convey("Above threshold", func() {
						suggestedClusters[1].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{OneDay: metrics.Counts{Residual: 100}}

						test()

						// Further updates do nothing.
						test()
					})
					Convey("Below threshold", func() {
						suggestedClusters[1].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{OneDay: metrics.Counts{Residual: 99}}
						expectCreate = false
						test()
					})
				})
				Convey("Test name cluster", func() {
					suggestedClusters[1].ClusterID = testIDClusterID(compiledCfg, "ui-test-1")
					suggestedClusters[1].TopTestIDs = []analysis.TopCount{
						{Value: "ui-test-1", Count: 10},
					}
					expectedRule.RuleDefinition = `test = "ui-test-1"`
					expectedRule.SourceCluster = suggestedClusters[1].ClusterID
					expectedBugSummary = "ui-test-1"
					expectedBugContents = []string{"ui-test-1"}

					// 34% more impact is required for a test name cluster to
					// be filed, compared to a failure reason cluster.
					Convey("Above threshold", func() {
						suggestedClusters[1].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{OneDay: metrics.Counts{Residual: 134}}
						test()

						// Further updates do nothing.
						test()
					})
					Convey("Below threshold", func() {
						suggestedClusters[1].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{OneDay: metrics.Counts{Residual: 133}}
						expectCreate = false
						test()
					})
				})
			})
			Convey("3d unexpected failures", func() {
				suggestedClusters[1].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{ThreeDay: metrics.Counts{Residual: 300}}
				test()

				// Further updates do nothing.
				test()
			})
			Convey("7d unexpected failures", func() {
				suggestedClusters[1].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{ThreeDay: metrics.Counts{Residual: 700}}
				test()

				// Further updates do nothing.
				test()
			})
			Convey("With existing rule filed", func() {
				suggestedClusters[1].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{OneDay: metrics.Counts{Residual: 100}}

				createTime := time.Date(2021, time.January, 5, 12, 30, 0, 0, time.UTC)
				fakeStore.StoreIssue(ctx, buganizer.NewFakeIssue(1))
				rule := rules.NewRule(1).
					WithBugSystem(bugs.BuganizerSystem).
					WithProject(project).
					WithCreationTime(createTime).
					WithPredicateLastUpdated(createTime.Add(1 * time.Hour)).
					WithLastUpdated(createTime.Add(2 * time.Hour)).
					WithBugPriorityManaged(true).
					WithBugPriorityManagedLastUpdated(createTime.Add(1 * time.Hour)).
					WithSourceCluster(sourceClusterID).Build()
				err := rules.SetForTesting(ctx, []*rules.Entry{
					rule,
				})
				So(err, ShouldBeNil)
				ignoreRuleID = rule.RuleID

				// Initially do not expect a new bug to be filed.
				expectCreate = false
				test()

				// Once re-clustering has incorporated the version of rules
				// that included this new rule, it is OK to file another bug
				// for the suggested cluster if sufficient impact remains.
				// This should only happen when the rule definition has been
				// manually narrowed in some way from the originally filed bug.
				err = runs.SetRunsForTesting(ctx, []*runs.ReclusteringRun{
					runs.NewRun(0).
						WithProject(project).
						WithAlgorithmsVersion(algorithms.AlgorithmsVersion).
						WithConfigVersion(projectCfg.LastUpdated.AsTime()).
						WithRulesVersion(createTime).
						WithCompletedProgress().Build(),
				})
				So(err, ShouldBeNil)
				progress, err := runs.ReadReclusteringProgress(ctx, project)
				So(err, ShouldBeNil)
				opts.reclusteringProgress = progress

				expectCreate = true
				expectedRule.BugID.ID = "2"
				testWithIssueId(2)
			})
			Convey("Without re-clustering caught up to latest algorithms", func() {
				suggestedClusters[1].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{OneDay: metrics.Counts{Residual: 100}}

				err = runs.SetRunsForTesting(ctx, []*runs.ReclusteringRun{
					runs.NewRun(0).
						WithProject(project).
						WithAlgorithmsVersion(algorithms.AlgorithmsVersion - 1).
						WithConfigVersion(projectCfg.LastUpdated.AsTime()).
						WithRulesVersion(rules.StartingEpoch).
						WithCompletedProgress().Build(),
				})
				So(err, ShouldBeNil)
				progress, err := runs.ReadReclusteringProgress(ctx, project)
				So(err, ShouldBeNil)
				opts.reclusteringProgress = progress

				expectCreate = false
				test()
			})
			Convey("Without re-clustering caught up to latest config", func() {
				suggestedClusters[1].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{OneDay: metrics.Counts{Residual: 100}}

				err = runs.SetRunsForTesting(ctx, []*runs.ReclusteringRun{
					runs.NewRun(0).
						WithProject(project).
						WithAlgorithmsVersion(algorithms.AlgorithmsVersion).
						WithConfigVersion(projectCfg.LastUpdated.AsTime().Add(-1 * time.Hour)).
						WithRulesVersion(rules.StartingEpoch).
						WithCompletedProgress().Build(),
				})
				So(err, ShouldBeNil)
				progress, err := runs.ReadReclusteringProgress(ctx, project)
				So(err, ShouldBeNil)
				opts.reclusteringProgress = progress

				expectCreate = false
				test()
			})
		})
		Convey("With both failure reason and test name clusters above bug-filing threshold", func() {
			// Reason cluster above the 3-day failure threshold.
			suggestedClusters[2] = makeReasonCluster(compiledCfg, 2)
			suggestedClusters[2].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{
				ThreeDay: metrics.Counts{Residual: 400},
				SevenDay: metrics.Counts{Residual: 400},
			}
			suggestedClusters[2].PostsubmitBuildsWithFailures7d.Residual = 1

			// Test name cluster with 33% more impact.
			suggestedClusters[1] = makeTestNameCluster(compiledCfg, 3)
			suggestedClusters[1].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{
				ThreeDay: metrics.Counts{Residual: 532},
				SevenDay: metrics.Counts{Residual: 532},
			}
			suggestedClusters[1].PostsubmitBuildsWithFailures7d.Residual = 1

			// Limit to one bug filed each time, so that
			// we test change throttling.
			opts.maxBugsFiledPerRun = 1

			Convey("Reason clusters preferred over test name clusters", func() {
				// Test name cluster has <34% more impact than the reason
				// cluster.
				err = updateBugsForProject(ctx, opts)
				So(err, ShouldBeNil)

				// Reason cluster filed.
				bugClusters, err := rules.ReadActive(span.Single(ctx), project)
				So(err, ShouldBeNil)
				So(len(bugClusters), ShouldEqual, 1)
				So(bugClusters[0].SourceCluster, ShouldResemble, suggestedClusters[2].ClusterID)
				So(bugClusters[0].SourceCluster.IsFailureReasonCluster(), ShouldBeTrue)
			})
			Convey("Test name clusters can be filed if significantly more impact", func() {
				// Reduce impact of the reason-based cluster so that the
				// test name cluster has >34% more impact than the reason
				// cluster.
				suggestedClusters[2].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{
					ThreeDay: metrics.Counts{Residual: 390},
					SevenDay: metrics.Counts{Residual: 390},
				}

				err = updateBugsForProject(ctx, opts)
				So(err, ShouldBeNil)

				// Test name cluster filed first.
				bugClusters, err := rules.ReadActive(span.Single(ctx), project)
				So(err, ShouldBeNil)
				So(len(bugClusters), ShouldEqual, 1)
				So(bugClusters[0].SourceCluster, ShouldResemble, suggestedClusters[1].ClusterID)
				So(bugClusters[0].SourceCluster.IsTestNameCluster(), ShouldBeTrue)
			})
		})
		Convey("With multiple suggested clusters above impact threshold", func() {
			expectBugClusters := func(count int) {
				bugClusters, err := rules.ReadActive(span.Single(ctx), project)
				So(err, ShouldBeNil)
				So(len(bugClusters), ShouldEqual, count)
				So(len(fakeStore.Issues), ShouldEqual, count)
			}
			// Use a mix of test name and failure reason clusters for
			// code path coverage.
			suggestedClusters[0] = makeTestNameCluster(compiledCfg, 0)
			suggestedClusters[0].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{
				SevenDay: metrics.Counts{Residual: 940},
			}

			suggestedClusters[0].PostsubmitBuildsWithFailures7d.Residual = 1
			suggestedClusters[1] = makeReasonCluster(compiledCfg, 1)
			suggestedClusters[1].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{
				ThreeDay: metrics.Counts{Residual: 300},
				SevenDay: metrics.Counts{Residual: 300},
			}
			suggestedClusters[1].PostsubmitBuildsWithFailures7d.Residual = 1
			suggestedClusters[2] = makeReasonCluster(compiledCfg, 2)
			suggestedClusters[2].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{
				OneDay:   metrics.Counts{Residual: 200},
				ThreeDay: metrics.Counts{Residual: 200},
				SevenDay: metrics.Counts{Residual: 200},
			}
			suggestedClusters[2].PostsubmitBuildsWithFailures7d.Residual = 1

			// Limit to one bug filed each time, so that
			// we test change throttling.
			opts.maxBugsFiledPerRun = 1

			err = updateBugsForProject(ctx, opts)
			So(err, ShouldBeNil)
			expectBugClusters(1)

			err = updateBugsForProject(ctx, opts)
			So(err, ShouldBeNil)
			expectBugClusters(2)

			err = updateBugsForProject(ctx, opts)
			So(err, ShouldBeNil)

			expectedRules := []*rules.Entry{
				{
					Project:               "chromeos",
					RuleDefinition:        `test = "testname-0"`,
					BugID:                 bugs.BugID{System: bugs.BuganizerSystem, ID: "1"},
					SourceCluster:         suggestedClusters[0].ClusterID,
					IsActive:              true,
					IsManagingBug:         true,
					IsManagingBugPriority: true,
					CreationUser:          rules.LUCIAnalysisSystem,
					LastUpdatedUser:       rules.LUCIAnalysisSystem,
					BugManagementState:    &bugspb.BugManagementState{},
				},
				{
					Project:               "chromeos",
					RuleDefinition:        `reason LIKE "want foo, got bar"`,
					BugID:                 bugs.BugID{System: bugs.BuganizerSystem, ID: "2"},
					SourceCluster:         suggestedClusters[1].ClusterID,
					IsActive:              true,
					IsManagingBug:         true,
					IsManagingBugPriority: true,
					CreationUser:          rules.LUCIAnalysisSystem,
					LastUpdatedUser:       rules.LUCIAnalysisSystem,
					BugManagementState:    &bugspb.BugManagementState{},
				},
				{
					Project:               "chromeos",
					RuleDefinition:        `reason LIKE "want foofoo, got bar"`,
					BugID:                 bugs.BugID{System: bugs.BuganizerSystem, ID: "3"},
					SourceCluster:         suggestedClusters[2].ClusterID,
					IsActive:              true,
					IsManagingBug:         true,
					IsManagingBugPriority: true,
					CreationUser:          rules.LUCIAnalysisSystem,
					LastUpdatedUser:       rules.LUCIAnalysisSystem,
					BugManagementState:    &bugspb.BugManagementState{},
				},
			}

			expectRulesWithExtraIssues := func(expectedRules []*rules.Entry, numExtraIssues int) {
				// Check final set of rules is as expected.
				rs, err := rules.ReadAll(span.Single(ctx), "chromeos")
				So(err, ShouldBeNil)
				for _, r := range rs {
					So(r.RuleID, ShouldNotBeEmpty)
					So(r.CreationTime, ShouldNotBeZeroValue)
					So(r.LastUpdated, ShouldNotBeZeroValue)
					So(r.PredicateLastUpdated, ShouldNotBeZeroValue)
					So(r.IsManagingBugPriorityLastUpdated, ShouldNotBeZeroValue)
					// Accept whatever values the implementation has set.
					r.RuleID = ""
					r.CreationTime = time.Time{}
					r.LastUpdated = time.Time{}
					r.PredicateLastUpdated = time.Time{}
					r.IsManagingBugPriorityLastUpdated = time.Time{}
				}

				sortedExpected := make([]*rules.Entry, len(expectedRules))
				copy(sortedExpected, expectedRules)
				sort.Slice(sortedExpected, func(i, j int) bool {
					return sortedExpected[i].BugID.System < sortedExpected[j].BugID.System ||
						(sortedExpected[i].BugID.System == sortedExpected[j].BugID.System &&
							sortedExpected[i].BugID.ID < sortedExpected[j].BugID.ID)
				})

				So(rs, ShouldResembleProto, sortedExpected)
				So(len(fakeStore.Issues), ShouldEqual, len(sortedExpected)+numExtraIssues)
			}

			expectRules := func(expectedRules []*rules.Entry) {
				expectRulesWithExtraIssues(expectedRules, 0)
			}

			expectRules(expectedRules)

			// Further updates do nothing.
			err = updateBugsForProject(ctx, opts)
			So(err, ShouldBeNil)
			expectRules(expectedRules)

			rs, err := rules.ReadActive(span.Single(ctx), project)
			So(err, ShouldBeNil)

			bugClusters := []*analysis.Cluster{
				makeBugCluster(rs[0].RuleID),
				makeBugCluster(rs[1].RuleID),
				makeBugCluster(rs[2].RuleID),
			}

			Convey("Re-clustering in progress", func() {
				analysisClient.clusters = append(suggestedClusters, bugClusters[1:]...)

				Convey("Negligable cluster impact does not affect issue priority or status", func() {
					issue := fakeStore.Issues[1].Issue
					So(issue.IssueId, ShouldEqual, 1)
					originalPriority := issue.IssueState.Priority
					originalStatus := issue.IssueState.Status
					So(originalStatus, ShouldNotEqual, issuetracker.Issue_VERIFIED)

					SetResidualImpact(
						bugClusters[1], bugs.ClosureImpact())
					err = updateBugsForProject(ctx, opts)
					So(err, ShouldBeNil)

					So(len(fakeStore.Issues), ShouldEqual, 3)
					issue = fakeStore.Issues[1].Issue
					So(issue.IssueId, ShouldEqual, 1)
					So(issue.IssueState.Priority, ShouldEqual, originalPriority)
					So(issue.IssueState.Status, ShouldEqual, originalStatus)

					expectRules(expectedRules)
				})
			})
			Convey("Re-clustering complete", func() {
				analysisClient.clusters = append(suggestedClusters, bugClusters[1:]...)

				// Copy impact from suggested clusters to new bug clusters.
				bugClusters[0].MetricValues = suggestedClusters[0].MetricValues
				bugClusters[1].MetricValues = suggestedClusters[1].MetricValues
				bugClusters[2].MetricValues = suggestedClusters[2].MetricValues

				// Clear residual impact on suggested clusters
				suggestedClusters[0].MetricValues = emptyMetricValues()
				suggestedClusters[1].MetricValues = emptyMetricValues()
				suggestedClusters[2].MetricValues = emptyMetricValues()

				// Mark reclustering complete.
				err := runs.SetRunsForTesting(ctx, []*runs.ReclusteringRun{
					runs.NewRun(0).
						WithProject(project).
						WithAlgorithmsVersion(algorithms.AlgorithmsVersion).
						WithConfigVersion(projectCfg.LastUpdated.AsTime()).
						WithRulesVersion(rs[2].PredicateLastUpdated).
						WithCompletedProgress().Build(),
				})
				So(err, ShouldBeNil)
				progress, err := runs.ReadReclusteringProgress(ctx, project)
				So(err, ShouldBeNil)
				opts.reclusteringProgress = progress

				Convey("no bug filing thresholds, but still update existing bug priority", func() {
					projectCfg.BugFilingThresholds = nil
					err = config.SetTestProjectConfig(ctx, projectsCfg)
					So(err, ShouldBeNil)
					issue := fakeStore.Issues[3].Issue
					So(issue.IssueState.Priority, ShouldNotEqual, issuetracker.Issue_P0)

					SetResidualImpact(
						bugClusters[2], bugs.P0Impact())
					err = updateBugsForProject(ctx, opts)
					So(err, ShouldBeNil)

					So(len(fakeStore.Issues), ShouldEqual, 3)
					issue = fakeStore.Issues[3].Issue
					So(issue.IssueId, ShouldEqual, 3)
					So(issue.IssueState.Priority, ShouldEqual, issuetracker.Issue_P0)

					expectRules(expectedRules)
				})

				Convey("Cluster impact does not change if bug not managed by rule", func() {
					// Set IsManagingBug to false on one rule.
					rs[2].IsManagingBug = false
					So(rules.SetForTesting(ctx, rs), ShouldBeNil)

					issue := fakeStore.Issues[3].Issue
					So(issue.IssueId, ShouldEqual, 3)
					originalPriority := issue.IssueState.Priority
					originalStatus := issue.IssueState.Status
					So(originalPriority, ShouldNotEqual, issuetracker.Issue_P0)

					// Set P0 impact on the cluster.
					SetResidualImpact(
						bugClusters[2], bugs.P0Impact())
					err = updateBugsForProject(ctx, opts)
					So(err, ShouldBeNil)

					// Check that the rule priority and status has not changed.
					So(len(fakeStore.Issues), ShouldEqual, 3)
					issue = fakeStore.Issues[3].Issue
					So(issue.IssueState.Status, ShouldEqual, originalStatus)
					So(issue.IssueState.Priority, ShouldEqual, originalPriority)
				})
				Convey("Increasing cluster impact increases issue priority", func() {
					issue := fakeStore.Issues[3].Issue
					So(issue.IssueState.Priority, ShouldNotEqual, issuetracker.Issue_P0)

					SetResidualImpact(
						bugClusters[2], bugs.P0Impact())
					err = updateBugsForProject(ctx, opts)
					So(err, ShouldBeNil)

					So(len(fakeStore.Issues), ShouldEqual, 3)
					issue = fakeStore.Issues[3].Issue
					So(issue.IssueId, ShouldEqual, 3)
					So(issue.IssueState.Priority, ShouldEqual, issuetracker.Issue_P0)

					expectRules(expectedRules)
				})
				Convey("Decreasing cluster impact decreases issue priority", func() {
					issue := fakeStore.Issues[3].Issue
					So(issue.IssueState.Priority, ShouldNotEqual, issuetracker.Issue_P3)

					SetResidualImpact(
						bugClusters[2], bugs.P3Impact())
					err = updateBugsForProject(ctx, opts)
					So(err, ShouldBeNil)

					So(len(fakeStore.Issues), ShouldEqual, 3)
					issue = fakeStore.Issues[3].Issue
					So(issue.IssueState.Status, ShouldEqual, issuetracker.Issue_NEW)
					So(issue.IssueState.Priority, ShouldEqual, issuetracker.Issue_P3)

					expectRules(expectedRules)
				})
				Convey("Manually setting a priority prevents bug updates.", func() {
					// Setup
					fakeStore.Issues[3].IssueUpdates = append(fakeStore.Issues[3].IssueUpdates, &issuetracker.IssueUpdate{
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
					So(fakeStore.Issues[3].Issue.IssueState.Priority, ShouldNotEqual, issuetracker.Issue_P0)
					originalPriority := fakeStore.Issues[3].Issue.IssueState.Priority

					SetResidualImpact(bugClusters[2], bugs.P0Impact())
					err = updateBugsForProject(ctx, opts)
					So(err, ShouldBeNil)

					So(len(fakeStore.Issues), ShouldEqual, 3)
					issue := fakeStore.Issues[3].Issue
					So(issue.IssueId, ShouldEqual, 3)
					So(issue.IssueState.Priority, ShouldEqual, originalPriority)
					expectedRules[2].IsManagingBugPriority = false
					expectRules(expectedRules)

					Convey("Further updates leave no comments", func() {
						initialComments := len(fakeStore.Issues[3].Comments)
						err = updateBugsForProject(ctx, opts)
						So(err, ShouldBeNil)

						So(len(fakeStore.Issues), ShouldEqual, 3)
						So(len(fakeStore.Issues[3].Comments), ShouldEqual, initialComments)
						issue := fakeStore.Issues[3].Issue
						So(issue.IssueId, ShouldEqual, 3)
						So(issue.IssueState.Priority, ShouldEqual, originalPriority)
						expectRules(expectedRules)
					})
				})
				Convey("Disabling IsManagingBugPriority prevents priority updates.", func() {

					rs[2].IsManagingBugPriority = false
					So(rules.SetForTesting(ctx, rs), ShouldBeNil)

					originalPriority := fakeStore.Issues[3].Issue.IssueState.Priority
					So(fakeStore.Issues[3].Issue.IssueState.Priority, ShouldNotEqual, issuetracker.Issue_P0)

					SetResidualImpact(
						bugClusters[2], bugs.P0Impact())
					err = updateBugsForProject(ctx, opts)
					So(err, ShouldBeNil)

					So(len(fakeStore.Issues), ShouldEqual, 3)
					issue := fakeStore.Issues[3].Issue
					So(issue.IssueId, ShouldEqual, 3)
					So(issue.IssueState.Priority, ShouldEqual, originalPriority)
				})
				Convey("Deleting cluster closes issue", func() {
					issue := fakeStore.Issues[3].Issue
					So(issue.IssueId, ShouldEqual, 3)

					So(issue.IssueState.Status, ShouldEqual, issuetracker.Issue_NEW)

					// Drop the bug cluster at index 2.
					bugClusters = bugClusters[:2]
					analysisClient.clusters = append(suggestedClusters, bugClusters...)
					err = updateBugsForProject(ctx, opts)
					So(err, ShouldBeNil)

					So(len(fakeStore.Issues), ShouldEqual, 3)
					issue = fakeStore.Issues[3].Issue
					So(issue.IssueId, ShouldEqual, 3)
					So(issue.IssueState.Status, ShouldEqual, issuetracker.Issue_VERIFIED)

					expectRules(expectedRules)

					Convey("Rule automatically archived after 30 days", func() {
						tc.Add(time.Hour * 24 * 30)

						// Act
						err = updateBugsForProject(ctx, opts)
						So(err, ShouldBeNil)

						// Verify
						expectedRules[2].IsActive = false
						expectRules(expectedRules)

						So(len(fakeStore.Issues), ShouldEqual, 3)
						issue = fakeStore.Issues[3].Issue
						So(issue.IssueId, ShouldEqual, 3)
						So(issue.IssueState.Status, ShouldEqual, issuetracker.Issue_VERIFIED)
					})
				})
				Convey("Bug marked as duplicate of bug with rule", func() {
					// Setup
					issueOne := fakeStore.Issues[2].Issue
					So(issueOne.IssueId, ShouldEqual, 2)

					issueTwo := fakeStore.Issues[3].Issue
					So(issueTwo.IssueId, ShouldEqual, 3)

					issueOne.IssueState.Status = issuetracker.Issue_DUPLICATE
					issueOne.IssueState.CanonicalIssueId = issueTwo.IssueId

					Convey("Happy path", func() {
						// Act
						err = updateBugsForProject(ctx, opts)
						So(err, ShouldBeNil)

						// Verify
						expectedRules[1].IsActive = false
						expectedRules[2].RuleDefinition = "reason LIKE \"want foo, got bar\" OR\nreason LIKE \"want foofoo, got bar\""
						expectRules(expectedRules)

						So(fakeStore.Issues[2].Comments, ShouldHaveLength, 2)
						So(fakeStore.Issues[2].Comments[1].Comment, ShouldContainSubstring, "LUCI Analysis has merged the failure association rule for this bug into the rule for the canonical bug.")
						So(fakeStore.Issues[2].Comments[1].Comment, ShouldContainSubstring, expectedRules[2].RuleID)

						So(fakeStore.Issues[3].Comments, ShouldHaveLength, 2)
						So(fakeStore.Issues[3].Comments[1].Comment, ShouldContainSubstring, "LUCI Analysis has merged the failure association rule for that bug into the rule for this bug.")
					})
					Convey("Happy path, with comments for duplicate bugs disabled", func() {
						// Setup
						projectCfg.BugManagement = &configpb.BugManagement{
							DisableDuplicateBugComments: true,
						}
						projectsCfg := map[string]*configpb.ProjectConfig{
							project: projectCfg,
						}
						err = config.SetTestProjectConfig(ctx, projectsCfg)
						So(err, ShouldBeNil)

						// Act
						err = updateBugsForProject(ctx, opts)
						So(err, ShouldBeNil)

						// Verify
						expectedRules[1].IsActive = false
						expectedRules[2].RuleDefinition = "reason LIKE \"want foo, got bar\" OR\nreason LIKE \"want foofoo, got bar\""
						expectRules(expectedRules)

						So(fakeStore.Issues[1].Comments, ShouldHaveLength, 2)
						So(fakeStore.Issues[2].Comments, ShouldHaveLength, 1)
					})
					Convey("Bugs are in a duplicate bug cycle", func() {
						// Note that this is a simple cycle with only two bugs.
						// The implementation allows for larger cycles, however.
						issueTwo.IssueState.Status = issuetracker.Issue_DUPLICATE
						issueTwo.IssueState.CanonicalIssueId = issueOne.IssueId

						// Act
						err = updateBugsForProject(ctx, opts)
						So(err, ShouldBeNil)

						// Verify
						// Issue one kicked out of duplicate status.
						So(issueOne.IssueState.Status, ShouldNotEqual, issuetracker.Issue_DUPLICATE)

						// As the cycle is now broken, issue two is merged into
						// issue one.
						expectedRules[1].RuleDefinition = "reason LIKE \"want foo, got bar\" OR\nreason LIKE \"want foofoo, got bar\""
						expectedRules[2].IsActive = false
						expectRules(expectedRules)

						So(fakeStore.Issues[2].Comments, ShouldHaveLength, 3)
						So(fakeStore.Issues[2].Comments[1].Comment, ShouldContainSubstring, "a cycle was detected in the bug merged-into graph")
						So(fakeStore.Issues[2].Comments[2].Comment, ShouldContainSubstring, "LUCI Analysis has merged the failure association rule for that bug into the rule for this bug.")
					})
					Convey("Merged rule would be too long", func() {
						// Setup
						// Make one of the rules we will be merging very close
						// to the rule length limit.
						longRule := fmt.Sprintf("test = \"%s\"", strings.Repeat("a", rules.MaxRuleDefinitionLength-10))

						_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
							issueOneRule, err := rules.ReadByBug(ctx, bugs.BugID{System: bugs.BuganizerSystem, ID: "2"})
							if err != nil {
								return err
							}
							issueOneRule[0].RuleDefinition = longRule

							return rules.Update(ctx, issueOneRule[0], rules.UpdateOptions{
								PredicateUpdated: true,
							}, rules.LUCIAnalysisSystem)
						})
						So(err, ShouldBeNil)

						// Act
						err = updateBugsForProject(ctx, opts)
						So(err, ShouldBeNil)

						// Verify
						// Rules should not have changed (except for the update we made).
						expectedRules[1].RuleDefinition = longRule
						expectRules(expectedRules)

						// Issue one kicked out of duplicate status.
						So(issueOne.IssueState.Status, ShouldNotEqual, issuetracker.Issue_DUPLICATE)

						// Comment should appear on the bug.
						So(fakeStore.Issues[2].Comments, ShouldHaveLength, 2)
						So(fakeStore.Issues[2].Comments[1].Comment, ShouldContainSubstring, "the merged failure association rule would be too long")
					})
					Convey("Bug marked as duplicate of bug we cannot access", func() {
						fakeStore.Issues[3].ShouldReturnAccessPermissionError = true
						// Act
						err = updateBugsForProject(ctx, opts)
						So(err, ShouldBeNil)

						// Verify
						// Issue one kicked out of duplicate status.
						So(issueOne.IssueState.Status, ShouldNotEqual, issuetracker.Issue_DUPLICATE)

						So(fakeStore.Issues[2].Comments, ShouldHaveLength, 2)
						So(fakeStore.Issues[2].Comments[1].Comment, ShouldContainSubstring, "LUCI Analysis cannot merge the association rule for this bug into the rule")
					})
					Convey("Bug marked as dupliacte of bug with an assignee", func() {
						fakeStore.Issues[3].ShouldReturnAccessPermissionError = true
						issueOne.IssueState.Assignee = &issuetracker.User{
							EmailAddress: "user@google.com",
						}
						// Act
						err = updateBugsForProject(ctx, opts)
						So(err, ShouldBeNil)

						// Verify
						// Issue one kicked out of duplicate status.
						So(issueOne.IssueState.Status, ShouldEqual, issuetracker.Issue_ASSIGNED)

						So(fakeStore.Issues[2].Comments, ShouldHaveLength, 2)
						So(fakeStore.Issues[2].Comments[1].Comment, ShouldContainSubstring, "LUCI Analysis cannot merge the association rule for this bug into the rule")
					})
				})

				Convey("Bug marked as duplicate of bug without a rule in this project", func() {
					// Setup
					issueOne := fakeStore.Issues[2].Issue

					issueOne.IssueState.Status = issuetracker.Issue_DUPLICATE
					issueOne.IssueState.CanonicalIssueId = 1234

					Convey("Bug managed by a rule in another project", func() {
						fakeStore.StoreIssue(ctx, buganizer.NewFakeIssue(1234))

						extraRule := &rules.Entry{
							Project:               "otherproject",
							RuleDefinition:        `reason LIKE "blah"`,
							RuleID:                "1234567890abcdef1234567890abcdef",
							BugID:                 bugs.BugID{System: bugs.BuganizerSystem, ID: "1234"},
							IsActive:              true,
							IsManagingBug:         true,
							IsManagingBugPriority: true,
							BugManagementState:    &bugspb.BugManagementState{},
						}
						_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
							return rules.Create(ctx, extraRule, "user@chromium.org")
						})
						So(err, ShouldBeNil)

						// Act
						err = updateBugsForProject(ctx, opts)
						So(err, ShouldBeNil)

						// Verify
						expectedRules[1].BugID = bugs.BugID{System: bugs.BuganizerSystem, ID: "1234"}
						expectedRules[1].IsManagingBug = false // Let the other rule continue to manage the bug.
						expectRulesWithExtraIssues(expectedRules, 1)

						So(fakeStore.Issues[2].Comments, ShouldHaveLength, 2)
						So(fakeStore.Issues[2].Comments[1].Comment, ShouldContainSubstring, "LUCI Analysis has merged the failure association rule for this bug into the rule for the canonical bug.")
						So(fakeStore.Issues[2].Comments[1].Comment, ShouldContainSubstring, expectedRules[1].RuleID)
					})
					Convey("Bug not managed by a rule in another project", func() {
						fakeStore.StoreIssue(ctx, buganizer.NewFakeIssue(1234))

						// Act
						err = updateBugsForProject(ctx, opts)
						So(err, ShouldBeNil)

						// Verify
						expectedRules[1].BugID = bugs.BugID{System: bugs.BuganizerSystem, ID: "1234"}
						expectedRules[1].IsManagingBug = true
						expectRulesWithExtraIssues(expectedRules, 1)

						So(fakeStore.Issues[2].Comments, ShouldHaveLength, 2)
						So(fakeStore.Issues[2].Comments[1].Comment, ShouldContainSubstring, "LUCI Analysis has merged the failure association rule for this bug into the rule for the canonical bug.")
						So(fakeStore.Issues[2].Comments[1].Comment, ShouldContainSubstring, expectedRules[1].RuleID)
					})
				})
				Convey("Bug marked as archived should archive rule", func() {
					issueOne := fakeStore.Issues[2].Issue
					issueOne.IsArchived = true

					// Act
					err = updateBugsForProject(ctx, opts)
					So(err, ShouldBeNil)

					// Assert
					expectedRules[1].IsActive = false
					expectRules(expectedRules)
				})
			})
		})
	})
}
