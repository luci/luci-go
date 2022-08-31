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
	"time"

	"cloud.google.com/go/spanner"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/bugs"
	"go.chromium.org/luci/analysis/internal/clustering"
	spanutil "go.chromium.org/luci/analysis/internal/span"
	"go.chromium.org/luci/analysis/internal/testutil"
)

const testProject = "myproject"

// RuleBuilder provides methods to build a failure asociation rule
// for testing.
type RuleBuilder struct {
	rule FailureAssociationRule
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

	rule := FailureAssociationRule{
		Project:              testProject,
		RuleID:               hex.EncodeToString(ruleIDBytes[0:16]),
		RuleDefinition:       "reason LIKE \"%exit code 5%\" AND test LIKE \"tast.arc.%\"",
		BugID:                bugID,
		IsActive:             true,
		IsManagingBug:        true,
		CreationTime:         time.Date(1900, 1, 2, 3, 4, 5, uniqifier, time.UTC),
		CreationUser:         LUCIAnalysisSystem,
		LastUpdated:          time.Date(1900, 1, 2, 3, 4, 7, uniqifier, time.UTC),
		LastUpdatedUser:      "user@google.com",
		PredicateLastUpdated: time.Date(1900, 1, 2, 3, 4, 6, uniqifier, time.UTC),
		SourceCluster: clustering.ClusterID{
			Algorithm: fmt.Sprintf("clusteralg%v-v9", uniqifier),
			ID:        hex.EncodeToString([]byte(fmt.Sprintf("id%v", uniqifier))),
		},
	}
	return &RuleBuilder{
		rule: rule,
	}
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

// WithLastUpdated specifies the "last updated" time on the rule.
func (b *RuleBuilder) WithLastUpdated(lastUpdated time.Time) *RuleBuilder {
	b.rule.LastUpdated = lastUpdated
	return b
}

// WithLastUpdatedUser specifies the "last updated" user on the rule.
func (b *RuleBuilder) WithLastUpdatedUser(user string) *RuleBuilder {
	b.rule.LastUpdatedUser = user
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

func (b *RuleBuilder) Build() *FailureAssociationRule {
	// Copy the result, so that calling further methods on the builder does
	// not change the returned rule.
	result := new(FailureAssociationRule)
	*result = b.rule
	return result
}

// SetRulesForTesting replaces the set of stored rules to match the given set.
func SetRulesForTesting(ctx context.Context, rs []*FailureAssociationRule) error {
	testutil.MustApply(ctx,
		spanner.Delete("FailureAssociationRules", spanner.AllKeys()))
	// Insert some FailureAssociationRules.
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		for _, r := range rs {
			ms := spanutil.InsertMap("FailureAssociationRules", map[string]interface{}{
				"Project":              r.Project,
				"RuleId":               r.RuleID,
				"RuleDefinition":       r.RuleDefinition,
				"CreationTime":         r.CreationTime,
				"CreationUser":         r.CreationUser,
				"LastUpdated":          r.LastUpdated,
				"LastUpdatedUser":      r.LastUpdatedUser,
				"BugSystem":            r.BugID.System,
				"BugID":                r.BugID.ID,
				"PredicateLastUpdated": r.PredicateLastUpdated,
				// Uses the value 'NULL' to indicate false, and true to indicate true.
				"IsActive":               spanner.NullBool{Bool: r.IsActive, Valid: r.IsActive},
				"IsManagingBug":          spanner.NullBool{Bool: r.IsManagingBug, Valid: r.IsManagingBug},
				"SourceClusterAlgorithm": r.SourceCluster.Algorithm,
				"SourceClusterId":        r.SourceCluster.ID,
			})
			span.BufferWrite(ctx, ms)
		}
		return nil
	})
	return err
}
