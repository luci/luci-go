// Copyright 2026 The LUCI Authors.
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

package artifacts

import (
	"context"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"
)

// FilterHasArtifacts returns the subset of invIDs that have at least one
// artifact.
func FilterHasArtifacts(ctx context.Context, invIDs invocations.IDSet) (invocations.IDSet, error) {
	if len(invIDs) == 0 {
		return nil, nil
	}

	rowIDToInvID := make(map[string]invocations.ID, len(invIDs))
	ids := make([]string, 0, len(invIDs))
	for id := range invIDs {
		rid := id.RowID()
		ids = append(ids, rid)
		rowIDToInvID[rid] = id
	}

	st := spanner.NewStatement(`
		SELECT DISTINCT InvocationId
		FROM Artifacts
		WHERE InvocationId IN UNNEST(@invIDs)
	`)
	st.Params = map[string]any{
		"invIDs": ids,
	}

	iter := span.Query(ctx, st)
	defer iter.Stop()

	hasArtifacts := make(invocations.IDSet)
	err := iter.Do(func(row *spanner.Row) error {
		var rid string
		if err := row.Columns(&rid); err != nil {
			return err
		}
		if invID, ok := rowIDToInvID[rid]; ok {
			hasArtifacts.Add(invID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return hasArtifacts, nil
}

// ListFuzzyMatches returns artifacts that match the given artifact ID after
// removing digits.
func ListFuzzyMatches(ctx context.Context, invID invocations.ID, parentID string, artifactID string, limit int) ([]*Artifact, error) {
	st := spanner.NewStatement(`
		SELECT InvocationId, ParentId, ArtifactId, ContentType, ArtifactType, Size, RBECASHash, GcsURI, RBEURI
		FROM Artifacts
		WHERE InvocationId = @invID
			AND ParentId = @parentID
			AND REGEXP_REPLACE(ArtifactId, r'[0-9]', '') = REGEXP_REPLACE(@artifactId, r'[0-9]', '')
		LIMIT @limit
	`)
	st.Params = map[string]any{
		"invID":      invID.RowID(),
		"parentID":   parentID,
		"artifactId": artifactID,
		"limit":      limit,
	}

	var arts []*Artifact
	var b spanutil.Buffer
	err := spanutil.Query(ctx, st, func(row *spanner.Row) error {
		a := &Artifact{
			Artifact: &pb.Artifact{},
		}
		var rowInvID invocations.ID
		var rowParentID string
		var contentType spanner.NullString
		var artifactType spanner.NullString
		var size spanner.NullInt64
		var rbecasHash spanner.NullString
		var gcsURI spanner.NullString
		var rbeURI spanner.NullString

		if err := b.FromSpanner(row, &rowInvID, &rowParentID, &a.ArtifactId, &contentType, &artifactType, &size, &rbecasHash, &gcsURI, &rbeURI); err != nil {
			return err
		}

		// Initialize artifact name.
		parsedInvID := rowInvID
		switch testID, resultID, err := ParseParentID(rowParentID); {
		case err != nil:
			return err
		case testID == "":
			if parsedInvID.IsWorkUnit() {
				wuID := workunits.MustParseLegacyInvocationID(parsedInvID)
				a.Name = pbutil.WorkUnitArtifactName(string(wuID.RootInvocationID), wuID.WorkUnitID, a.ArtifactId)
			} else {
				a.Name = pbutil.LegacyInvocationArtifactName(string(parsedInvID), a.ArtifactId)
			}
		default:
			if parsedInvID.IsWorkUnit() {
				wuID := workunits.MustParseLegacyInvocationID(parsedInvID)
				a.Name = pbutil.TestResultArtifactName(string(wuID.RootInvocationID), wuID.WorkUnitID, testID, resultID, a.ArtifactId)
			} else {
				a.Name = pbutil.LegacyTestResultArtifactName(string(parsedInvID), testID, resultID, a.ArtifactId)
			}
		}

		a.ContentType = contentType.StringVal
		a.ArtifactType = artifactType.StringVal
		a.SizeBytes = size.Int64
		a.RBECASHash = rbecasHash.StringVal
		a.GcsUri = gcsURI.StringVal
		a.RbeUri = rbeURI.StringVal
		a.HasLines = IsLogSupportedArtifact(a.ArtifactId, a.ContentType)

		arts = append(arts, a)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return arts, nil
}
