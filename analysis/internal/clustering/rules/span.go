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

// Package rules contains methods to read and write failure association rules.
package rules

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/bugs"
	bugspb "go.chromium.org/luci/analysis/internal/bugs/proto"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/clustering/rules/lang"
	spanutil "go.chromium.org/luci/analysis/internal/span"
	"go.chromium.org/luci/analysis/pbutil"
)

// RuleIDRe is the regular expression pattern that matches validly
// formed rule IDs.
const RuleIDRePattern = `[0-9a-f]{32}`

// PolicyIDRePattern is the regular expression pattern that matches
// validly formed bug management policy IDs.
// Designed to conform to google.aip.dev/122 resource ID requirements.
const PolicyIDRePattern = `[a-z]([a-z0-9-]{0,62}[a-z0-9])?`

// MaxRuleDefinitionLength is the maximum length of a rule definition.
const MaxRuleDefinitionLength = 65536

// RuleIDRe matches validly formed rule IDs.
var RuleIDRe = regexp.MustCompile(`^` + RuleIDRePattern + `$`)

// PolicyIDRe matches validly formed bug management policy IDs.
var PolicyIDRe = regexp.MustCompile(`^` + PolicyIDRePattern + `$`)

// UserRe matches valid users. These are email addresses or the special
// value "system".
var UserRe = regexp.MustCompile(`^system|([a-zA-Z0-9_.+-]+@[a-zA-Z0-9-]+\.[a-zA-Z0-9-.]+)$`)

// LUCIAnalysisSystem is the special user that identifies changes made by the
// LUCI Analysis system itself in audit fields.
const LUCIAnalysisSystem = "system"

// StartingEpoch is the rule last updated time used for projects that have
// no rules (active or otherwise). It is deliberately different from the
// timestamp zero value to be discernible from "timestamp not populated"
// programming errors.
var StartingEpoch = time.Date(1900, time.January, 1, 0, 0, 0, 0, time.UTC)

// StartingEpoch is the rule version used for projects that have
// no rules (active or otherwise).
var StartingVersion = Version{
	Predicates: StartingEpoch,
	Total:      StartingEpoch,
}

// NotExistsErr is returned by Read methods for a single failure
// association rule, if no matching rule exists.
var NotExistsErr = errors.New("no matching rule exists")

// Entry represents a failure association rule (a rule which associates
// failures with a bug). When the rule is used to match incoming test
// failures, the resultant cluster is known as a 'bug cluster' because
// the cluster is associated with a bug (via the failure association rule).
type Entry struct {
	// Identity fields.

	// The LUCI Project for which this rule is defined.
	Project string
	// The unique identifier for the failure association rule,
	// as 32 lowercase hexadecimal characters.
	RuleID string

	// Failure predicate fields (which failures are matched by the rule).

	// The rule predicate, defining which failures are being associated.
	RuleDefinition string
	// Whether the bug should be updated by LUCI Analysis, and whether failures
	// should still be matched against the rule.
	IsActive bool
	// The time the rule was last updated in a way that caused the
	// matched failures to change, i.e. because of a change to RuleDefinition
	// or IsActive. (By contrast, updating BugID does NOT change
	// the matched failures, so does NOT update this field.)
	// When this value changes, it triggers re-clustering.
	// Compare with RulesVersion on ReclusteringRuns to identify
	// reclustering state.
	// Output only.
	PredicateLastUpdateTime time.Time

	// Bug fields.

	// BugID is the identifier of the bug that the failures are
	// associated with.
	BugID bugs.BugID
	// Whether this rule should manage the priority and verified status
	// of the associated bug based on the impact of the cluster defined
	// by this rule.
	IsManagingBug bool
	// Whether the bug priority should be updated based on the cluster's impact.
	// This flag is effective only if the IsManagingBug is true.
	// The default value will be false.
	IsManagingBugPriority bool
	// Tracks the last time the field `IsManagingBugPriority` was updated.
	// Defaults to nil which means the field was never updated.
	IsManagingBugPriorityLastUpdateTime time.Time

	// Immutable data.

	// The suggested cluster this rule was created from (if any).
	// Until re-clustering is complete and has reduced the residual impact
	// of the source cluster, this cluster ID tells bug filing to ignore
	// the source cluster when determining whether new bugs need to be filed.
	SourceCluster clustering.ClusterID

	// System-controlled data.

	// State used to control automatic bug management.
	// Invariant: BugManagementState != nil
	BugManagementState *bugspb.BugManagementState

	// Audit fields.

	// The time the rule was created. Output only.
	CreateTime time.Time
	// The user which created the rule. Output only.
	CreateUser string
	// The last time an auditable field was updated. An auditable field
	// is any field other than a system controlled data field. Output only.
	LastAuditableUpdateTime time.Time
	// The last user who updated an auditable field. An auditable field
	// is any field other than a system controlled data field. Output only.
	LastAuditableUpdateUser string
	// The time the rule was last updated. Output only.
	LastUpdateTime time.Time
}

