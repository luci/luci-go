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
	"context"
	"fmt"
	"regexp"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/third_party/google.golang.org/genproto/googleapis/devtools/issuetracker/v1"

	"go.chromium.org/luci/analysis/internal/bugs"
	"go.chromium.org/luci/analysis/internal/bugs/buganizer"
	bugspb "go.chromium.org/luci/analysis/internal/bugs/proto"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/failurereason"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/testname"
	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/perms"
	"go.chromium.org/luci/analysis/pbutil"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

const (
	// Maximum length of a new issue title, in UTF-8 bytes.
	maxTitleLengthBytes = 250

	// Maximum length of a new issue description, in UTF-8 bytes.
	maxCommentLengthBytes = 100_000

	// Users who can create issues in Buganizer through LUCI Analysis,
	// using the LUCI Analysis service account.
	//
	// Access is managed via a single group, not as a project-level
	// permission, as the same service account is used to interact
	// with Buganizer for all components. I.E. anyone who can use this
	// account can file into any buganizer component that LUCI Analysis
	// has write access to, regardless of the project that owns that
	// component.
	//
	// Use of a single group allow us to restrict this access only
	// to trusted groups, i.e. Googlers and partners only.
	buganizerAccessGroup = "luci-analysis-buganizer-access"
)

// Rules implements pb.RulesServer.
type rulesServer struct {
	// Base URL for UI. Should not include trailing slash.
	// E.g. "https://luci-analyis.appspot.com".
	uiBaseURL string
	// Client for interacting with buganizer.
	client buganizer.Client
	// The account used to interact with buganizer.
	selfEmail string
}

// NewRulesServer returns a new pb.RulesServer.
func NewRulesServer(uiBaseURL string, client buganizer.Client, selfEmail string) pb.RulesServer {
	return &pb.DecoratedRules{
		Prelude: checkAllowedPrelude,
		Service: &rulesServer{
			uiBaseURL: uiBaseURL,
			client:    client,
			selfEmail: selfEmail,
		},
		Postlude: gRPCifyAndLogPostlude,
	}
}

// Retrieves a rule.
func (*rulesServer) Get(ctx context.Context, req *pb.GetRuleRequest) (*pb.Rule, error) {
	project, ruleID, err := parseRuleName(req.Name)
	if err != nil {
		return nil, invalidArgumentError(errors.Annotate(err, "name").Err())
	}
	if err := perms.VerifyProjectPermissions(ctx, project, perms.PermGetRule); err != nil {
		return nil, err
	}
	ruleMask, err := ruleFieldAccess(ctx, project)
	if err != nil {
		return nil, err
	}

	cfg, err := readProjectConfig(ctx, project)
	if err != nil {
		return nil, err
	}

	r, err := rules.Read(span.Single(ctx), project, ruleID)
	if err != nil {
		if errors.Is(err, rules.NotExistsErr) {
			return nil, appstatus.Error(codes.NotFound, "rule does not exist")
		}
		// This will result in an internal error being reported to the caller.
		return nil, errors.Annotate(err, "reading rule %s", ruleID).Err()
	}
	return createRulePB(r, cfg.Config, ruleMask), nil
}

// ruleMask captures the fields the caller has access to see.
type ruleMask struct {
	// Include the definition of the rule.
	// Guarded by analysis.rules.getDefinition permission.
	IncludeDefinition bool
	// Include the email address of users who created/modified the rule.
	// Limited to Googlers only.
	IncludeAuditUsers bool
}

// ruleFieldAccess checks the caller's access to rule fields in the given
// project and returns a ruleMask corresponding to the fields they are
// allowed to see.
func ruleFieldAccess(ctx context.Context, project string) (ruleMask, error) {
	var result ruleMask
	var err error
	result.IncludeDefinition, err = perms.HasProjectPermission(ctx, project, perms.PermGetRuleDefinition)
	if err != nil {
		return ruleMask{}, errors.Annotate(err, "determining access to rule definition").Err()
	}
	result.IncludeAuditUsers, err = auth.IsMember(ctx, auditUsersAccessGroup)
	if err != nil {
		return ruleMask{}, errors.Annotate(err, "determining access to read created/last modified users").Err()
	}
	return result, nil
}

