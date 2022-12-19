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
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/analysis/internal/analysis"
	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	"go.chromium.org/luci/analysis/internal/bugs"
	"go.chromium.org/luci/analysis/internal/bugs/monorail"
	"go.chromium.org/luci/analysis/internal/bugs/monorail/api_proto"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/failurereason"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/rulesalgorithm"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/testname"
	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/clustering/runs"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/config/compiledcfg"
	"go.chromium.org/luci/analysis/internal/testutil"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/span"
)

func TestRun(t *testing.T) {
	Convey("Run bug updates", t, func() {
		ctx := testutil.IntegrationTestContext(t)
		ctx = memory.Use(ctx)

		f := &monorail.FakeIssuesStore{
			NextID:            100,
			PriorityFieldName: "projects/chromium/fieldDefs/11",
			ComponentNames: []string{
				"projects/chromium/componentDefs/Blink",
				"projects/chromium/componentDefs/Blink>Layout",
				"projects/chromium/componentDefs/Blink>Network",
			},
		}
		user := monorail.AutomationUsers[0]
		mc, err := monorail.NewClient(monorail.UseFakeIssuesClient(ctx, f, user), "myhost")
		So(err, ShouldBeNil)

		const project = "chromium"
		monorailCfg := monorail.ChromiumTestConfig()
		thres := &configpb.ImpactThreshold{
			// Should be more onerous than the "keep-open" thresholds
			// configured for each individual bug manager.
			TestResultsFailed: &configpb.MetricThreshold{
				OneDay:   proto.Int64(100),
				ThreeDay: proto.Int64(300),
				SevenDay: proto.Int64(700),
			},
		}
		projectCfg := &configpb.ProjectConfig{
			Monorail:           monorailCfg,
			BugFilingThreshold: thres,
			LastUpdated:        timestamppb.New(time.Date(2030, time.July, 1, 0, 0, 0, 0, time.UTC)),
		}
		projectsCfg := map[string]*configpb.ProjectConfig{
			project: projectCfg,
		}
		err = config.SetTestProjectConfig(ctx, projectsCfg)
		So(err, ShouldBeNil)

		compiledCfg, err := compiledcfg.NewConfig(projectCfg)
		So(err, ShouldBeNil)

		suggestedClusters := []*analysis.Cluster{
			makeReasonCluster(compiledCfg, 0),
			makeReasonCluster(compiledCfg, 1),
			makeReasonCluster(compiledCfg, 2),
			makeReasonCluster(compiledCfg, 3),
		}
		ac := &fakeAnalysisClient{
			clusters: suggestedClusters,
		}

		opts := updateOptions{
			appID:              "luci-analysis-test",
			project:            project,
			analysisClient:     ac,
			monorailClient:     mc,
			enableBugUpdates:   true,
			maxBugsFiledPerRun: 1,
		}

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

		// Mock current time. This is needed to control behaviours like
		// automatic archiving of rules after 30 days of bug being marked
		// Closed (Verified).
		now := time.Date(2055, time.May, 5, 5, 5, 5, 5, time.UTC)
		ctx, tc := testclock.UseTime(ctx, now)

		Convey("Configuration used for testing is valid", func() {
			c := validation.Context{Context: context.Background()}

			config.ValidateProjectConfig(&c, projectCfg)
			So(c.Finalize(), ShouldBeNil)
		})
		Convey("With no impactful clusters", func() {
			err = updateAnalysisAndBugsForProject(ctx, opts)
			So(err, ShouldBeNil)

			// No failure association rules.
			rs, err := rules.ReadActive(span.Single(ctx), project)
			So(err, ShouldBeNil)
			So(rs, ShouldResemble, []*rules.FailureAssociationRule{})

			// No monorail issues.
			So(f.Issues, ShouldBeNil)
		})
		Convey("With buganizer bugs", func() {
			rs := []*rules.FailureAssociationRule{
				rules.NewRule(1).WithProject(project).WithBug(bugs.BugID{
					System: "buganizer", ID: "12345678",
				}).Build(),
			}
			rules.SetRulesForTesting(ctx, rs)

			// Bug filing should not encounter errors.
			err = updateAnalysisAndBugsForProject(ctx, opts)
			So(err, ShouldBeNil)
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

			suggestedClusters[1].TopMonorailComponents = []analysis.TopCount{
				{Value: "Blink>Layout", Count: 40},  // >30% of failures.
				{Value: "Blink>Network", Count: 31}, // >30% of failures.
				{Value: "Blink>Other", Count: 4},
			}

			ignoreRuleID := ""
			expectCreate := true

			expectedRule := &rules.FailureAssociationRule{
				Project:         "chromium",
				RuleDefinition:  `reason LIKE "Failed to connect to %.%.%.%."`,
				BugID:           bugs.BugID{System: "monorail", ID: "chromium/100"},
				IsActive:        true,
				IsManagingBug:   true,
				SourceCluster:   sourceClusterID,
				CreationUser:    rules.LUCIAnalysisSystem,
				LastUpdatedUser: rules.LUCIAnalysisSystem,
			}

			expectedBugSummary := "Failed to connect to 100.1.1.105."

			// Expect the bug description to contain the top tests.
			expectedBugContents := []string{
				"network-test-1",
				"network-test-2",
			}

			test := func() {
				err = updateAnalysisAndBugsForProject(ctx, opts)
				So(err, ShouldBeNil)

				rs, err := rules.ReadActive(span.Single(ctx), project)
				So(err, ShouldBeNil)

				var cleanedRules []*rules.FailureAssociationRule
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
				So(rule, ShouldResemble, expectedRule)

				So(len(f.Issues), ShouldEqual, 1)
				So(f.Issues[0].Issue.Name, ShouldEqual, "projects/chromium/issues/100")
				So(f.Issues[0].Issue.Summary, ShouldContainSubstring, expectedBugSummary)
				So(f.Issues[0].Issue.Components, ShouldHaveLength, 2)
				So(f.Issues[0].Issue.Components[0].Component, ShouldEqual, "projects/chromium/componentDefs/Blink>Layout")
				So(f.Issues[0].Issue.Components[1].Component, ShouldEqual, "projects/chromium/componentDefs/Blink>Network")
				So(len(f.Issues[0].Comments), ShouldEqual, 2)
				for _, expectedContent := range expectedBugContents {
					So(f.Issues[0].Comments[0].Content, ShouldContainSubstring, expectedContent)
				}
				// Expect a link to the bug and the rule.
				So(f.Issues[0].Comments[1].Content, ShouldContainSubstring, "https://luci-analysis-test.appspot.com/b/chromium/100")
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
				rule := rules.NewRule(1).
					WithProject(project).
					WithCreationTime(createTime).
					WithPredicateLastUpdated(createTime.Add(1 * time.Hour)).
					WithLastUpdated(createTime.Add(2 * time.Hour)).
					WithSourceCluster(sourceClusterID).Build()
				err := rules.SetRulesForTesting(ctx, []*rules.FailureAssociationRule{
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

				expectCreate = true
				test()
			})
			Convey("With bug updates disabled", func() {
				suggestedClusters[1].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{OneDay: metrics.Counts{Residual: 100}}

				opts.enableBugUpdates = false

				expectCreate = false
				test()
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
				err = updateAnalysisAndBugsForProject(ctx, opts)
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

				err = updateAnalysisAndBugsForProject(ctx, opts)
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
				So(len(f.Issues), ShouldEqual, count)
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

			err = updateAnalysisAndBugsForProject(ctx, opts)
			So(err, ShouldBeNil)
			expectBugClusters(1)

			err = updateAnalysisAndBugsForProject(ctx, opts)
			So(err, ShouldBeNil)
			expectBugClusters(2)

			err = updateAnalysisAndBugsForProject(ctx, opts)
			So(err, ShouldBeNil)

			expectedRules := []*rules.FailureAssociationRule{
				{
					Project:         "chromium",
					RuleDefinition:  `test = "testname-0"`,
					BugID:           bugs.BugID{System: "monorail", ID: "chromium/100"},
					SourceCluster:   suggestedClusters[0].ClusterID,
					IsActive:        true,
					IsManagingBug:   true,
					CreationUser:    rules.LUCIAnalysisSystem,
					LastUpdatedUser: rules.LUCIAnalysisSystem,
				},
				{
					Project:         "chromium",
					RuleDefinition:  `reason LIKE "want foo, got bar"`,
					BugID:           bugs.BugID{System: "monorail", ID: "chromium/101"},
					SourceCluster:   suggestedClusters[1].ClusterID,
					IsActive:        true,
					IsManagingBug:   true,
					CreationUser:    rules.LUCIAnalysisSystem,
					LastUpdatedUser: rules.LUCIAnalysisSystem,
				},
				{
					Project:         "chromium",
					RuleDefinition:  `reason LIKE "want foofoo, got bar"`,
					BugID:           bugs.BugID{System: "monorail", ID: "chromium/102"},
					SourceCluster:   suggestedClusters[2].ClusterID,
					IsActive:        true,
					IsManagingBug:   true,
					CreationUser:    rules.LUCIAnalysisSystem,
					LastUpdatedUser: rules.LUCIAnalysisSystem,
				},
			}

			expectRules := func(expectedRules []*rules.FailureAssociationRule) {
				// Check final set of rules is as expected.
				rs, err := rules.ReadAll(span.Single(ctx), "chromium")
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

				sortedExpected := make([]*rules.FailureAssociationRule, len(expectedRules))
				copy(sortedExpected, expectedRules)
				sort.Slice(sortedExpected, func(i, j int) bool {
					return sortedExpected[i].BugID.System < sortedExpected[j].BugID.System ||
						(sortedExpected[i].BugID.System == sortedExpected[j].BugID.System &&
							sortedExpected[i].BugID.ID < sortedExpected[j].BugID.ID)
				})

				So(rs, ShouldResemble, sortedExpected)
				So(len(f.Issues), ShouldEqual, len(sortedExpected))
			}
			expectRules(expectedRules)

			// Further updates do nothing.
			originalIssues := monorail.CopyIssuesStore(f)
			err = updateAnalysisAndBugsForProject(ctx, opts)
			So(err, ShouldBeNil)
			So(f, monorail.ShouldResembleIssuesStore, originalIssues)
			expectRules(expectedRules)

			rs, err := rules.ReadActive(span.Single(ctx), project)
			So(err, ShouldBeNil)

			bugClusters := []*analysis.Cluster{
				makeBugCluster(rs[0].RuleID),
				makeBugCluster(rs[1].RuleID),
				makeBugCluster(rs[2].RuleID),
			}

			Convey("Re-clustering in progress", func() {
				ac.clusters = append(suggestedClusters, bugClusters[1:]...)

				Convey("Negligable cluster impact does not affect issue priority or status", func() {
					issue := f.Issues[0].Issue
					So(issue.Name, ShouldEqual, "projects/chromium/issues/100")
					originalPriority := monorail.ChromiumTestIssuePriority(issue)
					originalStatus := issue.Status.Status
					So(originalStatus, ShouldNotEqual, monorail.VerifiedStatus)

					SetResidualImpact(
						bugClusters[1], monorail.ChromiumClosureImpact())
					err = updateAnalysisAndBugsForProject(ctx, opts)
					So(err, ShouldBeNil)

					So(len(f.Issues), ShouldEqual, 3)
					issue = f.Issues[0].Issue
					So(issue.Name, ShouldEqual, "projects/chromium/issues/100")
					So(monorail.ChromiumTestIssuePriority(issue), ShouldEqual, originalPriority)
					So(issue.Status.Status, ShouldEqual, originalStatus)

					expectRules(expectedRules)
				})
			})
			Convey("Re-clustering complete", func() {
				ac.clusters = append(suggestedClusters, bugClusters[1:]...)

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

				Convey("Cluster impact does not change if bug not managed by rule", func() {
					// Set IsManagingBug to false on one rule.
					rs[2].IsManagingBug = false
					rules.SetRulesForTesting(ctx, rs)

					issue := f.Issues[2].Issue
					So(issue.Name, ShouldEqual, "projects/chromium/issues/102")
					originalPriority := monorail.ChromiumTestIssuePriority(issue)
					originalStatus := issue.Status.Status
					So(originalPriority, ShouldNotEqual, "0")

					// Set P0 impact on the cluster.
					SetResidualImpact(
						bugClusters[2], monorail.ChromiumP0Impact())
					err = updateAnalysisAndBugsForProject(ctx, opts)
					So(err, ShouldBeNil)

					// Check that the rule priority and status has not changed.
					So(len(f.Issues), ShouldEqual, 3)
					issue = f.Issues[2].Issue
					So(issue.Name, ShouldEqual, "projects/chromium/issues/102")
					So(issue.Status.Status, ShouldEqual, originalStatus)
					So(monorail.ChromiumTestIssuePriority(issue), ShouldEqual, originalPriority)
				})
				Convey("Increasing cluster impact increases issue priority", func() {
					issue := f.Issues[2].Issue
					So(issue.Name, ShouldEqual, "projects/chromium/issues/102")
					So(monorail.ChromiumTestIssuePriority(issue), ShouldNotEqual, "0")

					SetResidualImpact(
						bugClusters[2], monorail.ChromiumP0Impact())
					err = updateAnalysisAndBugsForProject(ctx, opts)
					So(err, ShouldBeNil)

					So(len(f.Issues), ShouldEqual, 3)
					issue = f.Issues[2].Issue
					So(issue.Name, ShouldEqual, "projects/chromium/issues/102")
					So(monorail.ChromiumTestIssuePriority(issue), ShouldEqual, "0")

					expectRules(expectedRules)
				})
				Convey("Decreasing cluster impact decreases issue priority", func() {
					issue := f.Issues[2].Issue
					So(issue.Name, ShouldEqual, "projects/chromium/issues/102")
					So(monorail.ChromiumTestIssuePriority(issue), ShouldNotEqual, "3")

					SetResidualImpact(
						bugClusters[2], monorail.ChromiumP3Impact())
					err = updateAnalysisAndBugsForProject(ctx, opts)
					So(err, ShouldBeNil)

					So(len(f.Issues), ShouldEqual, 3)
					issue = f.Issues[2].Issue
					So(issue.Name, ShouldEqual, "projects/chromium/issues/102")
					So(issue.Status.Status, ShouldEqual, monorail.UntriagedStatus)
					So(monorail.ChromiumTestIssuePriority(issue), ShouldEqual, "3")

					expectRules(expectedRules)
				})
				Convey("Deleting cluster closes issue", func() {
					issue := f.Issues[2].Issue
					So(issue.Name, ShouldEqual, "projects/chromium/issues/102")
					So(issue.Status.Status, ShouldEqual, monorail.UntriagedStatus)

					// Drop the bug cluster at index 2.
					bugClusters = bugClusters[:2]
					ac.clusters = append(suggestedClusters, bugClusters...)
					err = updateAnalysisAndBugsForProject(ctx, opts)
					So(err, ShouldBeNil)

					So(len(f.Issues), ShouldEqual, 3)
					issue = f.Issues[2].Issue
					So(issue.Name, ShouldEqual, "projects/chromium/issues/102")
					So(issue.Status.Status, ShouldEqual, monorail.VerifiedStatus)

					expectRules(expectedRules)

					Convey("Rule automatically archived after 30 days", func() {
						tc.Add(time.Hour * 24 * 30)

						// Act
						err = updateAnalysisAndBugsForProject(ctx, opts)
						So(err, ShouldBeNil)

						// Verify
						expectedRules[2].IsActive = false
						expectRules(expectedRules)

						So(len(f.Issues), ShouldEqual, 3)
						issue = f.Issues[2].Issue
						So(issue.Name, ShouldEqual, "projects/chromium/issues/102")
						So(issue.Status.Status, ShouldEqual, monorail.VerifiedStatus)
					})
				})
				Convey("Bug marked as duplicate of bug with rule", func() {
					// Setup
					issueOne := f.Issues[1].Issue
					So(issueOne.Name, ShouldEqual, "projects/chromium/issues/101")

					issueTwo := f.Issues[2].Issue
					So(issueTwo.Name, ShouldEqual, "projects/chromium/issues/102")

					issueOne.Status.Status = monorail.DuplicateStatus
					issueOne.MergedIntoIssueRef = &api_proto.IssueRef{
						Issue: issueTwo.Name,
					}

					Convey("Happy path", func() {
						// Act
						err = updateAnalysisAndBugsForProject(ctx, opts)
						So(err, ShouldBeNil)

						// Verify
						expectedRules[1].IsActive = false
						expectedRules[2].RuleDefinition = "reason LIKE \"want foo, got bar\" OR\nreason LIKE \"want foofoo, got bar\""
						expectRules(expectedRules)

						So(f.Issues[1].Comments, ShouldHaveLength, 3)
						So(f.Issues[1].Comments[2].Content, ShouldContainSubstring, "LUCI Analysis has merged the failure association rule for this bug into the rule for the canonical bug.")
						So(f.Issues[1].Comments[2].Content, ShouldContainSubstring, expectedRules[2].RuleID)

						So(f.Issues[2].Comments, ShouldHaveLength, 3)
						So(f.Issues[2].Comments[2].Content, ShouldContainSubstring, "LUCI Analysis has merged the failure association rule for that bug into the rule for this bug.")
					})
					Convey("Bugs are in a duplicate bug cycle", func() {
						// Note that this is a simple cycle with only two bugs.
						// The implementation allows for larger cycles, however.
						issueTwo.Status.Status = monorail.DuplicateStatus
						issueTwo.MergedIntoIssueRef = &api_proto.IssueRef{
							Issue: issueOne.Name,
						}

						// Act
						err = updateAnalysisAndBugsForProject(ctx, opts)
						So(err, ShouldBeNil)

						// Verify
						// Issue one kicked out of duplicate status.
						So(issueOne.Status.Status, ShouldNotEqual, monorail.DuplicateStatus)

						// As the cycle is now broken, issue two is merged into
						// issue one.
						expectedRules[1].RuleDefinition = "reason LIKE \"want foo, got bar\" OR\nreason LIKE \"want foofoo, got bar\""
						expectedRules[2].IsActive = false
						expectRules(expectedRules)

						So(f.Issues[1].Comments, ShouldHaveLength, 4)
						So(f.Issues[1].Comments[2].Content, ShouldContainSubstring, "a cycle was detected in the bug merged-into graph")
						So(f.Issues[1].Comments[3].Content, ShouldContainSubstring, "LUCI Analysis has merged the failure association rule for that bug into the rule for this bug.")
					})
					Convey("Merged rule would be too long", func() {
						// Setup
						// Make one of the rules we will be merging very close
						// to the rule length limit.
						longRule := fmt.Sprintf("test = \"%s\"", strings.Repeat("a", rules.MaxRuleDefinitionLength-10))

						_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
							issueOneRule, err := rules.ReadByBug(ctx, bugs.BugID{System: "monorail", ID: "chromium/101"})
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
						err = updateAnalysisAndBugsForProject(ctx, opts)
						So(err, ShouldBeNil)

						// Verify
						// Rules should not have changed (except for the update we made).
						expectedRules[1].RuleDefinition = longRule
						expectRules(expectedRules)

						// Issue one kicked out of duplicate status.
						So(issueOne.Status.Status, ShouldNotEqual, monorail.DuplicateStatus)

						// Comment should appear on the bug.
						So(f.Issues[1].Comments, ShouldHaveLength, 3)
						So(f.Issues[1].Comments[2].Content, ShouldContainSubstring, "the merged failure association rule would be too long")
					})
				})
				Convey("Bug marked as duplicate of bug without a rule in this project", func() {
					// Setup
					issueOne := f.Issues[1].Issue
					So(issueOne.Name, ShouldEqual, "projects/chromium/issues/101")

					issueOne.Status.Status = monorail.DuplicateStatus
					issueOne.MergedIntoIssueRef = &api_proto.IssueRef{
						ExtIdentifier: "b/1234",
					}

					Convey("Bug managed by a rule in another project", func() {
						extraRule := &rules.FailureAssociationRule{
							Project:        "otherproject",
							RuleDefinition: `reason LIKE "blah"`,
							RuleID:         "1234567890abcdef1234567890abcdef",
							BugID:          bugs.BugID{System: "buganizer", ID: "1234"},
							IsActive:       true,
							IsManagingBug:  true,
						}
						_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
							return rules.Create(ctx, extraRule, "user@chromium.org")
						})
						So(err, ShouldBeNil)

						// Act
						err = updateAnalysisAndBugsForProject(ctx, opts)
						So(err, ShouldBeNil)

						// Verify
						expectedRules[1].BugID = bugs.BugID{System: "buganizer", ID: "1234"}
						expectedRules[1].IsManagingBug = false // Let the other rule continue to manage the bug.
						expectRules(expectedRules)

						So(f.Issues[1].Comments, ShouldHaveLength, 3)
						So(f.Issues[1].Comments[2].Content, ShouldContainSubstring, "LUCI Analysis has merged the failure association rule for this bug into the rule for the canonical bug.")
						So(f.Issues[1].Comments[2].Content, ShouldContainSubstring, expectedRules[1].RuleID)
					})
					Convey("Bug not managed by a rule in another project", func() {
						// Act
						err = updateAnalysisAndBugsForProject(ctx, opts)
						So(err, ShouldBeNil)

						// Verify
						expectedRules[1].BugID = bugs.BugID{System: "buganizer", ID: "1234"}
						expectedRules[1].IsManagingBug = true
						expectRules(expectedRules)

						So(f.Issues[1].Comments, ShouldHaveLength, 3)
						So(f.Issues[1].Comments[2].Content, ShouldContainSubstring, "LUCI Analysis has merged the failure association rule for this bug into the rule for the canonical bug.")
						So(f.Issues[1].Comments[2].Content, ShouldContainSubstring, expectedRules[1].RuleID)
					})
				})
				Convey("Bug marked as archived should archive rule", func() {
					issueOne := f.Issues[1].Issue
					So(issueOne.Name, ShouldEqual, "projects/chromium/issues/101")
					issueOne.Status = &api_proto.Issue_StatusValue{Status: "Archived"}

					// Act
					err = updateAnalysisAndBugsForProject(ctx, opts)
					So(err, ShouldBeNil)

					// Assert
					expectedRules[1].IsActive = false
					expectRules(expectedRules)
				})
			})
		})
	})
}

func emptyMetricValues() map[metrics.ID]metrics.TimewiseCounts {
	result := make(map[metrics.ID]metrics.TimewiseCounts)
	for _, m := range metrics.ComputedMetrics {
		result[m.ID] = metrics.TimewiseCounts{}
	}
	return result
}

func makeTestNameCluster(config *compiledcfg.ProjectConfig, uniqifier int) *analysis.Cluster {
	testID := fmt.Sprintf("testname-%v", uniqifier)
	return &analysis.Cluster{
		ClusterID: testIDClusterID(config, testID),
		MetricValues: map[metrics.ID]metrics.TimewiseCounts{
			metrics.Failures.ID: {
				OneDay:   metrics.Counts{Residual: 9},
				ThreeDay: metrics.Counts{Residual: 29},
				SevenDay: metrics.Counts{Residual: 69},
			},
		},
		TopTestIDs: []analysis.TopCount{{Value: testID, Count: 1}},
	}
}

func makeReasonCluster(config *compiledcfg.ProjectConfig, uniqifier int) *analysis.Cluster {
	// Because the failure reason clustering algorithm removes numbers
	// when clustering failure reasons, it is better not to use the
	// uniqifier directly in the reason, to avoid cluster ID collisions.
	var foo strings.Builder
	for i := 0; i < uniqifier; i++ {
		foo.WriteString("foo")
	}
	reason := fmt.Sprintf("want %s, got bar", foo.String())

	return &analysis.Cluster{
		ClusterID: reasonClusterID(config, reason),
		MetricValues: map[metrics.ID]metrics.TimewiseCounts{
			metrics.Failures.ID: {
				OneDay:   metrics.Counts{Residual: 9},
				ThreeDay: metrics.Counts{Residual: 29},
				SevenDay: metrics.Counts{Residual: 69},
			},
		},
		TopTestIDs: []analysis.TopCount{
			{Value: fmt.Sprintf("testname-a-%v", uniqifier), Count: 1},
			{Value: fmt.Sprintf("testname-b-%v", uniqifier), Count: 1},
		},
		ExampleFailureReason: bigquery.NullString{Valid: true, StringVal: reason},
	}
}

func makeBugCluster(ruleID string) *analysis.Cluster {
	return &analysis.Cluster{
		ClusterID: bugClusterID(ruleID),
		MetricValues: map[metrics.ID]metrics.TimewiseCounts{
			metrics.Failures.ID: {
				OneDay:   metrics.Counts{Residual: 9},
				ThreeDay: metrics.Counts{Residual: 29},
				SevenDay: metrics.Counts{Residual: 69},
			},
		},
		TopTestIDs: []analysis.TopCount{{Value: "testname-0", Count: 1}},
	}
}

func testIDClusterID(config *compiledcfg.ProjectConfig, testID string) clustering.ClusterID {
	testAlg, err := algorithms.SuggestingAlgorithm(testname.AlgorithmName)
	So(err, ShouldBeNil)

	return clustering.ClusterID{
		Algorithm: testname.AlgorithmName,
		ID: hex.EncodeToString(testAlg.Cluster(config, &clustering.Failure{
			TestID: testID,
		})),
	}
}

func reasonClusterID(config *compiledcfg.ProjectConfig, reason string) clustering.ClusterID {
	reasonAlg, err := algorithms.SuggestingAlgorithm(failurereason.AlgorithmName)
	So(err, ShouldBeNil)

	return clustering.ClusterID{
		Algorithm: failurereason.AlgorithmName,
		ID: hex.EncodeToString(reasonAlg.Cluster(config, &clustering.Failure{
			Reason: &pb.FailureReason{PrimaryErrorMessage: reason},
		})),
	}
}

func bugClusterID(ruleID string) clustering.ClusterID {
	return clustering.ClusterID{
		Algorithm: rulesalgorithm.AlgorithmName,
		ID:        ruleID,
	}
}

type fakeAnalysisClient struct {
	analysisBuilt bool
	clusters      []*analysis.Cluster
}

func (f *fakeAnalysisClient) RebuildAnalysis(ctx context.Context, project string) error {
	f.analysisBuilt = true
	return nil
}

func (f *fakeAnalysisClient) PurgeStaleRows(ctx context.Context, luciProject string) error {
	return nil
}

func (f *fakeAnalysisClient) ReadImpactfulClusters(ctx context.Context, opts analysis.ImpactfulClusterReadOptions) ([]*analysis.Cluster, error) {
	if !f.analysisBuilt {
		return nil, errors.New("cluster_summaries does not exist")
	}
	var results []*analysis.Cluster
	for _, c := range f.clusters {
		include := opts.AlwaysIncludeBugClusters && c.ClusterID.IsBugCluster()
		if opts.Thresholds.CriticalFailuresExonerated != nil {
			include = include || meetsMetricThreshold(c.MetricValues[metrics.CriticalFailuresExonerated.ID], opts.Thresholds.CriticalFailuresExonerated)
		}
		if opts.Thresholds.TestResultsFailed != nil {
			include = include || meetsMetricThreshold(c.MetricValues[metrics.Failures.ID], opts.Thresholds.TestResultsFailed)
		}
		if opts.Thresholds.TestRunsFailed != nil {
			include = include || meetsMetricThreshold(c.MetricValues[metrics.TestRunsFailed.ID], opts.Thresholds.TestRunsFailed)
		}
		if opts.Thresholds.PresubmitRunsFailed != nil {
			include = include || meetsMetricThreshold(c.MetricValues[metrics.HumanClsFailedPresubmit.ID], opts.Thresholds.PresubmitRunsFailed)
		}
		if include {
			results = append(results, c)
		}
	}
	return results, nil
}

func meetsMetricThreshold(values metrics.TimewiseCounts, threshold *configpb.MetricThreshold) bool {
	return meetsThreshold(values.OneDay.Residual, threshold.OneDay) ||
		meetsThreshold(values.ThreeDay.Residual, threshold.ThreeDay) ||
		meetsThreshold(values.SevenDay.Residual, threshold.SevenDay)
}

func meetsThreshold(value int64, threshold *int64) bool {
	// threshold == nil is treated as an unsatisfiable threshold.
	return threshold != nil && value >= *threshold
}