// Clone makes a deep copy of the rule.
func (r Entry) Clone() *Entry {
	result := &Entry{}
	*result = r
	result.BugManagementState = proto.Clone(result.BugManagementState).(*bugspb.BugManagementState)
	return result
}

// Read reads the failure association rule with the given rule ID.
// If no rule exists, NotExistsErr will be returned.
func Read(ctx context.Context, project string, id string) (*Entry, error) {
	whereClause := `Project = @project AND RuleId = @ruleId`
	params := map[string]any{
		"project": project,
		"ruleId":  id,
	}
	rs, err := readWhere(ctx, whereClause, params)
	if err != nil {
		return nil, errors.Annotate(err, "query rule by id").Err()
	}
	if len(rs) == 0 {
		return nil, NotExistsErr
	}
	return rs[0], nil
}

// ReadAllForTesting reads all LUCI Analysis failure association rules.
// This method is not expected to scale -- for testing use only.
func ReadAllForTesting(ctx context.Context) ([]*Entry, error) {
	whereClause := `TRUE`
	params := map[string]any{}
	rs, err := readWhere(ctx, whereClause, params)
	if err != nil {
		return nil, errors.Annotate(err, "query all rules").Err()
	}
	return rs, nil
}

// ReadActive reads all active LUCI Analysis failure association rules in
// the given LUCI project.
func ReadActive(ctx context.Context, project string) ([]*Entry, error) {
	whereClause := `Project = @project AND IsActive`
	params := map[string]any{
		"project": project,
	}
	rs, err := readWhere(ctx, whereClause, params)
	if err != nil {
		return nil, errors.Annotate(err, "query active rules").Err()
	}
	return rs, nil
}

// ReadWithMonorailForProject reads all LUCI Analysis failure association rules
// using monorail in the given LUCI Project.
//
// This RPC was introduced for the specific purpose of supporting monorail
// migration for projects. As rules are never deleted by LUCI Analysis, and
// this method does no pagination, this style of method will cease to scale
// at some point.
//
// It has implemented this way only because monorail has a limited remaining life
// and we seem able to get away with a pagination-free approach for now.
func ReadWithMonorailForProject(ctx context.Context, project string) ([]*Entry, error) {
	whereClause := `Project = @project AND BugSystem = 'monorail'`
	params := map[string]any{
		"project": project,
	}
	rs, err := readWhere(ctx, whereClause, params)
	if err != nil {
		return nil, errors.Annotate(err, "query monorail rules for project").Err()
	}
	return rs, nil
}

// ReadByBug reads the failure association rules associated with the given bug.
// At most one rule will be returned per project.
func ReadByBug(ctx context.Context, bugID bugs.BugID) ([]*Entry, error) {
	whereClause := `BugSystem = @bugSystem and BugId = @bugId`
	params := map[string]any{
		"bugSystem": bugID.System,
		"bugId":     bugID.ID,
	}
	rs, err := readWhere(ctx, whereClause, params)
	if err != nil {
		return nil, errors.Annotate(err, "query rule by bug").Err()
	}
	return rs, nil
}

// ReadDelta reads the changed failure association rules since the given
// timestamp, in the given LUCI project.
func ReadDelta(ctx context.Context, project string, sinceTime time.Time) ([]*Entry, error) {
	if sinceTime.Before(StartingEpoch) {
		return nil, errors.New("cannot query rule deltas from before project inception")
	}
	whereClause := `Project = @project AND LastUpdated > @sinceTime`
	params := map[string]any{
		"project":   project,
		"sinceTime": sinceTime,
	}
	rs, err := readWhere(ctx, whereClause, params)
	if err != nil {
		return nil, errors.Annotate(err, "query rules since").Err()
	}
	return rs, nil
}