// Lists rules.
func (*rulesServer) List(ctx context.Context, req *pb.ListRulesRequest) (*pb.ListRulesResponse, error) {
	project, err := parseProjectName(req.Parent)
	if err != nil {
		return nil, invalidArgumentError(errors.Annotate(err, "parent").Err())
	}
	if err := perms.VerifyProjectPermissions(ctx, project, perms.PermListRules); err != nil {
		return nil, err
	}
	ruleMask, err := ruleFieldAccess(ctx, project)
	if err != nil {
		return nil, err
	}

	cfg, err := readProjectConfig(ctx, project)
	if err != nil {
		return nil, err
	}

	// TODO: Update to read all rules (not just active), and implement pagination.
	rs, err := rules.ReadActive(span.Single(ctx), project)
	if err != nil {
		// GRPCifyAndLog will log this, and report an internal error.
		return nil, errors.Annotate(err, "reading rules").Err()
	}

	rpbs := make([]*pb.Rule, 0, len(rs))
	for _, r := range rs {
		rpbs = append(rpbs, createRulePB(r, cfg.Config, ruleMask))
	}
	response := &pb.ListRulesResponse{
		Rules: rpbs,
	}
	return response, nil
}

// Creates a new rule.
func (*rulesServer) Create(ctx context.Context, req *pb.CreateRuleRequest) (*pb.Rule, error) {
	project, err := parseProjectName(req.Parent)
	if err != nil {
		return nil, invalidArgumentError(errors.Annotate(err, "parent").Err())
	}
	if err := perms.VerifyProjectPermissions(ctx, project, perms.PermCreateRule, perms.PermGetRuleDefinition); err != nil {
		return nil, err
	}
	cfg, err := readProjectConfig(ctx, project)
	if err != nil {
		return nil, err
	}
	if err := validateCreateRuleRequest(req, cfg.Config); err != nil {
		return nil, invalidArgumentError(err)
	}

	ruleMask, err := ruleFieldAccess(ctx, project)
	if err != nil {
		return nil, err
	}

	ruleID, err := rules.GenerateID()
	if err != nil {
		return nil, errors.Annotate(err, "generating rule ID").Err()
	}
	user := auth.CurrentUser(ctx).Email

	r := ruleEntryFromRequest(project, ruleID, req.Rule)
	r, err = createRuleEntry(ctx, r, user)
	if err != nil {
		return nil, err
	}
	return createRulePB(r, cfg.Config, ruleMask), nil
}

func createRuleEntry(ctx context.Context, r *rules.Entry, user string) (*rules.Entry, error) {
	commitTime, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		// Verify the bug is not used by another rule in this project.
		bugRules, err := rules.ReadByBug(ctx, r.BugID)
		if err != nil {
			return err
		}
		for _, otherRule := range bugRules {
			if otherRule.IsManagingBug {
				// Avoid conflicts by silently making the bug not managed
				// by this rule if there is another rule managing it.
				// Note: this validation implicitly discloses the existence
				// of rules in projects other than those the user may have
				// access to.
				r.IsManagingBug = false
			}
			if otherRule.Project == r.Project {
				return failedPreconditionError(fmt.Errorf("rule: bug: bug already used by a rule in the same project (%s/%s)", otherRule.Project, otherRule.RuleID))
			}
		}

		ms, err := rules.Create(r, user)
		if err != nil {
			return err
		}
		span.BufferWrite(ctx, ms)
		return nil
	})
	if err != nil {
		return nil, err
	}
	r.CreateTime = commitTime.In(time.UTC)
	r.CreateUser = user
	r.LastAuditableUpdateTime = commitTime.In(time.UTC)
	r.LastAuditableUpdateUser = user
	r.LastUpdateTime = commitTime.In(time.UTC)
	r.PredicateLastUpdateTime = commitTime.In(time.UTC)
	r.IsManagingBugPriorityLastUpdateTime = commitTime.In(time.UTC)

	// Log rule changes to provide a way of recovering old system state
	// if malicious or unintended updates occur.
	logging.Infof(ctx, "Rule created (%s/%s): %s", r.Project, r.RuleID, formatRule(r))
	return r, nil
}

func (s *rulesServer) CreateWithNewIssue(ctx context.Context, req *pb.CreateRuleWithNewIssueRequest) (*pb.Rule, error) {
	// Verify the user is allowed to create issues in Buganizer through LUCI Analysis.
	if err := checkAllowed(ctx, buganizerAccessGroup); err != nil {
		return nil, err
	}
	project, err := parseProjectName(req.Parent)
	if err != nil {
		return nil, invalidArgumentError(errors.Annotate(err, "parent").Err())
	}
	// Check standard rule creation permissions.
	if err := perms.VerifyProjectPermissions(ctx, project, perms.PermCreateRule, perms.PermGetRuleDefinition); err != nil {
		return nil, err
	}
	if err := validateCreateRuleWithNewIssueRequest(req); err != nil {
		return nil, invalidArgumentError(err)
	}
	if err := s.checkComponentAccess(ctx, req.Issue.Component.GetIssueTracker()); err != nil {
		return nil, err
	}

	cfg, err := readProjectConfig(ctx, project)
	if err != nil {
		return nil, err
	}

	ruleMask, err := ruleFieldAccess(ctx, project)
	if err != nil {
		return nil, err
	}

	ruleID, err := rules.GenerateID()
	if err != nil {
		return nil, errors.Annotate(err, "generating rule ID").Err()
	}
	user := auth.CurrentUser(ctx).Email
	ruleURL := bugs.RuleURL(s.uiBaseURL, project, ruleID)

	request := prepareBuganizerIssueRequest(req.Issue, ruleURL, user)
	issue, err := s.client.CreateIssue(ctx, request)
	if err != nil {
		code := status.Code(err)
		if code == codes.FailedPrecondition ||
			code == codes.InvalidArgument ||
			code == codes.NotFound ||
			code == codes.OutOfRange {
			// Error indicates something wrong with the request,
			// surface these back to the caller.
			return nil, invalidArgumentError(errors.Annotate(err, "creating issue").Err())
		}

		// GRPCifyAndLog will log this, and report an internal error to the caller.
		return nil, err
	}

	r := ruleEntryFromRequest(project, ruleID, req.Rule)
	// Use bug we just filed for the rule.
	r.BugID = bugs.BugID{
		System: bugs.BuganizerSystem,
		ID:     fmt.Sprintf("%v", issue.IssueId),
	}
	r, err = createRuleEntry(ctx, r, user)
	if err != nil {
		return nil, err
	}

	return createRulePB(r, cfg.Config, ruleMask), nil
}

