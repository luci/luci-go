// Copyright 2018 The LUCI Authors.
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

package serviceaccounts

import (
	"context"
	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/projectscope"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/revocation"
	"time"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/signing"
	"golang.org/x/oauth2"

	"fmt"

	"github.com/golang/protobuf/jsonpb"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging"
	tokenserver "go.chromium.org/luci/tokenserver/api"
	"go.chromium.org/luci/tokenserver/api/minter/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MintOAuthTokenViaGrantRPC implements TokenMinter.MintOAuthTokenViaGrant
// method.
type MintServiceOAuthTokenRPC struct {
	// Signer is mocked in tests.
	//
	// In prod it is gaesigner.Signer.
	Signer signing.Signer

	// Rules returns service account rules to use for the request.
	//
	// In prod it is GlobalRulesCache.Rules.
	Rules func(context.Context) (*Rules, error)

	// MintAccessToken produces an OAuth token for a service account.
	//
	// In prod it is auth.MintAccessTokenForServiceAccount.
	MintAccessToken func(context.Context, auth.MintAccessTokenParams) (*oauth2.Token, error)

	// LogOAuthToken is mocked in tests.
	//
	// In prod it is LogOAuthToken from oauth_token_bigquery_log.go.
	LogOAuthToken func(context.Context, LoggableOAuthTokenInfo) error

	// Storage manages project scoped identities.
	//
	// In  prod it is projectscope.persistentIdentityManager
	Storage projectscope.ScopedIdentityManager
}

func (r *MintServiceOAuthTokenRPC) getGCPProject() (string) {
	return ""
}

func (r *MintServiceOAuthTokenRPC) resolveServiceAccountIdentity(c context.Context, service, project string) (string, error) {
	return r.Storage.GetOrCreateIdentity(c, service, project, "")
}

func (r *MintServiceOAuthTokenRPC) MintServiceOAuthToken(c context.Context, req *minter.MintServiceOAuthTokenRequest) (*minter.MintServiceOAuthTokenResponse, error) {
	state := auth.GetState(c)

	callerId := state.User().Identity
	if callerId != state.PeerIdentity() {
		logging.Errorf(c, "Trying to use delegation, it's forbidden")
		return nil, status.Errorf(codes.PermissionDenied, "delegation is forbidden for this API call")
	}

	rule, err := r.validateRequest(c, req, callerId)
	if err != nil {
		return nil, err
	}
	if req.MinValidityDuration == 0 {
		if rule.Rule.MaxGrantValidityDuration > 3600 {
			req.MinValidityDuration = 3600
		} else {
			req.MinValidityDuration = rule.Rule.MaxGrantValidityDuration
		}
	}

	accountId, err := r.resolveServiceAccountIdentity(c, string(callerId), req.LuciProject)

	accessTok, err := r.MintAccessToken(c, auth.MintAccessTokenParams{
		ServiceAccount: accountId,
		Scopes: req.OauthScope,
		MinTTL: time.Duration(int64(time.Second) * req.MinValidityDuration),
		Project: r.getGCPProject(),
	})

	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to mint scoped oauth token for service %q in project %q", callerId, req.LuciProject)
		code := codes.InvalidArgument
		if transient.Tag.In(err) {
			code = codes.Internal
		}
		return nil, status.Errorf(code, "failed to mint scoped oauth token for service %q in project %q")
	}

	serviceVer, err := utils.ServiceVersion(c, r.Signer)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "can't grab service version - %s", err)
	}

	resp := &minter.MintServiceOAuthTokenResponse{
		AccessToken: accessTok.AccessToken,
		Expiry: google.NewTimestamp(accessTok.Expiry),
		ServiceVersion: serviceVer,
	}

	// Log it to BigQuery
	if r.LogOAuthToken != nil {
		// Errors during logging are considered not fatal. bqlog library has
		// a monitoring counter that tracks number of errors, so they are not
		// totally invisible.
		info := MintedServiceOAuthTokenInfo{
			Request:     req,
			Response:    resp,
			OAuthTokenInfo: OAuthTokenInfo{
				RequestedAt: clock.Now(c),
				ConfigRev:   rule.Revision,
				Rule:        rule.Rule,
				PeerIP:      state.PeerIP(),
				RequestID:   info.RequestID(c),
				AuthDBRev:   authdb.Revision(state.DB()),
			},
		}
		if logErr := r.LogOAuthToken(c, &info); logErr != nil {
			logging.WithError(logErr).Errorf(c, "Failed to insert the oauth token into the BigQuery log")
		}
	}

	return resp, nil
}

