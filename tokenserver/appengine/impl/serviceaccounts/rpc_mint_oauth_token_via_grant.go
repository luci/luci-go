// Copyright 2017 The LUCI Authors.
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
	"fmt"
	"time"

	"github.com/golang/protobuf/jsonpb"

	"golang.org/x/oauth2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/gae/service/info"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/signing"

	tokenserver "go.chromium.org/luci/tokenserver/api"
	"go.chromium.org/luci/tokenserver/api/minter/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils"
)

// MintOAuthTokenViaGrantRPC implements TokenMinter.MintOAuthTokenViaGrant
// method.
type MintOAuthTokenViaGrantRPC struct {
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
	LogOAuthToken func(context.Context, *MintedOAuthTokenInfo) error
}

// MintOAuthTokenViaGrant produces new OAuth token given a grant.
func (r *MintOAuthTokenViaGrantRPC) MintOAuthTokenViaGrant(c context.Context, req *minter.MintOAuthTokenViaGrantRequest) (*minter.MintOAuthTokenViaGrantResponse, error) {
	state := auth.GetState(c)

	// Don't allow delegation tokens here to reduce total number of possible
	// scenarios. Proxies aren't expected to use delegation for these tokens.
	callerID := state.User().Identity
	if callerID != state.PeerIdentity() {
		logging.Errorf(c, "Trying to use delegation, it's forbidden")
		return nil, status.Errorf(codes.PermissionDenied, "delegation is forbidden for this API call")
	}

	// Prevent GrantToken from leaking into the logs.
	reqcpy := *req
	if reqcpy.GrantToken != "" {
		reqcpy.GrantToken = "..."
	}
	utils.LogRequest(c, r, &reqcpy, callerID)

	if err := utils.ValidateAndNormalizeRequest(c, req.OauthScope, &req.MinValidityDuration, req.AuditTags); err != nil {
		if status.Code(err) == codes.Unknown {
			return nil, status.Errorf(codes.InvalidArgument, "invalid request: %q", err)
		}
		return nil, err
	}
	grantBody, rule, err := r.validateRequest(c, req, callerID)
	if err != nil {
		// err was already logged.
		return nil, err
	}

	accessTok, err := r.MintAccessToken(c, auth.MintAccessTokenParams{
		ServiceAccount: grantBody.ServiceAccount,
		Scopes:         req.OauthScope,
		MinTTL:         time.Duration(req.MinValidityDuration) * time.Second,
	})
	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to mint oauth token for %q", grantBody.ServiceAccount)
		code := codes.InvalidArgument // mostly likely misconfigured IAM roles
		if transient.Tag.In(err) {
			code = codes.Internal
		}
		return nil, status.Errorf(code, "failed to mint oauth token for %q - %s", grantBody.ServiceAccount, err)
	}

	// Grab a string that identifies token server version. This almost always
	// just hits local memory cache.
	serviceVer, err := utils.ServiceVersion(c, r.Signer)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "can't grab service version - %s", err)
	}

	// The RPC response.
	resp := &minter.MintOAuthTokenViaGrantResponse{
		AccessToken:    accessTok.AccessToken,
		Expiry:         google.NewTimestamp(accessTok.Expiry),
		ServiceVersion: serviceVer,
	}

	// Log it to BigQuery.
	if r.LogOAuthToken != nil {
		// Errors during logging are considered not fatal. bqlog library has
		// a monitoring counter that tracks number of errors, so they are not
		// totally invisible.
		info := MintedOAuthTokenInfo{
			RequestedAt: clock.Now(c),
			Request:     req,
			Response:    resp,
			GrantBody:   grantBody,
			ConfigRev:   rule.Revision,
			Rule:        rule.Rule,
			PeerIP:      state.PeerIP(),
			RequestID:   info.RequestID(c),
			AuthDBRev:   authdb.Revision(state.DB()),
		}
		if logErr := r.LogOAuthToken(c, &info); logErr != nil {
			logging.WithError(logErr).Errorf(c, "Failed to insert the oauth token into the BigQuery log")
		}
	}

	return resp, nil
}