// checkComponentAccess checks LUCI Analysis is able to file a bug into
// the specified issue tracker component.
func (s *rulesServer) checkComponentAccess(ctx context.Context, component *pb.IssueTrackerComponent) error {
	if s.client == nil || s.selfEmail == "" {
		return appstatus.Error(codes.Unimplemented, "Issue tracker integration is not enabled on this deployment.")
	}
	accessChecker := buganizer.NewComponentAccessChecker(s.client, s.selfEmail)
	componentID := component.ComponentId
	access, err := accessChecker.CheckAccess(ctx, componentID)
	if err != nil {
		// Internal error.
		return err
	}
	if !access.Appender {
		return appstatus.Errorf(codes.FailedPrecondition, "LUCI Analysis is missing permission to create issues in component %v. Please follow the steps in go/luci-analysis-bug-creation-failed to grant access.", componentID)
	}
	if !access.IssueDefaultsAppender {
		return appstatus.Errorf(codes.FailedPrecondition, "LUCI Analysis is missing permission to append comments to issues in component %v. Please follow the steps in go/luci-analysis-bug-creation-failed to grant access.", componentID)
	}
	return nil
}

func prepareBuganizerIssueRequest(request *pb.CreateRuleWithNewIssueRequest_Issue, ruleLink string, userEmail string) *issuetracker.CreateIssueRequest {
	comment := fmt.Sprintf("%s\n\nView example failures and modify the failures associated with this bug in LUCI Analysis: %s. Filed on behalf of %s.",
		request.Comment,
		ruleLink,
		userEmail,
	)

	issue := &issuetracker.Issue{
		IssueState: &issuetracker.IssueState{
			ComponentId: request.Component.GetIssueTracker().ComponentId,
			Type:        issuetracker.Issue_BUG,
			Status:      issuetracker.Issue_NEW,
			Priority:    toBuganizerPriority(request.Priority),
			Severity:    issuetracker.Issue_S2,
			Title:       request.Title,
			AccessLimit: &issuetracker.IssueAccessLimit{
				AccessLevel: toBuganizerAccessLevel(request.AccessLimit),
			},
		},
		IssueComment: &issuetracker.IssueComment{
			Comment: comment,
		},
	}

	return &issuetracker.CreateIssueRequest{
		Issue: issue,
		TemplateOptions: &issuetracker.CreateIssueRequest_TemplateOptions{
			ApplyTemplate: true,
		},
	}
}

func toBuganizerPriority(priority pb.BuganizerPriority) issuetracker.Issue_Priority {
	switch priority {
	case pb.BuganizerPriority_P0:
		return issuetracker.Issue_P0
	case pb.BuganizerPriority_P1:
		return issuetracker.Issue_P1
	case pb.BuganizerPriority_P2:
		return issuetracker.Issue_P2
	case pb.BuganizerPriority_P3:
		return issuetracker.Issue_P3
	case pb.BuganizerPriority_P4:
		return issuetracker.Issue_P4
	default:
		panic(fmt.Errorf("unknown priority: %v", priority))
	}
}

func toBuganizerAccessLevel(accessLevel pb.CreateRuleWithNewIssueRequest_Issue_IssueAccessLimit) issuetracker.IssueAccessLimit_AccessLevel {
	switch accessLevel {
	case pb.CreateRuleWithNewIssueRequest_Issue_None:
		return issuetracker.IssueAccessLimit_LIMIT_NONE
	case pb.CreateRuleWithNewIssueRequest_Issue_Trusted:
		return issuetracker.IssueAccessLimit_LIMIT_VIEW_TRUSTED
	default:
		panic(fmt.Errorf("unknown access level: %v", accessLevel))
	}
}