// ReadDeltaAllProjects reads the changed failure association rules since the given
// timestamp for all LUCI projects.
func ReadDeltaAllProjects(ctx context.Context, sinceTime time.Time) ([]*Entry, error) {
	if sinceTime.Before(StartingEpoch) {
		return nil, errors.New("cannot query rule deltas from before project inception")
	}
	whereClause := `LastUpdated > @sinceTime`
	params := map[string]any{"sinceTime": sinceTime}

	rs, err := readWhere(ctx, whereClause, params)
	if err != nil {
		return nil, errors.Annotate(err, "query rules since").Err()
	}
	return rs, nil
}

// ReadMany reads the failure association rules with the given rule IDs.
// The returned slice of rules will correspond one-to-one the IDs requested
// (so returned[i].RuleId == ids[i], assuming the rule exists, else
// returned[i] == nil). If a rule does not exist, a value of nil will be
// returned for that ID. The same rule can be requested multiple times.
func ReadMany(ctx context.Context, project string, ids []string) ([]*Entry, error) {
	whereClause := `Project = @project AND RuleId IN UNNEST(@ruleIds)`
	params := map[string]any{
		"project": project,
		"ruleIds": ids,
	}
	rs, err := readWhere(ctx, whereClause, params)
	if err != nil {
		return nil, errors.Annotate(err, "query rules by id").Err()
	}
	ruleByID := make(map[string]Entry)
	for _, r := range rs {
		ruleByID[r.RuleID] = *r
	}
	var result []*Entry
	for _, id := range ids {
		var entry *Entry
		rule, ok := ruleByID[id]
		if ok {
			// Copy the rule to ensure the rules in the result
			// are not aliased, even if the same rule ID is requested
			// multiple times.
			entry = rule.Clone()
		}
		result = append(result, entry)
	}
	return result, nil
}

// readWhere failure association rules matching the given where clause,
// substituting params for any SQL parameters used in that clause.
func readWhere(ctx context.Context, whereClause string, params map[string]any) ([]*Entry, error) {
	stmt := spanner.NewStatement(`
		SELECT
			Project, RuleId,
			RuleDefinition,
			IsActive,
			PredicateLastUpdated,
			BugSystem, BugId,
		  IsManagingBug,
			IsManagingBugPriority,
			IsManagingBugPriorityLastUpdated,
		  SourceClusterAlgorithm, SourceClusterId,
			BugManagementState,
			CreationTime, LastUpdated,
		  CreationUser,
			LastAuditableUpdate,
			LastAuditableUpdateUser
		FROM FailureAssociationRules
		WHERE (` + whereClause + `)
		ORDER BY BugSystem, BugId, Project
	`)
	stmt.Params = params

	it := span.Query(ctx, stmt)
	rs := []*Entry{}
	var b spanutil.Buffer
	err := it.Do(func(r *spanner.Row) error {
		var project, ruleID string
		var ruleDefinition string
		var isActive spanner.NullBool
		var predicateLastUpdated time.Time
		var bugSystem, bugID string
		var isManagingBug spanner.NullBool
		var isManagingBugPriority bool
		var isManagingBugPriorityLastUpdated spanner.NullTime
		var sourceClusterAlgorithm, sourceClusterID string
		var bugManagementStateCompressed spanutil.Compressed
		var creationTime, lastUpdated time.Time
		var creationUser string
		var lastAuditableUpdate spanner.NullTime
		var lastAuditableUpdateUser spanner.NullString

		err := b.FromSpanner(r,
			&project, &ruleID,
			&ruleDefinition,
			&isActive,
			&predicateLastUpdated,
			&bugSystem, &bugID,
			&isManagingBug,
			&isManagingBugPriority,
			&isManagingBugPriorityLastUpdated,
			&sourceClusterAlgorithm, &sourceClusterID,
			&bugManagementStateCompressed,
			&creationTime, &lastUpdated,
			&creationUser,
			&lastAuditableUpdate,
			&lastAuditableUpdateUser,
		)
		if err != nil {
			return errors.Annotate(err, "read rule row").Err()
		}

		bugManagementState := &bugspb.BugManagementState{}
		if len(bugManagementStateCompressed) > 0 {
			if err := proto.Unmarshal(bugManagementStateCompressed, bugManagementState); err != nil {
				return errors.Annotate(err, "unmarshal bug management state").Err()
			}
		}

		lastAuditableUpdateTime := lastAuditableUpdate.Time
		if !lastAuditableUpdate.Valid {
			// Some rows may have been created before this field existed. Treat
			// all previous updates as auditable updates.
			lastAuditableUpdateTime = lastUpdated
		}

		rule := &Entry{
			Project:                             project,
			RuleID:                              ruleID,
			RuleDefinition:                      ruleDefinition,
			IsActive:                            isActive.Valid && isActive.Bool,
			PredicateLastUpdateTime:             predicateLastUpdated,
			BugID:                               bugs.BugID{System: bugSystem, ID: bugID},
			IsManagingBug:                       isManagingBug.Valid && isManagingBug.Bool,
			IsManagingBugPriority:               isManagingBugPriority,
			IsManagingBugPriorityLastUpdateTime: isManagingBugPriorityLastUpdated.Time,
			SourceCluster: clustering.ClusterID{
				Algorithm: sourceClusterAlgorithm,
				ID:        sourceClusterID,
			},
			BugManagementState:      bugManagementState,
			CreateTime:              creationTime,
			CreateUser:              creationUser,
			LastAuditableUpdateTime: lastAuditableUpdateTime,
			LastAuditableUpdateUser: lastAuditableUpdateUser.StringVal,
			LastUpdateTime:          lastUpdated,
		}
		rs = append(rs, rule)
		return nil
	})
	return rs, err
}

