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

// Package artifacts contains tools for reading and processing artifacts.
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
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

// MustParseLegacyName extracts invocation, test, result and artifactIDs.
// Test and result IDs are "" if this is a invocation-level artifact.
// Panics on failure.
func MustParseLegacyName(name string) (invID invocations.ID, testID, resultID, artifactID string) {
	invIDStr, testID, resultID, artifactID, err := pbutil.ParseLegacyArtifactName(name)
	if err != nil {
		panic(err)
	}
	invID = invocations.ID(invIDStr)
	return
}

// ParseName extracts work unit, test, result and artifact IDs from an artifact name.
// testID and resultID are "" if this is a work unit-level artifact.
func ParseName(name string) (wuID workunits.ID, testID, resultID, artifactID string, err error) {
	parts, err := pbutil.ParseArtifactName(name)
	if err != nil {
		return workunits.ID{}, "", "", "", err
	}
	wuID = workunits.ID{RootInvocationID: rootinvocations.ID(parts.RootInvocationID), WorkUnitID: parts.WorkUnitID}
	testID = parts.TestID
	resultID = parts.ResultID
	artifactID = parts.ArtifactID
	return wuID, testID, resultID, artifactID, nil
}

// MustParseName extracts the work unit, test result and artifact IDs
// from the given artifact name.
// Test and result IDs are "" if this is a work unit-level artifact.
// Panics on failure.
func MustParseName(name string) (wuID workunits.ID, testID, resultID, artifactID string) {
	var err error
	wuID, testID, resultID, artifactID, err = ParseName(name)
	if err != nil {
		panic(err)
	}
	return wuID, testID, resultID, artifactID
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
		return "", "", errors.Fmt("unrecognized artifact parent ID %q", parentID)
	}
	parentID = strings.TrimPrefix(parentID, "tr/")

	lastSlash := strings.LastIndexByte(parentID, '/')
	if lastSlash == -1 || lastSlash == 0 || lastSlash == len(parentID)-1 {
		return "", "", errors.Fmt("unrecognized artifact parent ID %q", parentID)
	}

	return parentID[:lastSlash], parentID[lastSlash+1:], nil
}

// Read reads an artifact and returns an Artifact row from the database.
// It contains the following fields:
// * ContentType
// * Size
// * GcsURI
// * RbeURI
// * RBECASHash
// * ArtifactType
func Read(ctx context.Context, name string) (*Artifact, error) {
	var invID invocations.ID
	var testID, resultID, artifactID string
	var err error
	if pbutil.IsLegacyArtifactName(name) {
		var invIDStr string
		invIDStr, testID, resultID, artifactID, err = pbutil.ParseLegacyArtifactName(name)
		if err != nil {
			return nil, err
		}
		invID = invocations.ID(invIDStr)
	} else {
		var wuID workunits.ID
		wuID, testID, resultID, artifactID, err = ParseName(name)
		if err != nil {
			return nil, err
		}
		invID = wuID.LegacyInvocationID()
	}

	parentID := ParentID(testID, resultID)

	ret := &Artifact{
		Artifact: &pb.Artifact{
			Name:       name,
			ArtifactId: artifactID,
		},
	}

	var contentType spanner.NullString
	var size spanner.NullInt64
	var gcsURI spanner.NullString
	var rbeURI spanner.NullString
	var rbeCASHash spanner.NullString
	var artifactType spanner.NullString
	var moduleVariant *pb.Variant
	// Populate fields from Artifacts table.
	err = spanutil.ReadRow(ctx, "Artifacts", invID.Key(parentID, artifactID), map[string]any{
		"ContentType":   &contentType,
		"Size":          &size,
		"GcsURI":        &gcsURI,
		"RbeURI":        &rbeURI,
		"RBECASHash":    &rbeCASHash,
		"ArtifactType":  &artifactType,
		"ModuleVariant": &moduleVariant,
	})

	switch {
	case spanner.ErrCode(err) == codes.NotFound:
		return nil, appstatus.Attachf(err, codes.NotFound, "%s not found", name)

	case err != nil:
		return nil, errors.Fmt("failed to fetch %q: %w", name, err)
	default:
	}

	ret.Artifact.ContentType = contentType.StringVal
	ret.Artifact.SizeBytes = size.Int64
	ret.Artifact.GcsUri = gcsURI.StringVal
	ret.Artifact.RbeUri = rbeURI.StringVal
	ret.Artifact.ArtifactType = artifactType.StringVal
	ret.RBECASHash = rbeCASHash.StringVal
	ret.Artifact.HasLines = IsLogSupportedArtifact(ret.Artifact.ArtifactId, ret.Artifact.ContentType)

	if testID != "" {
		testIDStructured, err := pbutil.ParseStructuredTestIdentifierForOutput(testID, moduleVariant)
		if err != nil {
			panic(fmt.Errorf("data corruption: stored test ID and variant is invalid, artifact name = %q", name))
		}
		ret.Artifact.ResultId = resultID
		ret.Artifact.TestId = testID
		ret.Artifact.TestIdStructured = testIDStructured
	}
	return ret, nil
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

// VerifyReadArtifactPermission verifies if the caller has enough permissions to read the artifact.
func VerifyReadArtifactPermission(ctx context.Context, name string) error {
	invIDStr, _, _, _, inputErr := pbutil.ParseLegacyArtifactName(name)
	if inputErr != nil {
		return appstatus.BadRequest(inputErr)
	}

	return permissions.VerifyInvocation(ctx, invocations.ID(invIDStr), rdbperms.PermGetArtifact)
}
