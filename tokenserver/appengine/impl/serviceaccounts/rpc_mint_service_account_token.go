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

package serviceaccounts

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/trace"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/signing"

	"go.chromium.org/luci/tokenserver/api/minter/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils"
)

var (
	// Grants permission to mint tokens for accounts that belong to a realm.
	permMintToken = realms.RegisterPermission("luci.serviceAccounts.mintToken")
	// Grants permission to *be* a service account that is in the realm.
	permExistInRealm = realms.RegisterPermission("luci.serviceAccounts.existInRealm")
)

// MintServiceAccountTokenRPC implements the corresponding method.
type MintServiceAccountTokenRPC struct {
	// Signer is used only for its ServiceInfo.
	//
	// In prod it is the default server signer that uses server's service account.
	Signer signing.Signer

	// Mapping returns project<->account mapping to use for the request.
	//
	// In prod it is GlobalMappingCache.Mapping.
	Mapping func(context.Context) (*Mapping, error)

	// MintAccessToken produces an OAuth token for a service account.
	//
	// In prod it is auth.MintAccessTokenForServiceAccount.
	MintAccessToken func(context.Context, auth.MintAccessTokenParams) (*auth.Token, error)

	// MintIDToken produces an ID token for a service account.
	//
	// In prod it is auth.MintIDTokenForServiceAccount.
	MintIDToken func(context.Context, auth.MintIDTokenParams) (*auth.Token, error)

	// LogToken is mocked in tests.
	//
	// In prod it is produced by NewTokenLogger.
	LogToken TokenLogger
}

// validatedRequest is extracted from MintServiceAccountTokenRequest.
type validatedRequest struct {
	kind            minter.ServiceAccountTokenKind
	account         string   // e.g. "something@blah.iam.gserviceaccount.com"
	realm           string   // e.g. "<project>:<realm>"
	project         string   // just "<project>" part
	oauthScopes     []string // non-empty iff kind is ..._ACCESS_TOKEN
	idTokenAudience string   // non-empty iff kind is ..._ID_TOKEN
	minTTL          time.Duration
	auditTags       []string
}

// callEnv groups a bunch of arguments to simplify passing them to functions.
//
// They all are basically extracted from context.Context and do not depend on
// the body of the request.
type callEnv struct {
	state   auth.State
	db      authdb.DB
	caller  identity.Identity // used in ACLs
	peer    identity.Identity // used in logs only
	mapping *Mapping
}