// Version captures version information about a project's rules.
type Version struct {
	// Predicates is the last time any rule changed its
	// rule predicate (RuleDefinition or IsActive).
	// Also known as "Rules Version" in clustering contexts.
	Predicates time.Time
	// Total is the last time any rule was updated in any way.
	// Pass to ReadDelta when seeking to read changed rules.
	Total time.Time
}

// ReadVersion reads information about when rules in the given project
// were last updated. This is used to version the set of rules retrieved
// by ReadActive and is typically called in the same transaction.
// It is also used to implement change detection on rule predicates
// for the purpose of triggering re-clustering.
//
// Simply reading the last LastUpdated time of the rules read by ReadActive
// is not sufficient to version the set of rules read, as the most recent
// update may have been to mark a rule inactive (removing it from the set
// that is read).
//
// If the project has no failure association rules, the timestamp
// StartingEpoch is returned.
func ReadVersion(ctx context.Context, projectID string) (Version, error) {
	stmt := spanner.NewStatement(`
		SELECT
		  Max(PredicateLastUpdated) as PredicateLastUpdated,
		  MAX(LastUpdated) as LastUpdated
		FROM FailureAssociationRules
		WHERE Project = @projectID
	`)
	stmt.Params = map[string]any{
		"projectID": projectID,
	}
	var predicateLastUpdated, lastUpdated spanner.NullTime
	it := span.Query(ctx, stmt)
	err := it.Do(func(r *spanner.Row) error {
		err := r.Columns(&predicateLastUpdated, &lastUpdated)
		if err != nil {
			return errors.Annotate(err, "read last updated row").Err()
		}
		return nil
	})
	if err != nil {
		return Version{}, errors.Annotate(err, "query last updated").Err()
	}
	result := Version{
		Predicates: StartingEpoch,
		Total:      StartingEpoch,
	}
	// predicateLastUpdated / lastUpdated are only invalid if there
	// are no failure association rules.
	if predicateLastUpdated.Valid {
		result.Predicates = predicateLastUpdated.Time
	}
	if lastUpdated.Valid {
		result.Total = lastUpdated.Time
	}
	return result, nil
}

