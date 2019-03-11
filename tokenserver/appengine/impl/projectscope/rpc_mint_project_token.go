// Copyright 2019 The LUCI Authors.
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

package projectscope

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"golang.org/x/oauth2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/signing"

	"go.chromium.org/luci/tokenserver/api/minter/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/projectidentity"
)

const (
	// maxAllowedMinValidityDuration specifies the maximum project identity token validity period
	// that a client may ask for.
	maxAllowedMinValidityDuration = 30 * time.Minute

	// A value for minimal returned token lifetime if 'min_validity_duration'
	// field is not specified in the request.
	defaultMinValidityDuration = 5 * time.Minute

	// projectActorsGroup is a group of identities and subgroups authorized to obtain project tokens.
	projectActorsGroup = "auth-project-actors"
)

// MintProjectTokenRPC implements TokenMinter.MintProjectToken.
// method.
type MintProjectTokenRPC struct {
	// Signer is mocked in tests.
	//
	// In prod it is gaesigner.Signer.
	Signer signing.Signer

	// MintAccessToken produces an OAuth token for a service account.
	//
	// In prod it is auth.MintAccessTokenForServiceAccount.
	MintAccessToken func(context.Context, auth.MintAccessTokenParams) (*oauth2.Token, error)

	// ProjectIdentities manages project scoped identities.
	//
	// In  prod it is projectscope.persistentIdentityManager
	ProjectIdentities func(context.Context) projectidentity.Storage
}

// logRequest logs the body of the request.
func (r *MintProjectTokenRPC) logRequest(c context.Context, req *minter.MintProjectTokenRequest, caller identity.Identity) {
	if !logging.IsLogging(c, logging.Debug) {
		return
	}
	m := jsonpb.Marshaler{Indent: " "}
	dump, err := m.MarshalToString(req)
	if err != nil {
		panic(err)
	}
	logging.Debugf(c, "Identity: %s", caller)
	logging.Debugf(c, "MintProjectTokenRequest:\n%s", dump)
}

// validateRequest validates the request fields.
func (r *MintProjectTokenRPC) validateRequest(c context.Context, req *minter.MintProjectTokenRequest) error {
	minDur := time.Duration(req.MinValidityDuration) * time.Second
	switch {
	case req.LuciProject == "":
		return fmt.Errorf("luci_project is empty")
	case len(req.OauthScope) <= 0:
		return fmt.Errorf("oauth_scope is required")
	case minDur < 0:
		return fmt.Errorf("min_validity_duration must be positive")
	case minDur > maxAllowedMinValidityDuration:
		return fmt.Errorf("min_validity_duration must not exceed %d", maxAllowedMinValidityDuration/time.Second)
	}
	return nil
}

// MintProjectToken mints a project-scoped service account OAuth2 token.
//
// Project-scoped service accounts are identities tied to an individual LUCI project.
// Therefore they provide a way to safely interact with LUCI APIs and prevent accidental
// cross-project operations.
func (r *MintProjectTokenRPC) MintProjectToken(c context.Context, req *minter.MintProjectTokenRequest) (*minter.MintProjectTokenResponse, error) {
	state := auth.GetState(c)
	callerID := state.User().Identity

	// Make sure we log the request as early as possible.
	r.logRequest(c, req, callerID)

	if err := r.validateRequest(c, req); err != nil {
		logging.WithError(err).Errorf(c, "invalid request %v", req)
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %s", err.Error())
	}

	// Using delegation to obtain a project scoped account is forbidden.
	if callerID != state.PeerIdentity() {
		logging.Errorf(c, "Trying to use delegation, it's forbidden")
		return nil, status.Errorf(codes.PermissionDenied, "delegation is forbidden for this API call")
	}

	// Perform authorization check first.
	// Internal error: Retry
	// !Member: PermissionDenied
	// Member: Continue
	member, err := auth.IsMember(c, projectActorsGroup)
	switch {
	case err != nil:
		logging.WithError(err).Errorf(c, "unable to perform group check of member %s and group %s", callerID, projectActorsGroup)
		return nil, status.Errorf(codes.Internal, "internal authorization error")
	case !member:
		logging.Infof(c, "Denied access to %s, authorization failed", callerID)
		return nil, status.Errorf(codes.PermissionDenied, "access denied")
	}

	// Perform bounds checking on the requested token validity lifetime.
	minValidityDuration := time.Duration(req.MinValidityDuration) * time.Second
	if minValidityDuration == 0 {
		minValidityDuration = defaultMinValidityDuration
	}

	projectIdentity, err := r.ProjectIdentities(c).LookupByProject(c, req.LuciProject)
	if err != nil {
		switch {
		case err == projectidentity.ErrNotFound:
			logging.WithError(err).Errorf(c, "no project identity for project %s", req.LuciProject)
			return nil, status.Errorf(codes.NotFound, "unable to find project identity for project %s", req.LuciProject)
		case err != nil:
			logging.WithError(err).Errorf(c, "error while looking for scoped identity of project %s", req.LuciProject)
			return nil, status.Errorf(codes.Internal, "internal error")

		}
	}

	// All checks passed, mint the token.
	accessTok, err := r.MintAccessToken(c, auth.MintAccessTokenParams{
		ServiceAccount: projectIdentity.Email,
		Scopes:         req.OauthScope,
		MinTTL:         minValidityDuration,
	})
	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to mint project scoped oauth token for caller %q in project %q for identity %q",
			callerID, req.LuciProject, projectIdentity.Email)
		return nil, status.Errorf(codes.Internal, "failed to mint token for service account %s", projectIdentity.Email)
	}

	// Determine service version for token logging.
	serviceVer, err := utils.ServiceVersion(c, r.Signer)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "can't grab service version - %s", err)
	}

	// Create response object.
	resp := &minter.MintProjectTokenResponse{
		ServiceAccountEmail: projectIdentity.Email,
		AccessToken:         accessTok.AccessToken,
		Expiry:              google.NewTimestamp(accessTok.Expiry),
		ServiceVersion:      serviceVer,
	}

	return resp, nil
}
