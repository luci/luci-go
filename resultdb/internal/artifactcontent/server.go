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

package artifactcontent

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tokens"

	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
)

var artifactNameTokenKind = tokens.TokenKind{
	Algo:       tokens.TokenAlgoHmacSHA256,
	Expiration: time.Hour,
	SecretKey:  "artifact_name",
	Version:    1,
}

// HostnameProvider returns a hostname to use in generated signed URLs.
//
// As input it accepts `host` metadata value of the GetArtifacts etc. requests.
// It may be an empty string. HostnameProvider must return some host name in
// this case too.
type HostnameProvider func(requestHost string) string

// Server can serve artifact content, and generate signed URLs to the content.
type Server struct {
	// Use http:// (not https://) for generated URLs.
	InsecureURLs bool

	// Returns a hostname to use in generated signed URLs.
	HostnameProvider HostnameProvider

	// Reads a blob from RBE-CAS.
	ReadCASBlob func(ctx context.Context, req *bytestream.ReadRequest) (bytestream.ByteStream_ReadClient, error)

	// Reads a blob from RBE-CAS by a project-scoped account.
	ReadCASBlobByProject func(ctx context.Context, req *bytestream.ReadRequest, project string) (bytestream.ByteStream_ReadClient, error)

	// Full name of the RBE-CAS instance used to store artifacts,
	// e.g. "projects/luci-resultdb/instances/artifacts".
	RBECASInstanceName string
}

// InstallHandlers installs handlers to serve artifact content.
//
// May be called multiple times to install the handler into multiple virtual
// hosts.
func (s *Server) InstallHandlers(r *router.Router) {
	// TODO(nodir): use OAuth2.0 middleware to allow OAuth credentials.

	// Ideally we use a more narrow pattern, but we cannot because of
	// https://github.com/julienschmidt/httprouter/issues/208
	// This is triggered by URL-escaped test IDs.
	r.GET("/invocations/*rest", nil, s.handleGET)
	r.GET("/rootInvocations/*rest", nil, s.handleGET)
	r.OPTIONS("/invocations/*rest", nil, s.handleOPTIONS)
	r.OPTIONS("/rootInvocations/*rest", nil, s.handleOPTIONS)
}

func (s *Server) handleGET(c *router.Context) {
	req := &contentRequest{Server: s, w: c.Writer}
	req.handle(c)
}

func (s *Server) handleOPTIONS(c *router.Context) {
	s.setAccessControlHeaders(c, true)
	c.Writer.WriteHeader(http.StatusOK)
}

// setAccessControlHeaders allows CORS.
func (s *Server) setAccessControlHeaders(c *router.Context, preflight bool) {
	h := c.Writer.Header()
	h.Add("Access-Control-Allow-Origin", "*")
	h.Add("Access-Control-Allow-Credentials", "false")

	if preflight {
		h.Add("Access-Control-Allow-Headers", "Origin, Authorization")
		h.Add("Access-Control-Allow-Methods", "OPTIONS, GET")
	}
}

type contentRequest struct {
	*Server
	w http.ResponseWriter

	artifactName string

	// Either work unit ID or invocation ID will be set, not both.
	workUnitID workunits.ID
	invID      invocations.ID

	parentID   string
	artifactID string
	limit      int64 // Maximum size of the artifact, in bytes.

	contentType spanner.NullString
	size        spanner.NullInt64
}

func (r *contentRequest) handle(c *router.Context) {
	r.setAccessControlHeaders(c, false)

	if err := r.parseRequest(c.Request); err != nil {
		r.sendError(c.Request.Context(), appstatus.BadRequest(err))
		return
	}

	embedded, err := r.checkAccess(c.Request.Context(), c.Request)
	if err != nil {
		r.sendError(c.Request.Context(), err)
		return
	}

	// Read the state from database.
	var rbeCASHash, rbeURI spanner.NullString
	var key spanner.Key
	if r.invID != "" {
		key = r.invID.Key(r.parentID, r.artifactID)
	} else {
		key = r.workUnitID.LegacyInvocationID().Key(r.parentID, r.artifactID)
	}
	err = spanutil.ReadRow(span.Single(c.Request.Context()), "Artifacts", key, map[string]any{
		"ContentType": &r.contentType,
		"Size":        &r.size,
		"RBECASHash":  &rbeCASHash,
		"RbeURI":      &rbeURI,
	})

	// Check the error and write content to the response body.
	ctx := c.Request.Context()
	switch {
	case spanner.ErrCode(err) == codes.NotFound:
		err = appstatus.Attachf(err, codes.NotFound, "%s not found", r.artifactName)
		r.sendError(ctx, err)

	case err != nil:
		r.sendError(ctx, err)

	case rbeCASHash.Valid && rbeCASHash.StringVal != "":
		mw := NewMetricsWriter(c)
		defer mw.Download(ctx, r.size.Int64)
		r.handleRBECASContent(c, rbeCASHash.StringVal)

	case rbeURI.Valid && rbeURI.StringVal != "":
		project := embedded["project"]
		if project == "" {
			r.sendError(ctx, appstatus.Errorf(codes.PermissionDenied, "project is not specified in the token"))
			return
		}

		mw := NewMetricsWriter(c)
		defer mw.Download(ctx, r.size.Int64)
		r.handleRBECASContentWithURI(c, rbeURI.StringVal, project)

	default:
		err = appstatus.Attachf(err, codes.NotFound, "%s not found", r.artifactName)
		r.sendError(ctx, err)
	}
}