// MintServiceAccountToken mints an OAuth2 access token or OpenID ID token
// that belongs to some service account using LUCI Realms for authorization.
//
// As an input it takes a service account email and a name of a LUCI Realm the
// caller is operating in. To authorize the call the token server checks the
// following conditions:
//   1. The caller has luci.serviceAccounts.mintToken permission in the
//      realm, allowing them to "impersonate" all service accounts belonging
//      to this realm.
//   2. The service account has luci.serviceAccounts.existInRealm permission
//      in the realm. This makes the account "belong" to the realm.
//   3. Realm's LUCI project has the service account associated with it in
//      the project_owned_accounts.cfg global config file. This makes sure
//      different LUCI projects can't just arbitrary use each others accounts
//      by adding them to their respective realms.cfg. See also comments for
//      ServiceAccountsProjectMapping in api/admin/v1/config.proto.
func (r *MintServiceAccountTokenRPC) MintServiceAccountToken(ctx context.Context, req *minter.MintServiceAccountTokenRequest) (*minter.MintServiceAccountTokenResponse, error) {
	state := auth.GetState(ctx)
	env := &callEnv{
		state:  state,
		db:     state.DB(),
		caller: state.User().Identity,
		peer:   state.PeerIdentity(),
	}

	// Mapping is needed to check ACLs (step 3).
	var err error
	if env.mapping, err = r.Mapping(ctx); err != nil {
		logging.Errorf(ctx, "Failed to grab Mapping: %s", err)
		return nil, status.Errorf(codes.Internal, "internal server error")
	}

	// Log the request and details about the call environment.
	r.logRequest(ctx, env, req)

	// Validate the format of the request (e.g. check required fields and so on).
	validated, err := r.validateRequest(req)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, err.Error())
	}

	// Check it passes ACLs as described in the comment for this function.
	if err := r.checkACLs(ctx, env, validated); err != nil {
		return nil, err
	}

	// Mint the token of the corresponding kind.
	var tok *auth.Token
	switch {
	case validated.kind == minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN:
		tok, err = r.MintAccessToken(ctx, auth.MintAccessTokenParams{
			ServiceAccount: validated.account,
			Scopes:         validated.oauthScopes,
			MinTTL:         validated.minTTL,
		})
	case validated.kind == minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ID_TOKEN:
		tok, err = r.MintIDToken(ctx, auth.MintIDTokenParams{
			ServiceAccount: validated.account,
			Audience:       validated.idTokenAudience,
			MinTTL:         validated.minTTL,
		})
	default:
		panic("impossible") // already checked in validateRequest
	}

	if err != nil {
		logging.Errorf(ctx, "Failed to mint a token for %q: %s", validated.account, err)
		code := codes.InvalidArgument // mostly likely misconfigured IAM roles
		if transient.Tag.In(err) {
			code = codes.Internal
		}
		return nil, status.Errorf(code, "failed to mint token for %q - %s", validated.account, err)
	}

	// Grab a string that identifies token server version. This almost always
	// just hits local memory cache.
	serviceVer, err := utils.ServiceVersion(ctx, r.Signer)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "can't grab service version - %s", err)
	}

	// The RPC response.
	resp := &minter.MintServiceAccountTokenResponse{
		Token:          tok.Token,
		Expiry:         timestamppb.New(tok.Expiry),
		ServiceVersion: serviceVer,
	}

	// Log it to BigQuery.
	if r.LogToken != nil {
		info := MintedTokenInfo{
			Request:         req,
			Response:        resp,
			RequestedAt:     clock.Now(ctx),
			OAuthScopes:     validated.oauthScopes,
			RequestIdentity: env.caller,
			PeerIdentity:    env.peer,
			ConfigRev:       env.mapping.ConfigRevision(),
			PeerIP:          env.state.PeerIP(),
			RequestID:       trace.SpanContext(ctx),
			AuthDBRev:       authdb.Revision(state.DB()),
		}
		// Errors during logging are considered not fatal. We have a monitoring
		// counter that tracks number of errors, so they are not totally invisible.
		if err := r.LogToken(ctx, &info); err != nil {
			logging.Errorf(ctx, "Failed to insert the token info into the BigQuery log: %s", err)
		}
	}

	return resp, nil
}

// logRequest logs the body of the request and details about the call.
func (r *MintServiceAccountTokenRPC) logRequest(ctx context.Context, env *callEnv, req *minter.MintServiceAccountTokenRequest) {
	if !logging.IsLogging(ctx, logging.Debug) {
		return
	}
	opts := protojson.MarshalOptions{Indent: "  "}
	logging.Debugf(ctx, "Peer:     %s", env.peer)
	logging.Debugf(ctx, "Identity: %s", env.caller)
	logging.Debugf(ctx, "Mapping:  %s", env.mapping.ConfigRevision())
	logging.Debugf(ctx, "AuthDB:   %d", authdb.Revision(env.db))
	logging.Debugf(ctx, "MintServiceAccountTokenRequest:\n%s", opts.Format(req))
}

