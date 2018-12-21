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

package projectscope

import (
	"context"
	"time"

	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/tokenserver/appengine/impl/serviceaccounts"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/projectscope"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/signing"
	"golang.org/x/oauth2"

	"fmt"

	"github.com/golang/protobuf/jsonpb"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging"
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
	LogOAuthToken func(context.Context, serviceaccounts.LoggableOAuthTokenInfo) error

	// Storage manages project scoped identities.
	//
	// In  prod it is projectscope.persistentIdentityManager
	Storage projectscope.ScopedIdentityManager

	// CreateServiceAccount can be set to use in tests
	CreateServiceAccount projectscope.ServiceAccountCreator
}

func (r *MintServiceOAuthTokenRPC) getGCPProject() string {
	return ""
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
		if rule.Rule.MaxValidityDuration > 3600 {
			req.MinValidityDuration = 3600
		} else {
			req.MinValidityDuration = rule.Rule.MaxValidityDuration
		}
	}

	// Scoped identity should not exist yet, and we'll try to eagerly create the IAM service account
	scopedIdentity, created, err := r.Storage.GetOrCreate(c, string(callerId), req.LuciProject, rule.CloudProjectID, r.CreateServiceAccount)
	if err != nil {
		logging.WithError(err)
		return nil, status.Errorf(codes.Internal, err.Error())

	}
	if !created {
		return nil, status.Errorf(codes.AlreadyExists, "the project scoped service account already exists")
	}

	accessTok, err := r.MintAccessToken(c, auth.MintAccessTokenParams{
		ServiceAccount: scopedIdentity.AccountId,
		Scopes:         req.OauthScope,
		MinTTL:         time.Duration(int64(time.Second) * req.MinValidityDuration),
	})

	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to mint scoped oauth token for service %q in project %q", callerId, req.LuciProject)
		code := codes.InvalidArgument
		if transient.Tag.In(err) {
			code = codes.Internal
		}
		return nil, status.Errorf(code, "failed to mint scoped oauth token for service %q in project %q", callerId, req.LuciProject)
	}

	serviceVer, err := utils.ServiceVersion(c, r.Signer)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "can't grab service version - %s", err)
	}

	resp := &minter.MintServiceOAuthTokenResponse{
		ServiceAccount: scopedIdentity.ServiceAccount.Email,
		AccessToken:    accessTok.AccessToken,
		Expiry:         google.NewTimestamp(accessTok.Expiry),
		ServiceVersion: serviceVer,
	}

	// Log it to BigQuery
	if r.LogOAuthToken != nil {
		// Errors during logging are considered not fatal. bqlog library has
		// a monitoring counter that tracks number of errors, so they are not
		// totally invisible.
		info := serviceaccounts.MintedServiceOAuthTokenInfo{
			Request:  req,
			Response: resp,
			Rule:     rule.Rule,
			OAuthTokenInfo: serviceaccounts.OAuthTokenInfo{
				RequestedAt: clock.Now(c),
				ConfigRev:   rule.Revision,
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

// validateRequest performs basic request validation and pulls the configuration context according to the caller identity.
func (r *MintServiceOAuthTokenRPC) validateRequest(c context.Context, req *minter.MintServiceOAuthTokenRequest, caller identity.Identity) (*Rule, error) {
	r.logRequest(c, req, caller)

	// Check requires proto fields
	if req.LuciProject == "" {
		return nil, status.Errorf(codes.InvalidArgument, "luci project must be specified")
	}
	if len(req.OauthScope) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "oauth scopes must be specified")
	}

	// Pick the proper configuration for the calling service
	rules, err := r.Rules(c)
	if err != nil {
		return nil, err
	}
	rule, found := rules.rulesPerService[string(caller)]
	if !found {
		return nil, status.Errorf(codes.NotFound, "service %s not registered", caller)
	}
	return rule, nil
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
	rule, found := rules.rulesPerService[string(caller)]
	if !found {
		return nil, status.Errorf(codes.NotFound, "service %s not registered", caller)
	}

	if req.MinValidityDuration > rule.Rule.MaxValidityDuration {
		logging.Errorf(c, "Requested validity is larger than allowed: %d > %d", req.MinValidityDuration, rule.Rule.MaxValidityDuration)
		return nil, status.Errorf(codes.InvalidArgument, "per service config %q the validity duration should be <= %d", rule.Rule.Service, rule.Rule.MaxValidityDuration)
	}
	return rule, nil
}
