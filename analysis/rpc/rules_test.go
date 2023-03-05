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

package rpc

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"
	"go.chromium.org/luci/server/span"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/analysis/internal/bugs"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/testname"
	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/perms"
	"go.chromium.org/luci/analysis/internal/testutil"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestRules(t *testing.T) {
	Convey("With Server", t, func() {
		ctx := testutil.IntegrationTestContext(t)

		// For user identification.
		ctx = authtest.MockAuthConfig(ctx)
		authState := &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"luci-analysis-access"},
		}
		ctx = auth.WithState(ctx, authState)
		ctx = secrets.Use(ctx, &testsecrets.Store{})

		// Provides datastore implementation needed for project config.
		ctx = memory.Use(ctx)

		srv := NewRulesSever()

		ruleManagedBuilder := rules.NewRule(0).
			WithProject(testProject).
			WithBug(bugs.BugID{System: "monorail", ID: "monorailproject/111"})
		ruleManaged := ruleManagedBuilder.Build()
		ruleTwoProject := rules.NewRule(1).
			WithProject(testProject).
			WithBug(bugs.BugID{System: "monorail", ID: "monorailproject/222"}).
			WithBugManaged(false).
			Build()
		ruleTwoProjectOther := rules.NewRule(2).
			WithProject("otherproject").
			WithBug(bugs.BugID{System: "monorail", ID: "monorailproject/222"}).
			Build()
		ruleUnmanagedOther := rules.NewRule(3).
			WithProject("otherproject").
			WithBug(bugs.BugID{System: "monorail", ID: "monorailproject/444"}).
			WithBugManaged(false).
			Build()
		ruleManagedOther := rules.NewRule(4).
			WithProject("otherproject").
			WithBug(bugs.BugID{System: "monorail", ID: "monorailproject/555"}).
			WithBugManaged(true).
			Build()
		ruleBuganizer := rules.NewRule(5).
			WithProject(testProject).
			WithBug(bugs.BugID{System: "buganizer", ID: "666"}).
			Build()

		err := rules.SetRulesForTesting(ctx, []*rules.FailureAssociationRule{
			ruleManaged,
			ruleTwoProject,
			ruleTwoProjectOther,
			ruleUnmanagedOther,
			ruleManagedOther,
			ruleBuganizer,
		})
		So(err, ShouldBeNil)

		cfg := &configpb.ProjectConfig{
			Monorail: &configpb.MonorailProject{
				Project:          "monorailproject",
				DisplayPrefix:    "mybug.com",
				MonorailHostname: "monorailhost.com",
			},
		}
		err = config.SetTestProjectConfig(ctx, map[string]*configpb.ProjectConfig{
			"testproject": cfg,
		})
		So(err, ShouldBeNil)

		Convey("Unauthorised requests are rejected", func() {
			// Ensure no access to luci-analysis-access.
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				// Not a member of luci-analysis-access.
				IdentityGroups: []string{"other-group"},
			})

			// Make some request (the request should not matter, as
			// a common decorator is used for all requests.)
			request := &pb.GetRuleRequest{
				Name: fmt.Sprintf("projects/%s/rules/%s", ruleManaged.Project, ruleManaged.RuleID),
			}

			rule, err := srv.Get(ctx, request)
			So(err, ShouldBeRPCPermissionDenied, "not a member of luci-analysis-access")
			So(rule, ShouldBeNil)
		})
		Convey("Get", func() {
			authState.IdentityPermissions = []authtest.RealmPermission{
				{
					Realm:      "testproject:@root",
					Permission: perms.PermGetRule,
				},
				{
					Realm:      "testproject:@root",
					Permission: perms.PermGetRuleDefinition,
				},
			}

			Convey("No get rule permission", func() {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetRule)

				request := &pb.GetRuleRequest{
					Name: fmt.Sprintf("projects/%s/rules/%s", ruleManaged.Project, ruleManaged.RuleID),
				}

				rule, err := srv.Get(ctx, request)
				So(err, ShouldBeRPCPermissionDenied, "caller does not have permission analysis.rules.get")
				So(rule, ShouldBeNil)
			})
			Convey("Rule exists", func() {
				Convey("Read rule with Monorail bug", func() {
					expectedRule := &pb.Rule{
						Name:           fmt.Sprintf("projects/%s/rules/%s", ruleManaged.Project, ruleManaged.RuleID),
						Project:        ruleManaged.Project,
						RuleId:         ruleManaged.RuleID,
						RuleDefinition: ruleManaged.RuleDefinition,
						Bug: &pb.AssociatedBug{
							System:   "monorail",
							Id:       "monorailproject/111",
							LinkText: "mybug.com/111",
							Url:      "https://monorailhost.com/p/monorailproject/issues/detail?id=111",
						},
						IsActive:                         true,
						IsManagingBug:                    true,
						IsManagingBugPriority:            true,
						IsManagingBugPriorityLastUpdated: timestamppb.New(ruleManaged.IsManagingBugPriorityLastUpdated),
						SourceCluster: &pb.ClusterId{
							Algorithm: ruleManaged.SourceCluster.Algorithm,
							Id:        ruleManaged.SourceCluster.ID,
						},
						CreateTime:              timestamppb.New(ruleManaged.CreationTime),
						CreateUser:              ruleManaged.CreationUser,
						LastUpdateTime:          timestamppb.New(ruleManaged.LastUpdated),
						LastUpdateUser:          ruleManaged.LastUpdatedUser,
						PredicateLastUpdateTime: timestamppb.New(ruleManaged.PredicateLastUpdated),
					}

					Convey("With get rule definition permission", func() {
						request := &pb.GetRuleRequest{
							Name: fmt.Sprintf("projects/%s/rules/%s", ruleManaged.Project, ruleManaged.RuleID),
						}

						rule, err := srv.Get(ctx, request)
						So(err, ShouldBeNil)
						includeDefinition := true
						So(rule, ShouldResembleProto, createRulePB(ruleManaged, cfg, includeDefinition))

						// Also verify createRulePB works as expected, so we do not need
						// to test that again in later tests.
						expectedRule.Etag = ruleETag(ruleManaged, includeDefinition)
						So(rule, ShouldResembleProto, expectedRule)
					})
					Convey("Without get rule definition permission", func() {
						authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetRuleDefinition)

						request := &pb.GetRuleRequest{
							Name: fmt.Sprintf("projects/%s/rules/%s", ruleManaged.Project, ruleManaged.RuleID),
						}

						rule, err := srv.Get(ctx, request)
						So(err, ShouldBeNil)
						includeDefinition := false
						So(rule, ShouldResembleProto, createRulePB(ruleManaged, cfg, includeDefinition))

						// Also verify createRulePB works as expected, so we do not need
						// to test that again in later tests.
						expectedRule.RuleDefinition = ""
						expectedRule.Etag = ruleETag(ruleManaged, includeDefinition)
						So(rule, ShouldResembleProto, expectedRule)
					})
				})
				Convey("Read rule with Buganizer bug", func() {
					request := &pb.GetRuleRequest{
						Name: fmt.Sprintf("projects/%s/rules/%s", ruleBuganizer.Project, ruleBuganizer.RuleID),
					}

					rule, err := srv.Get(ctx, request)
					So(err, ShouldBeNil)
					includeDefinition := true
					So(rule, ShouldResembleProto, createRulePB(ruleBuganizer, cfg, includeDefinition))

					// Also verify createRulePB works as expected, so we do not need
					// to test that again in later tests.
					So(rule, ShouldResembleProto, &pb.Rule{
						Name:           fmt.Sprintf("projects/%s/rules/%s", ruleBuganizer.Project, ruleBuganizer.RuleID),
						Project:        ruleBuganizer.Project,
						RuleId:         ruleBuganizer.RuleID,
						RuleDefinition: ruleBuganizer.RuleDefinition,
						Bug: &pb.AssociatedBug{
							System:   "buganizer",
							Id:       "666",
							LinkText: "b/666",
							Url:      "https://issuetracker.google.com/issues/666",
						},
						IsActive:                         true,
						IsManagingBug:                    true,
						IsManagingBugPriority:            true,
						IsManagingBugPriorityLastUpdated: timestamppb.New(ruleBuganizer.IsManagingBugPriorityLastUpdated),
						SourceCluster: &pb.ClusterId{
							Algorithm: ruleBuganizer.SourceCluster.Algorithm,
							Id:        ruleBuganizer.SourceCluster.ID,
						},
						CreateTime:              timestamppb.New(ruleBuganizer.CreationTime),
						CreateUser:              ruleBuganizer.CreationUser,
						LastUpdateTime:          timestamppb.New(ruleBuganizer.LastUpdated),
						LastUpdateUser:          ruleBuganizer.LastUpdatedUser,
						PredicateLastUpdateTime: timestamppb.New(ruleBuganizer.PredicateLastUpdated),
						Etag:                    ruleETag(ruleBuganizer, includeDefinition),
					})
				})
			})
			Convey("Rule does not exist", func() {
				ruleID := strings.Repeat("00", 16)
				request := &pb.GetRuleRequest{
					Name: fmt.Sprintf("projects/%s/rules/%s", ruleManaged.Project, ruleID),
				}

				rule, err := srv.Get(ctx, request)
				So(rule, ShouldBeNil)
				So(err, ShouldBeRPCNotFound)
			})
		})
		Convey("List", func() {
			authState.IdentityPermissions = []authtest.RealmPermission{
				{
					Realm:      "testproject:@root",
					Permission: perms.PermListRules,
				},
				{
					Realm:      "testproject:@root",
					Permission: perms.PermGetRuleDefinition,
				},
			}

			request := &pb.ListRulesRequest{
				Parent: fmt.Sprintf("projects/%s", testProject),
			}
			Convey("No list rules permission", func() {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermListRules)

				response, err := srv.List(ctx, request)
				So(err, ShouldBeRPCPermissionDenied, "caller does not have permission analysis.rules.list")
				So(response, ShouldBeNil)
			})
			Convey("Non-Empty", func() {
				test := func(includeDefinition bool) {
					rs := []*rules.FailureAssociationRule{
						ruleManaged,
						ruleBuganizer,
						rules.NewRule(2).WithProject(testProject).Build(),
						rules.NewRule(3).WithProject(testProject).Build(),
						rules.NewRule(4).WithProject(testProject).Build(),
						// In other project.
						ruleManagedOther,
					}
					err := rules.SetRulesForTesting(ctx, rs)
					So(err, ShouldBeNil)

					response, err := srv.List(ctx, request)
					So(err, ShouldBeNil)

					expected := &pb.ListRulesResponse{
						Rules: []*pb.Rule{
							createRulePB(rs[0], cfg, includeDefinition),
							createRulePB(rs[1], cfg, includeDefinition),
							createRulePB(rs[2], cfg, includeDefinition),
							createRulePB(rs[3], cfg, includeDefinition),
							createRulePB(rs[4], cfg, includeDefinition),
						},
					}
					sort.Slice(expected.Rules, func(i, j int) bool {
						return expected.Rules[i].RuleId < expected.Rules[j].RuleId
					})
					sort.Slice(response.Rules, func(i, j int) bool {
						return response.Rules[i].RuleId < response.Rules[j].RuleId
					})
					So(response, ShouldResembleProto, expected)
				}
				Convey("With get rule definition permission", func() {
					includeDefinition := true
					test(includeDefinition)
				})
				Convey("Without get rule definition permission", func() {
					authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetRuleDefinition)

					includeDefinition := false
					test(includeDefinition)
				})
			})
			Convey("Empty", func() {
				err := rules.SetRulesForTesting(ctx, nil)
				So(err, ShouldBeNil)

				response, err := srv.List(ctx, request)
				So(err, ShouldBeNil)

				expected := &pb.ListRulesResponse{}
				So(response, ShouldResembleProto, expected)
			})
			Convey("With project not configured", func() {
				err = config.SetTestProjectConfig(ctx, map[string]*configpb.ProjectConfig{})
				So(err, ShouldBeNil)

				// Run
				response, err := srv.List(ctx, request)

				// Verify
				So(err, ShouldBeRPCFailedPrecondition, "project does not exist in LUCI Analysis")
				So(response, ShouldBeNil)
			})
		})
		Convey("Update", func() {
			authState.IdentityPermissions = []authtest.RealmPermission{
				{
					Realm:      "testproject:@root",
					Permission: perms.PermUpdateRule,
				},
				{
					Realm:      "testproject:@root",
					Permission: perms.PermGetRuleDefinition,
				},
			}
			request := &pb.UpdateRuleRequest{
				Rule: &pb.Rule{
					Name:           fmt.Sprintf("projects/%s/rules/%s", ruleManaged.Project, ruleManaged.RuleID),
					RuleDefinition: `test = "updated"`,
					Bug: &pb.AssociatedBug{
						System: "monorail",
						Id:     "monorailproject/2",
					},
					IsManagingBug: false,
					IsActive:      false,
				},
				UpdateMask: &fieldmaskpb.FieldMask{
					// On the client side, we use JSON equivalents, i.e. ruleDefinition,
					// bug, isActive, isManagingBug.
					Paths: []string{"rule_definition", "bug", "is_active", "is_managing_bug", "is_managing_bug_priority"},
				},
				Etag: ruleETag(ruleManaged, true /*includeDefinition*/),
			}

			Convey("No update rules permission", func() {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermUpdateRule)

				rule, err := srv.Update(ctx, request)
				So(err, ShouldBeRPCPermissionDenied, "caller does not have permission analysis.rules.update")
				So(rule, ShouldBeNil)
			})
			Convey("No rule definition permission", func() {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetRuleDefinition)

				rule, err := srv.Update(ctx, request)
				So(err, ShouldBeRPCPermissionDenied, "caller does not have permission analysis.rules.getDefinition")
				So(rule, ShouldBeNil)
			})
			Convey("Success", func() {
				Convey("Predicate updated", func() {
					rule, err := srv.Update(ctx, request)
					So(err, ShouldBeNil)

					storedRule, err := rules.Read(span.Single(ctx), testProject, ruleManaged.RuleID)
					So(err, ShouldBeNil)

					So(storedRule.LastUpdated, ShouldNotEqual, ruleManaged.LastUpdated)

					expectedRule := ruleManagedBuilder.
						WithRuleDefinition(`test = "updated"`).
						WithBug(bugs.BugID{System: "monorail", ID: "monorailproject/2"}).
						WithActive(false).
						WithBugManaged(false).
						WithBugPriorityManaged(false).
						WithBugPriorityManagedLastUpdated(storedRule.LastUpdated).
						// Accept whatever the new last updated time is.
						WithLastUpdated(storedRule.LastUpdated).
						WithLastUpdatedUser("someone@example.com").
						// The predicate last updated time should be the same as
						// the last updated time.
						WithPredicateLastUpdated(storedRule.LastUpdated).
						Build()

					// Verify the rule was correctly updated in the database.
					So(storedRule, ShouldResemble, expectedRule)

					// Verify the returned rule matches what was expected.
					So(rule, ShouldResembleProto, createRulePB(expectedRule, cfg, true /*includeDefinition*/))
				})
				Convey("Predicate not updated", func() {
					request.UpdateMask.Paths = []string{"bug"}
					request.Rule.Bug = &pb.AssociatedBug{
						System: "buganizer",
						Id:     "99999999",
					}

					rule, err := srv.Update(ctx, request)
					So(err, ShouldBeNil)

					storedRule, err := rules.Read(span.Single(ctx), testProject, ruleManaged.RuleID)
					So(err, ShouldBeNil)

					// Check the rule was updated, but that predicate last
					// updated time was NOT updated.
					So(storedRule.LastUpdated, ShouldNotEqual, ruleManaged.LastUpdated)

					expectedRule := ruleManagedBuilder.
						WithBug(bugs.BugID{System: "buganizer", ID: "99999999"}).
						// Accept whatever the new last updated time is.
						WithLastUpdated(storedRule.LastUpdated).
						WithLastUpdatedUser("someone@example.com").
						Build()

					// Verify the rule was correctly updated in the database.
					So(storedRule, ShouldResemble, expectedRule)

					// Verify the returned rule matches what was expected.
					So(rule, ShouldResembleProto, createRulePB(expectedRule, cfg, true /*includeDefinition*/))
				})
				Convey("Managing bug priority updated", func() {
					request.UpdateMask.Paths = []string{"is_managing_bug_priority"}
					request.Rule.IsManagingBugPriority = false

					rule, err := srv.Update(ctx, request)
					So(err, ShouldBeNil)

					storedRule, err := rules.Read(span.Single(ctx), testProject, ruleManaged.RuleID)
					So(err, ShouldBeNil)

					// Check the rule was updated.
					So(storedRule.LastUpdated, ShouldNotEqual, ruleManaged.LastUpdated)

					expectedRule := ruleManagedBuilder.
						WithBugPriorityManaged(false).
						WithBugPriorityManagedLastUpdated(storedRule.LastUpdated).
						// Accept whatever the new last updated time is.
						WithLastUpdated(storedRule.LastUpdated).
						WithLastUpdatedUser("someone@example.com").
						Build()

					// Verify the rule was correctly updated in the database.
					So(storedRule, ShouldResemble, expectedRule)

					// Verify the returned rule matches what was expected.
					So(rule, ShouldResembleProto, createRulePB(expectedRule, cfg, true /*includeDefinition*/))
				})
				Convey("Re-use of bug managed by another project", func() {
					request.UpdateMask.Paths = []string{"bug"}
					request.Rule.Bug = &pb.AssociatedBug{
						System: ruleManagedOther.BugID.System,
						Id:     ruleManagedOther.BugID.ID,
					}

					rule, err := srv.Update(ctx, request)
					So(err, ShouldBeNil)

					storedRule, err := rules.Read(span.Single(ctx), testProject, ruleManaged.RuleID)
					So(err, ShouldBeNil)

					// Check the rule was updated.
					So(storedRule.LastUpdated, ShouldNotEqual, ruleManaged.LastUpdated)

					expectedRule := ruleManagedBuilder.
						// Verify the bug was updated, but that IsManagingBug
						// was silently set to false, because ruleManagedOther
						// already controls the bug.
						WithBug(ruleManagedOther.BugID).
						WithBugManaged(false).
						// Accept whatever the new last updated time is.
						WithLastUpdated(storedRule.LastUpdated).
						WithLastUpdatedUser("someone@example.com").
						Build()

					// Verify the rule was correctly updated in the database.
					So(storedRule, ShouldResemble, expectedRule)

					// Verify the returned rule matches what was expected.
					So(rule, ShouldResembleProto, createRulePB(expectedRule, cfg, true /*includeDefinition*/))
				})
			})
			Convey("Concurrent Modification", func() {
				_, err := srv.Update(ctx, request)
				So(err, ShouldBeNil)

				// Attempt the same modification again without
				// requerying.
				rule, err := srv.Update(ctx, request)
				So(rule, ShouldBeNil)
				So(err, ShouldBeRPCAborted)
			})
			Convey("Rule does not exist", func() {
				ruleID := strings.Repeat("00", 16)
				request.Rule.Name = fmt.Sprintf("projects/%s/rules/%s", testProject, ruleID)

				rule, err := srv.Update(ctx, request)
				So(rule, ShouldBeNil)
				So(err, ShouldBeRPCNotFound)
			})
			Convey("Validation error", func() {
				Convey("Invalid bug monorail project", func() {
					request.Rule.Bug.Id = "otherproject/2"

					rule, err := srv.Update(ctx, request)
					So(rule, ShouldBeNil)
					So(err, ShouldBeRPCInvalidArgument, "bug not in expected monorail project (monorailproject)")
				})
				Convey("Re-use of same bug in same project", func() {
					// Use the same bug as another rule.
					request.Rule.Bug = &pb.AssociatedBug{
						System: ruleTwoProject.BugID.System,
						Id:     ruleTwoProject.BugID.ID,
					}

					rule, err := srv.Update(ctx, request)
					So(rule, ShouldBeNil)
					So(err, ShouldBeRPCInvalidArgument,
						fmt.Sprintf("bug already used by a rule in the same project (%s/%s)",
							ruleTwoProject.Project, ruleTwoProject.RuleID))
				})
				Convey("Bug managed by another rule", func() {
					// Select a bug already managed by another rule.
					request.Rule.Bug = &pb.AssociatedBug{
						System: ruleManagedOther.BugID.System,
						Id:     ruleManagedOther.BugID.ID,
					}
					// Request we manage this bug.
					request.Rule.IsManagingBug = true
					request.Rule.IsManagingBugPriority = true
					request.UpdateMask.Paths = []string{"bug", "is_managing_bug"}

					rule, err := srv.Update(ctx, request)
					So(rule, ShouldBeNil)
					So(err, ShouldBeRPCInvalidArgument,
						fmt.Sprintf("bug already managed by a rule in another project (%s/%s)",
							ruleManagedOther.Project, ruleManagedOther.RuleID))
				})
				Convey("Invalid rule definition", func() {
					// Use an invalid failure association rule.
					request.Rule.RuleDefinition = ""

					rule, err := srv.Update(ctx, request)
					So(rule, ShouldBeNil)
					So(err, ShouldBeRPCInvalidArgument, "rule definition is not valid")
				})
			})
		})
		Convey("Create", func() {
			authState.IdentityPermissions = []authtest.RealmPermission{
				{
					Realm:      "testproject:@root",
					Permission: perms.PermCreateRule,
				},
			}
			request := &pb.CreateRuleRequest{
				Parent: fmt.Sprintf("projects/%s", testProject),
				Rule: &pb.Rule{
					RuleDefinition: `test = "create"`,
					Bug: &pb.AssociatedBug{
						System: "monorail",
						Id:     "monorailproject/2",
					},
					IsActive:              false,
					IsManagingBug:         true,
					IsManagingBugPriority: true,
					SourceCluster: &pb.ClusterId{
						Algorithm: testname.AlgorithmName,
						Id:        strings.Repeat("aa", 16),
					},
				},
			}

			Convey("No create rule permission", func() {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermCreateRule)

				rule, err := srv.Create(ctx, request)
				So(err, ShouldBeRPCPermissionDenied, "caller does not have permission analysis.rules.create")
				So(rule, ShouldBeNil)
			})
			Convey("Success", func() {
				expectedRuleBuilder := rules.NewRule(0).
					WithProject(testProject).
					WithRuleDefinition(`test = "create"`).
					WithActive(false).
					WithBugManaged(true).
					WithBugPriorityManaged(true).
					WithCreationUser("someone@example.com").
					WithLastUpdatedUser("someone@example.com").
					WithSourceCluster(clustering.ClusterID{
						Algorithm: testname.AlgorithmName,
						ID:        strings.Repeat("aa", 16),
					})

				Convey("Bug not managed by another rule", func() {
					// Re-use the same bug as a rule in another project,
					// where the other rule is not managing the bug.
					request.Rule.Bug = &pb.AssociatedBug{
						System: ruleUnmanagedOther.BugID.System,
						Id:     ruleUnmanagedOther.BugID.ID,
					}

					rule, err := srv.Create(ctx, request)
					So(err, ShouldBeNil)

					storedRule, err := rules.Read(span.Single(ctx), testProject, rule.RuleId)
					So(err, ShouldBeNil)

					expectedRule := expectedRuleBuilder.
						// Accept the randomly generated rule ID.
						WithRuleID(rule.RuleId).
						WithBug(ruleUnmanagedOther.BugID).
						// Accept whatever CreationTime was assigned, as it
						// is determined by Spanner commit time.
						// Rule spanner data access code tests already validate
						// this is populated correctly.
						WithCreationTime(storedRule.CreationTime).
						WithLastUpdated(storedRule.CreationTime).
						WithPredicateLastUpdated(storedRule.CreationTime).
						WithBugPriorityManagedLastUpdated(storedRule.CreationTime).
						Build()

					// Verify the rule was correctly created in the database.
					So(storedRule, ShouldResemble, expectedRuleBuilder.Build())

					// Verify the returned rule matches our expectations.
					So(rule, ShouldResembleProto, createRulePB(expectedRule, cfg, true /*includeDefinition*/))
				})
				Convey("Bug managed by another rule", func() {
					// Re-use the same bug as a rule in another project,
					// where that rule is managing the bug.
					request.Rule.Bug = &pb.AssociatedBug{
						System: ruleManagedOther.BugID.System,
						Id:     ruleManagedOther.BugID.ID,
					}

					rule, err := srv.Create(ctx, request)
					So(err, ShouldBeNil)

					storedRule, err := rules.Read(span.Single(ctx), testProject, rule.RuleId)
					So(err, ShouldBeNil)

					expectedRule := expectedRuleBuilder.
						// Accept the randomly generated rule ID.
						WithRuleID(rule.RuleId).
						WithBug(ruleManagedOther.BugID).
						// Because another rule is managing the bug, this rule
						// should be silenlty stopped from managing the bug.
						WithBugManaged(false).
						// Accept whatever CreationTime was assigned.
						WithCreationTime(storedRule.CreationTime).
						WithLastUpdated(storedRule.CreationTime).
						WithPredicateLastUpdated(storedRule.CreationTime).
						WithBugPriorityManagedLastUpdated(storedRule.CreationTime).
						Build()

					// Verify the rule was correctly created in the database.
					So(storedRule, ShouldResemble, expectedRuleBuilder.Build())

					// Verify the returned rule matches our expectations.
					So(rule, ShouldResembleProto, createRulePB(expectedRule, cfg, true /*includeDefinition*/))
				})
				Convey("Buganizer", func() {
					request.Rule.Bug = &pb.AssociatedBug{
						System: "buganizer",
						Id:     "1111111111",
					}

					rule, err := srv.Create(ctx, request)
					So(err, ShouldBeNil)

					storedRule, err := rules.Read(span.Single(ctx), testProject, rule.RuleId)
					So(err, ShouldBeNil)

					expectedRule := expectedRuleBuilder.
						// Accept the randomly generated rule ID.
						WithRuleID(rule.RuleId).
						WithBug(bugs.BugID{System: "buganizer", ID: "1111111111"}).
						// Accept whatever CreationTime was assigned, as it
						// is determined by Spanner commit time.
						// Rule spanner data access code tests already validate
						// this is populated correctly.
						WithCreationTime(storedRule.CreationTime).
						WithLastUpdated(storedRule.CreationTime).
						WithPredicateLastUpdated(storedRule.CreationTime).
						WithBugPriorityManagedLastUpdated(storedRule.CreationTime).
						Build()

					// Verify the rule was correctly created in the database.
					So(storedRule, ShouldResemble, expectedRuleBuilder.Build())

					// Verify the returned rule matches our expectations.
					So(rule, ShouldResembleProto, createRulePB(expectedRule, cfg, true /*includeDefinition*/))
				})
			})
			Convey("Validation error", func() {
				Convey("Invalid bug monorail project", func() {
					request.Rule.Bug.Id = "otherproject/2"

					rule, err := srv.Create(ctx, request)
					So(rule, ShouldBeNil)
					So(err, ShouldBeRPCInvalidArgument,
						"bug not in expected monorail project (monorailproject)")
				})
				Convey("Re-use of same bug in same project", func() {
					// Use the same bug as another rule, in the same project.
					request.Rule.Bug = &pb.AssociatedBug{
						System: ruleTwoProject.BugID.System,
						Id:     ruleTwoProject.BugID.ID,
					}

					rule, err := srv.Create(ctx, request)
					So(rule, ShouldBeNil)
					So(err, ShouldBeRPCInvalidArgument,
						fmt.Sprintf("bug already used by a rule in the same project (%s/%s)",
							ruleTwoProject.Project, ruleTwoProject.RuleID))
				})
				Convey("Invalid rule definition", func() {
					// Use an invalid failure association rule.
					request.Rule.RuleDefinition = ""

					rule, err := srv.Create(ctx, request)
					So(rule, ShouldBeNil)
					So(err, ShouldBeRPCInvalidArgument, "rule definition is not valid")
				})
			})
		})
		Convey("LookupBug", func() {
			authState.IdentityPermissions = []authtest.RealmPermission{
				{
					Realm:      "testproject:@root",
					Permission: perms.PermListRules,
				},
				{
					Realm:      "otherproject:@root",
					Permission: perms.PermListRules,
				},
			}

			Convey("Exists None", func() {
				request := &pb.LookupBugRequest{
					System: "monorail",
					Id:     "notexists/1",
				}

				response, err := srv.LookupBug(ctx, request)
				So(err, ShouldBeNil)
				So(response, ShouldResembleProto, &pb.LookupBugResponse{
					Rules: []string{},
				})
			})
			Convey("Exists One", func() {
				request := &pb.LookupBugRequest{
					System: ruleManaged.BugID.System,
					Id:     ruleManaged.BugID.ID,
				}

				response, err := srv.LookupBug(ctx, request)
				So(err, ShouldBeNil)
				So(response, ShouldResembleProto, &pb.LookupBugResponse{
					Rules: []string{
						fmt.Sprintf("projects/%s/rules/%s",
							ruleManaged.Project, ruleManaged.RuleID),
					},
				})

				Convey("If no permission in relevant project", func() {
					authState.IdentityPermissions = nil

					response, err := srv.LookupBug(ctx, request)
					So(err, ShouldBeNil)
					So(response, ShouldResembleProto, &pb.LookupBugResponse{
						Rules: []string{},
					})
				})
			})
			Convey("Exists Many", func() {
				request := &pb.LookupBugRequest{
					System: ruleTwoProject.BugID.System,
					Id:     ruleTwoProject.BugID.ID,
				}

				response, err := srv.LookupBug(ctx, request)
				So(err, ShouldBeNil)
				So(response, ShouldResembleProto, &pb.LookupBugResponse{
					Rules: []string{
						// Rules are returned alphabetically by project.
						fmt.Sprintf("projects/otherproject/rules/%s", ruleTwoProjectOther.RuleID),
						fmt.Sprintf("projects/testproject/rules/%s", ruleTwoProject.RuleID),
					},
				})

				Convey("If list permission exists in only some projects", func() {
					authState.IdentityPermissions = []authtest.RealmPermission{
						{
							Realm:      "testproject:@root",
							Permission: perms.PermListRules,
						},
					}

					response, err := srv.LookupBug(ctx, request)
					So(err, ShouldBeNil)
					So(response, ShouldResembleProto, &pb.LookupBugResponse{
						Rules: []string{
							fmt.Sprintf("projects/testproject/rules/%s", ruleTwoProject.RuleID),
						},
					})
				})
			})
		})
	})
	Convey("formatRule", t, func() {
		rule := rules.NewRule(0).
			WithProject(testProject).
			WithBug(bugs.BugID{System: "monorail", ID: "monorailproject/123456"}).
			WithRuleDefinition(`test = "create"`).
			WithActive(false).
			WithBugManaged(true).
			WithSourceCluster(clustering.ClusterID{
				Algorithm: testname.AlgorithmName,
				ID:        strings.Repeat("aa", 16),
			}).Build()
		expectedRule := `{
	RuleDefinition: "test = \"create\"",
	BugID: "monorail:monorailproject/123456",
	IsActive: false,
	IsManagingBug: true,
	IsManagingBugPriority: true,
	SourceCluster: "testname-v4:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	LastUpdated: "1900-01-02T03:04:07Z"
}`
		So(formatRule(rule), ShouldEqual, expectedRule)
	})
}