func (r *contentRequest) parseRequest(req *http.Request) error {
	// We should not use URL.Path because it is important to preserve escaping
	// of test IDs.
	r.artifactName = strings.Trim(req.URL.EscapedPath(), "/")

	if pbutil.IsLegacyArtifactName(r.artifactName) {
		invID, testID, resultID, artifactID, err := pbutil.ParseLegacyArtifactName(r.artifactName)
		if err != nil {
			return errors.Fmt("invalid artifact name %q: %w", r.artifactName, err)
		}
		r.invID = invocations.ID(invID)
		r.parentID = artifacts.ParentID(testID, resultID)
		r.artifactID = artifactID
	} else {
		wuID, testID, resultID, artifactID, err := artifacts.ParseName(r.artifactName)
		if err != nil {
			return errors.Fmt("invalid artifact name %q: %w", r.artifactName, err)
		}
		r.workUnitID = wuID
		r.parentID = artifacts.ParentID(testID, resultID)
		r.artifactID = artifactID
	}

	limitStr := req.URL.Query().Get("n")
	if limitStr == "" {
		return nil
	}

	var err error
	r.limit, err = strconv.ParseInt(limitStr, 10, 64)
	if err != nil {
		return errors.Fmt("query parmeter n must be an integer, but got %q: %w", limitStr, err)
	}
	if r.limit < 0 {
		return errors.Fmt("query parmeter n must be >= 0, got %q", limitStr)
	}
	return nil
}

// checkAccess ensures that the requester has access to the artifact content and
// then returns a map of embedded data from the token, such as the "project",
// and an error.
//
// Checks access using signed token query string param.
func (r *contentRequest) checkAccess(ctx context.Context, req *http.Request) (map[string]string, error) {
	token := req.URL.Query().Get("token")
	if token == "" {
		return nil, appstatus.Errorf(codes.Unauthenticated, "no token")
	}

	embedded, err := artifactNameTokenKind.Validate(ctx, token, []byte(r.artifactName))
	if err != nil {
		if !transient.Tag.In(err) {
			return nil, appstatus.Attachf(err, codes.PermissionDenied, "invalid token")
		}
		return nil, err
	}
	return embedded, nil
}

func (r *contentRequest) sendError(ctx context.Context, err error) {
	if err == nil {
		panic("err is nil")
	}
	st, ok := appstatus.Get(err)
	httpCode := grpcutil.CodeStatus(st.Code())
	if !ok || httpCode == http.StatusInternalServerError {
		logging.Errorf(ctx, "responding with: %s", err)
		http.Error(r.w, "Internal server error", http.StatusInternalServerError)
	} else {
		logging.Warningf(ctx, "responding with: %s", st.Message())
		http.Error(r.w, st.Message(), httpCode)
	}
}

func (r *contentRequest) writeContentHeaders() {
	if r.contentType.Valid {
		r.w.Header().Set("Content-Type", r.contentType.StringVal)
	}
	if r.size.Valid {
		length := r.size.Int64
		if r.limit > 0 && r.limit < length {
			length = r.limit
		}
		r.w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
	}
}

// GenerateSignedURL generates a signed HTTPS URL back to this server. If a
// project is specified, the project name will be embedded in the token, and
// the corresponding project scoped account will be used to access the RBE
// Artifact.
// The returned token works only with the same artifact name.
func (s *Server) GenerateSignedURL(ctx context.Context, requestHost, artifactName string, embedded map[string]string) (url string, expiration time.Time, err error) {
	now := clock.Now(ctx).UTC()

	tok, err := artifactNameTokenKind.Generate(ctx, []byte(artifactName), embedded, artifactNameTokenKind.Expiration)
	if err != nil {
		return "", time.Time{}, err
	}

	scheme := "https"
	if s.InsecureURLs {
		scheme = "http"
	}

	// Derive the hostname for generated URL from the request host. This is used
	// to make sure GetArtifacts requests that hit "canary.*" API host also get
	// "canary.*" artifact links.
	hostname := s.HostnameProvider(requestHost)
	if hostname == "" {
		return "", time.Time{}, errors.New("empty content hostname")
	}

	// Using url.URL here is hard because it escapes artifact name which we don't want.
	url = fmt.Sprintf("%s://%s/%s?token=%s", scheme, hostname, artifactName, tok)
	expiration = now.Add(artifactNameTokenKind.Expiration)
	return
}

// ValidateTokenForTesting validates a token for the given artifact name.
// NOTE: this function is for testing ONLY.
func ValidateTokenForTesting(ctx context.Context, token, artifactName string) (map[string]string, error) {
	return artifactNameTokenKind.Validate(ctx, token, []byte(artifactName))
}
