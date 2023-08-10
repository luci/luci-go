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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"cloud.google.com/go/spanner"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/analysis/internal/bugs"
	bugspb "go.chromium.org/luci/analysis/internal/bugs/proto"
	"go.chromium.org/luci/analysis/internal/clustering"
	spanutil "go.chromium.org/luci/analysis/internal/span"
	"go.chromium.org/luci/analysis/internal/testutil"
)

const testProject = "myproject"

// RuleBuilder provides methods to build a failure asociation rule
// for testing.
type RuleBuilder struct {
	rule      Entry
	uniqifier int
}

// NewRule starts building a new Rule.
func NewRule(uniqifier int) *RuleBuilder {
	ruleIDBytes := sha256.Sum256([]byte(fmt.Sprintf("rule-id%v", uniqifier)))
	var bugID bugs.BugID
	if uniqifier%2 == 0 {
		bugID = bugs.BugID{System: "monorail", ID: fmt.Sprintf("chromium/%v", uniqifier)}
	} else {
		bugID = bugs.BugID{System: "buganizer", ID: fmt.Sprintf("%v", uniqifier)}
	}

	rule := Entry{
		Project:              testProject,
		RuleID:               hex.EncodeToString(ruleIDBytes[0:16]),
		RuleDefinition:       "reason LIKE \"%exit code 5%\" AND test LIKE \"tast.arc.%\"",
		IsActive:             true,
		PredicateLastUpdated: time.Date(1904, 4, 4, 4, 4, 4, uniqifier, time.UTC),

		BugID:                            bugID,
		IsManagingBug:                    true,
		IsManagingBugPriority:            true,
		IsManagingBugPriorityLastUpdated: time.Date(1905, 5, 5, 5, 5, 5, uniqifier, time.UTC),
		SourceCluster: clustering.ClusterID{
			Algorithm: fmt.Sprintf("clusteralg%v-v9", uniqifier),
			ID:        hex.EncodeToString([]byte(fmt.Sprintf("id%v", uniqifier))),
		},
		BugManagementState: &bugspb.BugManagementState{
			RuleAssociationNotified: true,
			PolicyState: map[string]*bugspb.BugManagementState_PolicyState{
				"policy-a": {
					IsActive:           true,
					LastActivationTime: timestamppb.New(time.Date(1908, 8, 8, 8, 8, 8, uniqifier, time.UTC)),
					ActivationNotified: true,
				},
			},
		},
		CreationTime:            time.Date(1900, 1, 2, 3, 4, 5, uniqifier, time.UTC),
		CreationUser:            LUCIAnalysisSystem,
		LastAuditableUpdate:     time.Date(1907, 7, 7, 7, 7, 7, uniqifier, time.UTC),
		LastAuditableUpdateUser: "user@google.com",
		LastUpdated:             time.Date(1909, 9, 9, 9, 9, 9, uniqifier, time.UTC),
	}
	return &RuleBuilder{
		rule:      rule,
		uniqifier: uniqifier,
	}
}

// WithBugSystem specifies the bug system to use on the rule.
func (b *RuleBuilder) WithBugSystem(bugSystem string) *RuleBuilder {
	var bugID bugs.BugID
	if bugSystem == bugs.MonorailSystem {
		bugID = bugs.BugID{System: bugSystem, ID: fmt.Sprintf("chromium/%v", b.uniqifier)}
	} else {
		bugID = bugs.BugID{System: bugSystem, ID: fmt.Sprintf("%v", b.uniqifier)}
	}
	b.rule.BugID = bugID
	return b
}

// WithProject specifies the project to use on the rule.
func (b *RuleBuilder) WithProject(project string) *RuleBuilder {
	b.rule.Project = project
	return b
}

// WithRuleID specifies the Rule ID to use on the rule.
func (b *RuleBuilder) WithRuleID(id string) *RuleBuilder {
	b.rule.RuleID = id
	return b
}

// WithActive specifies whether the rule will be active.
func (b *RuleBuilder) WithActive(active bool) *RuleBuilder {
	b.rule.IsActive = active
	return b
}

// WithBugManaged specifies whether the rule's bug will be managed by
// LUCI Analysis.
func (b *RuleBuilder) WithBugManaged(value bool) *RuleBuilder {
	b.rule.IsManagingBug = value
	return b
}

// WithBugPriorityManaged determines whether the rule's bug's priority
// is managed by LUCI Analysis.
func (b *RuleBuilder) WithBugPriorityManaged(isPriorityManaged bool) *RuleBuilder {
	b.rule.IsManagingBugPriority = isPriorityManaged
	return b
}

func (b *RuleBuilder) WithBugPriorityManagedLastUpdated(isManagingBugPriorityLastUpdated time.Time) *RuleBuilder {
	b.rule.IsManagingBugPriorityLastUpdated = isManagingBugPriorityLastUpdated
	return b
}

