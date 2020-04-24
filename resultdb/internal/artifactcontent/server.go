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
	"io"
	"net/http"
	"strconv"
	"strings"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	clientauth "go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
)

// Server can serve artifact content, and generate signed URLs to the content.
type Server struct {
	// Use http:// (not https://) for generated URLs.
	InsecureURLs bool

	// Included in generated signed URLs and required in content requests.
	Hostname string

	// used for isolate client
	anonClient, authClient *http.Client

	// mock for isolate fetching
	testFetchIsolate func(ctx context.Context, isolateHost, ns, digest string, w io.Writer) error
}

// NewServer creates a Server.
func NewServer(ctx context.Context, insecureURLs bool, hostname string) (*Server, error) {
	anonTransport, err := auth.GetRPCTransport(ctx, auth.NoAuth)
	if err != nil {
		return nil, err
	}
	selfTransport, err := auth.GetRPCTransport(ctx, auth.AsSelf)
	if err != nil {
		return nil, err
	}

	return &Server{
		InsecureURLs: insecureURLs,
		Hostname:     hostname,
		anonClient:   &http.Client{Transport: anonTransport},
		authClient:   &http.Client{Transport: selfTransport},
	}, nil
}

// InstallHandlers installs handlers to serve artifact content.
func (s *Server) InstallHandlers(r *router.Router) {
	a := &auth.Authenticator{
		Methods: []auth.Method{
			&auth.GoogleOAuth2Method{
				Scopes: []string{clientauth.OAuthScopeEmail},
			},
		},
	}
	authMC := router.NewMiddlewareChain(a.GetMiddleware())
	// Ideally we use a more narrow patterns, but we cannot because of
	// https://github.com/julienschmidt/httprouter/issues/208
	// This is triggered by URL-escaped test IDs.
	r.GET("/invocations/*rest", authMC, s.handleArtifactContent)
}

type contentRequest struct {
	*Server
	w http.ResponseWriter

	artifactName string

	invID      span.InvocationID
	parentID   string
	artifactID string

	contentType spanner.NullString
	length      spanner.NullInt64
}

func (r *contentRequest) WriteHeaders() {
	if r.contentType.Valid {
		r.w.Header().Set("Content-Type", r.contentType.StringVal)
	}
	if r.length.Valid {
		r.w.Header().Set("Content-Length", strconv.FormatInt(r.length.Int64, 10))
	}
}

func (r *contentRequest) parse(ctx context.Context, req *http.Request) error {
	// We should not use URL.Path because it is important to preserve escaping
	// of test IDs.
	r.artifactName = strings.TrimPrefix(req.URL.RawPath, "/")
	if err := r.checkAccess(ctx, req, r.artifactName); err != nil {
		return err
	}

	invID, testID, resultID, artifactID, err := pbutil.ParseArtifactName(r.artifactName)
	if err != nil {
		return err
	}
	r.invID = span.InvocationID(invID)
	r.parentID = span.ArtifactParentID(testID, resultID)
	r.artifactID = artifactID
	return nil
}

func (s *Server) handleArtifactContent(c *router.Context) {
	req := &contentRequest{
		Server: s,
		w:      c.Writer,
	}

	if err := req.parse(c.Context, c.Request); err != nil {
		req.sendError(c.Context, err)
		return
	}

	txn := span.Client(c.Context).ReadOnlyTransaction()
	defer txn.Close()

	var isolateURL spanner.NullString
	key := req.invID.Key(req.parentID, req.artifactID)
	err := span.ReadRow(c.Context, txn, "Artifacts", key, map[string]interface{}{
		"ContentType": &req.contentType,
		"Size":        &req.length,
		"IsolateURL":  &isolateURL,
	})
	switch {
	case spanner.ErrCode(err) == codes.NotFound:
		http.Error(c.Writer, req.artifactName+" not found", http.StatusNotFound)

	case err != nil:
		req.sendError(c.Context, err)

	default:
		req.handleIsolateContent(c.Context, isolateURL.StringVal)
	}
}

// checkAccess ensures the requester has access to the artifact content.
func (r *contentRequest) checkAccess(ctx context.Context, req *http.Request, artifactName string) error {
	token := req.URL.Query().Get("token")
	if token == "" {
		return internal.VerifyAccess(ctx)
	}

	if _, err := artifactNameTokenKind.Validate(ctx, token, []byte(artifactName)); err != nil {
		if !transient.Tag.In(err) {
			return appstatus.Attachf(err, codes.PermissionDenied, "invalid token")
		}
		return err
	}

	return nil
}

func (r *contentRequest) sendError(ctx context.Context, err error) {
	st, ok := appstatus.Get(err)
	httpCode := grpcutil.CodeStatus(st.Code())
	if !ok || httpCode == http.StatusInternalServerError {
		logging.Errorf(ctx, "%s", err)
		http.Error(r.w, "Internal server error", http.StatusInternalServerError)
	} else {
		logging.Warningf(ctx, "%s", st.Message())
		http.Error(r.w, st.Message(), httpCode)
	}
}