// Updates a rule.
func (*rulesServer) Update(ctx context.Context, req *pb.UpdateRuleRequest) (*pb.Rule, error) {
	project, ruleID, err := parseRuleName(req.Rule.GetName())
	if err != nil {
		return nil, invalidArgumentError(errors.Annotate(err, "rule: name").Err())
	}
	if err := perms.VerifyProjectPermissions(ctx, project, perms.PermUpdateRule, perms.PermGetRuleDefinition); err != nil {
		return nil, err
	}
	cfg, err := readProjectConfig(ctx, project)
	if err != nil {
		return nil, err
	}
	if err := validateUpdateRuleRequest(req, cfg.Config); err != nil {
		return nil, invalidArgumentError(err)
	}
	ruleMask, err := ruleFieldAccess(ctx, project)
	if err != nil {
		return nil, err
	}

	user := auth.CurrentUser(ctx).Email

	var predicateUpdated bool
	var managingBugPriorityUpdated bool
	var originalRule *rules.Entry
	var updatedRule *rules.Entry
	f := func(ctx context.Context) error {
		rule, err := rules.Read(ctx, project, ruleID)
		if err != nil {
			if errors.Is(err, rules.NotExistsErr) {
				return appstatus.Error(codes.NotFound, "rule does not exist")
			}
			// This will result in an internal error being reported to the
			// caller.
			return errors.Annotate(err, "read rule").Err()
		}
		originalRule = &rules.Entry{}
		*originalRule = *rule

		if req.Etag != "" && !isETagMatching(rule, req.Etag) {
			// Attach a codes.Aborted appstatus to a vanilla error to avoid
			// ReadWriteTransaction interpreting this case for a scenario
			// in which it should retry the transaction.
			err := errors.New("etag mismatch")
			return appstatus.Attach(err, status.New(codes.Aborted, "the rule was modified since it was last read; the update was not applied."))
		}

		// If we are updating the RuleDefinition or IsActive field.
		updatePredicate := false

		// If we are updating the Bug field
		updatingBug := false

		// If we are updating the IsManagingBug field.
		updatingManaged := false

		// If we are updating the IsManagingBugPriority field.
		updatingIsManagingBugPriority := false

		// Tracks if the caller explicitly requested .IsManagingBug = true, even
		// if this is a no-op.
		requestedManagedTrue := false

		for _, path := range req.UpdateMask.Paths {
			// Only limited fields may be modified by the client.
			// Keep this switch statement in sync with validateUpdateRuleRequest.
			switch path {
			case "rule_definition":
				if rule.RuleDefinition != req.Rule.RuleDefinition {
					rule.RuleDefinition = req.Rule.RuleDefinition
					updatePredicate = true
				}
			case "bug":
				bugID := bugs.BugID{
					System: req.Rule.Bug.GetSystem(),
					ID:     req.Rule.Bug.GetId(),
				}
				if rule.BugID != bugID {
					updatingBug = true // Triggers validation.
					rule.BugID = bugID

					// Changing the associated bug requires us to reset flags
					// tracking notifications sent to the associated bug.
					rule.BugManagementState.RuleAssociationNotified = false
					if rule.BugManagementState.PolicyState != nil {
						for _, policyState := range rule.BugManagementState.PolicyState {
							policyState.ActivationNotified = false
						}
					}
				}
			case "is_active":
				if rule.IsActive != req.Rule.IsActive {
					rule.IsActive = req.Rule.IsActive
					updatePredicate = true
				}
			case "is_managing_bug":
				if req.Rule.IsManagingBug {
					requestedManagedTrue = true
				}
				if rule.IsManagingBug != req.Rule.IsManagingBug {
					updatingManaged = true // Triggers validation.
					rule.IsManagingBug = req.Rule.IsManagingBug
				}
			case "is_managing_bug_priority":
				if rule.IsManagingBugPriority != req.Rule.IsManagingBugPriority {
					updatingIsManagingBugPriority = true
					rule.IsManagingBugPriority = req.Rule.IsManagingBugPriority
				}
			default:
				// This should never be reached, as update masks are
				// checked in validateUpdateRuleRequest.
				return fmt.Errorf("unsupported field mask: %s", path)
			}
		}

		if updatingBug || updatingManaged {
			// Verify the new bug is not used by another rule in the
			// same project, and that there are not multiple rules
			// managing the same bug.
			bugRules, err := rules.ReadByBug(ctx, rule.BugID)
			if err != nil {
				// This will result in an internal error being reported
				// to the caller.
				return err
			}
			for _, otherRule := range bugRules {
				if otherRule.Project == project && otherRule.RuleID != ruleID {
					return failedPreconditionError(fmt.Errorf("rule: bug: bug already used by a rule in the same project (%s/%s)", otherRule.Project, otherRule.RuleID))
				}
			}
			for _, otherRule := range bugRules {
				if otherRule.Project != project && otherRule.IsManagingBug {
					if requestedManagedTrue {
						// The caller explicitly requested an update of
						// IsManagingBug to true, but we cannot do this.
						return failedPreconditionError(fmt.Errorf("rule: bug: bug already managed by a rule in another project (%s/%s)", otherRule.Project, otherRule.RuleID))
					}
					// If only changing the bug, avoid conflicts by silently
					// making the bug not managed by this rule if there is
					// another rule managing it.
					// Note: this step implicitly discloses the existence
					// of rules in projects other than those the user may have
					// access to.
					rule.IsManagingBug = false
				}
			}
		}

		ms, err := rules.Update(rule, rules.UpdateOptions{
			IsAuditableUpdate:            true,
			PredicateUpdated:             updatePredicate,
			IsManagingBugPriorityUpdated: updatingIsManagingBugPriority,
		}, user)
		if err != nil {
			return err
		}
		span.BufferWrite(ctx, ms)
		updatedRule = rule
		predicateUpdated = updatePredicate
		managingBugPriorityUpdated = updatingIsManagingBugPriority
		return nil
	}
	commitTime, err := span.ReadWriteTransaction(ctx, f)
	if err != nil {
		return nil, err
	}
	updatedRule.LastAuditableUpdateTime = commitTime.In(time.UTC)
	updatedRule.LastAuditableUpdateUser = user
	updatedRule.LastUpdateTime = commitTime.In(time.UTC)
	if predicateUpdated {
		updatedRule.PredicateLastUpdateTime = commitTime.In(time.UTC)
	}
	if managingBugPriorityUpdated {
		updatedRule.IsManagingBugPriorityLastUpdateTime = commitTime.In(time.UTC)
	}
	// Log rule changes to provide a way of recovering old system state
	// if malicious or unintended updates occur.
	logRuleUpdate(ctx, originalRule, updatedRule)

	return createRulePB(updatedRule, cfg.Config, ruleMask), nil
}