// validateRequest decodes the request and checks that it is allowed.
//
// Logs and returns verified deserialized token body and corresponding rule on
// success or a grpc error on error.
func (r *MintOAuthTokenViaGrantRPC) validateRequest(c context.Context, req *minter.MintOAuthTokenViaGrantRequest, caller identity.Identity) (*tokenserver.OAuthTokenGrantBody, *Rule, error) {
	// Grab the token body, if it is valid.
	grantBody, err := r.decodeAndValidateToken(c, req.GrantToken)
	if err != nil {
		return nil, nil, err
	}

	// The token is usable only by whoever requested it in the first place.
	if grantBody.Proxy != string(caller) {
		// Note: grantBody.Proxy is part of the token already, caller knows it, so
		// we aren't exposing any new information by returning it in the message.
		logging.Errorf(c, "Unauthorized caller (expecting %q)", grantBody.Proxy)
		return nil, nil, status.Errorf(codes.PermissionDenied, "unauthorized caller (expecting %s)", grantBody.Proxy)
	}

	// Check that rules still allow this token (rules could have changed since
	// the grant was generated).
	rule, err := r.recheckRules(c, grantBody)
	if err != nil {
		return nil, nil, err
	}

	// OAuth scopes check is specific to this RPC, it's not done by recheckRules.
	if err := rule.CheckScopes(req.OauthScope); err != nil {
		logging.WithError(err).Errorf(c, "Bad scopes")
		return nil, nil, status.Errorf(codes.PermissionDenied, "bad scopes - %s", err)
	}

	return grantBody, rule, nil
}

// decodeAndValidateToken checks the token signature, expiration time and
// unmarshals it.
//
// Logs and returns deserialized token body on success or a grpc error on error.
func (r *MintOAuthTokenViaGrantRPC) decodeAndValidateToken(c context.Context, grantToken string) (*tokenserver.OAuthTokenGrantBody, error) {
	// Attempt to decode the grant and log all information we can get (even if the
	// token is no longer technically valid). This information helps to debug
	// invalid tokens. InspectGrant returns an error only if the inspection
	// operation itself fails. If the token is invalid, it returns an inspection
	// with non-empty InvalidityReason.
	inspection, err := InspectGrant(c, r.Signer, grantToken)
	if err != nil {
		return nil, status.Errorf(codes.Internal, err.Error())
	}

	// This is non-nil for tokens with a valid body, even if they aren't properly
	// signed or they have already expired. These additional checks are handled
	// below after we log the body.
	grantBody, _ := inspection.Body.(*tokenserver.OAuthTokenGrantBody)
	if grantBody == nil {
		logging.Errorf(c, "Malformed grant token - %s", inspection.InvalidityReason)
		return nil, status.Errorf(codes.InvalidArgument, "malformed grant token - %s", inspection.InvalidityReason)
	}
	if logging.IsLogging(c, logging.Debug) {
		m := jsonpb.Marshaler{Indent: "  "}
		dump, _ := m.MarshalToString(grantBody)
		logging.Debugf(c, "OAuthTokenGrantBody:\n%s", dump)
	}

	if inspection.InvalidityReason != "" {
		logging.Errorf(c, "Invalid grant token - %s", inspection.InvalidityReason)
		return nil, status.Errorf(codes.InvalidArgument, "invalid grant token - %s", inspection.InvalidityReason)
	}

	// At this point we've verified 'grantToken' was issued by us (it is signed)
	// and it hasn't expired yet. Assert this.
	if !inspection.Signed || !inspection.NonExpired {
		panic(fmt.Sprintf("assertion failure Signed=%v, NonExpired=%v", inspection.Signed, inspection.NonExpired))
	}

	return grantBody, nil
}

// recheckRules verifies the token is still allowed by the rules.
//
// Returns a grpc error if the token no longer matches the rules.
func (r *MintOAuthTokenViaGrantRPC) recheckRules(c context.Context, grantBody *tokenserver.OAuthTokenGrantBody) (*Rule, error) {
	// Check that rules still allow this token (rules could have changed since
	// the grant was generated).
	rules, err := r.Rules(c)
	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to load service accounts rules")
		return nil, status.Errorf(codes.Internal, "failed to load service accounts rules")
	}
	return rules.Check(c, &RulesQuery{
		ServiceAccount: grantBody.ServiceAccount,
		Proxy:          identity.Identity(grantBody.Proxy),
		EndUser:        identity.Identity(grantBody.EndUser),
	})
}

// Name implements utils.RPC interface.
func (r *MintOAuthTokenViaGrantRPC) Name() string {
	return "MintOAuthTokenViaGrantRPC"
}