// ReadTotalActiveRules reads the number active rules, for each LUCI Project.
// Only returns entries for projects that have any rules (at all). Combine
// with config if you need zero entries for projects that are defined but
// have no rules.
func ReadTotalActiveRules(ctx context.Context) (map[string]int64, error) {
	stmt := spanner.NewStatement(`
		SELECT
		  project,
		  COUNTIF(IsActive) as active_rules,
		FROM FailureAssociationRules
		GROUP BY project
	`)
	result := make(map[string]int64)
	it := span.Query(ctx, stmt)
	err := it.Do(func(r *spanner.Row) error {
		var project string
		var activeRules int64
		err := r.Columns(&project, &activeRules)
		if err != nil {
			return errors.Annotate(err, "read row").Err()
		}
		result[project] = activeRules
		return nil
	})
	if err != nil {
		return nil, errors.Annotate(err, "query total active rules by project").Err()
	}
	return result, nil
}

// Create creates a mutation to insert a new failure association rule.
func Create(rule *Entry, user string) (*spanner.Mutation, error) {
	if err := validateRule(rule); err != nil {
		return nil, err
	}
	if err := validateUser(user); err != nil {
		return nil, err
	}

	bugManagementStateBuf, err := proto.Marshal(rule.BugManagementState)
	if err != nil {
		return nil, errors.Annotate(err, "marshal bug management state").Err()
	}

	ms := spanutil.InsertMap("FailureAssociationRules", map[string]any{
		"Project":        rule.Project,
		"RuleId":         rule.RuleID,
		"RuleDefinition": rule.RuleDefinition,
		// IsActive uses the value NULL to indicate false, and TRUE to indicate true.
		"IsActive":             spanner.NullBool{Bool: rule.IsActive, Valid: rule.IsActive},
		"PredicateLastUpdated": spanner.CommitTimestamp,
		"BugSystem":            rule.BugID.System,
		"BugId":                rule.BugID.ID,
		// IsManagingBug uses the value NULL to indicate false, and TRUE to indicate true.
		"IsManagingBug":                    spanner.NullBool{Bool: rule.IsManagingBug, Valid: rule.IsManagingBug},
		"IsManagingBugPriority":            rule.IsManagingBugPriority,
		"IsManagingBugPriorityLastUpdated": spanner.CommitTimestamp,
		"SourceClusterAlgorithm":           rule.SourceCluster.Algorithm,
		"SourceClusterId":                  rule.SourceCluster.ID,
		"BugManagementState":               spanutil.Compress(bugManagementStateBuf),
		"CreationTime":                     spanner.CommitTimestamp,
		"CreationUser":                     user,
		"LastAuditableUpdate":              spanner.CommitTimestamp,
		"LastAuditableUpdateUser":          user,
		"LastUpdated":                      spanner.CommitTimestamp,
	})
	return ms, nil
}

// UpdateOptions are the options that are using during
// the update of a rule.
type UpdateOptions struct {
	// IsAuditableUpdate should be set if any auditable field
	// (that is, any field other than a system-controlled data field)
	// has been updated.
	// If set, the values of these fields will be saved and
	// LastAuditableUpdate and LastAuditableUpdateUser will be updated.
	IsAuditableUpdate bool
	// PredicateUpdated should be set if IsActive and/or RuleDefinition
	// have been updated.
	// If set, these new values of these fields will be saved and
	// PredicateLastUpdated will be updated.
	PredicateUpdated bool
	// IsManagingBugPriorityUpdated should be set if IsManagingBugPriority
	// has been updated.
	// If set, the IsManagingBugPriorityLastUpdated will be updated.
	IsManagingBugPriorityUpdated bool
}