func logRuleUpdate(ctx context.Context, old *rules.Entry, new *rules.Entry) {
	logging.Infof(ctx, "Rule updated (%s/%s): from %s to %s", old.Project, old.RuleID, formatRule(old), formatRule(new))
}

func formatRule(r *rules.Entry) string {
	return fmt.Sprintf("{\n"+
		"\tRuleDefinition: %q,\n"+
		"\tBugID: %q,\n"+
		"\tIsActive: %v,\n"+
		"\tIsManagingBug: %v,\n"+
		"\tIsManagingBugPriority: %v,\n"+
		"\tSourceCluster: %q\n"+
		"\tLastAuditableUpdate: %q\n"+
		"\tLastUpdated: %q\n"+
		"}", r.RuleDefinition, r.BugID, r.IsActive, r.IsManagingBug,
		r.IsManagingBugPriority, r.SourceCluster,
		r.LastAuditableUpdateTime.Format(time.RFC3339Nano),
		r.LastUpdateTime.Format(time.RFC3339Nano))
}

// LookupBug looks up the rule associated with the given bug.
func (*rulesServer) LookupBug(ctx context.Context, req *pb.LookupBugRequest) (*pb.LookupBugResponse, error) {
	bug := bugs.BugID{
		System: req.System,
		ID:     req.Id,
	}
	if err := bug.Validate(); err != nil {
		return nil, invalidArgumentError(err)
	}
	rules, err := rules.ReadByBug(span.Single(ctx), bug)
	if err != nil {
		// This will result in an internal error being reported to the caller.
		return nil, errors.Annotate(err, "reading rule by bug %s:%s", bug.System, bug.ID).Err()
	}
	ruleNames := make([]string, 0, len(rules))
	for _, rule := range rules {
		allowed, err := perms.HasProjectPermission(ctx, rule.Project, perms.PermListRules)
		if err != nil {
			return nil, err
		}
		if allowed {
			ruleNames = append(ruleNames, ruleName(rule.Project, rule.RuleID))
		}
	}
	return &pb.LookupBugResponse{
		Rules: ruleNames,
	}, nil
}

