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

package rules

import (
	"context"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/bugs"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/testutil"
)

func TestSpan(t *testing.T) {
	Convey(`With Spanner Test Database`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		Convey(`Read`, func() {
			Convey(`Not Exists`, func() {
				ruleID := strings.Repeat("00", 16)
				rule, err := Read(span.Single(ctx), testProject, ruleID)
				So(err, ShouldEqual, NotExistsErr)
				So(rule, ShouldBeNil)
			})
			Convey(`Exists`, func() {
				expectedRule := NewRule(100).Build()
				err := SetRulesForTesting(ctx, []*FailureAssociationRule{expectedRule})
				So(err, ShouldBeNil)

				rule, err := Read(span.Single(ctx), testProject, expectedRule.RuleID)
				So(err, ShouldBeNil)
				So(rule, ShouldResemble, expectedRule)
			})
		})
		Convey(`ReadActive`, func() {
			Convey(`Empty`, func() {
				err := SetRulesForTesting(ctx, nil)
				So(err, ShouldBeNil)

				rules, err := ReadActive(span.Single(ctx), testProject)
				So(err, ShouldBeNil)
				So(rules, ShouldResemble, []*FailureAssociationRule{})
			})
			Convey(`Multiple`, func() {
				rulesToCreate := []*FailureAssociationRule{
					NewRule(0).Build(),
					NewRule(1).WithProject("otherproject").Build(),
					NewRule(2).WithActive(false).Build(),
					NewRule(3).Build(),
				}
				err := SetRulesForTesting(ctx, rulesToCreate)
				So(err, ShouldBeNil)

				rules, err := ReadActive(span.Single(ctx), testProject)
				So(err, ShouldBeNil)
				So(rules, ShouldResemble, []*FailureAssociationRule{
					rulesToCreate[3],
					rulesToCreate[0],
				})
			})
		})
		Convey(`ReadByBug`, func() {
			bugID := bugs.BugID{System: "monorail", ID: "monorailproject/1"}
			Convey(`Empty`, func() {
				rules, err := ReadByBug(span.Single(ctx), bugID)
				So(err, ShouldBeNil)
				So(rules, ShouldBeEmpty)
			})
			Convey(`Multiple`, func() {
				expectedRule := NewRule(100).
					WithProject("testproject").
					WithBug(bugID).
					Build()
				expectedRule2 := NewRule(100).
					WithProject("testproject2").
					WithBug(bugID).
					WithBugManaged(false).
					WithBugPriorityManaged(false).
					Build()
				expectedRules := []*FailureAssociationRule{expectedRule, expectedRule2}
				err := SetRulesForTesting(ctx, expectedRules)
				So(err, ShouldBeNil)

				rules, err := ReadByBug(span.Single(ctx), bugID)
				So(err, ShouldBeNil)
				So(rules, ShouldResemble, expectedRules)
			})
		})
		Convey(`ReadDelta`, func() {
			Convey(`Invalid since time`, func() {
				_, err := ReadDelta(span.Single(ctx), testProject, time.Time{})
				So(err, ShouldErrLike, "cannot query rule deltas from before project inception")
			})
			Convey(`Empty`, func() {
				err := SetRulesForTesting(ctx, nil)
				So(err, ShouldBeNil)

				rules, err := ReadDelta(span.Single(ctx), testProject, StartingEpoch)
				So(err, ShouldBeNil)
				So(rules, ShouldResemble, []*FailureAssociationRule{})
			})
			Convey(`Multiple`, func() {
				reference := time.Date(2020, 1, 2, 3, 4, 5, 6000, time.UTC)
				rulesToCreate := []*FailureAssociationRule{
					NewRule(0).WithLastUpdated(reference).Build(),
					NewRule(1).WithProject("otherproject").WithLastUpdated(reference.Add(time.Minute)).Build(),
					NewRule(2).WithActive(false).WithLastUpdated(reference.Add(time.Minute)).Build(),
					NewRule(3).WithLastUpdated(reference.Add(time.Microsecond)).Build(),
				}
				err := SetRulesForTesting(ctx, rulesToCreate)
				So(err, ShouldBeNil)

				rules, err := ReadDelta(span.Single(ctx), testProject, StartingEpoch)
				So(err, ShouldBeNil)
				So(rules, ShouldResemble, []*FailureAssociationRule{
					rulesToCreate[3],
					rulesToCreate[0],
					rulesToCreate[2],
				})

				rules, err = ReadDelta(span.Single(ctx), testProject, reference)
				So(err, ShouldBeNil)
				So(rules, ShouldResemble, []*FailureAssociationRule{
					rulesToCreate[3],
					rulesToCreate[2],
				})

				rules, err = ReadDelta(span.Single(ctx), testProject, reference.Add(time.Minute))
				So(err, ShouldBeNil)
				So(rules, ShouldResemble, []*FailureAssociationRule{})
			})
		})
		Convey(`ReadMany`, func() {
			rulesToCreate := []*FailureAssociationRule{
				NewRule(0).Build(),
				NewRule(1).WithProject("otherproject").Build(),
				NewRule(2).WithActive(false).Build(),
				NewRule(3).Build(),
			}
			err := SetRulesForTesting(ctx, rulesToCreate)
			So(err, ShouldBeNil)

			ids := []string{
				rulesToCreate[0].RuleID,
				rulesToCreate[1].RuleID, // Should not exist, exists in different project.
				rulesToCreate[2].RuleID,
				rulesToCreate[3].RuleID,
				strings.Repeat("01", 16), // Non-existent ID, should not exist.
				strings.Repeat("02", 16), // Non-existent ID, should not exist.
				rulesToCreate[2].RuleID,  // Repeat of existent ID.
				strings.Repeat("01", 16), // Repeat of non-existent ID, should not exist.
			}
			rules, err := ReadMany(span.Single(ctx), testProject, ids)
			So(err, ShouldBeNil)
			So(rules, ShouldResemble, []*FailureAssociationRule{
				rulesToCreate[0],
				nil,
				rulesToCreate[2],
				rulesToCreate[3],
				nil,
				nil,
				rulesToCreate[2],
				nil,
			})
		})
		Convey(`ReadVersion`, func() {
			Convey(`Empty`, func() {
				err := SetRulesForTesting(ctx, nil)
				So(err, ShouldBeNil)

				timestamp, err := ReadVersion(span.Single(ctx), testProject)
				So(err, ShouldBeNil)
				So(timestamp, ShouldResemble, StartingVersion)
			})
			Convey(`Multiple`, func() {
				// Spanner commit timestamps are in microsecond
				// (not nanosecond) granularity. The MAX operator
				// on timestamps truncates to microseconds. For this
				// reason, we use microsecond resolution timestamps
				// when testing.
				reference := time.Date(2020, 1, 2, 3, 4, 5, 6000, time.UTC)
				rulesToCreate := []*FailureAssociationRule{
					NewRule(0).
						WithPredicateLastUpdated(reference.Add(-1 * time.Hour)).
						WithLastUpdated(reference.Add(-1 * time.Hour)).
						Build(),
					NewRule(1).WithProject("otherproject").
						WithPredicateLastUpdated(reference.Add(time.Hour)).
						WithLastUpdated(reference.Add(time.Hour)).
						Build(),
					NewRule(2).WithActive(false).
						WithPredicateLastUpdated(reference.Add(-1 * time.Second)).
						WithLastUpdated(reference).
						Build(),
					NewRule(3).
						WithPredicateLastUpdated(reference.Add(-2 * time.Hour)).
						WithLastUpdated(reference.Add(-2 * time.Hour)).
						Build(),
				}
				err := SetRulesForTesting(ctx, rulesToCreate)
				So(err, ShouldBeNil)

				version, err := ReadVersion(span.Single(ctx), testProject)
				So(err, ShouldBeNil)
				So(version, ShouldResemble, Version{
					Predicates: reference.Add(-1 * time.Second),
					Total:      reference,
				})
			})
		})
		Convey(`ReadTotalActiveRules`, func() {
			Convey(`Empty`, func() {
				err := SetRulesForTesting(ctx, nil)
				So(err, ShouldBeNil)

				result, err := ReadTotalActiveRules(span.Single(ctx))
				So(err, ShouldBeNil)
				So(result, ShouldResemble, map[string]int64{})
			})
			Convey(`Multiple`, func() {
				rulesToCreate := []*FailureAssociationRule{
					// Two active and one inactive rule for Project A.
					NewRule(0).WithProject("project-a").WithActive(true).Build(),
					NewRule(1).WithProject("project-a").WithActive(false).Build(),
					NewRule(2).WithProject("project-a").WithActive(true).Build(),
					// One inactive rule for Project B.
					NewRule(3).WithProject("project-b").WithActive(false).Build(),
					// One active rule for Project C.
					NewRule(4).WithProject("project-c").WithActive(true).Build(),
				}
				err := SetRulesForTesting(ctx, rulesToCreate)
				So(err, ShouldBeNil)

				result, err := ReadTotalActiveRules(span.Single(ctx))
				So(err, ShouldBeNil)
				So(result, ShouldResemble, map[string]int64{
					"project-a": 2,
					"project-b": 0,
					"project-c": 1,
				})
			})
		})
		Convey(`Create`, func() {
			testCreate := func(bc *FailureAssociationRule, user string) (time.Time, error) {
				commitTime, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					return Create(ctx, bc, user)
				})
				return commitTime.In(time.UTC), err
			}
			r := NewRule(100).Build()
			r.CreationUser = LUCIAnalysisSystem
			r.LastUpdatedUser = LUCIAnalysisSystem

			Convey(`Valid`, func() {
				testExists := func(expectedRule FailureAssociationRule) {
					txn, cancel := span.ReadOnlyTransaction(ctx)
					defer cancel()
					rules, err := ReadActive(txn, testProject)

					So(err, ShouldBeNil)
					So(len(rules), ShouldEqual, 1)

					readRule := rules[0]
					So(*readRule, ShouldResemble, expectedRule)
				}

				Convey(`With Source Cluster`, func() {
					So(r.SourceCluster.Algorithm, ShouldNotBeEmpty)
					So(r.SourceCluster.ID, ShouldNotBeNil)
					commitTime, err := testCreate(r, LUCIAnalysisSystem)
					So(err, ShouldBeNil)

					expectedRule := *r
					expectedRule.LastUpdated = commitTime
					expectedRule.PredicateLastUpdated = commitTime
					expectedRule.IsManagingBugPriorityLastUpdated = commitTime
					expectedRule.CreationTime = commitTime
					testExists(expectedRule)
				})
				Convey(`Without Source Cluster`, func() {
					// E.g. in case of a manually created rule.
					r.SourceCluster = clustering.ClusterID{}
					r.CreationUser = "user@google.com"
					r.LastUpdatedUser = "user@google.com"
					commitTime, err := testCreate(r, "user@google.com")
					So(err, ShouldBeNil)

					expectedRule := *r
					expectedRule.LastUpdated = commitTime
					expectedRule.PredicateLastUpdated = commitTime
					expectedRule.IsManagingBugPriorityLastUpdated = commitTime
					expectedRule.CreationTime = commitTime
					testExists(expectedRule)
				})
				Convey(`With Buganizer Bug`, func() {
					r.BugID = bugs.BugID{System: "buganizer", ID: "1234567890"}
					commitTime, err := testCreate(r, LUCIAnalysisSystem)
					So(err, ShouldBeNil)

					expectedRule := *r
					expectedRule.LastUpdated = commitTime
					expectedRule.PredicateLastUpdated = commitTime
					expectedRule.IsManagingBugPriorityLastUpdated = commitTime
					expectedRule.CreationTime = commitTime
					testExists(expectedRule)
				})
				Convey(`With Monorail Bug`, func() {
					r.BugID = bugs.BugID{System: "monorail", ID: "project/1234567890"}
					commitTime, err := testCreate(r, LUCIAnalysisSystem)
					So(err, ShouldBeNil)

					expectedRule := *r
					expectedRule.LastUpdated = commitTime
					expectedRule.PredicateLastUpdated = commitTime
					expectedRule.IsManagingBugPriorityLastUpdated = commitTime
					expectedRule.CreationTime = commitTime
					testExists(expectedRule)
				})
			})
			Convey(`With invalid Project`, func() {
				Convey(`Missing`, func() {
					r.Project = ""
					_, err := testCreate(r, LUCIAnalysisSystem)
					So(err, ShouldErrLike, "project must be valid")
				})
				Convey(`Invalid`, func() {
					r.Project = "!"
					_, err := testCreate(r, LUCIAnalysisSystem)
					So(err, ShouldErrLike, "project must be valid")
				})
			})
			Convey(`With invalid Rule Definition`, func() {
				r.RuleDefinition = "invalid"
				_, err := testCreate(r, LUCIAnalysisSystem)
				So(err, ShouldErrLike, "rule definition is not valid")
			})
			Convey(`With too long Rule Definition`, func() {
				r.RuleDefinition = strings.Repeat(" ", MaxRuleDefinitionLength+1)
				_, err := testCreate(r, LUCIAnalysisSystem)
				So(err, ShouldErrLike, "rule definition exceeds maximum length of 65536")
			})
			Convey(`With invalid Bug ID`, func() {
				r.BugID.System = ""
				_, err := testCreate(r, LUCIAnalysisSystem)
				So(err, ShouldErrLike, "bug ID is not valid")
			})
			Convey(`With invalid Source Cluster`, func() {
				So(r.SourceCluster.ID, ShouldNotBeNil)
				r.SourceCluster.Algorithm = ""
				_, err := testCreate(r, LUCIAnalysisSystem)
				So(err, ShouldErrLike, "source cluster ID is not valid")
			})
			Convey(`With invalid User`, func() {
				_, err := testCreate(r, "")
				So(err, ShouldErrLike, "user must be valid")
			})
		})
		Convey(`Update`, func() {
			testExists := func(expectedRule *FailureAssociationRule) {
				txn, cancel := span.ReadOnlyTransaction(ctx)
				defer cancel()
				rule, err := Read(txn, expectedRule.Project, expectedRule.RuleID)
				So(err, ShouldBeNil)
				So(rule, ShouldResemble, expectedRule)
			}
			testUpdate := func(bc *FailureAssociationRule, options UpdateOptions, user string) (time.Time, error) {
				commitTime, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					return Update(ctx, bc, options, user)
				})
				return commitTime.In(time.UTC), err
			}
			r := NewRule(100).Build()
			err := SetRulesForTesting(ctx, []*FailureAssociationRule{r})
			So(err, ShouldBeNil)

			Convey(`Valid`, func() {
				Convey(`Update predicate`, func() {
					r.RuleDefinition = `test = "UpdateTest"`
					r.BugID = bugs.BugID{System: "monorail", ID: "chromium/651234"}
					r.IsActive = false
					r.SourceCluster = clustering.ClusterID{Algorithm: "testname-v1", ID: "00112233445566778899aabbccddeeff"}
					commitTime, err := testUpdate(r, UpdateOptions{
						PredicateUpdated: true,
					}, "testuser@google.com")
					So(err, ShouldBeNil)

					expectedRule := *r
					expectedRule.PredicateLastUpdated = commitTime
					expectedRule.LastUpdated = commitTime
					expectedRule.LastUpdatedUser = "testuser@google.com"
					testExists(&expectedRule)
				})
				Convey(`Do not update predicate`, func() {
					r.BugID = bugs.BugID{System: "monorail", ID: "chromium/651234"}
					r.SourceCluster = clustering.ClusterID{Algorithm: "testname-v1", ID: "00112233445566778899aabbccddeeff"}
					commitTime, err := testUpdate(r, UpdateOptions{
						PredicateUpdated: false,
					}, "testuser@google.com")
					So(err, ShouldBeNil)

					expectedRule := *r
					expectedRule.LastUpdated = commitTime
					expectedRule.LastUpdatedUser = "testuser@google.com"
					testExists(&expectedRule)
				})
				Convey(`Update IsManagingBugPriority`, func() {
					r.IsManagingBugPriority = false
					commitTime, err := testUpdate(r, UpdateOptions{
						IsManagingBugPriorityUpdated: true,
					}, "testuser@google.com")
					So(err, ShouldBeNil)

					expectedRule := *r
					expectedRule.IsManagingBugPriorityLastUpdated = commitTime
					expectedRule.LastUpdated = commitTime
					expectedRule.LastUpdatedUser = "testuser@google.com"
					testExists(&expectedRule)
				})
			})
			Convey(`Invalid`, func() {
				Convey(`With invalid User`, func() {
					_, err := testUpdate(r, UpdateOptions{
						PredicateUpdated: false,
					}, "")
					So(err, ShouldErrLike, "user must be valid")
				})
				Convey(`With invalid Rule Definition`, func() {
					r.RuleDefinition = "invalid"
					_, err := testUpdate(r, UpdateOptions{
						PredicateUpdated: true,
					}, LUCIAnalysisSystem)
					So(err, ShouldErrLike, "rule definition is not valid")
				})
				Convey(`With invalid Bug ID`, func() {
					r.BugID.System = ""
					_, err := testUpdate(r, UpdateOptions{
						PredicateUpdated: false,
					}, LUCIAnalysisSystem)
					So(err, ShouldErrLike, "bug ID is not valid")
				})
				Convey(`With invalid Source Cluster`, func() {
					So(r.SourceCluster.ID, ShouldNotBeNil)
					r.SourceCluster.Algorithm = ""
					_, err := testUpdate(r, UpdateOptions{
						PredicateUpdated: false,
					}, LUCIAnalysisSystem)
					So(err, ShouldErrLike, "source cluster ID is not valid")
				})
			})
		})
	})
}
