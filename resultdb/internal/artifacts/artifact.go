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

package artifacts

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// MustParseName extracts invocation, test, result and artifactIDs.
// Test and result IDs are "" if this is a invocation-level artifact.
// Panics on failure.
func MustParseName(name string) (invID invocations.ID, testID, resultID, artifactID string) {
	invIDStr, testID, resultID, artifactID, err := pbutil.ParseArtifactName(name)
	if err != nil {
		panic(err)
	}
	invID = invocations.ID(invIDStr)
	return
}

// ParentID returns a value for Artifacts.ParentId Spanner column.
func ParentID(testID, resultID string) string {
	if testID != "" {
		return fmt.Sprintf("tr/%s/%s", testID, resultID)
	}
	return ""
}

// ParseParentID parses parentID into testID and resultID.
// If the artifact's parent is invocation, then testID and resultID are "".
func ParseParentID(parentID string) (testID, resultID string, err error) {
	if parentID == "" {
		return "", "", nil
	}

	if !strings.HasPrefix(parentID, "tr/") {
		return "", "", errors.Reason("unrecognized artifact parent ID %q", parentID).Err()
	}
	parentID = strings.TrimPrefix(parentID, "tr/")

	lastSlash := strings.LastIndexByte(parentID, '/')
	if lastSlash == -1 || lastSlash == 0 || lastSlash == len(parentID)-1 {
		return "", "", errors.Reason("unrecognized artifact parent ID %q", parentID).Err()
	}

	return parentID[:lastSlash], parentID[lastSlash+1:], nil
}

// Read reads an artifact from Spanner with GCS URI if one is available.
// If it does not exist, the returned error is annotated with NotFound GRPC
// code.
// Does not return artifact content.
func Read(ctx context.Context, name string) (*pb.Artifact, error) {
	invIDStr, testID, resultID, artifactID, err := pbutil.ParseArtifactName(name)
	if err != nil {
		return nil, err
	}
	invID := invocations.ID(invIDStr)
	parentID := ParentID(testID, resultID)

	ret := &pb.Artifact{
		Name:       name,
		ArtifactId: artifactID,
	}

	// Populate fields from Artifacts table.
	var contentType spanner.NullString
	var size spanner.NullInt64
	var gcsURI spanner.NullString

	err = spanutil.ReadRow(ctx, "Artifacts", invID.Key(parentID, artifactID), map[string]interface{}{
		"ContentType": &contentType,
		"Size":        &size,
		"GcsURI":      &gcsURI,
	})

	switch {
	case spanner.ErrCode(err) == codes.NotFound:
		return nil, appstatus.Attachf(err, codes.NotFound, "%s not found", ret.Name)

	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch %q", ret.Name).Err()

	default:
		ret.ContentType = contentType.StringVal
		ret.SizeBytes = size.Int64
		ret.GcsUri = gcsURI.StringVal
		return ret, nil
	}
}

const (
	// HashFunc is the name of the hash func used for generating RBECASHash.
	hashFunc      = "sha256"
	hashPrefix    = hashFunc + ":"
	sha256Pattern = `[0-9a-f]{64}$`
)

var ContentHashRe = regexp.MustCompile(fmt.Sprintf(`^%s:%s$`, hashFunc, sha256Pattern))

// AddHashPrefix adds HashFunc to a given hash string.
func AddHashPrefix(hash string) string {
	return strings.Join([]string{hashPrefix, hash}, "")
}

// TrimHashPrefix removes HashFunc from a given hash string.
func TrimHashPrefix(hash string) string {
	return strings.TrimPrefix(hash, hashPrefix)
}
