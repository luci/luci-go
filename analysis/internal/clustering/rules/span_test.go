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
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/bugs"
	bugspb "go.chromium.org/luci/analysis/internal/bugs/proto"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/testutil"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSpan(t *testing.T) {
	ftt.Run(`With Spanner Test Database`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		t.Run(`Read`, func(t *ftt.Test) {
			t.Run(`Not Exists`, func(t *ftt.Test) {
				ruleID := strings.Repeat("00", 16)
				rule, err := Read(span.Single(ctx), testProject, ruleID)
				assert.Loosely(t, err, should.Equal(NotExistsErr))
				assert.Loosely(t, rule, should.BeNil)
			})
			t.Run(`Exists`, func(t *ftt.Test) {
				expectedRule := NewRule(100).Build()
				err := SetForTesting(ctx, t, []*Entry{expectedRule})
				assert.Loosely(t, err, should.BeNil)

				rule, err := Read(span.Single(ctx), testProject, expectedRule.RuleID)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rule, should.Resemble(expectedRule))
			})
			t.Run(`With null IsManagingBugPriorityLastUpdated`, func(t *ftt.Test) {
				expectedRule := NewRule(100).WithBugPriorityManagedLastUpdateTime(time.Time{}).Build()
				err := SetForTesting(ctx, t, []*Entry{expectedRule})
				assert.Loosely(t, err, should.BeNil)
				_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					stmt := spanner.NewStatement(`UPDATE FailureAssociationRules
												SET IsManagingBugPriorityLastUpdated = NULL
												WHERE TRUE`)
					_, err := span.Update(ctx, stmt)
					return err
				})
				assert.Loosely(t, err, should.BeNil)
				rule, err := Read(span.Single(ctx), testProject, expectedRule.RuleID)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rule.IsManagingBugPriorityLastUpdateTime.IsZero(), should.BeTrue)
				assert.Loosely(t, rule, should.Resemble(expectedRule))
			})
		})
		t.Run(`ReadActive`, func(t *ftt.Test) {
			t.Run(`Empty`, func(t *ftt.Test) {
				err := SetForTesting(ctx, t, nil)
				assert.Loosely(t, err, should.BeNil)

				rules, err := ReadActive(span.Single(ctx), testProject)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rules, should.Resemble([]*Entry{}))
			})
			t.Run(`Multiple`, func(t *ftt.Test) {
				rulesToCreate := []*Entry{
					NewRule(0).Build(),
					NewRule(1).WithProject("otherproject").Build(),
					NewRule(2).WithActive(false).Build(),
					NewRule(3).Build(),
				}
				err := SetForTesting(ctx, t, rulesToCreate)
				assert.Loosely(t, err, should.BeNil)

				rules, err := ReadActive(span.Single(ctx), testProject)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rules, should.Resemble([]*Entry{
					rulesToCreate[3],
					rulesToCreate[0],
				}))
			})
		})
		t.Run(`ReadByBug`, func(t *ftt.Test) {
			bugID := bugs.BugID{System: "monorail", ID: "monorailproject/1"}
			t.Run(`Empty`, func(t *ftt.Test) {
				rules, err := ReadByBug(span.Single(ctx), bugID)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rules, should.BeEmpty)
			})
			t.Run(`Multiple`, func(t *ftt.Test) {
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
				err := SetForTesting(ctx, t, expectedRules)
				assert.Loosely(t, err, should.BeNil)

				rules, err := ReadByBug(span.Single(ctx), bugID)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rules, should.Resemble(expectedRules))
			})
		})
		t.Run(`ReadDelta`, func(t *ftt.Test) {
			t.Run(`Invalid since time`, func(t *ftt.Test) {
				_, err := ReadDelta(span.Single(ctx), testProject, time.Time{})
				assert.Loosely(t, err, should.ErrLike("cannot query rule deltas from before project inception"))
			})
			t.Run(`Empty`, func(t *ftt.Test) {
				err := SetForTesting(ctx, t, nil)
				assert.Loosely(t, err, should.BeNil)

				rules, err := ReadDelta(span.Single(ctx), testProject, StartingEpoch)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rules, should.Resemble([]*Entry{}))
			})
			t.Run(`Multiple`, func(t *ftt.Test) {
				reference := time.Date(2020, 1, 2, 3, 4, 5, 6000, time.UTC)
				rulesToCreate := []*Entry{
					NewRule(0).WithLastUpdateTime(reference).Build(),
					NewRule(1).WithProject("otherproject").WithLastUpdateTime(reference.Add(time.Minute)).Build(),
					NewRule(2).WithActive(false).WithLastUpdateTime(reference.Add(time.Minute)).Build(),
					NewRule(3).WithLastUpdateTime(reference.Add(time.Microsecond)).Build(),
				}
				err := SetForTesting(ctx, t, rulesToCreate)
				assert.Loosely(t, err, should.BeNil)

				rules, err := ReadDelta(span.Single(ctx), testProject, StartingEpoch)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rules, should.Resemble([]*Entry{
					rulesToCreate[3],
					rulesToCreate[0],
					rulesToCreate[2],
				}))

				rules, err = ReadDelta(span.Single(ctx), testProject, reference)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rules, should.Resemble([]*Entry{
					rulesToCreate[3],
					rulesToCreate[2],
				}))

				rules, err = ReadDelta(span.Single(ctx), testProject, reference.Add(time.Minute))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rules, should.Resemble([]*Entry{}))
			})
		})

		t.Run(`ReadDeltaAllProjects`, func(t *ftt.Test) {
			t.Run(`Invalid since time`, func(t *ftt.Test) {
				_, err := ReadDeltaAllProjects(span.Single(ctx), time.Time{})
				assert.Loosely(t, err, should.ErrLike("cannot query rule deltas from before project inception"))
			})
			t.Run(`Empty`, func(t *ftt.Test) {
				err := SetForTesting(ctx, t, nil)
				assert.Loosely(t, err, should.BeNil)

				rules, err := ReadDeltaAllProjects(span.Single(ctx), StartingEpoch)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rules, should.Resemble([]*Entry{}))
			})
			t.Run(`Multiple`, func(t *ftt.Test) {
				reference := time.Date(2020, 1, 2, 3, 4, 5, 6000, time.UTC)
				rulesToCreate := []*Entry{
					NewRule(0).WithLastUpdateTime(reference).Build(),
					NewRule(1).WithProject("otherproject").WithLastUpdateTime(reference.Add(time.Minute)).Build(),
					NewRule(2).WithActive(false).WithLastUpdateTime(reference.Add(time.Minute)).Build(),
					NewRule(3).WithLastUpdateTime(reference.Add(time.Microsecond)).Build(),
				}
				err := SetForTesting(ctx, t, rulesToCreate)
				assert.Loosely(t, err, should.BeNil)

				rules, err := ReadDeltaAllProjects(span.Single(ctx), StartingEpoch)
				assert.Loosely(t, err, should.BeNil)
				expected := []*Entry{
					rulesToCreate[3],
					rulesToCreate[0],
					rulesToCreate[2],
					rulesToCreate[1],
				}
				sortByID(expected)
				sortByID(rules)
				assert.Loosely(t, rules, should.Resemble(expected))

				rules, err = ReadDeltaAllProjects(span.Single(ctx), reference)
				assert.Loosely(t, err, should.BeNil)
				expected = []*Entry{
					rulesToCreate[3],
					rulesToCreate[2],
					rulesToCreate[1],
				}
				sortByID(expected)
				sortByID(rules)
				assert.Loosely(t, rules, should.Resemble(expected))

				rules, err = ReadDeltaAllProjects(span.Single(ctx), reference.Add(time.Minute))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rules, should.Resemble([]*Entry{}))
			})
		})

		t.Run(`ReadMany`, func(t *ftt.Test) {
			rulesToCreate := []*Entry{
				NewRule(0).Build(),
				NewRule(1).WithProject("otherproject").Build(),
				NewRule(2).WithActive(false).Build(),
				NewRule(3).Build(),
			}
			err := SetForTesting(ctx, t, rulesToCreate)
			assert.Loosely(t, err, should.BeNil)

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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rules, should.Resemble([]*Entry{
				rulesToCreate[0],
				nil,
				rulesToCreate[2],
				rulesToCreate[3],
				nil,
				nil,
				rulesToCreate[2],
				nil,
			}))
		})
		t.Run(`ReadVersion`, func(t *ftt.Test) {
			t.Run(`Empty`, func(t *ftt.Test) {
				err := SetForTesting(ctx, t, nil)
				assert.Loosely(t, err, should.BeNil)

				timestamp, err := ReadVersion(span.Single(ctx), testProject)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, timestamp, should.Resemble(StartingVersion))
			})
			t.Run(`Multiple`, func(t *ftt.Test) {
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
				err := SetForTesting(ctx, t, rulesToCreate)
				assert.Loosely(t, err, should.BeNil)

				version, err := ReadVersion(span.Single(ctx), testProject)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, version, should.Resemble(Version{
					Predicates: reference.Add(-1 * time.Second),
					Total:      reference,
				}))
			})
		})
		t.Run(`ReadTotalActiveRules`, func(t *ftt.Test) {
			t.Run(`Empty`, func(t *ftt.Test) {
				err := SetForTesting(ctx, t, nil)
				assert.Loosely(t, err, should.BeNil)

				result, err := ReadTotalActiveRules(span.Single(ctx))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, result, should.Resemble(map[string]int64{}))
			})
			t.Run(`Multiple`, func(t *ftt.Test) {
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
				err := SetForTesting(ctx, t, rulesToCreate)
				assert.Loosely(t, err, should.BeNil)

				result, err := ReadTotalActiveRules(span.Single(ctx))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, result, should.Resemble(map[string]int64{
					"project-a": 2,
					"project-b": 0,
					"project-c": 1,
				}))
			})
		})
		t.Run(`Create`, func(t *ftt.Test) {
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

			t.Run(`Valid`, func(t *ftt.Test) {
				testExists := func(expectedRule Entry) {
					txn, cancel := span.ReadOnlyTransaction(ctx)
					defer cancel()
					rules, err := ReadActive(txn, testProject)

					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(rules), should.Equal(1))

					readRule := rules[0]
					assert.Loosely(t, *readRule, should.Resemble(expectedRule))
				}

				t.Run(`With Source Cluster`, func(t *ftt.Test) {
					assert.Loosely(t, r.SourceCluster.Algorithm, should.NotBeEmpty)
					assert.Loosely(t, r.SourceCluster.ID, should.NotBeZero)
					commitTime, err := testCreate(r, LUCIAnalysisSystem)
					assert.Loosely(t, err, should.BeNil)

					expectedRule := *r
					expectedRule.LastUpdateTime = commitTime
					expectedRule.LastAuditableUpdateTime = commitTime
					expectedRule.PredicateLastUpdateTime = commitTime
					expectedRule.IsManagingBugPriorityLastUpdateTime = commitTime
					expectedRule.CreateTime = commitTime
					testExists(expectedRule)
				})
				t.Run(`Without Source Cluster`, func(t *ftt.Test) {
					// E.g. in case of a manually created rule.
					r.SourceCluster = clustering.ClusterID{}
					r.CreateUser = "user@google.com"
					r.LastAuditableUpdateUser = "user@google.com"
					commitTime, err := testCreate(r, "user@google.com")
					assert.Loosely(t, err, should.BeNil)

					expectedRule := *r
					expectedRule.LastUpdateTime = commitTime
					expectedRule.LastAuditableUpdateTime = commitTime
					expectedRule.PredicateLastUpdateTime = commitTime
					expectedRule.IsManagingBugPriorityLastUpdateTime = commitTime
					expectedRule.CreateTime = commitTime
					testExists(expectedRule)
				})
				t.Run(`With Buganizer Bug`, func(t *ftt.Test) {
					r.BugID = bugs.BugID{System: "buganizer", ID: "1234567890"}
					commitTime, err := testCreate(r, LUCIAnalysisSystem)
					assert.Loosely(t, err, should.BeNil)

					expectedRule := *r
					expectedRule.LastUpdateTime = commitTime
					expectedRule.LastAuditableUpdateTime = commitTime
					expectedRule.PredicateLastUpdateTime = commitTime
					expectedRule.IsManagingBugPriorityLastUpdateTime = commitTime
					expectedRule.CreateTime = commitTime
					testExists(expectedRule)
				})
				t.Run(`With Monorail Bug`, func(t *ftt.Test) {
					r.BugID = bugs.BugID{System: "monorail", ID: "project/1234567890"}
					commitTime, err := testCreate(r, LUCIAnalysisSystem)
					assert.Loosely(t, err, should.BeNil)

					expectedRule := *r
					expectedRule.LastUpdateTime = commitTime
					expectedRule.LastAuditableUpdateTime = commitTime
					expectedRule.PredicateLastUpdateTime = commitTime
					expectedRule.IsManagingBugPriorityLastUpdateTime = commitTime
					expectedRule.CreateTime = commitTime
					testExists(expectedRule)
				})
			})
			t.Run(`With invalid Project`, func(t *ftt.Test) {
				t.Run(`Unspecified`, func(t *ftt.Test) {
					r.Project = ""
					_, err := testCreate(r, LUCIAnalysisSystem)
					assert.Loosely(t, err, should.ErrLike(`project: unspecified`))
				})
				t.Run(`Invalid`, func(t *ftt.Test) {
					r.Project = "!"
					_, err := testCreate(r, LUCIAnalysisSystem)
					assert.Loosely(t, err, should.ErrLike(`project: must match ^[a-z0-9\-]{1,40}$`))
				})
			})
			t.Run(`With invalid Rule Definition`, func(t *ftt.Test) {
				r.RuleDefinition = "invalid"
				_, err := testCreate(r, LUCIAnalysisSystem)
				assert.Loosely(t, err, should.ErrLike("rule definition: parse: syntax error"))
			})
			t.Run(`With too long Rule Definition`, func(t *ftt.Test) {
				r.RuleDefinition = strings.Repeat(" ", MaxRuleDefinitionLength+1)
				_, err := testCreate(r, LUCIAnalysisSystem)
				assert.Loosely(t, err, should.ErrLike("rule definition: exceeds maximum length of 65536"))
			})
			t.Run(`With invalid Bug ID`, func(t *ftt.Test) {
				r.BugID.System = ""
				_, err := testCreate(r, LUCIAnalysisSystem)
				assert.Loosely(t, err, should.ErrLike("bug ID: invalid bug tracking system"))
			})
			t.Run(`With invalid Source Cluster`, func(t *ftt.Test) {
				assert.Loosely(t, r.SourceCluster.ID, should.NotBeZero)
				r.SourceCluster.Algorithm = ""
				_, err := testCreate(r, LUCIAnalysisSystem)
				assert.Loosely(t, err, should.ErrLike("source cluster ID: algorithm not valid"))
			})
			t.Run(`With invalid User`, func(t *ftt.Test) {
				_, err := testCreate(r, "")
				assert.Loosely(t, err, should.ErrLike("user must be valid"))
			})
		})
		t.Run(`Update`, func(t *ftt.Test) {
			testExists := func(expectedRule *Entry) {
				txn, cancel := span.ReadOnlyTransaction(ctx)
				defer cancel()
				rule, err := Read(txn, expectedRule.Project, expectedRule.RuleID)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rule, should.Resemble(expectedRule))
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
			err := SetForTesting(ctx, t, []*Entry{r})
			assert.Loosely(t, err, should.BeNil)

			t.Run(`Valid`, func(t *ftt.Test) {
				t.Run(`Update predicate`, func(t *ftt.Test) {
					r.RuleDefinition = `test = "UpdateTest"`
					r.BugID = bugs.BugID{System: "monorail", ID: "chromium/651234"}
					r.IsActive = false
					commitTime, err := testUpdate(r, UpdateOptions{
						IsAuditableUpdate: true,
						PredicateUpdated:  true,
					}, "testuser@google.com")
					assert.Loosely(t, err, should.BeNil)

					expectedRule := *r
					expectedRule.PredicateLastUpdateTime = commitTime
					expectedRule.LastAuditableUpdateTime = commitTime
					expectedRule.LastAuditableUpdateUser = "testuser@google.com"
					expectedRule.LastUpdateTime = commitTime
					testExists(&expectedRule)
				})
				t.Run(`Update IsManagingBugPriority`, func(t *ftt.Test) {
					r.IsManagingBugPriority = false
					commitTime, err := testUpdate(r, UpdateOptions{
						IsAuditableUpdate:            true,
						IsManagingBugPriorityUpdated: true,
					}, "testuser@google.com")
					assert.Loosely(t, err, should.BeNil)

					expectedRule := *r
					expectedRule.IsManagingBugPriorityLastUpdateTime = commitTime
					expectedRule.LastAuditableUpdateTime = commitTime
					expectedRule.LastAuditableUpdateUser = "testuser@google.com"
					expectedRule.LastUpdateTime = commitTime
					testExists(&expectedRule)
				})
				t.Run(`Standard auditable update`, func(t *ftt.Test) {
					r.BugID = bugs.BugID{System: "monorail", ID: "chromium/651234"}
					commitTime, err := testUpdate(r, UpdateOptions{
						IsAuditableUpdate: true,
					}, "testuser@google.com")
					assert.Loosely(t, err, should.BeNil)

					expectedRule := *r
					expectedRule.LastAuditableUpdateTime = commitTime
					expectedRule.LastAuditableUpdateUser = "testuser@google.com"
					expectedRule.LastUpdateTime = commitTime
					testExists(&expectedRule)
				})
				t.Run(`SourceCluster is immutable`, func(t *ftt.Test) {
					originalSourceCluster := r.SourceCluster
					r.SourceCluster = clustering.ClusterID{Algorithm: "testname-v1", ID: "00112233445566778899aabbccddeeff"}
					commitTime, err := testUpdate(r, UpdateOptions{
						IsAuditableUpdate: true,
					}, "testuser@google.com")
					assert.Loosely(t, err, should.BeNil)

					expectedRule := *r
					expectedRule.SourceCluster = originalSourceCluster
					expectedRule.LastAuditableUpdateTime = commitTime
					expectedRule.LastAuditableUpdateUser = "testuser@google.com"
					expectedRule.LastUpdateTime = commitTime
					testExists(&expectedRule)
				})
				t.Run(`Non-auditable update`, func(t *ftt.Test) {
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
					assert.Loosely(t, err, should.BeNil)

					expectedRule := *r
					expectedRule.LastUpdateTime = commitTime
					testExists(&expectedRule)
				})
			})
			t.Run(`Invalid`, func(t *ftt.Test) {
				t.Run(`With invalid User`, func(t *ftt.Test) {
					_, err := testUpdate(r, UpdateOptions{
						IsAuditableUpdate: true,
					}, "")
					assert.Loosely(t, err, should.ErrLike("user must be valid"))
				})
				t.Run(`With invalid Rule Definition`, func(t *ftt.Test) {
					r.RuleDefinition = "invalid"
					_, err := testUpdate(r, UpdateOptions{
						IsAuditableUpdate: true,
						PredicateUpdated:  true,
					}, LUCIAnalysisSystem)
					assert.Loosely(t, err, should.ErrLike("rule definition: parse: syntax error"))
				})
				t.Run(`With invalid Bug ID`, func(t *ftt.Test) {
					r.BugID.System = ""
					_, err := testUpdate(r, UpdateOptions{
						IsAuditableUpdate: true,
					}, LUCIAnalysisSystem)
					assert.Loosely(t, err, should.ErrLike("bug ID: invalid bug tracking system"))
				})
			})
		})
		t.Run(`One rule managing bug constraint is correctly enforced`, func(t *ftt.Test) {
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
				assert.Loosely(t, err, should.BeNil)
			}
			t.Run("Cannot create a second rule managing a bug", func(t *ftt.Test) {
				// Cannot create a second rule managing the same bug.
				secondManagingRule := NewRule(3).WithProject("project-d").WithBug(bug).WithBugManaged(true).Build()
				err := testCreate(secondManagingRule, LUCIAnalysisSystem)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.AlreadyExists))
			})
			t.Run("Cannot update a rule to manage the same bug", func(t *ftt.Test) {
				ruleToUpdate := rulesToCreate[2]
				ruleToUpdate.IsManagingBug = true
				err := testUpdate(ruleToUpdate, UpdateOptions{IsAuditableUpdate: true}, LUCIAnalysisSystem)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.AlreadyExists))
			})
			t.Run("Can swap which rule is managing a bug", func(t *ftt.Test) {
				// Stop the first rule from managing the bug.
				ruleToUpdate := rulesToCreate[0]
				ruleToUpdate.IsManagingBug = false
				err := testUpdate(ruleToUpdate, UpdateOptions{IsAuditableUpdate: true}, LUCIAnalysisSystem)
				assert.Loosely(t, err, should.BeNil)

				ruleToUpdate = rulesToCreate[2]
				ruleToUpdate.IsManagingBug = true
				err = testUpdate(ruleToUpdate, UpdateOptions{IsAuditableUpdate: true}, LUCIAnalysisSystem)
				assert.Loosely(t, err, should.BeNil)
			})
		})
	})
}
