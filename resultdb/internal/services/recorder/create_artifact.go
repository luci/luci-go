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
	"fmt"
	"net/http"
	"regexp"
	"strconv"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/resultdb/internal/appstatus"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
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
		http.Error(c.Writer, st.Message(), grpcutil.CodeStatus(st.Code()))
	case err != nil:
		http.Error(c.Writer, err.Error(), http.StatusInternalServerError)
	default:
		c.Writer.WriteHeader(http.StatusNoContent)
	}
}

func (ac *artifactCreator) handle(c *router.Context) error {
	// Parse and validate the request.
	if err := ac.parseRequest(c); err != nil {
		return err
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
	if ac.testID != "" {
		ac.localParentID = fmt.Sprintf("tr/%s/%s", ac.testID, ac.resultID)
	}

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
