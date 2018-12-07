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
	LogOAuthToken func(context.Context, *MintedOAuthTokenInfo) error
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

	return &minter.MintServiceOAuthTokenResponse{}, nil
}

func (r *MintServiceOAuthTokenRPC) validateRequest(c context.Context, req *minter.MintServiceOAuthTokenRequest, caller identity.Identity) (*Rule, error) {
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
	return nil, nil, nil
}
