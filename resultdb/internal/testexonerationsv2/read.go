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
	"context"
	"sort"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// ReadAllForTesting reads all test exoneration rows from Spanner for testing.
// This method should not be used in production as it does not scale.
func ReadAllForTesting(ctx context.Context) ([]*TestExonerationRow, error) {
	var rows []*TestExonerationRow
	var b spanutil.Buffer
	err := span.Read(ctx, "TestExonerationsV2", spanner.AllKeys(), []string{
		"RootInvocationShardId",
		"ModuleName",
		"ModuleScheme",
		"ModuleVariantHash",
		"T1CoarseName",
		"T2FineName",
		"T3CaseName",
		"WorkUnitId",
		"ExonerationId",
		"ModuleVariant",
		"CreateTime",
		"Realm",
		"ExplanationHTML",
		"Reason",
	}).Do(func(r *spanner.Row) error {
		row := &TestExonerationRow{}
		var explanationHTML spanutil.Compressed
		var moduleVariant *pb.Variant
		err := b.FromSpanner(r,
			&row.ID.RootInvocationShardID,
			&row.ID.ModuleName,
			&row.ID.ModuleScheme,
			&row.ID.ModuleVariantHash,
			&row.ID.CoarseName,
			&row.ID.FineName,
			&row.ID.CaseName,
			&row.ID.WorkUnitID,
			&row.ID.ExonerationID,
			&moduleVariant,
			&row.CreateTime,
			&row.Realm,
			&explanationHTML,
			&row.Reason,
		)
		if err != nil {
			return err
		}
		row.ExplanationHTML = string(explanationHTML)
		row.ModuleVariant = moduleVariant
		rows = append(rows, row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Sort in ascending table order.
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].ID.Before(rows[j].ID)
	})
	return rows, nil
}

// ToProto converts a TestExonerationRow to an unmasked TestExoneration proto.
func (r *TestExonerationRow) ToProto() *pb.TestExoneration {
	testID := pbutil.EncodeTestID(pbutil.BaseTestIdentifier{
		ModuleName:   r.ID.ModuleName,
		ModuleScheme: r.ID.ModuleScheme,
		CoarseName:   r.ID.CoarseName,
		FineName:     r.ID.FineName,
		CaseName:     r.ID.CaseName,
	})

	return &pb.TestExoneration{
		Name: pbutil.TestExonerationName(string(r.ID.RootInvocationShardID.RootInvocationID), r.ID.WorkUnitID, testID, r.ID.ExonerationID),
		TestIdStructured: &pb.TestIdentifier{
			ModuleName:        r.ID.ModuleName,
			ModuleScheme:      r.ID.ModuleScheme,
			ModuleVariantHash: r.ID.ModuleVariantHash,
			ModuleVariant:     r.ModuleVariant,
			CoarseName:        r.ID.CoarseName,
			FineName:          r.ID.FineName,
			CaseName:          r.ID.CaseName,
		},
		TestId:          testID,
		Variant:         r.ModuleVariant,
		ExonerationId:   r.ID.ExonerationID,
		ExplanationHtml: r.ExplanationHTML,
		VariantHash:     r.ID.ModuleVariantHash,
		Reason:          r.Reason,
		IsMasked:        false,
	}
}
