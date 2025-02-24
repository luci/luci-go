// Copyright 2024 The LUCI Authors.
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

package botapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	minterpb "go.chromium.org/luci/tokenserver/api/minter/v1"

	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/botsrv"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/validate"
)

// minTokenTTL is minimal possible lifetime of a returned minted token.
const minTokenTTL = 5 * time.Minute

// TokenRequest is sent by the bot.
type TokenRequest struct {
	// Session is a serialized Swarming Bot Session proto.
	Session []byte `json:"session"`

	// AccountID is either "system" or "task".
	//
	// Defines what sort of service account to mint a token for.
	AccountID string `json:"account_id"`

	// TaskID is the task ID that the bot thinks it is running.
	//
	// Required when minting tokens for "task" account. Will be checked against
	// the task ID associated with the bot in the datastore.
	TaskID string `json:"task_id,omitempty"`

	// Scopes is a list of OAuth scopes to mint a service account token with.
	//
	// Used only by OAuthToken handler. Must be unset in IDToken handler.
	Scopes []string `json:"scopes,omitempty"`

	// Audience is an audience to put into a minted ID token.
	//
	// Used only by IDToken handler. Must be unset in OAuthToken handler.
	Audience string `json:"audience,omitempty"`
}

func (r *TokenRequest) ExtractSession() []byte { return r.Session }
func (r *TokenRequest) ExtractDebugRequest() any {
	return &TokenRequest{
		Session:   nil,
		AccountID: r.AccountID,
		TaskID:    r.TaskID,
		Scopes:    r.Scopes,
		Audience:  r.Audience,
	}
}

// TokenResponse is returned by the server.
type TokenResponse struct {
	// ServiceAccount is a service account the token is from.
	//
	// This is either an email, or literals "bot" (if the bot is configured to use
	// its own service account as a system or task account) or "none" (if
	// there's no service account assigned at all).
	ServiceAccount string `json:"service_account"`

	// AccessToken is the minted OAuth access token.
	//
	// Returned only by OAuthToken handler when actually using an assigned service
	// account (i.e. not "bot" or "none"). Unset in IDToken handler response.
	AccessToken string `json:"access_token,omitempty"`

	// IDToken is the minted ID token.
	//
	// Returned only by IDToken handler when actually using an assigned service
	// account (i.e. not "bot" or "none"). Unset in OAuthToken handler response.
	IDToken string `json:"id_token,omitempty"`

	// Expiry is token's Unix expiration timestamp in seconds.
	//
	// Returned only when actually using an assigned service account (i.e. not
	// "bot" or "none").
	Expiry int64 `json:"expiry,omitempty"`
}

// OAuthToken mints OAuth tokens to be used inside the task.
//
// There are two flavors of service accounts the bot may use:
//   - "system": this account is associated directly with the bot (in bots.cfg),
//     and can be used at any time (when running a task or not).
//   - "task": this account is associated with the task currently executing on
//     the bot, and may be used only when bot is actually running this task.
//
// The returned token is expected to be alive for at least ~5 min, but can live
// longer (but no longer than ~1h). In general the client should assume the
// token is short-lived.
//
// Multiple bots may share the exact same access token if their configuration
// match (the token is cached by Swarming for performance reasons).
//
// Besides the token, the response also contains the actual service account
// email (if it is really configured), or two special strings in place of the
// email:
//   - "none" if the bot is not configured to use service accounts at all.
//   - "bot" if the bot should use tokens produced by bot_config.py hook.
//
// Returns following errors:
//   - INVALID_ARGUMENT on a bad request or if the service account is somehow
//     misconfigured.
//   - PERMISSION_DENIED if the caller is not allowed to use the service
//     account.
//   - INTERNAL on retriable transient errors.
func (srv *BotAPIServer) OAuthToken(ctx context.Context, body *TokenRequest, r *botsrv.Request) (botsrv.Response, error) {
	if len(body.Scopes) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, `"scopes" are required`)
	}
	if body.Audience != "" {
		return nil, status.Errorf(codes.InvalidArgument, `"audience" must not be used in OAuth access token request`)
	}
	sa, tok, exp, err := srv.mintToken(ctx, body, r, &oauthTokenMinter{
		scopes:            body.Scopes,
		tokenServerClient: srv.tokenServerClient,
	})
	if err != nil {
		return nil, err
	}
	return &TokenResponse{
		ServiceAccount: sa,
		AccessToken:    tok,
		Expiry:         exp,
	}, nil
}

