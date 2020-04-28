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
	"strings"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

var followAllEdges = &pb.QueryArtifactsRequest_EdgeTypeSet{
	IncludedInvocations: true,
	TestResults:         true,
}

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

// ParseArtifactParentID parses parentID into testID and resultID.
// If the artifact's parent is invocation, then testID and resultID are "".
func ParseArtifactParentID(parentID string) (testID, resultID string, err error) {
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

// ArtifactQuery specifies artifacts to fetch.
type ArtifactQuery struct {
	InvocationIDs       InvocationIDSet
	FollowEdges         *pb.QueryArtifactsRequest_EdgeTypeSet
	TestResultPredicate *pb.TestResultPredicate
	PageSize            int // must be positive
	PageToken           string
}

// parseArtifactPageToken parses the page token into invocation ID, parentID
// and artifactId.
func parseArtifactPageToken(pageToken string) (parentID string, inv InvocationID, artifactID string, err error) {
	switch pos, tokErr := pagination.ParseToken(pageToken); {
	case tokErr != nil:
		err = pageTokenError(tokErr)

	case pos == nil:

	case len(pos) != 3:
		err = pageTokenError(errors.Reason("expected 3 position strings, got %q", pos).Err())

	default:
		parentID = pos[0]
		inv = InvocationID(pos[1])
		artifactID = pos[2]
	}

	return
}

// Fetch returns a page of artifacts matching q.
//
// Returned artifacts are ordered by level (invocation or test result).
// Test result artifacts are sorted by test id.
func (q *ArtifactQuery) Fetch(ctx context.Context, txn Txn) (arts []*pb.Artifact, nextPageToken string, err error) {
	if q.PageSize <= 0 {
		panic("PageSize <= 0")
	}

	st := spanner.NewStatement(`
		SELECT InvocationId, ParentId, ArtifactId, ContentType, Size
		FROM Artifacts
		WHERE InvocationId IN UNNEST(@invIDs)
			# Skip artifacts after the one specified in the page token.
			AND (
				(ParentId > @afterParentId) OR
				(ParentId = @afterParentId AND InvocationId > @afterInvocationId) OR
				(ParentId = @afterParentId AND InvocationId = @afterInvocationId AND ArtifactId > @afterArtifactId)
			)
			AND REGEXP_CONTAINS(ParentId, @ParentIdRegexp)
		ORDER BY ParentId, InvocationId, ArtifactId
		LIMIT @limit
	`)

	st.Params["invIDs"] = q.InvocationIDs
	st.Params["limit"] = q.PageSize
	st.Params["afterParentId"],
		st.Params["afterInvocationId"],
		st.Params["afterArtifactId"],
		err = parseArtifactPageToken(q.PageToken)
	if err != nil {
		return
	}

	st.Params["ParentIdRegexp"] = q.parentIDRegexp()

	// TODO(nodir): add support for q.TestResultPredicate.Variant
	// TODO(nodir): add support for q.TestResultPredicate.Expectancy

	var b Buffer
	err = Query(ctx, txn, st, func(row *spanner.Row) error {
		var invID InvocationID
		var parentID string
		var contentType spanner.NullString
		var size spanner.NullInt64
		a := &pb.Artifact{}
		if err := b.FromSpanner(row, &invID, &parentID, &a.ArtifactId, &contentType, &size); err != nil {
			return err
		}

		// Initialize artifact name.
		switch testID, resultID, err := ParseArtifactParentID(parentID); {
		case err != nil:
			return err
		case testID == "":
			a.Name = pbutil.InvocationArtifactName(string(invID), a.ArtifactId)
		default:
			a.Name = pbutil.TestResultArtifactName(string(invID), testID, resultID, a.ArtifactId)
		}

		a.ContentType = contentType.StringVal
		a.SizeBytes = size.Int64

		arts = append(arts, a)
		return nil
	})
	if err != nil {
		arts = nil
		return
	}

	// If we got pageSize results, then we haven't exhausted the collection and
	// need to return the next page token.
	if len(arts) == q.PageSize {
		last := arts[q.PageSize-1]
		invID, testID, resultID, artifactID := MustParseArtifactName(last.Name)
		parentID := ArtifactParentID(testID, resultID)
		nextPageToken = pagination.Token(parentID, string(invID), artifactID)
	}
	return
}

// parentIDRegexp returns a regular expression for ParentId column.
// Uses q.FollowEdges and q.TestResultPredicate.TestIdRegexp to compute it.
func (q *ArtifactQuery) parentIDRegexp() string {
	testIDRE := q.TestResultPredicate.GetTestIdRegexp()
	hasTestIDRE := testIDRE != "" && testIDRE != ".*"

	edges := q.FollowEdges
	if edges == nil {
		edges = followAllEdges
	}

	if edges.IncludedInvocations && edges.TestResults && !hasTestIDRE {
		// Fast path.
		return ".*"
	}

	// Collect alternatives and then combine them with "|".
	var alts []string

	if edges.IncludedInvocations {
		// Invocation-level artifacts have empty parent ID.
		alts = append(alts, "")
	}

	if edges.TestResults {
		// TestResult-level artifacts have parent ID formatted as
		// "tr/{testID}/{resultID}"
		if hasTestIDRE {
			alts = append(alts, fmt.Sprintf("tr/%s/[^/]+", testIDRE))
		} else {
			alts = append(alts, "tr/.+")
		}
	}

	// Note: the surrounding parens are important. Without them any expression
	// matches.
	return fmt.Sprintf("^(%s)$", strings.Join(alts, "|"))
}