func (*rulesServer) PrepareDefaults(ctx context.Context, req *pb.PrepareRuleDefaultsRequest) (*pb.PrepareRuleDefaultsResponse, error) {
	project, err := parseProjectName(req.Parent)
	if err != nil {
		return nil, invalidArgumentError(errors.Annotate(err, "parent").Err())
	}
	if err := perms.VerifyProjectPermissions(ctx, project, perms.PermGetConfig); err != nil {
		return nil, err
	}
	if err := validatePrepareRuleDefaultsRequest(req); err != nil {
		return nil, invalidArgumentError(err)
	}
	cfg, err := readProjectConfig(ctx, project)
	if err != nil {
		return nil, err
	}

	rule := &pb.Rule{
		Project:        "", // Output-only field only set for created rules.
		RuleId:         "", // Output-only field only set for created rules.
		RuleDefinition: "",
		Bug:            nil, // To be determined by user.
		IsActive:       true,
		// Enabling auto-closure on manually created rules usually leads to the bug being immediately
		// closed because it did not meet the auto-bug filing thresholds. So let the user manaully
		// handle closure.
		IsManagingBug:         false,
		IsManagingBugPriority: false,
	}

	if req.TestResult != nil {
		example := &clustering.Failure{
			TestID: req.TestResult.TestId,
			Reason: req.TestResult.FailureReason,
		}

		testAlg := &testname.Algorithm{}
		ruleDefinition := testAlg.FailureAssociationRule(cfg, example)

		reasonAlg := &failurereason.Algorithm{}
		reasonAlg.FailureAssociationRule(cfg, example)
		if ruleCriteria := reasonAlg.FailureAssociationRule(cfg, example); ruleCriteria != "" {
			// Combine test and failure association reason criteria.
			ruleDefinition = fmt.Sprintf("%s AND %s", ruleDefinition, ruleCriteria)
		}
		rule.RuleDefinition = ruleDefinition
	}

	return &pb.PrepareRuleDefaultsResponse{
		Rule: rule,
	}, nil
}

func validateCreateRuleRequest(req *pb.CreateRuleRequest, cfg *configpb.ProjectConfig) error {
	// No need to validate Parent, that is already validated before we
	// get to this method. It needs to be parsed to do authorization checks.
	// Per https://google.aip.dev/211, authorization checks should occur before
	// validation.

	if err := validateRuleForCreate(req.Rule, true /* expectBug */); err != nil {
		return errors.Annotate(err, "rule").Err()
	}
	if err := validateBugAgainstConfig(cfg, req.Rule.Bug); err != nil {
		return errors.Annotate(err, "rule: bug").Err()
	}
	return nil
}

func validateCreateRuleWithNewIssueRequest(req *pb.CreateRuleWithNewIssueRequest) error {
	// No need to validate Parent, that is already validated before we
	// get to this method. It needs to be parsed to do authorization checks.
	// Per https://google.aip.dev/211, authorization checks should occur before
	// validation.

	if err := validateRuleForCreate(req.Rule, false /* expectBug */); err != nil {
		return errors.Annotate(err, "rule").Err()
	}
	if err := validateNewIssue(req.Issue); err != nil {
		return errors.Annotate(err, "issue").Err()
	}
	return nil
}

func validateUpdateRuleRequest(req *pb.UpdateRuleRequest, cfg *configpb.ProjectConfig) error {
	// No need to validate req.Rule.Name, that is already validated before we
	// get to this method. It needs to be parsed to do authorization checks.
	// Per https://google.aip.dev/211, authorization checks should occur before
	// validation.

	if req.UpdateMask == nil {
		return errors.New("update_mask: unspecified")
	}
	if len(req.UpdateMask.Paths) == 0 {
		return errors.New("update_mask: paths: unspecified")
	}
	for _, path := range req.UpdateMask.Paths {
		// Only limited fields may be modified by the client.
		switch path {
		case "rule_definition":
			if err := rules.ValidateRuleDefinition(req.Rule.RuleDefinition); err != nil {
				return errors.Annotate(err, "rule: rule_definition").Err()
			}
		case "bug":
			if req.Rule.Bug == nil {
				return errors.New("rule: bug: unspecified")
			}
			bugID := bugs.BugID{
				System: req.Rule.Bug.GetSystem(),
				ID:     req.Rule.Bug.GetId(),
			}
			if err := bugID.Validate(); err != nil {
				return errors.Annotate(err, "rule: bug").Err()
			}
			if err := validateBugAgainstConfig(cfg, req.Rule.Bug); err != nil {
				return errors.Annotate(err, "rule: bug").Err()
			}
		case "is_active":
			// Boolean value.
		case "is_managing_bug":
			// Boolean value.
		case "is_managing_bug_priority":
			// Boolean value.
		default:
			return fmt.Errorf("update_mask: paths: unsupported field path: %s", path)
		}
	}
	return nil
}

func validatePrepareRuleDefaultsRequest(req *pb.PrepareRuleDefaultsRequest) error {
	// No need to validate Parent, that is already validated before we
	// get to this method. It needs to be parsed to do authorization checks.
	// Per https://google.aip.dev/211, authorization checks should occur before
	// validation.

	if err := validatePrepareRuleDefaultsTestResult(req.TestResult); err != nil {
		return errors.Annotate(err, "test_result").Err()
	}
	return nil
}

