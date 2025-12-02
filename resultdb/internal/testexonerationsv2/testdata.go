// Copyright 2025 The LUCI Authors.
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

package testexonerationsv2

import (
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// Builder is a builder for TestExonerationRow for testing.
type Builder struct {
	row TestExonerationRow
}

// NewBuilder returns a new builder for a TestExonerationRow for testing.
// The builder is initialized with some default values.
func NewBuilder() *Builder {
	return &Builder{
		row: TestExonerationRow{
			ID: ID{
				RootInvocationShardID: rootinvocations.ShardID{RootInvocationID: "root-invocation", ShardIndex: 0},
				ModuleName:            "testmodule",
				ModuleScheme:          "testscheme",
				ModuleVariantHash:     pbutil.VariantHash(pbutil.Variant("key", "value")),
				CoarseName:            "testcoarsename",
				FineName:              "testfinename",
				CaseName:              "testcasename",
				WorkUnitID:            "testworkunit-id",
				ExonerationID:         "exoneration-id",
			},
			ModuleVariant:   pbutil.Variant("key", "value"),
			CreateTime:      time.Date(2025, 4, 25, 1, 2, 3, 4000, time.UTC),
			Realm:           "testproject:testrealm",
			ExplanationHTML: "<b>explanation</b>",
			Reason:          pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
		},
	}
}

// WithRootInvocationShardID sets the root invocation shard ID.
func (b *Builder) WithRootInvocationShardID(id rootinvocations.ShardID) *Builder {
	b.row.ID.RootInvocationShardID = id
	return b
}

// WithTestIdentifier sets the test identifier.
func (b *Builder) WithTestIdentifier(id *pb.TestIdentifier) *Builder {
	b.row.ID.ModuleName = id.ModuleName
	b.row.ID.ModuleScheme = id.ModuleScheme
	b.row.ID.ModuleVariantHash = pbutil.VariantHash(id.ModuleVariant)
	b.row.ID.CoarseName = id.CoarseName
	b.row.ID.FineName = id.FineName
	b.row.ID.CaseName = id.CaseName
	b.row.ModuleVariant = id.ModuleVariant
	return b
}

// WithWorkUnitID sets the work unit ID.
func (b *Builder) WithWorkUnitID(id string) *Builder {
	b.row.ID.WorkUnitID = id
	return b
}

// WithExonerationID sets the exoneration ID.
func (b *Builder) WithExonerationID(id string) *Builder {
	b.row.ID.ExonerationID = id
	return b
}

// WithCreateTime sets the create time.
func (b *Builder) WithCreateTime(t time.Time) *Builder {
	b.row.CreateTime = t
	return b
}

// WithRealm sets the realm.
func (b *Builder) WithRealm(realm string) *Builder {
	b.row.Realm = realm
	return b
}

// WithExplanationHTML sets the explanation HTML.
func (b *Builder) WithExplanationHTML(html string) *Builder {
	b.row.ExplanationHTML = html
	return b
}

// WithReason sets the exoneration reason.
func (b *Builder) WithReason(reason pb.ExonerationReason) *Builder {
	b.row.Reason = reason
	return b
}

// Build returns the constructed TestExonerationRow.
func (b *Builder) Build() *TestExonerationRow {
	r := b.row
	return &r
}

// InsertForTesting inserts the test exoneration row for testing purposes.
// This differs from Create in that it does not set CreateTime to
// spanner.CommitTimestamp.
func InsertForTesting(r *TestExonerationRow) *spanner.Mutation {
	row := map[string]interface{}{
		"RootInvocationShardId": r.ID.RootInvocationShardID,
		"ModuleName":            r.ID.ModuleName,
		"ModuleScheme":          r.ID.ModuleScheme,
		"ModuleVariantHash":     r.ID.ModuleVariantHash,
		"T1CoarseName":          r.ID.CoarseName,
		"T2FineName":            r.ID.FineName,
		"T3CaseName":            r.ID.CaseName,
		"WorkUnitId":            r.ID.WorkUnitID,
		"ExonerationId":         r.ID.ExonerationID,
		"ModuleVariant":         r.ModuleVariant,
		"CreateTime":            r.CreateTime,
		"Realm":                 r.Realm,
		"ExplanationHTML":       spanutil.Compressed(r.ExplanationHTML),
		"Reason":                r.Reason,
	}
	return spanutil.InsertMap("TestExonerationsV2", row)
}
