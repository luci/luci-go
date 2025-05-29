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
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"

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
		Project:                 testProject,
		RuleID:                  hex.EncodeToString(ruleIDBytes[0:16]),
		RuleDefinition:          "reason LIKE \"%exit code 5%\" AND test LIKE \"tast.arc.%\"",
		IsActive:                true,
		PredicateLastUpdateTime: time.Date(1904, 4, 4, 4, 4, 4, uniqifier, time.UTC),

		BugID:                               bugID,
		IsManagingBug:                       true,
		IsManagingBugPriority:               true,
		IsManagingBugPriorityLastUpdateTime: time.Date(1905, 5, 5, 5, 5, 5, uniqifier, time.UTC),
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
		CreateTime:              time.Date(1900, 1, 2, 3, 4, 5, uniqifier, time.UTC),
		CreateUser:              LUCIAnalysisSystem,
		LastAuditableUpdateTime: time.Date(1907, 7, 7, 7, 7, 7, uniqifier, time.UTC),
		LastAuditableUpdateUser: "user@google.com",
		LastUpdateTime:          time.Date(1909, 9, 9, 9, 9, 9, uniqifier, time.UTC),
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

func (b *RuleBuilder) WithBugPriorityManagedLastUpdateTime(isManagingBugPriorityLastUpdated time.Time) *RuleBuilder {
	b.rule.IsManagingBugPriorityLastUpdateTime = isManagingBugPriorityLastUpdated
	return b
}

// WithBug specifies the bug to use on the rule.
func (b *RuleBuilder) WithBug(bug bugs.BugID) *RuleBuilder {
	b.rule.BugID = bug
	return b
}

// WithCreateTime specifies the creation time of the rule.
func (b *RuleBuilder) WithCreateTime(value time.Time) *RuleBuilder {
	b.rule.CreateTime = value
	return b
}

// WithCreateUser specifies the "created" user on the rule.
func (b *RuleBuilder) WithCreateUser(user string) *RuleBuilder {
	b.rule.CreateUser = user
	return b
}

// WithLastAuditableUpdateTime specifies the "last auditable update" time on the rule.
func (b *RuleBuilder) WithLastAuditableUpdateTime(value time.Time) *RuleBuilder {
	b.rule.LastAuditableUpdateTime = value
	return b
}

// WithLastAuditableUpdateUser specifies the "last updated" user on the rule.
func (b *RuleBuilder) WithLastAuditableUpdateUser(user string) *RuleBuilder {
	b.rule.LastAuditableUpdateUser = user
	return b
}

// WithLastUpdateTime specifies the "last updated" time on the rule.
func (b *RuleBuilder) WithLastUpdateTime(lastUpdated time.Time) *RuleBuilder {
	b.rule.LastUpdateTime = lastUpdated
	return b
}

// WithPredicateLastUpdateTime specifies the "predicate last updated" time on the rule.
func (b *RuleBuilder) WithPredicateLastUpdateTime(value time.Time) *RuleBuilder {
	b.rule.PredicateLastUpdateTime = value
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
	return b.rule.Clone()
}

// SetForTesting replaces the set of stored rules to match the given set.
func SetForTesting(ctx context.Context, t testing.TB, rs []*Entry) error {
	t.Helper()
	testutil.MustApply(ctx, t,
		spanner.Delete("FailureAssociationRules", spanner.AllKeys()))
	// Insert some FailureAssociationRules.
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		for _, r := range rs {
			bugManagementStateBuf, err := proto.Marshal(r.BugManagementState)
			if err != nil {
				return errors.Fmt("marshal bug management state: %w", err)
			}

			ms := spanutil.InsertMap("FailureAssociationRules", map[string]any{
				"Project":        r.Project,
				"RuleId":         r.RuleID,
				"RuleDefinition": r.RuleDefinition,
				// Uses the value 'NULL' to indicate false, and true to indicate true.
				"IsActive":             spanner.NullBool{Bool: r.IsActive, Valid: r.IsActive},
				"PredicateLastUpdated": r.PredicateLastUpdateTime,
				"BugSystem":            r.BugID.System,
				"BugID":                r.BugID.ID,
				// Uses the value 'NULL' to indicate false, and true to indicate true.
				"IsManagingBug":                    spanner.NullBool{Bool: r.IsManagingBug, Valid: r.IsManagingBug},
				"IsManagingBugPriority":            r.IsManagingBugPriority,
				"IsManagingBugPriorityLastUpdated": r.IsManagingBugPriorityLastUpdateTime,
				"SourceClusterAlgorithm":           r.SourceCluster.Algorithm,
				"SourceClusterId":                  r.SourceCluster.ID,
				"BugManagementState":               spanutil.Compress(bugManagementStateBuf),
				"CreationTime":                     r.CreateTime,
				"CreationUser":                     r.CreateUser,
				"LastAuditableUpdate":              r.LastAuditableUpdateTime,
				"LastAuditableUpdateUser":          r.LastAuditableUpdateUser,
				"LastUpdated":                      r.LastUpdateTime,
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
