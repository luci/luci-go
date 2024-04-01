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

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/bugs"
	bugspb "go.chromium.org/luci/analysis/internal/bugs/proto"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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
				err := SetForTesting(ctx, []*Entry{expectedRule})
				So(err, ShouldBeNil)

				rule, err := Read(span.Single(ctx), testProject, expectedRule.RuleID)
				So(err, ShouldBeNil)
				So(rule, ShouldResembleProto, expectedRule)
			})
			Convey(`With null IsManagingBugPriorityLastUpdated`, func() {
				expectedRule := NewRule(100).WithBugPriorityManagedLastUpdateTime(time.Time{}).Build()
				err := SetForTesting(ctx, []*Entry{expectedRule})
				So(err, ShouldBeNil)
				_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					stmt := spanner.NewStatement(`UPDATE FailureAssociationRules
												SET IsManagingBugPriorityLastUpdated = NULL
												WHERE TRUE`)
					_, err := span.Update(ctx, stmt)
					return err
				})
				So(err, ShouldBeNil)
				rule, err := Read(span.Single(ctx), testProject, expectedRule.RuleID)
				So(err, ShouldBeNil)
				So(rule.IsManagingBugPriorityLastUpdateTime.IsZero(), ShouldBeTrue)
				So(rule, ShouldResembleProto, expectedRule)
			})
		})
		Convey(`ReadActive`, func() {
			Convey(`Empty`, func() {
				err := SetForTesting(ctx, nil)
				So(err, ShouldBeNil)

				rules, err := ReadActive(span.Single(ctx), testProject)
				So(err, ShouldBeNil)
				So(rules, ShouldResembleProto, []*Entry{})
			})
			Convey(`Multiple`, func() {
				rulesToCreate := []*Entry{
					NewRule(0).Build(),
					NewRule(1).WithProject("otherproject").Build(),
					NewRule(2).WithActive(false).Build(),
					NewRule(3).Build(),
				}
				err := SetForTesting(ctx, rulesToCreate)
				So(err, ShouldBeNil)

				rules, err := ReadActive(span.Single(ctx), testProject)
				So(err, ShouldBeNil)
				So(rules, ShouldResembleProto, []*Entry{
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
				expectedRules := []*Entry{expectedRule, expectedRule2}
				err := SetForTesting(ctx, expectedRules)
				So(err, ShouldBeNil)

				rules, err := ReadByBug(span.Single(ctx), bugID)
				So(err, ShouldBeNil)
				So(rules, ShouldResembleProto, expectedRules)
			})
		})
		Convey(`ReadDelta`, func() {
			Convey(`Invalid since time`, func() {
				_, err := ReadDelta(span.Single(ctx), testProject, time.Time{})
				So(err, ShouldErrLike, "cannot query rule deltas from before project inception")
			})
			Convey(`Empty`, func() {
				err := SetForTesting(ctx, nil)
				So(err, ShouldBeNil)

				rules, err := ReadDelta(span.Single(ctx), testProject, StartingEpoch)
				So(err, ShouldBeNil)
				So(rules, ShouldResembleProto, []*Entry{})
			})
			Convey(`Multiple`, func() {
				reference := time.Date(2020, 1, 2, 3, 4, 5, 6000, time.UTC)
				rulesToCreate := []*Entry{
					NewRule(0).WithLastUpdateTime(reference).Build(),
					NewRule(1).WithProject("otherproject").WithLastUpdateTime(reference.Add(time.Minute)).Build(),
					NewRule(2).WithActive(false).WithLastUpdateTime(reference.Add(time.Minute)).Build(),
					NewRule(3).WithLastUpdateTime(reference.Add(time.Microsecond)).Build(),
				}
				err := SetForTesting(ctx, rulesToCreate)
				So(err, ShouldBeNil)

				rules, err := ReadDelta(span.Single(ctx), testProject, StartingEpoch)
				So(err, ShouldBeNil)
				So(rules, ShouldResembleProto, []*Entry{
					rulesToCreate[3],
					rulesToCreate[0],
					rulesToCreate[2],
				})

				rules, err = ReadDelta(span.Single(ctx), testProject, reference)
				So(err, ShouldBeNil)
				So(rules, ShouldResembleProto, []*Entry{
					rulesToCreate[3],
					rulesToCreate[2],
				})

				rules, err = ReadDelta(span.Single(ctx), testProject, reference.Add(time.Minute))
				So(err, ShouldBeNil)
				So(rules, ShouldResembleProto, []*Entry{})
			})
		})

		Convey(`ReadDeltaAllProjects`, func() {
			Convey(`Invalid since time`, func() {
				_, err := ReadDeltaAllProjects(span.Single(ctx), time.Time{})
				So(err, ShouldErrLike, "cannot query rule deltas from before project inception")
			})
			Convey(`Empty`, func() {
				err := SetForTesting(ctx, nil)
				So(err, ShouldBeNil)

				rules, err := ReadDeltaAllProjects(span.Single(ctx), StartingEpoch)
				So(err, ShouldBeNil)
				So(rules, ShouldResembleProto, []*Entry{})
			})
			Convey(`Multiple`, func() {
				reference := time.Date(2020, 1, 2, 3, 4, 5, 6000, time.UTC)
				rulesToCreate := []*Entry{
					NewRule(0).WithLastUpdateTime(reference).Build(),
					NewRule(1).WithProject("otherproject").WithLastUpdateTime(reference.Add(time.Minute)).Build(),
					NewRule(2).WithActive(false).WithLastUpdateTime(reference.Add(time.Minute)).Build(),
					NewRule(3).WithLastUpdateTime(reference.Add(time.Microsecond)).Build(),
				}
				err := SetForTesting(ctx, rulesToCreate)
				So(err, ShouldBeNil)

				rules, err := ReadDeltaAllProjects(span.Single(ctx), StartingEpoch)
				So(err, ShouldBeNil)
				expected := []*Entry{
					rulesToCreate[3],
					rulesToCreate[0],
					rulesToCreate[2],
					rulesToCreate[1],
				}
				sortByID(expected)
				sortByID(rules)
				So(rules, ShouldResembleProto, expected)

				rules, err = ReadDeltaAllProjects(span.Single(ctx), reference)
				So(err, ShouldBeNil)
				expected = []*Entry{
					rulesToCreate[3],
					rulesToCreate[2],
					rulesToCreate[1],
				}
				sortByID(expected)
				sortByID(rules)
				So(rules, ShouldResembleProto, expected)

				rules, err = ReadDeltaAllProjects(span.Single(ctx), reference.Add(time.Minute))
				So(err, ShouldBeNil)
				So(rules, ShouldResembleProto, []*Entry{})
			})
		})

		Convey(`ReadMany`, func() {
			rulesToCreate := []*Entry{
				NewRule(0).Build(),
				NewRule(1).WithProject("otherproject").Build(),
				NewRule(2).WithActive(false).Build(),
				NewRule(3).Build(),
			}
			err := SetForTesting(ctx, rulesToCreate)
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
			So(rules, ShouldResembleProto, []*Entry{
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
				err := SetForTesting(ctx, nil)
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
				rulesToCreate := []*Entry{
					NewRule(0).
						WithPredicateLastUpdateTime(reference.Add(-1 * time.Hour)).
						WithLastUpdateTime(reference.Add(-1 * time.Hour)).
						Build(),
					NewRule(1).WithProject("otherproject").
						WithPredicateLastUpdateTime(reference.Add(time.Hour)).
						WithLastUpdateTime(reference.Add(time.Hour)).
						Build(),
					NewRule(2).WithActive(false).
						WithPredicateLastUpdateTime(reference.Add(-1 * time.Second)).
						WithLastUpdateTime(reference).
						Build(),
					NewRule(3).
						WithPredicateLastUpdateTime(reference.Add(-2 * time.Hour)).
						WithLastUpdateTime(reference.Add(-2 * time.Hour)).
						Build(),
				}
				err := SetForTesting(ctx, rulesToCreate)
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
				err := SetForTesting(ctx, nil)
				So(err, ShouldBeNil)

				result, err := ReadTotalActiveRules(span.Single(ctx))
				So(err, ShouldBeNil)
				So(result, ShouldResemble, map[string]int64{})
			})
			Convey(`Multiple`, func() {
				rulesToCreate := []*Entry{
					// Two active and one inactive rule for Project A.
					NewRule(0).WithProject("project-a").WithActive(true).Build(),
					NewRule(1).WithProject("project-a").WithActive(false).Build(),
					NewRule(2).WithProject("project-a").WithActive(true).Build(),
					// One inactive rule for Project B.
					NewRule(3).WithProject("project-b").WithActive(false).Build(),
					// One active rule for Project C.
					NewRule(4).WithProject("project-c").WithActive(true).Build(),
				}
				err := SetForTesting(ctx, rulesToCreate)
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
			testCreate := func(bc *Entry, user string) (time.Time, error) {
				commitTime, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					ms, err := Create(bc, user)
					if err != nil {
						return err
					}
					span.BufferWrite(ctx, ms)
					return nil
				})
				return commitTime.In(time.UTC), err
			}
			r := NewRule(100).Build()
			r.CreateUser = LUCIAnalysisSystem
			r.LastAuditableUpdateUser = LUCIAnalysisSystem

			Convey(`Valid`, func() {
				testExists := func(expectedRule Entry) {
					txn, cancel := span.ReadOnlyTransaction(ctx)
					defer cancel()
					rules, err := ReadActive(txn, testProject)

					So(err, ShouldBeNil)
					So(len(rules), ShouldEqual, 1)

					readRule := rules[0]
					So(*readRule, ShouldResembleProto, expectedRule)
				}

				Convey(`With Source Cluster`, func() {
					So(r.SourceCluster.Algorithm, ShouldNotBeEmpty)
					So(r.SourceCluster.ID, ShouldNotBeNil)
					commitTime, err := testCreate(r, LUCIAnalysisSystem)
					So(err, ShouldBeNil)

					expectedRule := *r
					expectedRule.LastUpdateTime = commitTime
					expectedRule.LastAuditableUpdateTime = commitTime
					expectedRule.PredicateLastUpdateTime = commitTime
					expectedRule.IsManagingBugPriorityLastUpdateTime = commitTime
					expectedRule.CreateTime = commitTime
					testExists(expectedRule)
				})
				Convey(`Without Source Cluster`, func() {
					// E.g. in case of a manually created rule.
					r.SourceCluster = clustering.ClusterID{}
					r.CreateUser = "user@google.com"
					r.LastAuditableUpdateUser = "user@google.com"
					commitTime, err := testCreate(r, "user@google.com")
					So(err, ShouldBeNil)

					expectedRule := *r
					expectedRule.LastUpdateTime = commitTime
					expectedRule.LastAuditableUpdateTime = commitTime
					expectedRule.PredicateLastUpdateTime = commitTime
					expectedRule.IsManagingBugPriorityLastUpdateTime = commitTime
					expectedRule.CreateTime = commitTime
					testExists(expectedRule)
				})
				Convey(`With Buganizer Bug`, func() {
					r.BugID = bugs.BugID{System: "buganizer", ID: "1234567890"}
					commitTime, err := testCreate(r, LUCIAnalysisSystem)
					So(err, ShouldBeNil)

					expectedRule := *r
					expectedRule.LastUpdateTime = commitTime
					expectedRule.LastAuditableUpdateTime = commitTime
					expectedRule.PredicateLastUpdateTime = commitTime
					expectedRule.IsManagingBugPriorityLastUpdateTime = commitTime
					expectedRule.CreateTime = commitTime
					testExists(expectedRule)
				})
				Convey(`With Monorail Bug`, func() {
					r.BugID = bugs.BugID{System: "monorail", ID: "project/1234567890"}
					commitTime, err := testCreate(r, LUCIAnalysisSystem)
					So(err, ShouldBeNil)

					expectedRule := *r
					expectedRule.LastUpdateTime = commitTime
					expectedRule.LastAuditableUpdateTime = commitTime
					expectedRule.PredicateLastUpdateTime = commitTime
					expectedRule.IsManagingBugPriorityLastUpdateTime = commitTime
					expectedRule.CreateTime = commitTime
					testExists(expectedRule)
				})
			})
			Convey(`With invalid Project`, func() {
				Convey(`Unspecified`, func() {
					r.Project = ""
					_, err := testCreate(r, LUCIAnalysisSystem)
					So(err, ShouldErrLike, `project: unspecified`)
				})
				Convey(`Invalid`, func() {
					r.Project = "!"
					_, err := testCreate(r, LUCIAnalysisSystem)
					So(err, ShouldErrLike, `project: must match ^[a-z0-9\-]{1,40}$`)
				})
			})
			Convey(`With invalid Rule Definition`, func() {
				r.RuleDefinition = "invalid"
				_, err := testCreate(r, LUCIAnalysisSystem)
				So(err, ShouldErrLike, "rule definition: parse: syntax error")
			})
			Convey(`With too long Rule Definition`, func() {
				r.RuleDefinition = strings.Repeat(" ", MaxRuleDefinitionLength+1)
				_, err := testCreate(r, LUCIAnalysisSystem)
				So(err, ShouldErrLike, "rule definition: exceeds maximum length of 65536")
			})
			Convey(`With invalid Bug ID`, func() {
				r.BugID.System = ""
				_, err := testCreate(r, LUCIAnalysisSystem)
				So(err, ShouldErrLike, "bug ID: invalid bug tracking system")
			})
			Convey(`With invalid Source Cluster`, func() {
				So(r.SourceCluster.ID, ShouldNotBeNil)
				r.SourceCluster.Algorithm = ""
				_, err := testCreate(r, LUCIAnalysisSystem)
				So(err, ShouldErrLike, "source cluster ID: algorithm not valid")
			})
			Convey(`With invalid User`, func() {
				_, err := testCreate(r, "")
				So(err, ShouldErrLike, "user must be valid")
			})
		})
		Convey(`Update`, func() {
			testExists := func(expectedRule *Entry) {
				txn, cancel := span.ReadOnlyTransaction(ctx)
				defer cancel()
				rule, err := Read(txn, expectedRule.Project, expectedRule.RuleID)
				So(err, ShouldBeNil)
				So(rule, ShouldResembleProto, expectedRule)
			}
			testUpdate := func(bc *Entry, options UpdateOptions, user string) (time.Time, error) {
				commitTime, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					ms, err := Update(bc, options, user)
					if err != nil {
						return err
					}
					span.BufferWrite(ctx, ms)
					return nil
				})
				return commitTime.In(time.UTC), err
			}
			r := NewRule(100).Build()
			err := SetForTesting(ctx, []*Entry{r})
			So(err, ShouldBeNil)

			Convey(`Valid`, func() {
				Convey(`Update predicate`, func() {
					r.RuleDefinition = `test = "UpdateTest"`
					r.BugID = bugs.BugID{System: "monorail", ID: "chromium/651234"}
					r.IsActive = false
					commitTime, err := testUpdate(r, UpdateOptions{
						IsAuditableUpdate: true,
						PredicateUpdated:  true,
					}, "testuser@google.com")
					So(err, ShouldBeNil)

					expectedRule := *r
					expectedRule.PredicateLastUpdateTime = commitTime
					expectedRule.LastAuditableUpdateTime = commitTime
					expectedRule.LastAuditableUpdateUser = "testuser@google.com"
					expectedRule.LastUpdateTime = commitTime
					testExists(&expectedRule)
				})
				Convey(`Update IsManagingBugPriority`, func() {
					r.IsManagingBugPriority = false
					commitTime, err := testUpdate(r, UpdateOptions{
						IsAuditableUpdate:            true,
						IsManagingBugPriorityUpdated: true,
					}, "testuser@google.com")
					So(err, ShouldBeNil)

					expectedRule := *r
					expectedRule.IsManagingBugPriorityLastUpdateTime = commitTime
					expectedRule.LastAuditableUpdateTime = commitTime
					expectedRule.LastAuditableUpdateUser = "testuser@google.com"
					expectedRule.LastUpdateTime = commitTime
					testExists(&expectedRule)
				})
				Convey(`Standard auditable update`, func() {
					r.BugID = bugs.BugID{System: "monorail", ID: "chromium/651234"}
					commitTime, err := testUpdate(r, UpdateOptions{
						IsAuditableUpdate: true,
					}, "testuser@google.com")
					So(err, ShouldBeNil)

					expectedRule := *r
					expectedRule.LastAuditableUpdateTime = commitTime
					expectedRule.LastAuditableUpdateUser = "testuser@google.com"
					expectedRule.LastUpdateTime = commitTime
					testExists(&expectedRule)
				})
				Convey(`SourceCluster is immutable`, func() {
					originalSourceCluster := r.SourceCluster
					r.SourceCluster = clustering.ClusterID{Algorithm: "testname-v1", ID: "00112233445566778899aabbccddeeff"}
					commitTime, err := testUpdate(r, UpdateOptions{
						IsAuditableUpdate: true,
					}, "testuser@google.com")
					So(err, ShouldBeNil)

					expectedRule := *r
					expectedRule.SourceCluster = originalSourceCluster
					expectedRule.LastAuditableUpdateTime = commitTime
					expectedRule.LastAuditableUpdateUser = "testuser@google.com"
					expectedRule.LastUpdateTime = commitTime
					testExists(&expectedRule)
				})
				Convey(`Non-auditable update`, func() {
					r.BugManagementState = &bugspb.BugManagementState{
						RuleAssociationNotified: true,
						PolicyState: map[string]*bugspb.BugManagementState_PolicyState{
							"policy-a": {
								IsActive:           true,
								LastActivationTime: timestamppb.New(time.Date(2050, 1, 2, 3, 4, 5, 6, time.UTC)),
							},
							"policy-b": {},
						},
					}

					commitTime, err := testUpdate(r, UpdateOptions{}, "system")
					So(err, ShouldBeNil)

					expectedRule := *r
					expectedRule.LastUpdateTime = commitTime
					testExists(&expectedRule)
				})
			})
			Convey(`Invalid`, func() {
				Convey(`With invalid User`, func() {
					_, err := testUpdate(r, UpdateOptions{
						IsAuditableUpdate: true,
					}, "")
					So(err, ShouldErrLike, "user must be valid")
				})
				Convey(`With invalid Rule Definition`, func() {
					r.RuleDefinition = "invalid"
					_, err := testUpdate(r, UpdateOptions{
						IsAuditableUpdate: true,
						PredicateUpdated:  true,
					}, LUCIAnalysisSystem)
					So(err, ShouldErrLike, "rule definition: parse: syntax error")
				})
				Convey(`With invalid Bug ID`, func() {
					r.BugID.System = ""
					_, err := testUpdate(r, UpdateOptions{
						IsAuditableUpdate: true,
					}, LUCIAnalysisSystem)
					So(err, ShouldErrLike, "bug ID: invalid bug tracking system")
				})
			})
		})
		Convey(`One rule managing bug constraint is correctly enforced`, func() {
			testCreate := func(r *Entry, user string) error {
				_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					ms, err := Create(r, user)
					if err != nil {
						return err
					}
					span.BufferWrite(ctx, ms)
					return nil
				})
				return err
			}
			testUpdate := func(r *Entry, options UpdateOptions, user string) error {
				_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					ms, err := Update(r, options, user)
					if err != nil {
						return err
					}
					span.BufferWrite(ctx, ms)
					return nil
				})
				return err
			}

			bug := bugs.BugID{System: "monorail", ID: "project/1234567890"}
			rulesToCreate := []*Entry{
				NewRule(0).WithProject("project-a").WithBug(bug).WithBugManaged(true).Build(),
				NewRule(1).WithProject("project-b").WithBug(bug).WithBugManaged(false).Build(),
				NewRule(2).WithProject("project-c").WithBug(bug).WithBugManaged(false).Build(),
			}
			for _, r := range rulesToCreate {
				err := testCreate(r, LUCIAnalysisSystem)
				So(err, ShouldBeNil)
			}
			Convey("Cannot create a second rule managing a bug", func() {
				// Cannot create a second rule managing the same bug.
				secondManagingRule := NewRule(3).WithProject("project-d").WithBug(bug).WithBugManaged(true).Build()
				err := testCreate(secondManagingRule, LUCIAnalysisSystem)
				So(err, ShouldBeRPCAlreadyExists)
			})
			Convey("Cannot update a rule to manage the same bug", func() {
				ruleToUpdate := rulesToCreate[2]
				ruleToUpdate.IsManagingBug = true
				err := testUpdate(ruleToUpdate, UpdateOptions{IsAuditableUpdate: true}, LUCIAnalysisSystem)
				So(err, ShouldBeRPCAlreadyExists)
			})
			Convey("Can swap which rule is managing a bug", func() {
				// Stop the first rule from managing the bug.
				ruleToUpdate := rulesToCreate[0]
				ruleToUpdate.IsManagingBug = false
				err := testUpdate(ruleToUpdate, UpdateOptions{IsAuditableUpdate: true}, LUCIAnalysisSystem)
				So(err, ShouldBeNil)

				ruleToUpdate = rulesToCreate[2]
				ruleToUpdate.IsManagingBug = true
				err = testUpdate(ruleToUpdate, UpdateOptions{IsAuditableUpdate: true}, LUCIAnalysisSystem)
				So(err, ShouldBeNil)
			})
		})
	})
}
