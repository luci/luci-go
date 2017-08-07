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

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/identity"
	"github.com/luci/luci-go/server/auth/signing"

	"github.com/luci/luci-go/tokenserver/api"
	"github.com/luci/luci-go/tokenserver/api/minter/v1"
	"github.com/luci/luci-go/tokenserver/appengine/impl/utils"
	"github.com/luci/luci-go/tokenserver/appengine/impl/utils/revocation"
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

	// mintMock call is used in tests.
	//
	// In prod it is 'mint'
	mintMock func(context.Context, *mintParams) (*minter.MintOAuthTokenGrantResponse, error)
}

// MintOAuthTokenGrant produces new OAuth token grant.
func (r *MintOAuthTokenGrantRPC) MintOAuthTokenGrant(c context.Context, req *minter.MintOAuthTokenGrantRequest) (*minter.MintOAuthTokenGrantResponse, error) {
	state := auth.GetState(c)

	// Dump the whole request and relevant auth state to the debug log.
	callerID := state.User().Identity
	if logging.IsLogging(c, logging.Debug) {
		m := jsonpb.Marshaler{Indent: "  "}
		dump, _ := m.MarshalToString(req)
		logging.Debugf(c, "Identity: %s", callerID)
		logging.Debugf(c, "MintOAuthTokenGrantRequest:\n%s", dump)
	}

	// Grab a string that identifies token server version. This almost always
	// just hits local memory cache.
	serviceVer, err := utils.ServiceVersion(c, r.Signer)
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, "can't grab service version - %s", err)
	}

	// Reject obviously bad requests (and parse end_user along the way).
	switch {
	case req.ServiceAccount == "":
		err = fmt.Errorf("service_account is required")
	case req.ValidityDuration < 0:
		err = fmt.Errorf("validity_duration must be positive, not %d", req.ValidityDuration)
	case req.EndUser == "":
		err = fmt.Errorf("end_user is required")
	}
	var endUserID identity.Identity
	if err == nil {
		if endUserID, err = identity.MakeIdentity(req.EndUser); err != nil {
			err = fmt.Errorf("bad end_user - %s", err)
		}
	}
	if err != nil {
		logging.WithError(err).Errorf(c, "Bad request")
		return nil, grpc.Errorf(codes.InvalidArgument, "bad request - %s", err)
	}

	// TODO(vadimsh): Verify that this user is present by requiring the end user's
	// credentials, e.g make Swarming forward user's OAuth token to the token
	// server, so it can be validated here.

	// Fetch service account rules. They are hot in memory most of the time.
	rules, err := r.Rules(c)
	if err != nil {
		// Don't put error details in the message, it may be returned to
		// unauthorized callers.
		logging.WithError(err).Errorf(c, "Failed to load service accounts rules")
		return nil, grpc.Errorf(codes.Internal, "failed to load service accounts rules")
	}

	// Check that requested usage is allowed and grab the corresponding rule.
	rule, err := rules.Check(c, &RulesQuery{
		ServiceAccount: req.ServiceAccount,
		Proxy:          callerID,
		EndUser:        endUserID,
	})
	if err != nil {
		return nil, err // it is already gRPC error, and it's already logged
	}

	// ValidityDuration check is specific to this RPC, it's not done by 'Check'.
	if req.ValidityDuration == 0 {
		req.ValidityDuration = 3600
	}
	if req.ValidityDuration > rule.Rule.MaxGrantValidityDuration {
		logging.Errorf(c, "Requested validity is larger than max allowed: %d > %d", req.ValidityDuration, rule.Rule.MaxGrantValidityDuration)
		return nil, grpc.Errorf(codes.InvalidArgument, "per rule %q the validity duration should be <= %d", rule.Rule.Name, rule.Rule.MaxGrantValidityDuration)
	}

	// All checks are done! Note that AllowedScopes is checked later during
	// MintOAuthTokenViaGrant. Here we don't even know what OAuth scopes will be
	// requested.
	var resp *minter.MintOAuthTokenGrantResponse
	p := mintParams{
		serviceAccount:   req.ServiceAccount,
		proxyID:          callerID,
		endUserID:        endUserID,
		validityDuration: req.ValidityDuration,
		serviceVer:       serviceVer,
	}
	if r.mintMock != nil {
		resp, err = r.mintMock(c, &p)
	} else {
		resp, err = r.mint(c, &p)
	}
	if err != nil {
		return nil, err
	}

	// TODO(vadimsh): Log the generated token to BigQuery.

	return resp, nil
}

type mintParams struct {
	serviceAccount   string
	proxyID          identity.Identity
	endUserID        identity.Identity
	validityDuration int64
	serviceVer       string
}

// mint is called to make the token after the request has been authorized.
func (r *MintOAuthTokenGrantRPC) mint(c context.Context, p *mintParams) (*minter.MintOAuthTokenGrantResponse, error) {
	id, err := revocation.GenerateTokenID(c, tokenIDSequenceKind)
	if err != nil {
		logging.WithError(err).Errorf(c, "Error when generating token ID")
		return nil, grpc.Errorf(codes.Internal, "error when generating token ID - %s", err)
	}

	now := clock.Now(c).UTC()
	expiry := now.Add(time.Duration(p.validityDuration) * time.Second)

	// All the stuff here has already been validated in 'MintOAuthTokenGrant'.
	signed, err := SignGrant(c, r.Signer, &tokenserver.OAuthTokenGrantBody{
		TokenId:          id,
		ServiceAccount:   p.serviceAccount,
		Proxy:            string(p.proxyID),
		EndUser:          string(p.endUserID),
		IssuedAt:         google.NewTimestamp(now),
		ValidityDuration: p.validityDuration,
	})
	if err != nil {
		logging.WithError(err).Errorf(c, "Error when signing the token")
		return nil, grpc.Errorf(codes.Internal, "error when signing the token - %s", err)
	}

	return &minter.MintOAuthTokenGrantResponse{
		GrantToken:     signed,
		Expiry:         google.NewTimestamp(expiry),
		ServiceVersion: p.serviceVer,
	}, nil
}