// Update creates a mutation to update an existing failure association rule.
// Correctly specify the nature of your changes in UpdateOptions to ensure
// those changes are saved and that audit timestamps are updated.
func Update(rule *Entry, options UpdateOptions, user string) (*spanner.Mutation, error) {
	if err := validateRule(rule); err != nil {
		return nil, err
	}
	if err := validateUser(user); err != nil {
		return nil, err
	}
	if options.PredicateUpdated && !options.IsAuditableUpdate {
		return nil, errors.Reason("predicate updates are auditable updates, did you forget to set IsAuditableUpdate?").Err()
	}
	if options.IsManagingBugPriorityUpdated && !options.IsAuditableUpdate {
		return nil, errors.Reason("is managing bug priority updates are auditable updates, did you forget to set IsAuditableUpdate?").Err()
	}

	bugManagementStateBuf, err := proto.Marshal(rule.BugManagementState)
	if err != nil {
		return nil, errors.Annotate(err, "marshal bug management state").Err()
	}

	update := map[string]any{
		"Project":            rule.Project,
		"RuleId":             rule.RuleID,
		"BugManagementState": spanutil.Compress(bugManagementStateBuf),
		"LastUpdated":        spanner.CommitTimestamp,
	}
	if options.IsAuditableUpdate {
		update["BugSystem"] = rule.BugID.System
		update["BugId"] = rule.BugID.ID
		// IsManagingBug uses the value NULL to indicate false, and TRUE to indicate true.
		update["IsManagingBug"] = spanner.NullBool{Bool: rule.IsManagingBug, Valid: rule.IsManagingBug}
		update["LastAuditableUpdate"] = spanner.CommitTimestamp
		update["LastAuditableUpdateUser"] = user

		if options.PredicateUpdated {
			update["RuleDefinition"] = rule.RuleDefinition
			// IsActive uses the value NULL to indicate false, and TRUE to indicate true.
			update["IsActive"] = spanner.NullBool{Bool: rule.IsActive, Valid: rule.IsActive}
			update["PredicateLastUpdated"] = spanner.CommitTimestamp
		}
		if options.IsManagingBugPriorityUpdated {
			update["IsManagingBugPriority"] = rule.IsManagingBugPriority
			update["IsManagingBugPriorityLastUpdated"] = spanner.CommitTimestamp
		}
	}
	ms := spanutil.UpdateMap("FailureAssociationRules", update)
	return ms, nil
}

// validateRule performs simple validation on the fields of a rule.
// Complex validation (validation against project configuration or
// to check rule is not for the same bug as another rule in the same
// project) is not performed.
func validateRule(r *Entry) error {
	if err := pbutil.ValidateProject(r.Project); err != nil {
		return errors.Annotate(err, "project").Err()
	}
	if !RuleIDRe.MatchString(r.RuleID) {
		return errors.Reason("rule ID: must match %s", RuleIDRe).Err()
	}
	if err := r.BugID.Validate(); err != nil {
		return errors.Annotate(r.BugID.Validate(), "bug ID").Err()
	}
	if r.SourceCluster.Validate() != nil && !r.SourceCluster.IsEmpty() {
		return errors.Annotate(r.SourceCluster.Validate(), "source cluster ID").Err()
	}
	if err := ValidateRuleDefinition(r.RuleDefinition); err != nil {
		return errors.Annotate(err, "rule definition").Err()
	}
	if err := validateBugManagementState(r.BugManagementState); err != nil {
		return errors.Annotate(err, "bug management state").Err()
	}
	return nil
}

func ValidateRuleDefinition(ruleDefinition string) error {
	if ruleDefinition == "" {
		return errors.New("unspecified")
	}
	if len(ruleDefinition) > MaxRuleDefinitionLength {
		return errors.Reason("exceeds maximum length of %v", MaxRuleDefinitionLength).Err()
	}
	_, err := lang.Parse(ruleDefinition)
	if err != nil {
		return errors.Annotate(err, "parse").Err()
	}
	return nil
}

func validateBugManagementState(state *bugspb.BugManagementState) error {
	if state == nil {
		return errors.Reason("must be set").Err()
	}
	for policy, state := range state.PolicyState {
		if !PolicyIDRe.MatchString(policy) {
			return errors.Reason("policy_state[%q]: key must match pattern %s", policy, PolicyIDRe).Err()
		}
		if state.LastActivationTime != nil {
			if err := state.LastActivationTime.CheckValid(); err != nil {
				return errors.Annotate(err, "policy_state[%q]: last_activation_time", policy).Err()
			}
		}
		if state.LastDeactivationTime != nil {
			if err := state.LastDeactivationTime.CheckValid(); err != nil {
				return errors.Annotate(err, "policy_state[%q]: last_deactivation_time", policy).Err()
			}
		}
	}
	return nil
}

func validateUser(u string) error {
	if !UserRe.MatchString(u) {
		return errors.New("user must be valid")
	}
	return nil
}

// GenerateID returns a random 128-bit rule ID, encoded as
// 32 lowercase hexadecimal characters.
func GenerateID() (string, error) {
	randomBytes := make([]byte, 16)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(randomBytes), nil
}