func validatePrepareRuleDefaultsTestResult(result *pb.PrepareRuleDefaultsRequest_TestResult) error {
	if result == nil {
		// Test result is optional.
		return nil
	}
	if err := pbutil.ValidateTestID(result.TestId); err != nil {
		return errors.Annotate(err, "test_id").Err()
	}
	// Failure reason is optional.
	if result.FailureReason != nil {
		if err := pbutil.ValidateFailureReason(result.FailureReason); err != nil {
			return errors.Annotate(err, "failure_reason").Err()
		}
	}
	return nil
}

func validateRuleForCreate(rule *pb.Rule, expectBug bool) error {
	if rule == nil {
		return errors.New("unspecified")
	}
	if expectBug {
		if rule.Bug == nil {
			return errors.New("bug: unspecified")
		}
		b := bugs.BugID{System: rule.Bug.System, ID: rule.Bug.Id}
		if err := b.Validate(); err != nil {
			return errors.Annotate(err, "bug").Err()
		}
	} else {
		if rule.Bug != nil {
			return errors.New("bug: must not be specified, as a new bug will be created by this RPC")
		}
	}
	if rule.SourceCluster != nil {
		cluster := clustering.ClusterID{
			Algorithm: rule.SourceCluster.Algorithm,
			ID:        rule.SourceCluster.Id,
		}
		if !cluster.IsEmpty() {
			if err := cluster.Validate(); err != nil {
				return errors.Annotate(err, "source_cluster").Err()
			}
		}
	}
	if err := rules.ValidateRuleDefinition(rule.RuleDefinition); err != nil {
		return errors.Annotate(err, "rule_definition").Err()
	}
	return nil
}

func validateNewIssue(issue *pb.CreateRuleWithNewIssueRequest_Issue) error {
	if issue == nil {
		return errors.New("unspecified")
	}
	if err := validateMandatoryString(issue.Title, maxTitleLengthBytes); err != nil {
		return errors.Annotate(err, "title").Err()
	}
	if err := validateMandatoryString(issue.Comment, maxCommentLengthBytes); err != nil {
		return errors.Annotate(err, "comment").Err()
	}
	if err := validateBugComponent(issue.Component); err != nil {
		return errors.Annotate(err, "component").Err()
	}
	if err := validateIssueAccessLimit(issue.AccessLimit); err != nil {
		return errors.Annotate(err, "access_limit").Err()
	}
	if err := validateBuganizerPriority(issue.Priority); err != nil {
		return errors.Annotate(err, "priority").Err()
	}
	return nil
}

func validateMandatoryString(s string, maxLengthBytes int) error {
	if s == "" {
		return errors.New("unspecified")
	}
	if len(s) > maxLengthBytes {
		return errors.Reason("longer than %v bytes", maxLengthBytes).Err()
	}
	if !utf8.ValidString(s) {
		return errors.Reason("not a valid utf8 string").Err()
	}
	// W3C recommends Normalization form C is used for all content moving across the internet.
	// Read more about Unicode and normalization:
	// - https://google.aip.dev/210
	// - https://unicode.org/reports/tr15/
	if !norm.NFC.IsNormalString(s) {
		return errors.Reason("not in unicode normalized form C").Err()
	}
	for i, rune := range s {
		if !unicode.IsPrint(rune) {
			return fmt.Errorf("non-printable rune %+q at byte index %d", rune, i)
		}
	}
	return nil
}

func validateBugComponent(b *pb.BugComponent) error {
	if b.System == nil {
		return errors.New("system: unspecified")
	}
	if b.GetMonorail() != nil {
		// Not supported by this RPC.
		return errors.New("monorail: filing bugs into monorail is not supported by this RPC")
	}
	it := b.GetIssueTracker()
	if it == nil {
		return errors.New("issue_tracker: unspecified")
	}
	if it.ComponentId == 0 {
		return errors.New("issue_tracker: component_id: unspecified")
	}
	if it.ComponentId < 0 {
		return errors.New("issue_tracker: component_id: must be positive")
	}
	return nil
}

func validateIssueAccessLimit(limit pb.CreateRuleWithNewIssueRequest_Issue_IssueAccessLimit) error {
	if limit == pb.CreateRuleWithNewIssueRequest_Issue_ISSUE_ACCESS_LIMIT_UNSPECIFIED {
		return errors.New("unspecified")
	}
	if _, ok := pb.CreateRuleWithNewIssueRequest_Issue_IssueAccessLimit_name[int32(limit)]; !ok {
		return errors.New("invalid value, must be a valid IssueAccessLimit")
	}
	return nil
}

func validateBuganizerPriority(priority pb.BuganizerPriority) error {
	if priority == pb.BuganizerPriority_BUGANIZER_PRIORITY_UNSPECIFIED {
		return errors.New("unspecified")
	}
	if _, ok := pb.BuganizerPriority_name[int32(priority)]; !ok {
		return errors.New("invalid value, must be a valid BuganizerPriority")
	}
	return nil
}

