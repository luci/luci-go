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

package recorder

import (
	"context"
	"net/http"
	"regexp"
	"strconv"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

const (
	artifactContentHashHeaderKey = "Content-Hash"
	artifactContentSizeHeaderKey = "Content-Length"
	updateTokenHeaderKey         = "Update-Token"
	maxArtifactContentSize       = 68719476736 // 64 MiB.
)

var artifactContentHashRe = regexp.MustCompile("^sha256:[0-9a-f]{64}$")

// artifactCreator implements an artifact creation HTTP handler.
type artifactCreator struct {
	artifactName  string
	invID         span.InvocationID
	testID        string
	resultID      string
	artifactID    string
	localParentID string

	hash string
	size int64
}

// handleArtifactCreation is an http.Handler that creates an artifact.
//
// Request:
//  - Router parameter "artifact" MUST be a valid artifact name.
//  - The request body MUST be the artifact contents.
//  - The request MUST include an Update-Token header with the value of
//    invocation's update token.
//  - The request MUST include a Content-Length header. It MUST be <= 64 MiB..
//  - The request MUST include a Content-Hash header with value "sha256:{hash}"
//    where {hash} is a lower-case hex-encoded SHA256 hash of the artifact
//    contents.
//  - The request SHOULD have a Content-Type header.
func handleArtifactCreation(c *router.Context) {
	var ac artifactCreator
	err := ac.handle(c)
	st, ok := appstatus.Get(err)
	switch {
	case ok:
		logging.Warningf(c.Context, "Responding with %s: %s", st.Code(), err)
		http.Error(c.Writer, st.Message(), grpcutil.CodeStatus(st.Code()))
	case err != nil:
		logging.Errorf(c.Context, "Internal server error: %s", err)
		http.Error(c.Writer, err.Error(), http.StatusInternalServerError)
	default:
		c.Writer.WriteHeader(http.StatusNoContent)
	}
}

func (ac *artifactCreator) handle(c *router.Context) error {
	ctx := c.Context

	// Parse and validate the request.
	if err := ac.parseRequest(c); err != nil {
		return err
	}

	// Read and verify the current state.
	switch sameExists, err := ac.verifyStateBeforeWriting(ctx); {
	case err != nil:
		return err
	case sameExists:
		return nil
	}

	// TODO(crbug.com/1071258): implement the rest.
	return nil
}

// parseRequest populates ac fields based on the HTTP request.
func (ac *artifactCreator) parseRequest(c *router.Context) error {
	// Parse and validate the artifact name.
	ac.artifactName = c.Params.ByName("artifact")
	var invIDString string
	var err error
	invIDString, ac.testID, ac.resultID, ac.artifactID, err = pbutil.ParseArtifactName(ac.artifactName)
	if err != nil {
		return appstatus.Errorf(codes.InvalidArgument, "bad artifact name: %s", err)
	}
	ac.invID = span.InvocationID(invIDString)
	ac.localParentID = span.ArtifactParentID(ac.testID, ac.resultID)

	// Parse and validate the hash.
	switch ac.hash = c.Request.Header.Get(artifactContentHashHeaderKey); {
	case ac.hash == "":
		return appstatus.Errorf(codes.InvalidArgument, "%s header is missing", artifactContentHashHeaderKey)
	case !artifactContentHashRe.MatchString(ac.hash):
		return appstatus.Errorf(codes.InvalidArgument, "%s header value does not match %s", artifactContentHashHeaderKey, artifactContentHashRe)
	}

	// Parse and validate the size.
	sizeHeader := c.Request.Header.Get(artifactContentSizeHeaderKey)
	if sizeHeader == "" {
		return appstatus.Errorf(codes.InvalidArgument, "%s header is missing", artifactContentSizeHeaderKey)
	}
	switch ac.size, err = strconv.ParseInt(sizeHeader, 10, 64); {
	case err != nil:
		return appstatus.Errorf(codes.InvalidArgument, "%s header is malformed: %s", artifactContentSizeHeaderKey, err)
	case ac.size < 0 || ac.size > maxArtifactContentSize:
		return appstatus.Errorf(codes.InvalidArgument, "%s header must be a value between 0 and %d", artifactContentSizeHeaderKey, maxArtifactContentSize)
	}

	// Parse and validate the update token.
	updateToken := c.Request.Header.Get(updateTokenHeaderKey)
	if updateToken == "" {
		return appstatus.Errorf(codes.Unauthenticated, "%s header is missing", updateTokenHeaderKey)
	}
	if err := validateInvocationToken(c.Context, updateToken, ac.invID); err != nil {
		return appstatus.Errorf(codes.PermissionDenied, "invalid %s header value", updateTokenHeaderKey)
	}

	return nil
}

// verifyStateBeforeWriting checks Spanner state in a read-only transaction,
// see verifyState comment.
func (ac *artifactCreator) verifyStateBeforeWriting(ctx context.Context) (sameAlreadyExists bool, err error) {
	txn := span.Client(ctx).ReadOnlyTransaction()
	defer txn.Close()
	return ac.verifyState(ctx, txn)
}

// verifyState checks if the Spanner state is compatible with creation of the
// artifact. If an identical artifact already exists, sameAlreadyExists is true.
func (ac *artifactCreator) verifyState(ctx context.Context, txn span.Txn) (sameAlreadyExists bool, err error) {
	var (
		invState       pb.Invocation_State
		hash           spanner.NullString
		size           spanner.NullInt64
		artifactExists bool
	)

	// Read the state concurrently.
	err = parallel.FanOutIn(func(work chan<- func() error) {
		work <- func() (err error) {
			invState, err = span.ReadInvocationState(ctx, txn, ac.invID)
			return
		}

		work <- func() error {
			key := ac.invID.Key(ac.localParentID, ac.artifactID)
			err := span.ReadRow(ctx, txn, "Artifacts", key, map[string]interface{}{
				"RBECASHash": &hash,
				"Size":       &size,
			})
			artifactExists = err == nil
			if spanner.ErrCode(err) == codes.NotFound {
				// This is expected.
				return nil
			}
			return err
		}
	})

	// Interpret the state.
	switch {
	case err != nil:
		return false, err

	case invState != pb.Invocation_ACTIVE:
		return false, appstatus.Errorf(codes.FailedPrecondition, "%s is not active", ac.invID.Name())

	case hash.Valid && hash.StringVal == ac.hash && size.Valid && size.Int64 == ac.size:
		// The same artifact already exists.
		return true, nil

	case artifactExists:
		// A different artifact already exists.
		return false, appstatus.Errorf(codes.AlreadyExists, "artifact %q already exists", ac.artifactName)

	default:
		return false, nil
	}
}
