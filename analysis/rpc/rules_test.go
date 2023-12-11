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
	"time"

	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/bugs"
	bugspb "go.chromium.org/luci/analysis/internal/bugs/proto"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/testname"
	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/perms"
	"go.chromium.org/luci/analysis/internal/testutil"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRules(t *testing.T) {
	Convey("With Server", t, func() {
		ctx := testutil.IntegrationTestContext(t)

		// For user identification.
		ctx = authtest.MockAuthConfig(ctx)
		authState := &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{luciAnalysisAccessGroup, auditUsersAccessGroup},
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

		err := rules.SetForTesting(ctx, []*rules.Entry{
			ruleManaged,
			ruleTwoProject,
			ruleTwoProjectOther,
			ruleUnmanagedOther,
			ruleManagedOther,
			ruleBuganizer,
		})
		So(err, ShouldBeNil)

		cfg := &configpb.ProjectConfig{
			BugManagement: &configpb.BugManagement{
				Monorail: &configpb.MonorailProject{
					Project:          "monorailproject",
					DisplayPrefix:    "mybug.com",
					MonorailHostname: "monorailhost.com",
				},
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
					Realm:      "testproject:@project",
					Permission: perms.PermGetRule,
				},
				{
					Realm:      "testproject:@project",
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
				mask := ruleMask{
					IncludeDefinition: true,
					IncludeAuditUsers: true,
				}
				Convey("Read rule with Monorail bug", func() {
					Convey("Baseline", func() {
						request := &pb.GetRuleRequest{
							Name: fmt.Sprintf("projects/%s/rules/%s", ruleManaged.Project, ruleManaged.RuleID),
						}

						rule, err := srv.Get(ctx, request)
						So(err, ShouldBeNil)
						So(rule, ShouldResembleProto, createRulePB(ruleManaged, cfg, mask))
					})
					Convey("Without get rule definition permission", func() {
						authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetRuleDefinition)

						request := &pb.GetRuleRequest{
							Name: fmt.Sprintf("projects/%s/rules/%s", ruleManaged.Project, ruleManaged.RuleID),
						}

						rule, err := srv.Get(ctx, request)
						So(err, ShouldBeNil)
						mask.IncludeDefinition = false
						So(rule, ShouldResembleProto, createRulePB(ruleManaged, cfg, mask))
					})
					Convey("Without get rule audit users permission", func() {
						authState.IdentityGroups = removeGroup(authState.IdentityGroups, auditUsersAccessGroup)

						request := &pb.GetRuleRequest{
							Name: fmt.Sprintf("projects/%s/rules/%s", ruleManaged.Project, ruleManaged.RuleID),
						}

						rule, err := srv.Get(ctx, request)
						So(err, ShouldBeNil)
						mask.IncludeAuditUsers = false
						So(rule, ShouldResembleProto, createRulePB(ruleManaged, cfg, mask))
					})
				})
				Convey("Read rule with Buganizer bug", func() {
					request := &pb.GetRuleRequest{
						Name: fmt.Sprintf("projects/%s/rules/%s", ruleBuganizer.Project, ruleBuganizer.RuleID),
					}

					rule, err := srv.Get(ctx, request)
					So(err, ShouldBeNil)
					So(rule, ShouldResembleProto, createRulePB(ruleBuganizer, cfg, mask))
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
					Realm:      "testproject:@project",
					Permission: perms.PermListRules,
				},
				{
					Realm:      "testproject:@project",
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
				test := func(mask ruleMask, cfg *configpb.ProjectConfig) {
					rs := []*rules.Entry{
						ruleManaged,
						ruleBuganizer,
						rules.NewRule(2).WithProject(testProject).Build(),
						rules.NewRule(3).WithProject(testProject).Build(),
						rules.NewRule(4).WithProject(testProject).Build(),
						// In other project.
						ruleManagedOther,
					}
					err := rules.SetForTesting(ctx, rs)
					So(err, ShouldBeNil)

					response, err := srv.List(ctx, request)
					So(err, ShouldBeNil)

					expected := &pb.ListRulesResponse{
						Rules: []*pb.Rule{
							createRulePB(rs[0], cfg, mask),
							createRulePB(rs[1], cfg, mask),
							createRulePB(rs[2], cfg, mask),
							createRulePB(rs[3], cfg, mask),
							createRulePB(rs[4], cfg, mask),
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
				mask := ruleMask{
					IncludeDefinition: true,
					IncludeAuditUsers: true,
				}
				Convey("Baseline", func() {
					test(mask, cfg)
				})
				Convey("Without get rule definition permission", func() {
					authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetRuleDefinition)

					mask.IncludeDefinition = false
					test(mask, cfg)
				})
				Convey("Without get rule audit users permission", func() {
					authState.IdentityGroups = removeGroup(authState.IdentityGroups, auditUsersAccessGroup)

					mask.IncludeAuditUsers = false
					test(mask, cfg)
				})
				Convey("With project not configured", func() {
					err = config.SetTestProjectConfig(ctx, map[string]*configpb.ProjectConfig{})

					test(mask, config.NewEmptyProject())
				})
			})
			Convey("Empty", func() {
				err := rules.SetForTesting(ctx, nil)
				So(err, ShouldBeNil)

				response, err := srv.List(ctx, request)
				So(err, ShouldBeNil)

				expected := &pb.ListRulesResponse{}
				So(response, ShouldResembleProto, expected)
			})
		})
		Convey("Update", func() {
			authState.IdentityPermissions = []authtest.RealmPermission{
				{
					Realm:      "testproject:@project",
					Permission: perms.PermUpdateRule,
				},
				{
					Realm:      "testproject:@project",
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
					IsManagingBug:         false,
					IsManagingBugPriority: false,
					IsActive:              false,
				},
				UpdateMask: &fieldmaskpb.FieldMask{
					// On the client side, we use JSON equivalents, i.e. ruleDefinition,
					// bug, isActive, isManagingBug. The pRPC layer handles the
					// translation before the request hits our service.
					Paths: []string{"rule_definition", "bug", "is_active", "is_managing_bug", "is_managing_bug_priority"},
				},
				Etag: ruleETag(ruleManaged, ruleMask{IncludeDefinition: true, IncludeAuditUsers: false}),
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
					// The requested updates should be applied.
					expectedRule := ruleManagedBuilder.Build().Clone()
					expectedRule.RuleDefinition = `test = "updated"`
					expectedRule.BugID = bugs.BugID{System: "monorail", ID: "monorailproject/2"}
					expectedRule.IsActive = false
					expectedRule.IsManagingBug = false
					expectedRule.IsManagingBugPriority = false

					// The notification flags should be reset as the bug was changed.
					expectedRule.BugManagementState.RuleAssociationNotified = false
					for _, policyState := range expectedRule.BugManagementState.PolicyState {
						policyState.ActivationNotified = false
					}

					Convey("baseline", func() {
						// Act
						rule, err := srv.Update(ctx, request)

						// Verify
						So(err, ShouldBeNil)

						storedRule, err := rules.Read(span.Single(ctx), testProject, ruleManaged.RuleID)
						So(err, ShouldBeNil)
						So(storedRule.LastUpdateTime, ShouldNotEqual, ruleManaged.LastUpdateTime)

						// Accept the new last update time (this value is set non-deterministically).
						expectedRule.LastUpdateTime = storedRule.LastUpdateTime
						expectedRule.LastAuditableUpdateTime = storedRule.LastUpdateTime
						expectedRule.IsManagingBugPriorityLastUpdateTime = storedRule.LastUpdateTime
						expectedRule.PredicateLastUpdateTime = storedRule.LastUpdateTime
						expectedRule.LastAuditableUpdateUser = "someone@example.com"

						// Verify the rule was updated as expected.
						So(storedRule, ShouldResembleProto, expectedRule)

						// Verify the returned rule matches what was expected.
						mask := ruleMask{IncludeDefinition: true, IncludeAuditUsers: true}
						So(rule, ShouldResembleProto, createRulePB(expectedRule, cfg, mask))
					})
					Convey("No audit users permission", func() {
						authState.IdentityGroups = removeGroup(authState.IdentityGroups, auditUsersAccessGroup)

						// Act
						rule, err := srv.Update(ctx, request)

						// Verify
						So(err, ShouldBeNil)

						storedRule, err := rules.Read(span.Single(ctx), testProject, ruleManaged.RuleID)
						So(err, ShouldBeNil)
						So(storedRule.LastUpdateTime, ShouldNotEqual, ruleManaged.LastUpdateTime)

						// Accept the new last update time (this value is set non-deterministically).
						expectedRule.LastUpdateTime = storedRule.LastUpdateTime
						expectedRule.LastAuditableUpdateTime = storedRule.LastUpdateTime
						expectedRule.PredicateLastUpdateTime = storedRule.LastUpdateTime
						expectedRule.IsManagingBugPriorityLastUpdateTime = storedRule.LastUpdateTime
						expectedRule.LastAuditableUpdateUser = "someone@example.com"

						So(storedRule, ShouldResembleProto, expectedRule)

						// Verify audit users are omitted from the result.
						mask := ruleMask{IncludeDefinition: true, IncludeAuditUsers: false}
						So(rule, ShouldResembleProto, createRulePB(expectedRule, cfg, mask))
					})
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
					So(storedRule.LastUpdateTime, ShouldNotEqual, ruleManaged.LastUpdateTime)

					// Verify the rule was correctly updated in the database:
					// - The requested updates should be applied.
					expectedRule := ruleManagedBuilder.Build()
					expectedRule.BugID = bugs.BugID{System: "buganizer", ID: "99999999"}

					// - The notification flags should be reset as the bug was changed.
					expectedRule.BugManagementState.RuleAssociationNotified = false
					for _, policyState := range expectedRule.BugManagementState.PolicyState {
						policyState.ActivationNotified = false
					}

					// - Accept the new last update time (this value is set non-deterministically).
					expectedRule.LastUpdateTime = storedRule.LastUpdateTime
					expectedRule.LastAuditableUpdateTime = storedRule.LastUpdateTime
					expectedRule.LastAuditableUpdateUser = "someone@example.com"

					So(storedRule, ShouldResembleProto, expectedRule)

					// Verify the returned rule matches what was expected.
					mask := ruleMask{IncludeDefinition: true, IncludeAuditUsers: true}
					So(rule, ShouldResembleProto, createRulePB(expectedRule, cfg, mask))
				})
				Convey("Managing bug priority updated", func() {
					request.UpdateMask.Paths = []string{"is_managing_bug_priority"}
					request.Rule.IsManagingBugPriority = false

					rule, err := srv.Update(ctx, request)
					So(err, ShouldBeNil)

					storedRule, err := rules.Read(span.Single(ctx), testProject, ruleManaged.RuleID)
					So(err, ShouldBeNil)

					// Check the rule was updated.
					So(storedRule.LastUpdateTime, ShouldNotEqual, ruleManaged.LastUpdateTime)

					// Verify the rule was correctly updated in the database:
					// - The requested update should be applied.
					expectedRule := ruleManagedBuilder.Build()
					expectedRule.IsManagingBugPriority = false

					// - Accept the new last update time (this value is set non-deterministically).
					expectedRule.IsManagingBugPriorityLastUpdateTime = storedRule.LastUpdateTime
					expectedRule.LastUpdateTime = storedRule.LastUpdateTime
					expectedRule.LastAuditableUpdateTime = storedRule.LastUpdateTime
					expectedRule.LastAuditableUpdateUser = "someone@example.com"

					So(storedRule, ShouldResembleProto, expectedRule)

					// Verify the returned rule matches what was expected.
					mask := ruleMask{IncludeDefinition: true, IncludeAuditUsers: true}
					So(rule, ShouldResembleProto, createRulePB(expectedRule, cfg, mask))
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
					So(storedRule.LastUpdateTime, ShouldNotEqual, ruleManaged.LastUpdateTime)

					// Verify the rule was correctly updated in the database:
					// - The requested update should be applied.
					expectedRule := ruleManagedBuilder.Build()
					expectedRule.BugID = ruleManagedOther.BugID

					// - The notification flags should be reset as the bug was changed.
					expectedRule.BugManagementState.RuleAssociationNotified = false
					for _, policyState := range expectedRule.BugManagementState.PolicyState {
						policyState.ActivationNotified = false
					}

					// - IsManagingBug should be silently set to false, because ruleManagedOther
					//  already controls the bug.
					expectedRule.IsManagingBug = false

					// - Accept the new last update time (this value is set non-deterministically).
					expectedRule.LastUpdateTime = storedRule.LastUpdateTime
					expectedRule.LastAuditableUpdateTime = storedRule.LastUpdateTime
					expectedRule.LastAuditableUpdateUser = "someone@example.com"

					So(storedRule, ShouldResembleProto, expectedRule)

					// Verify the returned rule matches what was expected.
					mask := ruleMask{IncludeDefinition: true, IncludeAuditUsers: true}
					So(rule, ShouldResembleProto, createRulePB(expectedRule, cfg, mask))
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
				Convey("Monorail bug on project that does not support it", func() {
					cfg.BugManagement.Monorail = nil

					err = config.SetTestProjectConfig(ctx, map[string]*configpb.ProjectConfig{
						"testproject": cfg,
					})

					rule, err := srv.Update(ctx, request)
					So(rule, ShouldBeNil)
					So(err, ShouldBeRPCInvalidArgument, "monorail bug system not enabled for this LUCI project")
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
					So(err, ShouldBeRPCInvalidArgument, `rule definition: syntax error: 1:1: unexpected token "<EOF>"`)
				})
			})
		})
		Convey("Create", func() {
			authState.IdentityPermissions = []authtest.RealmPermission{
				{
					Realm:      "testproject:@project",
					Permission: perms.PermCreateRule,
				},
				{
					Realm:      "testproject:@project",
					Permission: perms.PermGetRuleDefinition,
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
			Convey("No create rule definition permission", func() {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetRuleDefinition)

				rule, err := srv.Create(ctx, request)
				So(err, ShouldBeRPCPermissionDenied, "caller does not have permission analysis.rules.getDefinition")
				So(rule, ShouldBeNil)
			})
			Convey("Success", func() {
				expectedRuleBuilder := rules.NewRule(0).
					WithProject(testProject).
					WithRuleDefinition(`test = "create"`).
					WithActive(false).
					WithBug(bugs.BugID{System: "monorail", ID: "monorailproject/2"}).
					WithBugManaged(true).
					WithBugPriorityManaged(true).
					WithCreateUser("someone@example.com").
					WithLastAuditableUpdateUser("someone@example.com").
					WithSourceCluster(clustering.ClusterID{
						Algorithm: testname.AlgorithmName,
						ID:        strings.Repeat("aa", 16),
					}).
					WithBugManagementState(&bugspb.BugManagementState{})

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
						WithCreateTime(storedRule.CreateTime).
						WithLastAuditableUpdateTime(storedRule.CreateTime).
						WithLastUpdateTime(storedRule.CreateTime).
						WithPredicateLastUpdateTime(storedRule.CreateTime).
						WithBugPriorityManagedLastUpdateTime(storedRule.CreateTime).
						Build()

					// Verify the rule was correctly created in the database.
					So(storedRule, ShouldResembleProto, expectedRuleBuilder.Build())

					// Verify the returned rule matches our expectations.
					mask := ruleMask{IncludeDefinition: true, IncludeAuditUsers: true}
					So(rule, ShouldResembleProto, createRulePB(expectedRule, cfg, mask))
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
						WithCreateTime(storedRule.CreateTime).
						WithLastAuditableUpdateTime(storedRule.CreateTime).
						WithLastUpdateTime(storedRule.CreateTime).
						WithPredicateLastUpdateTime(storedRule.CreateTime).
						WithBugPriorityManagedLastUpdateTime(storedRule.CreateTime).
						Build()

					// Verify the rule was correctly created in the database.
					So(storedRule, ShouldResembleProto, expectedRuleBuilder.Build())

					// Verify the returned rule matches our expectations.
					mask := ruleMask{IncludeDefinition: true, IncludeAuditUsers: true}
					So(rule, ShouldResembleProto, createRulePB(expectedRule, cfg, mask))
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
						WithCreateTime(storedRule.CreateTime).
						WithLastAuditableUpdateTime(storedRule.CreateTime).
						WithLastUpdateTime(storedRule.CreateTime).
						WithPredicateLastUpdateTime(storedRule.CreateTime).
						WithBugPriorityManagedLastUpdateTime(storedRule.CreateTime).
						Build()

					// Verify the rule was correctly created in the database.
					So(storedRule, ShouldResembleProto, expectedRuleBuilder.Build())

					// Verify the returned rule matches our expectations.
					mask := ruleMask{IncludeDefinition: true, IncludeAuditUsers: true}
					So(rule, ShouldResembleProto, createRulePB(expectedRule, cfg, mask))
				})
				Convey("No audit users permission", func() {
					authState.IdentityGroups = removeGroup(authState.IdentityGroups, auditUsersAccessGroup)

					rule, err := srv.Create(ctx, request)
					So(err, ShouldBeNil)

					storedRule, err := rules.Read(span.Single(ctx), testProject, rule.RuleId)
					So(err, ShouldBeNil)

					expectedRule := expectedRuleBuilder.
						// Accept the randomly generated rule ID.
						WithRuleID(rule.RuleId).
						// Accept whatever CreationTime was assigned, as it
						// is determined by Spanner commit time.
						// Rule spanner data access code tests already validate
						// this is populated correctly.
						WithCreateTime(storedRule.CreateTime).
						WithLastAuditableUpdateTime(storedRule.CreateTime).
						WithLastUpdateTime(storedRule.CreateTime).
						WithPredicateLastUpdateTime(storedRule.CreateTime).
						WithBugPriorityManagedLastUpdateTime(storedRule.CreateTime).
						Build()

					// Verify the rule was correctly created in the database.
					So(storedRule, ShouldResembleProto, expectedRuleBuilder.Build())

					// Verify the returned rule matches our expectations.
					mask := ruleMask{IncludeDefinition: true, IncludeAuditUsers: false}
					So(rule, ShouldResembleProto, createRulePB(expectedRule, cfg, mask))
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
					So(err, ShouldBeRPCInvalidArgument, `rule definition: syntax error: 1:1: unexpected token "<EOF>"`)
				})
			})
		})
		Convey("LookupBug", func() {
			authState.IdentityPermissions = []authtest.RealmPermission{
				{
					Realm:      "testproject:@project",
					Permission: perms.PermListRules,
				},
				{
					Realm:      "otherproject:@project",
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
							Realm:      "testproject:@project",
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
	Convey("createRulePB", t, func() {
		// The behaviour of createRulePB is assumed by many test cases.
		// This verifies that behaviour.

		rule := rules.NewRule(1).
			WithProject(testProject).
			WithBug(bugs.BugID{System: "monorail", ID: "monorailproject/111"}).Build()

		expectedRule := &pb.Rule{
			Name:                    "projects/testproject/rules/8109e4f8d9c218e9a2e33a7a21395455",
			Project:                 "testproject",
			RuleId:                  "8109e4f8d9c218e9a2e33a7a21395455",
			RuleDefinition:          "reason LIKE \"%exit code 5%\" AND test LIKE \"tast.arc.%\"",
			IsActive:                true,
			PredicateLastUpdateTime: timestamppb.New(time.Date(1904, 4, 4, 4, 4, 4, 1, time.UTC)),
			Bug: &pb.AssociatedBug{
				System:   "monorail",
				Id:       "monorailproject/111",
				LinkText: "mybug.com/111",
				Url:      "https://monorailhost.com/p/monorailproject/issues/detail?id=111",
			},
			IsManagingBug:                       true,
			IsManagingBugPriority:               true,
			IsManagingBugPriorityLastUpdateTime: timestamppb.New(time.Date(1905, 5, 5, 5, 5, 5, 1, time.UTC)),
			SourceCluster: &pb.ClusterId{
				Algorithm: "clusteralg1-v9",
				Id:        "696431",
			},
			BugManagementState: &pb.BugManagementState{
				PolicyState: []*pb.BugManagementState_PolicyState{
					{
						PolicyId:           "policy-a",
						IsActive:           true,
						LastActivationTime: timestamppb.New(time.Date(1908, 8, 8, 8, 8, 8, 1, time.UTC)),
					},
				},
			},
			CreateTime:              timestamppb.New(time.Date(1900, 1, 2, 3, 4, 5, 1, time.UTC)),
			CreateUser:              "system",
			LastAuditableUpdateTime: timestamppb.New(time.Date(1907, 7, 7, 7, 7, 7, 1, time.UTC)), //FIX ME
			LastAuditableUpdateUser: "user@google.com",
			LastUpdateTime:          timestamppb.New(time.Date(1909, 9, 9, 9, 9, 9, 1, time.UTC)),
			Etag:                    `W/"+d+u/1909-09-09T09:09:09.000000001Z"`,
		}
		cfg := &configpb.ProjectConfig{
			BugManagement: &configpb.BugManagement{
				Monorail: &configpb.MonorailProject{
					Project:          "monorailproject",
					DisplayPrefix:    "mybug.com",
					MonorailHostname: "monorailhost.com",
				},
			},
		}
		mask := ruleMask{
			IncludeDefinition: true,
			IncludeAuditUsers: true,
		}
		Convey("With all fields", func() {
			So(createRulePB(rule, cfg, mask), ShouldResembleProto, expectedRule)
		})
		Convey("Without definition field", func() {
			mask.IncludeDefinition = false
			expectedRule.RuleDefinition = ""
			expectedRule.Etag = `W/"+u/1909-09-09T09:09:09.000000001Z"`
			So(createRulePB(rule, cfg, mask), ShouldResembleProto, expectedRule)
		})
		Convey("Without audit users", func() {
			mask.IncludeAuditUsers = false
			expectedRule.CreateUser = ""
			expectedRule.LastAuditableUpdateUser = ""
			expectedRule.Etag = `W/"+d/1909-09-09T09:09:09.000000001Z"`
			So(createRulePB(rule, cfg, mask), ShouldResembleProto, expectedRule)
		})
	})
	Convey("isETagValid", t, func() {
		rule := rules.NewRule(0).
			WithProject(testProject).
			WithBug(bugs.BugID{System: "monorail", ID: "monorailproject/111"}).Build()

		Convey("Should match ETags for same rule version", func() {
			masks := []ruleMask{
				{IncludeDefinition: true, IncludeAuditUsers: true},
				{IncludeDefinition: true, IncludeAuditUsers: false},
				{IncludeDefinition: false, IncludeAuditUsers: true},
				{IncludeDefinition: false, IncludeAuditUsers: false},
			}
			for _, mask := range masks {
				So(isETagMatching(rule, ruleETag(rule, mask)), ShouldBeTrue)
			}
		})
		Convey("Should not match ETag for different version of rule", func() {
			mask := ruleMask{IncludeDefinition: true, IncludeAuditUsers: true}
			etag := ruleETag(rule, mask)

			rule.LastUpdateTime = time.Date(2021, 5, 4, 3, 2, 1, 0, time.UTC)
			So(isETagMatching(rule, etag), ShouldBeFalse)
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
	LastAuditableUpdate: "1907-07-07T07:07:07Z"
	LastUpdated: "1909-09-09T09:09:09Z"
}`
		So(formatRule(rule), ShouldEqual, expectedRule)
	})
}

func removeGroup(groups []string, group string) []string {
	var result []string
	for _, g := range groups {
		if g != group {
			result = append(result, g)
		}
	}
	return result
}
