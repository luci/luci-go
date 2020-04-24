// Copyright 2020 The LUCI Authors.
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

package span

import (
	"context"
	"fmt"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// MustParseArtifactName extracts invocation, test, result and artifactIDs.
// Test and result IDs are "" if this is a invocation-level artifact.
// Panics on failure.
func MustParseArtifactName(name string) (invID InvocationID, testID, resultID, artifactID string) {
	invIDStr, testID, resultID, artifactID, err := pbutil.ParseArtifactName(name)
	if err != nil {
		panic(err)
	}
	invID = InvocationID(invIDStr)
	return
}

// ArtifactParentID returns a value for Artifacts.ParentId Spanner column.
func ArtifactParentID(testID, resultID string) string {
	if testID != "" {
		return fmt.Sprintf("tr/%s/%s", testID, resultID)
	}
	return ""
}

// ReadArtifact reads an artifact from Spanner.
// If it does not exist, the returned error is annotated with NotFound GRPC
// code.
// Does not return artifact content or its location.
func ReadArtifact(ctx context.Context, txn Txn, name string) (*pb.Artifact, error) {
	invIDStr, testID, resultID, artifactID, err := pbutil.ParseArtifactName(name)
	if err != nil {
		return nil, err
	}
	invID := InvocationID(invIDStr)
	parentID := ArtifactParentID(testID, resultID)

	ret := &pb.Artifact{
		Name:       name,
		ArtifactId: artifactID,
	}

	// Populate fields from Artifacts table.
	var contentType spanner.NullString
	var size spanner.NullInt64
	err = ReadRow(ctx, txn, "Artifacts", invID.Key(parentID, artifactID), map[string]interface{}{
		"ContentType": &contentType,
		"Size":        &size,
	})
	switch {
	case spanner.ErrCode(err) == codes.NotFound:
		return nil, appstatus.Attachf(err, codes.NotFound, "%s not found", ret.Name)

	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch %q", ret.Name).Err()

	default:
		ret.ContentType = contentType.StringVal
		ret.SizeBytes = size.Int64
		return ret, nil
	}
}