func ruleEntryFromRequest(project, ruleID string, r *pb.Rule) *rules.Entry {
	return &rules.Entry{
		Project:        project,
		RuleID:         ruleID,
		RuleDefinition: r.GetRuleDefinition(),
		BugID: bugs.BugID{
			System: r.GetBug().GetSystem(),
			ID:     r.GetBug().GetId(),
		},
		IsActive:              r.GetIsActive(),
		IsManagingBug:         r.GetIsManagingBug(),
		IsManagingBugPriority: r.GetIsManagingBugPriority(),
		SourceCluster: clustering.ClusterID{
			Algorithm: r.GetSourceCluster().GetAlgorithm(),
			ID:        r.GetSourceCluster().GetId(),
		},
		BugManagementState: &bugspb.BugManagementState{},
	}
}

func createRulePB(r *rules.Entry, cfg *configpb.ProjectConfig, mask ruleMask) *pb.Rule {
	definition := ""
	if mask.IncludeDefinition {
		definition = r.RuleDefinition
	}
	creationUser := ""
	lastAuditableUpdateUser := ""
	if mask.IncludeAuditUsers {
		creationUser = r.CreateUser
		lastAuditableUpdateUser = r.LastAuditableUpdateUser
	}
	return &pb.Rule{
		Name:                                ruleName(r.Project, r.RuleID),
		Project:                             r.Project,
		RuleId:                              r.RuleID,
		RuleDefinition:                      definition,
		Bug:                                 createAssociatedBugPB(r.BugID, cfg),
		IsActive:                            r.IsActive,
		IsManagingBug:                       r.IsManagingBug,
		IsManagingBugPriority:               r.IsManagingBugPriority,
		IsManagingBugPriorityLastUpdateTime: timestamppb.New(r.IsManagingBugPriorityLastUpdateTime),
		SourceCluster: &pb.ClusterId{
			Algorithm: r.SourceCluster.Algorithm,
			Id:        r.SourceCluster.ID,
		},
		BugManagementState:      rules.ToExternalBugManagementStatePB(r.BugManagementState),
		CreateTime:              timestamppb.New(r.CreateTime),
		CreateUser:              creationUser,
		LastAuditableUpdateTime: timestamppb.New(r.LastAuditableUpdateTime),
		LastAuditableUpdateUser: lastAuditableUpdateUser,
		LastUpdateTime:          timestamppb.New(r.LastUpdateTime),
		PredicateLastUpdateTime: timestamppb.New(r.PredicateLastUpdateTime),
		Etag:                    ruleETag(r, mask),
	}
}

// ruleETag returns the HTTP ETag for the given rule.
func ruleETag(rule *rules.Entry, mask ruleMask) string {
	definitionFilter := ""
	if mask.IncludeDefinition {
		definitionFilter = "+d"
	}
	userFilter := ""
	if mask.IncludeAuditUsers {
		userFilter = "+u"
	}
	// Encode whether the definition and user were included,
	// so that if the user is granted access to either of
	// these fields, the browser cache is invalidated.
	return fmt.Sprintf(`W/"%s%s/%s"`, definitionFilter, userFilter, rule.LastUpdateTime.UTC().Format(time.RFC3339Nano))
}

// etagRegexp extracts the rule last modified timestamp from a rule ETag.
var etagRegexp = regexp.MustCompile(`^W/"(?:\+[ud])*/(.*)"$`)

// isETagMatching determines if the Etag is consistent with the specified
// Rule version.
func isETagMatching(rule *rules.Entry, etag string) bool {
	m := etagRegexp.FindStringSubmatch(etag)
	if len(m) < 2 {
		return false
	}
	return m[1] == rule.LastUpdateTime.UTC().Format(time.RFC3339Nano)
}

// validateBugAgainstConfig validates the specified bug is consistent with
// the project configuration.
func validateBugAgainstConfig(cfg *configpb.ProjectConfig, bug *pb.AssociatedBug) error {
	switch bug.System {
	case bugs.MonorailSystem:
		b := bugs.BugID{System: bug.System, ID: bug.Id}
		project, _, err := b.MonorailProjectAndID()
		if err != nil {
			return err
		}
		monorailCfg := cfg.BugManagement.GetMonorail()
		if monorailCfg == nil {
			return fmt.Errorf("monorail bug system not enabled for this LUCI project")
		}
		if project != monorailCfg.Project {
			return fmt.Errorf("bug not in expected monorail project (%s)", monorailCfg.Project)
		}
	case bugs.BuganizerSystem:
		// Buganizer bugs are permitted for all LUCI Analysis projects.
	default:
		return fmt.Errorf("system: unsupported bug system: %s", bug.System)
	}
	return nil
}
