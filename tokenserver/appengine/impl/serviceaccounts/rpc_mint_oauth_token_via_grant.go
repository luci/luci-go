// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package serviceaccounts

import (
	"fmt"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/gae/service/info"

	"go.chromium.org/luci/common/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/signing"

	"go.chromium.org/luci/tokenserver/api"
	"go.chromium.org/luci/tokenserver/api/minter/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils"
)

const (
	// Maximum allowed value for a duration conveyed via 'min_validity_duration'
	// field in MintOAuthTokenViaGrant RPC call.
	//
	// We restrict it because setting values close to 1h reduces effectiveness of
	// the OAuth token cache (maintained by the token server). For example, if
	// the client requests a token with min_validity_duration 59 min, we can reuse
	// cached tokens generated only within last minute. All older tokens will be
	// considered too old for this client.
	//
	// It is assumed that clients are calling MintOAuthTokenViaGrant right before
	// an OAuth token is needed (instead of preparing tokens far in advance). This
	// permits usage of small 'min_validity_duration' (on order of minutes), which
	// is good for the cache effectiveness.
	maxAllowedMinValidityDuration = 30 * time.Minute

	// A value for minimal returned token lifetime if 'min_validity_duration'
	// field is not specified in the request.
	defaultMinValidityDuration = 5 * time.Minute
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
		return nil, grpc.Errorf(codes.PermissionDenied, "delegation is forbidden for this API call")
	}

	grantBody, rule, err := r.validateRequest(c, req, callerID)
	if err != nil {
		return nil, err // the error is already logged
	}

	// Now that the request is verified, use parameters contained there to
	// generate the requested OAuth token.
	minValidityDuration := time.Duration(req.MinValidityDuration) * time.Second
	if minValidityDuration == 0 {
		minValidityDuration = defaultMinValidityDuration
	}
	accessTok, err := r.MintAccessToken(c, auth.MintAccessTokenParams{
		ServiceAccount: grantBody.ServiceAccount,
		Scopes:         req.OauthScope,
		MinTTL:         minValidityDuration,
	})
	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to mint oauth token for %q", grantBody.ServiceAccount)
		return nil, grpc.Errorf(codes.Internal, "failed to mint oauth token for %q - %s", grantBody.ServiceAccount, err)
	}

	// Grab a string that identifies token server version. This almost always
	// just hits local memory cache.
	serviceVer, err := utils.ServiceVersion(c, r.Signer)
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, "can't grab service version - %s", err)
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
	// Log everything but the token. It'll be logged later after base64 decoding.
	r.logRequest(c, req, caller)

	// Reject obviously bad requests.
	if err := r.checkRequestFormat(req); err != nil {
		logging.WithError(err).Errorf(c, "Bad request")
		return nil, nil, grpc.Errorf(codes.InvalidArgument, "bad request - %s", err)
	}

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
		return nil, nil, grpc.Errorf(codes.PermissionDenied, "unauthorized caller (expecting %s)", grantBody.Proxy)
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
		return nil, nil, grpc.Errorf(codes.PermissionDenied, "bad scopes - %s", err)
	}

	return grantBody, rule, nil
}

// logRequest logs the body of the request, omitting the grant token.
//
// The token is logged later after base64 decoding.
func (r *MintOAuthTokenViaGrantRPC) logRequest(c context.Context, req *minter.MintOAuthTokenViaGrantRequest, caller identity.Identity) {
	if !logging.IsLogging(c, logging.Debug) {
		return
	}
	cpy := *req
	if cpy.GrantToken != "" {
		cpy.GrantToken = "..."
	}
	m := jsonpb.Marshaler{Indent: "  "}
	dump, _ := m.MarshalToString(&cpy)
	logging.Debugf(c, "Identity: %s", caller)
	logging.Debugf(c, "MintOAuthTokenViaGrant:\n%s", dump)
}

// checkRequestFormat returns an error if the request is obviously wrong.
func (r *MintOAuthTokenViaGrantRPC) checkRequestFormat(req *minter.MintOAuthTokenViaGrantRequest) error {
	switch minDur := time.Duration(req.MinValidityDuration) * time.Second; {
	case minDur < 0:
		return fmt.Errorf("min_validity_duration must be positive")
	case minDur > maxAllowedMinValidityDuration:
		return fmt.Errorf("min_validity_duration must not exceed %d", maxAllowedMinValidityDuration/time.Second)
	case len(req.OauthScope) == 0:
		return fmt.Errorf("oauth_scope is required")
	}
	if err := utils.ValidateAuditTags(req.AuditTags); err != nil {
		return fmt.Errorf("bad audit_tags - %s", err)
	}
	return nil
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
		return nil, grpc.Errorf(codes.Internal, err.Error())
	}

	// This is non-nil for tokens with a valid body, even if they aren't properly
	// signed or they have already expired. These additional checks are handled
	// below after we log the body.
	grantBody, _ := inspection.Body.(*tokenserver.OAuthTokenGrantBody)
	if grantBody == nil {
		logging.Errorf(c, "Malformed grant token - %s", inspection.InvalidityReason)
		return nil, grpc.Errorf(codes.InvalidArgument, "malformed grant token - %s", inspection.InvalidityReason)
	}
	if logging.IsLogging(c, logging.Debug) {
		m := jsonpb.Marshaler{Indent: "  "}
		dump, _ := m.MarshalToString(grantBody)
		logging.Debugf(c, "OAuthTokenGrantBody:\n%s", dump)
	}

	if inspection.InvalidityReason != "" {
		logging.Errorf(c, "Invalid grant token - %s", inspection.InvalidityReason)
		return nil, grpc.Errorf(codes.InvalidArgument, "invalid grant token - %s", inspection.InvalidityReason)
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
		return nil, grpc.Errorf(codes.Internal, "failed to load service accounts rules")
	}
	return rules.Check(c, &RulesQuery{
		ServiceAccount: grantBody.ServiceAccount,
		Proxy:          identity.Identity(grantBody.Proxy),
		EndUser:        identity.Identity(grantBody.EndUser),
	})
}