// IDToken mints ID tokens to be used inside the task.
//
// See OAuthToken for details.
func (srv *BotAPIServer) IDToken(ctx context.Context, body *TokenRequest, r *botsrv.Request) (botsrv.Response, error) {
	if body.Audience == "" {
		return nil, status.Errorf(codes.InvalidArgument, `"audience" is required`)
	}
	if len(body.Scopes) != 0 {
		return nil, status.Errorf(codes.InvalidArgument, `"scopes" must not be used in ID token request`)
	}
	sa, tok, exp, err := srv.mintToken(ctx, body, r, &idTokenMinter{
		audience:          body.Audience,
		tokenServerClient: srv.tokenServerClient,
	})
	if err != nil {
		return nil, err
	}
	return &TokenResponse{
		ServiceAccount: sa,
		IDToken:        tok,
		Expiry:         exp,
	}, nil
}

// tokenMinter is implemented by oauthTokenMinter and idTokenMinter.
type tokenMinter interface {
	kind() string
	mintTaskToken(ctx context.Context, realm, serviceAccountEmail string, auditTags []string) (*auth.Token, error)
	mintSystemToken(ctx context.Context, serviceAccountEmail string) (*auth.Token, error)
}

// mintToken is a common implementation of OAuthToken and IDToken.
//
// It returns the server account email (or "bot" or "none") as well as the
// actual minted token along with its expiration time as a unix timestamp.
func (srv *BotAPIServer) mintToken(ctx context.Context, body *TokenRequest, r *botsrv.Request, tm tokenMinter) (sa, tok string, exp int64, err error) {
	var minted *auth.Token
	switch body.AccountID {
	case "task":
		sa, minted, err = srv.mintTaskToken(ctx, body, r, tm)
	case "system":
		sa, minted, err = srv.mintSystemToken(ctx, r, tm)
	default:
		err = status.Errorf(codes.InvalidArgument, `"account_id" must be either "system" or "task"`)
	}
	if err != nil {
		return "", "", 0, err
	}

	// "bot" and "none" are interpreted by the bot locally. The server is not
	// returning any tokens for them.
	if sa == "bot" || sa == "none" {
		return sa, "", 0, nil
	}

	// Log the token fingerprint, it matches the fingerprint in Token Server logs.
	digest := sha256.Sum256([]byte(minted.Token))
	fingerprint := hex.EncodeToString(digest[:16])
	logging.Infof(ctx,
		"Got %s %s: email=%s, fingerprint=%s, expiry=%d, expiry_in=%d",
		body.AccountID, tm.kind(), sa, fingerprint,
		minted.Expiry.Unix(), int(time.Until(minted.Expiry).Seconds()))

	return sa, minted.Token, minted.Expiry.Unix(), nil
}

