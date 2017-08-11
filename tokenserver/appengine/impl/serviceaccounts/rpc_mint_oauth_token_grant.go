// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package serviceaccounts

import (
	"fmt"
	"time"

	"github.com/golang/protobuf/jsonpb"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/gae/service/info"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/identity"
	"go.chromium.org/luci/server/auth/signing"

	"go.chromium.org/luci/tokenserver/api"
	"go.chromium.org/luci/tokenserver/api/minter/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/revocation"
)

// tokenIDSequenceKind defines the namespace of int64 IDs for grant tokens.
//
// Changing it will effectively reset the ID generation.
const tokenIDSequenceKind = "oauthTokenGrantID"

// MintOAuthTokenGrantRPC implements TokenMinter.MintOAuthTokenGrant method.
type MintOAuthTokenGrantRPC struct {
	// Signer is mocked in tests.
	//
	// In prod it is gaesigner.Signer.
	Signer signing.Signer

	// Rules returns service account rules to use for the request.
	//
	// In prod it is GlobalRulesCache.Rules.
	Rules func(context.Context) (*Rules, error)

	// LogGrant is mocked in tests.
	//
	// In prod it is LogGrant from grant_bigquery_log.go.
	LogGrant func(context.Context, *MintedGrantInfo) error

	// mintMock call is used in tests.
	//
	// In prod it is 'mint'
	mintMock func(context.Context, *mintParams) (*minter.MintOAuthTokenGrantResponse, *tokenserver.OAuthTokenGrantBody, error)
}

// MintOAuthTokenGrant produces new OAuth token grant.
func (r *MintOAuthTokenGrantRPC) MintOAuthTokenGrant(c context.Context, req *minter.MintOAuthTokenGrantRequest) (*minter.MintOAuthTokenGrantResponse, error) {
	state := auth.GetState(c)

	// Don't allow delegation tokens here to reduce total number of possible
	// scenarios. Proxies aren't expected to use delegation for these tokens.
	callerID := state.User().Identity
	if callerID != state.PeerIdentity() {
		logging.Errorf(c, "Trying to use delegation, it's forbidden")
		return nil, grpc.Errorf(codes.PermissionDenied, "delegation is forbidden for this API call")
	}

	// Check that the request is allowed by the rules, fill in defaults.
	rule, err := r.validateRequest(c, req, callerID)
	if err != nil {
		return nil, err // the error is already logged
	}
	if req.ValidityDuration == 0 {
		if rule.Rule.MaxGrantValidityDuration > 3600 {
			req.ValidityDuration = 3600
		} else {
			req.ValidityDuration = rule.Rule.MaxGrantValidityDuration
		}
	}

	// Grab a string that identifies token server version. This almost always
	// just hits local memory cache.
	serviceVer, err := utils.ServiceVersion(c, r.Signer)
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, "can't grab service version - %s", err)
	}

	// Generate and sign the token.
	var resp *minter.MintOAuthTokenGrantResponse
	var body *tokenserver.OAuthTokenGrantBody
	p := mintParams{
		serviceAccount:   req.ServiceAccount,
		proxyID:          callerID,
		endUserID:        identity.Identity(req.EndUser), // already validated
		validityDuration: req.ValidityDuration,
		serviceVer:       serviceVer,
	}
	if r.mintMock != nil {
		resp, body, err = r.mintMock(c, &p)
	} else {
		resp, body, err = r.mint(c, &p)
	}
	if err != nil {
		return nil, err
	}

	// Log it to BigQuery.
	if r.LogGrant != nil {
		// Errors during logging are considered not fatal. bqlog library has
		// a monitoring counter that tracks number of errors, so they are not
		// totally invisible.
		info := MintedGrantInfo{
			Request:   req,
			Response:  resp,
			GrantBody: body,
			ConfigRev: rule.Revision,
			Rule:      rule.Rule,
			PeerIP:    state.PeerIP(),
			RequestID: info.RequestID(c),
			AuthDB:    state.DB(),
		}
		if logErr := r.LogGrant(c, &info); logErr != nil {
			logging.WithError(logErr).Errorf(c, "Failed to insert the grant token into the BigQuery log")
		}
	}

	return resp, nil
}

