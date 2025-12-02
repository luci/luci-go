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

	"go.chromium.org/luci/resultdb/internal/spanutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// TestExonerationRow represents a row in the TestExonerationsV2 Spanner table.
type TestExonerationRow struct {
	ID              ID
	ModuleVariant   *pb.Variant
	CreateTime      time.Time
	Realm           string
	ExplanationHTML string
	Reason          pb.ExonerationReason
}

// Create returns a mutation to insert or update a TestExonerationRow.
func Create(r *TestExonerationRow) *spanner.Mutation {
	if err := r.ID.Validate(); err != nil {
		panic(err)
	}
	if r.ModuleVariant == nil {
		panic("ModuleVariant is required")
	}
	if r.Realm == "" {
		panic("Realm is required")
	}

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
		"CreateTime":            spanner.CommitTimestamp,
		"Realm":                 r.Realm,
		"ExplanationHTML":       spanutil.Compressed(r.ExplanationHTML),
		"Reason":                r.Reason,
	}
	return spanutil.InsertOrUpdateMap("TestExonerationsV2", row)
}