// mintTaskToken mints a token for "task" account (associated with the task).
func (srv *BotAPIServer) mintTaskToken(ctx context.Context, body *TokenRequest, r *botsrv.Request, tm tokenMinter) (sa string, minted *auth.Token, err error) {
	// Check "task_id" matches the task currently assigned to the bot. This is
	// mostly a precaution against confused bot processes that try to keep
	// executing a task even after a timeout.
	if body.TaskID == "" {
		return "", nil, status.Errorf(codes.InvalidArgument, `"task_id" is required when minting task tokens`)
	}
	taskReqKey, err := model.TaskIDToRequestKey(ctx, body.TaskID)
	if err != nil {
		return "", nil, status.Errorf(codes.InvalidArgument, `"task_id" %q: %s`, body.TaskID, err)
	}
	if r.CurrentTaskID != body.TaskID {
		return "", nil, status.Errorf(codes.InvalidArgument, "wrong task_id %q: the bot is not executing this task", body.TaskID)
	}

	// Look up the service account associated with the task.
	//
	// TODO: Cache this, this is static. Will save ~100 QPS to datastore.
	taskReq := &model.TaskRequest{Key: taskReqKey}
	switch err := datastore.Get(ctx, taskReq); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return "", nil, status.Errorf(codes.InvalidArgument, `"task_id" %q: no such task`, body.TaskID)
	case err != nil:
		logging.Errorf(ctx, "Looking up task request %q: %s", body.TaskID, err)
		return "", nil, status.Errorf(codes.Internal, "datastore error looking up the task")
	}
	sa = taskReq.ServiceAccount

	// "none" and "bot" are handled by the bot locally, return them as they are.
	switch sa {
	case "", "none":
		return "none", nil, nil
	case "bot":
		return "bot", nil, nil
	default:
		// Otherwise it should be an email, as checked when the task was created.
		if err := validate.ServiceAccount(sa); err != nil {
			return "", nil, status.Errorf(codes.FailedPrecondition, "unexpectedly invalid task service account: %s", err)
		}
	}

	// Need a realm to be able to impersonate accounts through it.
	if taskReq.Realm == "" {
		return "", nil, status.Errorf(codes.FailedPrecondition, "the task is unexpectedly not associated with a realm")
	}

	// Re-check if the service account is still allowed to be used inside the
	// realm, because this may have changed since the task was submitted (when
	// this permissions was checked initially).
	res := acls.NewChecker(ctx, srv.cfg.Cached(ctx)).CheckTaskCanActAsServiceAccount(ctx, taskReq.Realm, sa)
	if !res.Permitted || res.InternalError {
		return "", nil, res.ToGrpcErr()
	}

	// Actually request a new token.
	minted, err = tm.mintTaskToken(ctx, taskReq.Realm, sa, []string{
		fmt.Sprintf("swarming:bot_id:%s", r.Session.BotId),
		fmt.Sprintf("swarming:task_id:%s", body.TaskID),
		fmt.Sprintf("swarming:task_name:%s", taskReq.Name),
		fmt.Sprintf("swarming:trace_id:%s", trace.SpanContextFromContext(ctx).TraceID()),
		fmt.Sprintf("swarming:service_version:%s/%s", srv.project, srv.version),
	})
	return
}

// mintSystemToken mints a token for "system" account (associated with the bot).
func (srv *BotAPIServer) mintSystemToken(ctx context.Context, r *botsrv.Request, tm tokenMinter) (sa string, minted *auth.Token, err error) {
	sa = r.Session.BotConfig.SystemServiceAccount
	switch sa {
	case "", "none":
		return "none", nil, nil
	case "bot":
		return "bot", nil, nil
	default:
		if err := validate.ServiceAccount(sa); err != nil {
			return "", nil, status.Errorf(codes.FailedPrecondition, "misconfigured system service account: %s", err)
		}
	}
	minted, err = tm.mintSystemToken(ctx, sa)
	return
}

// oauthTokenMinter implements minting of OAuth access tokens.
type oauthTokenMinter struct {
	scopes            []string
	tokenServerClient func(ctx context.Context, realm string) (minterpb.TokenMinterClient, error)
}

func (m *oauthTokenMinter) kind() string {
	return "access token"
}

func (m *oauthTokenMinter) mintTaskToken(ctx context.Context, realm, serviceAccountEmail string, auditTags []string) (*auth.Token, error) {
	ts, err := m.tokenServerClient(ctx, realm)
	if err != nil {
		return nil, err
	}
	resp, err := ts.MintServiceAccountToken(ctx, &minterpb.MintServiceAccountTokenRequest{
		TokenKind:           minterpb.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
		ServiceAccount:      serviceAccountEmail,
		Realm:               realm,
		OauthScope:          m.scopes,
		MinValidityDuration: int64(minTokenTTL.Seconds()),
		AuditTags:           auditTags,
	})
	if err != nil {
		// Propagate the gRPC error as is, it already has correct status code.
		return nil, err
	}
	return &auth.Token{Token: resp.Token, Expiry: resp.Expiry.AsTime()}, nil
}