// validateRequest checks that the request is allowed.
//
// Returns corresponding config rule on success or a grpc error on error.
func (r *MintOAuthTokenGrantRPC) validateRequest(c context.Context, req *minter.MintOAuthTokenGrantRequest, caller identity.Identity) (*Rule, error) {
	// Dump the whole request and relevant auth state to the debug log.
	r.logRequest(c, req, caller)

	// Reject obviously bad requests.
	if err := r.checkRequestFormat(req); err != nil {
		logging.WithError(err).Errorf(c, "Bad request")
		return nil, grpc.Errorf(codes.InvalidArgument, "bad request - %s", err)
	}

	// TODO(vadimsh): Verify that this user is present by requiring the end user's
	// credentials, e.g make Swarming forward user's OAuth token to the token
	// server, so it can be validated here.

	// Check that requested usage is allowed and grab the corresponding rule.
	return r.checkRules(c, req, caller)
}

// logRequest logs the body of the request.
func (r *MintOAuthTokenGrantRPC) logRequest(c context.Context, req *minter.MintOAuthTokenGrantRequest, caller identity.Identity) {
	if !logging.IsLogging(c, logging.Debug) {
		return
	}
	m := jsonpb.Marshaler{Indent: "  "}
	dump, _ := m.MarshalToString(req)
	logging.Debugf(c, "Identity: %s", caller)
	logging.Debugf(c, "MintOAuthTokenGrantRequest:\n%s", dump)
}

// checkRequestFormat returns an error if the request is obviously wrong.
func (r *MintOAuthTokenGrantRPC) checkRequestFormat(req *minter.MintOAuthTokenGrantRequest) error {
	switch {
	case req.ServiceAccount == "":
		return fmt.Errorf("service_account is required")
	case req.ValidityDuration < 0:
		return fmt.Errorf("validity_duration must be positive, not %d", req.ValidityDuration)
	case req.EndUser == "":
		return fmt.Errorf("end_user is required")
	}
	if _, err := identity.MakeIdentity(req.EndUser); err != nil {
		return fmt.Errorf("bad end_user - %s", err)
	}
	return nil
}

// checkRules verifies the requested token is allowed by the rules.
//
// Returns the matching rule or a grpc error.
func (r *MintOAuthTokenGrantRPC) checkRules(c context.Context, req *minter.MintOAuthTokenGrantRequest, caller identity.Identity) (*Rule, error) {
	rules, err := r.Rules(c)
	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to load service accounts rules")
		return nil, grpc.Errorf(codes.Internal, "failed to load service accounts rules")
	}

	rule, err := rules.Check(c, &RulesQuery{
		ServiceAccount: req.ServiceAccount,
		Proxy:          caller,
		EndUser:        identity.Identity(req.EndUser),
	})
	if err != nil {
		return nil, err // it is already gRPC error, and it's already logged
	}

	// ValidityDuration check is specific to this RPC, it's not done by 'Check'.
	if req.ValidityDuration > rule.Rule.MaxGrantValidityDuration {
		logging.Errorf(c, "Requested validity is larger than max allowed: %d > %d", req.ValidityDuration, rule.Rule.MaxGrantValidityDuration)
		return nil, grpc.Errorf(codes.InvalidArgument, "per rule %q the validity duration should be <= %d", rule.Rule.Name, rule.Rule.MaxGrantValidityDuration)
	}

	// Note that AllowedScopes is checked later during MintOAuthTokenViaGrant.
	// Here we don't even know what OAuth scopes will be requested.

	return rule, nil
}

////////////////////////////////////////////////////////////////////////////////

type mintParams struct {
	serviceAccount   string
	proxyID          identity.Identity
	endUserID        identity.Identity
	validityDuration int64
	serviceVer       string
}

// mint is called to make the token after the request has been authorized.
func (r *MintOAuthTokenGrantRPC) mint(c context.Context, p *mintParams) (*minter.MintOAuthTokenGrantResponse, *tokenserver.OAuthTokenGrantBody, error) {
	id, err := revocation.GenerateTokenID(c, tokenIDSequenceKind)
	if err != nil {
		logging.WithError(err).Errorf(c, "Error when generating token ID")
		return nil, nil, grpc.Errorf(codes.Internal, "error when generating token ID - %s", err)
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
		return nil, nil, grpc.Errorf(codes.Internal, "error when signing the token - %s", err)
	}

	return &minter.MintOAuthTokenGrantResponse{
		GrantToken:     signed,
		Expiry:         google.NewTimestamp(expiry),
		ServiceVersion: p.serviceVer,
	}, body, nil
}