func (r *MintServiceOAuthTokenRPC) validateRequest(c context.Context, req *minter.MintServiceOAuthTokenRequest, caller identity.Identity) (*Rule, error) {
	r.logRequest(c, req, caller)
	return nil, nil
}

// logRequest logs the body of the request.
func (r *MintServiceOAuthTokenRPC) logRequest(c context.Context, req *minter.MintServiceOAuthTokenRequest, caller identity.Identity) {
	if !logging.IsLogging(c, logging.Debug) {
		return
	}
	m := jsonpb.Marshaler{Indent: " "}
	dump, _ := m.MarshalToString(req)
	logging.Debugf(c, "Identity: %s", caller)
	logging.Debugf(c, "MintServiceOAuthTokenRequest:\n%s", dump)
}

// checkRequestFormat returns an error if the request is not well formatted.
func (r *MintServiceOAuthTokenRPC) checkRequestFormat(req *minter.MintServiceOAuthTokenRequest) error {
	switch {
	case req.LuciProject == "":
		return fmt.Errorf("project name is required")
	case req.MinValidityDuration < 0:
		return fmt.Errorf("min_validity_duration must be positive, not %d", req.MinValidityDuration)
	case len(req.OauthScope) == 0:
		return fmt.Errorf("oauth_scope is required")
	}
	return nil
}

// checkRules verifies the requested token is allowed to be issued according to the rules.
func (r *MintServiceOAuthTokenRPC) checkRules(c context.Context, req *minter.MintServiceOAuthTokenRequest, caller identity.Identity) (*Rule, error) {
	rules, err := r.Rules(c)
	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to load project-scoped service account rules")
		return nil, status.Errorf(codes.Internal, "failed to load project-scoped service account rules")
	}
	rule, err := rules.Check(c, &RulesQuery{ /*TBD*/ })
	if err != nil {
		return nil, err
	}

	if req.MinValidityDuration > rule.Rule.MaxGrantValidityDuration {
		logging.Errorf(c, "Requested validity is larger than allowed: %d > %d", req.MinValidityDuration, rule.Rule.MaxGrantValidityDuration)
		return nil, status.Errorf(codes.InvalidArgument, "per rule %q the validity duration should be <= %d", rule.Rule.Name, rule.Rule.MaxGrantValidityDuration)
	}
	return rule, nil
}

func (r *MintServiceOAuthTokenRPC) mint(c context.Context, p *mintParams) (*minter.MintServiceOAuthTokenResponse, *tokenserver.OAuthTokenGrantBody, error) {
	id, err := revocation.GenerateTokenID(c, tokenIDSequenceKind)
	if err != nil {
		logging.WithError(err).Errorf(c, "Error when generating token ID")
		return nil, nil, status.Errorf(codes.Internal, "error when generating token ID - %s", err)
	}

	now := clock.Now(c).UTC()
	expiry := now.Add(time.Duration(p.validityDuration) * time.Second)

	// All the stuff here has already been validated in 'MintOAuthTokenGrant'.
	body := &tokenserver.OAuthTokenGrantBody{
		TokenId:          id,
		ServiceAccount:   p.serviceAccount,
		Proxy:            string(p.proxyID),
		EndUser:          string(p.endUserID),
		IssuedAt:         google.NewTimestamp(now),
		ValidityDuration: p.validityDuration,
	}
	signed, err := SignGrant(c, r.Signer, body)
	if err != nil {
		logging.WithError(err).Errorf(c, "Error when signing the token")
		return nil, nil, status.Errorf(codes.Internal, "error when signing the token - %s", err)
	}

	return &minter.MintServiceOAuthTokenResponse{
		GrantToken:     signed,
		Expiry:         google.NewTimestamp(expiry),
		ServiceVersion: p.serviceVer,
	}, body, nil
}