func (m *oauthTokenMinter) mintSystemToken(ctx context.Context, serviceAccountEmail string) (*auth.Token, error) {
	// This mints the token through Cloud IAM service account impersonation.
	// The minted token is cached and reused in subsequent calls.
	tok, err := auth.MintAccessTokenForServiceAccount(ctx, auth.MintAccessTokenParams{
		ServiceAccount: serviceAccountEmail,
		Scopes:         m.scopes,
		MinTTL:         minTokenTTL,
	})
	switch {
	case err == nil:
		return tok, nil
	case transient.Tag.In(err):
		return nil, status.Errorf(codes.Internal, "transient error minting an access token for %q: %s", serviceAccountEmail, err)
	default:
		// This is most likely IAM permission error, which we need to expose to the
		// bot as HTTP status 400 (aka "misconfiguration") which is represented by
		// InvalidArgument status code.
		return nil, status.Errorf(codes.InvalidArgument, "error minting an access token for %q: %s", serviceAccountEmail, err)
	}
}

// idTokenMinter implements minting of ID tokens.
type idTokenMinter struct {
	audience          string
	tokenServerClient func(ctx context.Context, realm string) (minterpb.TokenMinterClient, error)
}

func (m *idTokenMinter) kind() string {
	return "ID token"
}

func (m *idTokenMinter) mintTaskToken(ctx context.Context, realm, serviceAccountEmail string, auditTags []string) (*auth.Token, error) {
	ts, err := m.tokenServerClient(ctx, realm)
	if err != nil {
		return nil, err
	}
	resp, err := ts.MintServiceAccountToken(ctx, &minterpb.MintServiceAccountTokenRequest{
		TokenKind:           minterpb.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ID_TOKEN,
		ServiceAccount:      serviceAccountEmail,
		Realm:               realm,
		IdTokenAudience:     m.audience,
		MinValidityDuration: int64(minTokenTTL.Seconds()),
		AuditTags:           auditTags,
	})
	if err != nil {
		// Propagate the gRPC error as is, it already has correct status code.
		return nil, err
	}
	return &auth.Token{Token: resp.Token, Expiry: resp.Expiry.AsTime()}, nil
}

func (m *idTokenMinter) mintSystemToken(ctx context.Context, serviceAccountEmail string) (*auth.Token, error) {
	// This mints the token through Cloud IAM service account impersonation.
	// The minted token is cached and reused in subsequent calls.
	tok, err := auth.MintIDTokenForServiceAccount(ctx, auth.MintIDTokenParams{
		ServiceAccount: serviceAccountEmail,
		Audience:       m.audience,
		MinTTL:         minTokenTTL,
	})
	switch {
	case err == nil:
		return tok, nil
	case transient.Tag.In(err):
		return nil, status.Errorf(codes.Internal, "transient error minting an ID token for %q: %s", serviceAccountEmail, err)
	default:
		// This is most likely IAM permission error, which we need to expose to the
		// bot as HTTP status 400 (aka "misconfiguration") which is represented by
		// InvalidArgument
		return nil, status.Errorf(codes.InvalidArgument, "error minting an ID token for %q: %s", serviceAccountEmail, err)
	}
}

// tokenServerClient constructs a client to call the LUCI Token Server.
//
// Used in prod. Tests mock this out.
func tokenServerClient(ctx context.Context, realm string) (minterpb.TokenMinterClient, error) {
	db, err := auth.GetDB(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "no AuthDB: %s", err)
	}
	tokenServerURL, err := db.GetTokenServiceURL(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "no Token Server URL: %s", err)
	}
	parsed, err := url.Parse(tokenServerURL)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "bad Token Server URL %q: %s", tokenServerURL, err)
	}
	project, _ := realms.Split(realm)
	tr, err := auth.GetRPCTransport(ctx, auth.AsProject, auth.WithProject(project))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "getting RPC transport: %s", err)
	}
	return minterpb.NewTokenMinterClient(&prpc.Client{
		C:    &http.Client{Transport: tr},
		Host: parsed.Host,
	}), nil
}