// validateRequest checks the request is well-formed.
func (r *MintServiceAccountTokenRPC) validateRequest(req *minter.MintServiceAccountTokenRequest) (*validatedRequest, error) {
	// Validate TokenKind.
	switch req.TokenKind {
	case minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN:
	case minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ID_TOKEN:
		// good
	case minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_UNSPECIFIED:
		return nil, fmt.Errorf("token_kind is required")
	default:
		return nil, fmt.Errorf("unrecognized token_kind %d", req.TokenKind)
	}

	// Validate ServiceAccount.
	if req.ServiceAccount == "" {
		return nil, fmt.Errorf("service_account is required")
	}
	if _, err := identity.MakeIdentity("user:" + req.ServiceAccount); err != nil {
		return nil, fmt.Errorf("bad service_account: %s", err)
	}

	// Validate and parse Realm.
	if req.Realm == "" {
		return nil, fmt.Errorf("realm is required")
	}
	if err := realms.ValidateRealmName(req.Realm, realms.GlobalScope); err != nil {
		return nil, fmt.Errorf("bad realm: %s", err)
	}
	project, _ := realms.Split(req.Realm)

	// Validate SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN fields.
	var oauthScopes stringset.Set
	if req.TokenKind == minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN {
		if len(req.OauthScope) == 0 {
			return nil, fmt.Errorf("oauth_scope is required when token_kind is %s", req.TokenKind)
		}
		for _, scope := range req.OauthScope {
			if scope == "" {
				return nil, fmt.Errorf("bad oauth_scope: got an empty string")
			}
		}
		oauthScopes = stringset.NewFromSlice(req.OauthScope...)
	} else {
		if len(req.OauthScope) != 0 {
			return nil, fmt.Errorf("oauth_scope must not be used when token_kind is %s", req.TokenKind)
		}
	}

	// Validate SERVICE_ACCOUNT_TOKEN_ID_TOKEN fields.
	if req.TokenKind == minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ID_TOKEN {
		if req.IdTokenAudience == "" {
			return nil, fmt.Errorf("id_token_audience is required when token_kind is %s", req.TokenKind)
		}
	} else {
		if req.IdTokenAudience != "" {
			return nil, fmt.Errorf("id_token_audience must not be used when token_kind is %s", req.TokenKind)
		}
	}

	// Validate MinValidityDuration, substitute defaults.
	minTTL := time.Duration(req.MinValidityDuration) * time.Second
	if minTTL == 0 {
		minTTL = 5 * time.Minute
	}
	switch {
	case minTTL < 0:
		return nil, fmt.Errorf("bad min_validity_duration: got %d, must be positive", req.MinValidityDuration)
	case minTTL > time.Hour:
		return nil, fmt.Errorf("bad min_validity_duration: got %d, must be not greater than 3600", req.MinValidityDuration)
	}

	// Validate AuditTags.
	if err := utils.ValidateTags(req.AuditTags); err != nil {
		return nil, fmt.Errorf("bad audit_tags: %s", err)
	}

	return &validatedRequest{
		kind:            req.TokenKind,
		account:         req.ServiceAccount,
		realm:           req.Realm,
		project:         project,
		oauthScopes:     oauthScopes.ToSortedSlice(),
		idTokenAudience: req.IdTokenAudience,
		minTTL:          minTTL,
		auditTags:       req.AuditTags,
	}, nil
}

// checkACLs returns an grpc error if the request is forbidden.
//
// Logs errors inside.
func (r *MintServiceAccountTokenRPC) checkACLs(ctx context.Context, env *callEnv, req *validatedRequest) error {
	// Check that caller is allowed to mint tokens for accounts in the realm.
	switch yes, err := env.db.HasPermission(ctx, env.caller, permMintToken, req.realm); {
	case err != nil:
		logging.Errorf(ctx, "HasPermission(%q, %q, %q) failed: %s", env.caller, permMintToken, req.realm, err)
		return status.Errorf(codes.Internal, "internal server error")
	case !yes:
		logging.Errorf(ctx, "Caller %q has no permission to mint tokens in the realm %q or it doesn't exist", env.caller, req.realm)
		return status.Errorf(codes.PermissionDenied, "unknown realm or no permission to use service accounts there")
	}

	// Check the service account is defined in the realm.
	accountID := identity.Identity("user:" + req.account)
	switch yes, err := env.db.HasPermission(ctx, accountID, permExistInRealm, req.realm); {
	case err != nil:
		logging.Errorf(ctx, "HasPermission(%q, %q, %q) failed: %s", accountID, permExistInRealm, req.realm, err)
		return status.Errorf(codes.Internal, "internal server error")
	case !yes:
		logging.Errorf(ctx, "Service account %q is not in the realm %q", req.account, req.realm)
		return status.Errorf(codes.PermissionDenied, "the service account %q is not in the realm %q", req.account, req.realm)
	}

	// Check the service account is allowed to be defined in this realm at all
	// according to the global Token Server config.
	if !env.mapping.CanProjectUseAccount(req.project, req.account) {
		logging.Errorf(ctx, "Service account %q is not allowed to be used by the project %q", req.account, req.project)
		return status.Errorf(codes.PermissionDenied,
			"the service account %q is not allowed to be used by the project %q per %s configuration",
			req.account, req.project, configFileName)
	}

	return nil
}