// WithBug specifies the bug to use on the rule.
func (b *RuleBuilder) WithBug(bug bugs.BugID) *RuleBuilder {
	b.rule.BugID = bug
	return b
}

// WithCreationTime specifies the creation time of the rule.
func (b *RuleBuilder) WithCreationTime(value time.Time) *RuleBuilder {
	b.rule.CreationTime = value
	return b
}

// WithCreationUser specifies the "created" user on the rule.
func (b *RuleBuilder) WithCreationUser(user string) *RuleBuilder {
	b.rule.CreationUser = user
	return b
}

// WithLastAuditableUpdate specifies the "last auditable update" time on the rule.
func (b *RuleBuilder) WithLastAuditableUpdate(value time.Time) *RuleBuilder {
	b.rule.LastAuditableUpdate = value
	return b
}

// WithLastAuditableUpdateUser specifies the "last updated" user on the rule.
func (b *RuleBuilder) WithLastAuditableUpdateUser(user string) *RuleBuilder {
	b.rule.LastAuditableUpdateUser = user
	return b
}

// WithLastUpdated specifies the "last updated" time on the rule.
func (b *RuleBuilder) WithLastUpdated(lastUpdated time.Time) *RuleBuilder {
	b.rule.LastUpdated = lastUpdated
	return b
}

// WithPredicateLastUpdated specifies the "predicate last updated" time on the rule.
func (b *RuleBuilder) WithPredicateLastUpdated(value time.Time) *RuleBuilder {
	b.rule.PredicateLastUpdated = value
	return b
}

// WithRuleDefinition specifies the definition of the rule.
func (b *RuleBuilder) WithRuleDefinition(definition string) *RuleBuilder {
	b.rule.RuleDefinition = definition
	return b
}

// WithSourceCluster specifies the source suggested cluster that triggered the
// creation of the rule.
func (b *RuleBuilder) WithSourceCluster(value clustering.ClusterID) *RuleBuilder {
	b.rule.SourceCluster = value
	return b
}

// WithBugManagementState specifies the bug management state for the rule.
func (b *RuleBuilder) WithBugManagementState(state *bugspb.BugManagementState) *RuleBuilder {
	b.rule.BugManagementState = proto.Clone(state).(*bugspb.BugManagementState)
	return b
}

func (b *RuleBuilder) Build() *Entry {
	// Copy the result, so that calling further methods on the builder does
	// not change the returned rule.
	result := new(Entry)
	*result = b.rule
	return result
}

// SetForTesting replaces the set of stored rules to match the given set.
func SetForTesting(ctx context.Context, rs []*Entry) error {
	testutil.MustApply(ctx,
		spanner.Delete("FailureAssociationRules", spanner.AllKeys()))
	// Insert some FailureAssociationRules.
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		for _, r := range rs {
			bugManagementStateBuf, err := proto.Marshal(r.BugManagementState)
			if err != nil {
				return errors.Annotate(err, "marshal bug management state").Err()
			}

			ms := spanutil.InsertMap("FailureAssociationRules", map[string]any{
				"Project":        r.Project,
				"RuleId":         r.RuleID,
				"RuleDefinition": r.RuleDefinition,
				// Uses the value 'NULL' to indicate false, and true to indicate true.
				"IsActive":             spanner.NullBool{Bool: r.IsActive, Valid: r.IsActive},
				"PredicateLastUpdated": r.PredicateLastUpdated,
				"BugSystem":            r.BugID.System,
				"BugID":                r.BugID.ID,
				// Uses the value 'NULL' to indicate false, and true to indicate true.
				"IsManagingBug":                    spanner.NullBool{Bool: r.IsManagingBug, Valid: r.IsManagingBug},
				"IsManagingBugPriority":            r.IsManagingBugPriority,
				"IsManagingBugPriorityLastUpdated": r.IsManagingBugPriorityLastUpdated,
				"SourceClusterAlgorithm":           r.SourceCluster.Algorithm,
				"SourceClusterId":                  r.SourceCluster.ID,
				"BugManagementState":               spanutil.Compress(bugManagementStateBuf),
				"CreationTime":                     r.CreationTime,
				"CreationUser":                     r.CreationUser,
				"LastAuditableUpdate":              r.LastAuditableUpdate,
				"LastAuditableUpdateUser":          r.LastAuditableUpdateUser,
				"LastUpdated":                      r.LastUpdated,
			})
			span.BufferWrite(ctx, ms)
		}
		return nil
	})
	return err
}

func sortByID(rules []*Entry) {
	sort.Slice(rules, func(i, j int) bool { return rules[i].RuleID < rules[j].RuleID })
}
