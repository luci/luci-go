// Copyright 2016 The LUCI Authors.
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

package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/grpc/credentials"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/server/auth/delegation"
	"go.chromium.org/luci/server/auth/internal"
)

// RPCAuthorityKind defines under whose authority RPCs are made.
type RPCAuthorityKind int

const (
	// NoAuth is used for outbound RPCs that don't have any implicit auth headers.
	NoAuth RPCAuthorityKind = iota

	// AsSelf is used for outbound RPCs sent with the authority of the current
	// service itself.
	//
	// RPC requests done in this mode will have 'Authorization' header set to the
	// OAuth2 access token of the service's own service account.
	//
	// By default uses "https://www.googleapis.com/auth/userinfo.email" API scope.
	// Can be customized with WithScopes() options.
	AsSelf

	// AsUser is used for outbound RPCs that inherit the authority of a user
	// that initiated the request that is currently being handled, regardless of
	// how exactly the user was authenticated.
	//
	// The implementation is based on LUCI-specific protocol that uses special
	// delegation tokens. Only LUCI backends can understand them.
	//
	// If you need to call non-LUCI services, and incoming requests are
	// authenticated via OAuth access tokens, use AsCredentialsForwarder instead.
	//
	// If the current request was initiated by an anonymous caller, the RPC will
	// have no auth headers (just like in NoAuth mode).
	//
	// Can also be used together with MintDelegationToken to make requests on
	// user behalf asynchronously. For example, to associate end-user authority
	// with some delayed task, call MintDelegationToken (in a context of a user
	// initiated request) when this task is created and store the resulting token
	// along with the task. Then, to make an RPC on behalf of the user from the
	// task use GetRPCTransport(ctx, AsUser, WithDelegationToken(token)).
	AsUser

	// AsCredentialsForwarder is used for outbound RPCs that just forward the
	// user credentials, exactly as they were received by the service.
	//
	// For authenticated calls, works only if the current request was
	// authenticated via an OAuth access token.
	//
	// If the current request was initiated by an anonymous caller, the RPC will
	// have no auth headers (just like in NoAuth mode).
	//
	// An attempt to use GetRPCTransport(ctx, AsCredentialsForwarder) with
	// unsupported credentials results in an error.
	AsCredentialsForwarder

	// AsActor is used for outbound RPCs sent with the authority of some service
	// account that the current service has "iam.serviceAccountActor" role in.
	//
	// RPC requests done in this mode will have 'Authorization' header set to
	// the access token of the service account specified by WithServiceAccount()
	// option.
	//
	// By default uses "https://www.googleapis.com/auth/userinfo.email" API scope.
	// Can be customized with WithScopes() options.
	AsActor
)

// RPCOption is an option for GetRPCTransport or GetPerRPCCredentials functions.
type RPCOption interface {
	apply(opts *rpcOptions)
}

// WithScopes can be used to customize OAuth scopes for outbound RPC requests.
//
// If not used, the requests are made with "userinfo.email" scope.
func WithScopes(scopes ...string) RPCOption {
	return oauthScopesOption{scopes: scopes}
}

type oauthScopesOption struct {
	scopes []string
}

func (o oauthScopesOption) apply(opts *rpcOptions) {
	opts.scopes = append(opts.scopes, o.scopes...)
}

// WithServiceAccount option must be used with AsActor authority kind to specify
// what service account to act as.
func WithServiceAccount(email string) RPCOption {
	return serviceAccountOption{email: email}
}

type serviceAccountOption struct {
	email string
}

func (o serviceAccountOption) apply(opts *rpcOptions) {
	opts.serviceAccount = o.email
}

// WithDelegationToken can be used to attach an existing delegation token to
// requests made in AsUser mode.
//
// The token can be obtained earlier via MintDelegationToken call. The transport
// doesn't attempt to validate it and just blindly sends it to the other side.
func WithDelegationToken(token string) RPCOption {
	return delegationTokenOption{token: token}
}

type delegationTokenOption struct {
	token string
}

func (o delegationTokenOption) apply(opts *rpcOptions) {
	opts.delegationToken = o.token
}

type delegationTagsOption struct {
	tags []string
}

func (o delegationTagsOption) apply(opts *rpcOptions) {
	opts.delegationTags = o.tags
}

// WithDelegationTags can be used to attach tags to the delegation token used
// internally in AsUser mode.
//
// The recipient of the RPC that uses the delegation will be able to extract
// them, if necessary. They are also logged in the token server logs.
//
// Each tag is a key:value string.
//
// Note that any delegation tags are ignored if the current request was
// initiated by an anonymous caller, since delegation protocol is not actually
// used in this case.
func WithDelegationTags(tags ...string) RPCOption {
	return delegationTagsOption{tags: tags}
}

type monitoringClientOption struct {
	client string
}

func (o monitoringClientOption) apply(opts *rpcOptions) {
	opts.monitoringClient = o.client
}

// WithMonitoringClient allows to override 'client' field that goes into HTTP
// client monitoring metrics (such as 'http/response_status').
//
// The default value of the field is "luci-go-server".
//
// Note that the metrics also include hostname of the target service (in 'name'
// field), so in most cases it is fine to use the default client name.
// Overriding it may be useful if you want to differentiate between requests
// made to the same host from a bunch of different places in the code.
//
// This option has absolutely no effect when passed GetPerRPCCredentials() or
// GetTokenSource(). It applies only to GetRPCTransport().
func WithMonitoringClient(client string) RPCOption {
	return monitoringClientOption{client: client}
}

// GetRPCTransport returns http.RoundTripper to use for outbound HTTP RPC
// requests.
//
// Usage:
//    tr, err := auth.GetRPCTransport(c, auth.AsSelf, auth.WithScopes("..."))
//    if err != nil {
//      return err
//    }
//    client := &http.Client{Transport: tr}
//    ...
func GetRPCTransport(c context.Context, kind RPCAuthorityKind, opts ...RPCOption) (http.RoundTripper, error) {
	options, err := makeRPCOptions(kind, opts)
	if err != nil {
		return nil, err
	}

	config := getConfig(c)
	if config == nil || config.AnonymousTransport == nil {
		return nil, ErrNotConfigured
	}

	if options.checkCtx != nil {
		if err := options.checkCtx(c); err != nil {
			return nil, err
		}
	}

	baseTransport := metric.InstrumentTransport(c, config.AnonymousTransport(c), options.monitoringClient)
	if options.kind == NoAuth {
		return baseTransport, nil
	}

	return auth.NewModifyingTransport(baseTransport, func(req *http.Request) error {
		tok, extra, err := options.getRPCHeaders(c, req.URL.String(), options)
		if err != nil {
			return err
		}
		if tok != nil {
			req.Header.Set("Authorization", tok.TokenType+" "+tok.AccessToken)
		}
		for k, v := range extra {
			req.Header.Set(k, v)
		}
		return nil
	}), nil
}

// GetPerRPCCredentials returns gRPC's PerRPCCredentials implementation.
//
// It can be used to authenticate outbound gPRC RPC's.
func GetPerRPCCredentials(kind RPCAuthorityKind, opts ...RPCOption) (credentials.PerRPCCredentials, error) {
	options, err := makeRPCOptions(kind, opts)
	if err != nil {
		return nil, err
	}
	return perRPCCreds{options}, nil
}

type perRPCCreds struct {
	options *rpcOptions
}

func (creds perRPCCreds) GetRequestMetadata(c context.Context, uri ...string) (map[string]string, error) {
	if len(uri) == 0 {
		panic("perRPCCreds: no URI given")
	}
	tok, extra, err := creds.options.getRPCHeaders(c, uri[0], creds.options)
	switch {
	case err != nil:
		return nil, err
	case tok == nil && len(extra) == 0:
		return nil, nil
	}
	headers := make(map[string]string, 1+len(extra))
	if tok != nil {
		headers["Authorization"] = tok.TokenType + " " + tok.AccessToken
	}
	for k, v := range extra {
		headers[k] = v
	}
	return headers, nil
}

func (creds perRPCCreds) RequireTransportSecurity() bool {
	return true
}

// GetTokenSource returns an oauth2.TokenSource bound to the supplied Context.
//
// Supports only AsSelf, AsCredentialsForwarder and AsActor authorization kinds,
// since they are only ones that exclusively use OAuth2 tokens and no other
// extra headers.
//
// While GetPerRPCCredentials is preferred, this can be used by packages that
// cannot or do not properly handle this gRPC option.
func GetTokenSource(c context.Context, kind RPCAuthorityKind, opts ...RPCOption) (oauth2.TokenSource, error) {
	if kind != AsSelf && kind != AsCredentialsForwarder && kind != AsActor {
		return nil, fmt.Errorf("auth: GetTokenSource can only be used with AsSelf, AsCredentialsForwarder or AsActor authorization kind")
	}
	options, err := makeRPCOptions(kind, opts)
	if err != nil {
		return nil, err
	}
	if options.checkCtx != nil {
		if err := options.checkCtx(c); err != nil {
			return nil, err
		}
	}
	return &tokenSource{c, options}, nil
}

type tokenSource struct {
	ctx     context.Context
	options *rpcOptions
}

func (ts *tokenSource) Token() (*oauth2.Token, error) {
	tok, extra, err := ts.options.getRPCHeaders(ts.ctx, "", ts.options)
	switch {
	case err != nil:
		return nil, err
	case tok == nil:
		panic("oauth2.Token is unexpectedly nil")
	case extra != nil:
		keys := make([]string, 0, len(extra))
		for k := range extra {
			keys = append(keys, k)
		}
		panic(fmt.Errorf("extra headers are unexpectedly not empty: %s", keys))
	}
	return tok, nil
}

////////////////////////////////////////////////////////////////////////////////
// Internal stuff.

func init() {
	// This is needed to allow packages imported by 'server/auth' to make
	// authenticated calls. They can't use GetRPCTransport directly, since they
	// can't import 'server/auth' (it creates an import cycle).
	internal.RegisterClientFactory(func(c context.Context, scopes []string) (*http.Client, error) {
		var t http.RoundTripper
		var err error
		if len(scopes) == 0 {
			t, err = GetRPCTransport(c, NoAuth)
		} else {
			t, err = GetRPCTransport(c, AsSelf, WithScopes(scopes...))
		}
		if err != nil {
			return nil, err
		}
		return &http.Client{Transport: t}, nil
	})
}

// tokenFingerprint returns first 16 bytes of SHA256 of the token, as hex.
//
// Token fingerprints can be used to identify tokens without parsing them.
func tokenFingerprint(tok string) string {
	digest := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(digest[:16])
}

// rpcMocks are used exclusively in unit tests.
type rpcMocks struct {
	MintDelegationToken              func(context.Context, DelegationTokenParams) (*delegation.Token, error)
	MintAccessTokenForServiceAccount func(context.Context, MintAccessTokenParams) (*oauth2.Token, error)
}

// apply implements RPCOption interface.
func (o *rpcMocks) apply(opts *rpcOptions) {
	opts.rpcMocks = o
}

var defaultOAuthScopes = []string{auth.OAuthScopeEmail}

// headersGetter returns a main Authorization token and optional additional
// headers.
type headersGetter func(c context.Context, uri string, opts *rpcOptions) (*oauth2.Token, map[string]string, error)

type rpcOptions struct {
	kind             RPCAuthorityKind
	scopes           []string // for AsSelf and AsActor
	serviceAccount   string   // for AsActor
	delegationToken  string   // for AsUser
	delegationTags   []string // for AsUser
	monitoringClient string
	checkCtx         func(c context.Context) error // optional, may be skipped
	getRPCHeaders    headersGetter
	rpcMocks         *rpcMocks
}

// makeRPCOptions applies all options and validates them.
func makeRPCOptions(kind RPCAuthorityKind, opts []RPCOption) (*rpcOptions, error) {
	options := &rpcOptions{kind: kind}
	for _, o := range opts {
		o.apply(options)
	}

	// Set default scopes.
	asSelfOrActor := options.kind == AsSelf || options.kind == AsActor
	if asSelfOrActor && len(options.scopes) == 0 {
		options.scopes = defaultOAuthScopes
	}

	// Validate options.
	if !asSelfOrActor && len(options.scopes) != 0 {
		return nil, fmt.Errorf("auth: WithScopes can only be used with AsSelf or AsActor authorization kind")
	}
	if options.serviceAccount != "" && options.kind != AsActor {
		return nil, fmt.Errorf("auth: WithServiceAccount can only be used with AsActor authorization kind")
	}
	if options.serviceAccount == "" && options.kind == AsActor {
		return nil, fmt.Errorf("auth: AsActor authorization kind requires WithServiceAccount option")
	}
	if options.delegationToken != "" && options.kind != AsUser {
		return nil, fmt.Errorf("auth: WithDelegationToken can only be used with AsUser authorization kind")
	}
	if len(options.delegationTags) != 0 && options.kind != AsUser {
		return nil, fmt.Errorf("auth: WithDelegationTags can only be used with AsUser authorization kind")
	}
	if len(options.delegationTags) != 0 && options.delegationToken != "" {
		return nil, fmt.Errorf("auth: WithDelegationTags and WithDelegationToken cannot be used together")
	}

	// Validate 'kind' and pick correct implementation of getRPCHeaders.
	switch options.kind {
	case NoAuth:
		options.getRPCHeaders = noAuthHeaders
	case AsSelf:
		options.getRPCHeaders = asSelfHeaders
	case AsUser:
		options.getRPCHeaders = asUserHeaders
	case AsCredentialsForwarder:
		options.checkCtx = func(c context.Context) error {
			_, err := forwardedCreds(c)
			return err
		}
		options.getRPCHeaders = func(c context.Context, _ string, _ *rpcOptions) (*oauth2.Token, map[string]string, error) {
			tok, err := forwardedCreds(c)
			return tok, nil, err
		}
	case AsActor:
		options.getRPCHeaders = asActorHeaders
	default:
		return nil, fmt.Errorf("auth: unknown RPCAuthorityKind %d", options.kind)
	}

	// Default value for "client" field in monitoring metrics.
	if options.monitoringClient == "" {
		options.monitoringClient = "luci-go-server"
	}

	return options, nil
}

// noAuthHeaders is getRPCHeaders for NoAuth mode.
func noAuthHeaders(c context.Context, uri string, opts *rpcOptions) (*oauth2.Token, map[string]string, error) {
	return nil, nil, nil
}

// asSelfHeaders returns a map of authentication headers to add to outbound
// RPC requests done in AsSelf mode.
//
// This will be called by the transport layer on each request.
func asSelfHeaders(c context.Context, uri string, opts *rpcOptions) (*oauth2.Token, map[string]string, error) {
	cfg := getConfig(c)
	if cfg == nil || cfg.AccessTokenProvider == nil {
		return nil, nil, ErrNotConfigured
	}
	tok, err := cfg.AccessTokenProvider(c, opts.scopes)
	return tok, nil, err
}

// asUserHeaders returns a map of authentication headers to add to outbound
// RPC requests done in AsUser mode.
//
// This will be called by the transport layer on each request.
func asUserHeaders(c context.Context, uri string, opts *rpcOptions) (*oauth2.Token, map[string]string, error) {
	cfg := getConfig(c)
	if cfg == nil || cfg.AccessTokenProvider == nil {
		return nil, nil, ErrNotConfigured
	}

	delegationToken := ""
	if opts.delegationToken != "" {
		delegationToken = opts.delegationToken // WithDelegationToken was used
	} else {
		// Outbound RPC calls in the context of a request from anonymous caller are
		// anonymous too. No need to use any authentication headers.
		userIdent := CurrentIdentity(c)
		if userIdent == identity.AnonymousIdentity {
			return nil, nil, nil
		}

		// Grab root URL of the destination service. Only https:// are allowed.
		uri = strings.ToLower(uri)
		if !strings.HasPrefix(uri, "https://") {
			return nil, nil, fmt.Errorf("auth: refusing to use delegation tokens with non-https URL")
		}
		host := uri[len("https://"):]
		if idx := strings.IndexRune(host, '/'); idx != -1 {
			host = host[:idx]
		}

		// Grab a token that's good enough for at least 10 min. Outbound RPCs
		// shouldn't last longer than that.
		mintTokenCall := MintDelegationToken
		if opts.rpcMocks != nil && opts.rpcMocks.MintDelegationToken != nil {
			mintTokenCall = opts.rpcMocks.MintDelegationToken
		}
		tok, err := mintTokenCall(c, DelegationTokenParams{
			TargetHost: host,
			Tags:       opts.delegationTags,
			MinTTL:     10 * time.Minute,
		})
		if err != nil {
			return nil, nil, err
		}
		delegationToken = tok.Token
	}

	// Use our own OAuth token too, since the delegation token is bound to us.
	oauthTok, err := cfg.AccessTokenProvider(c, []string{auth.OAuthScopeEmail})
	if err != nil {
		return nil, nil, err
	}

	logging.Fields{
		"fingerprint": tokenFingerprint(delegationToken),
	}.Debugf(c, "auth: Sending delegation token")
	return oauthTok, map[string]string{delegation.HTTPHeaderName: delegationToken}, nil
}

// forwardedCreds returns the end user token, as it was received by the service.
//
// Returns (nil, nil) if the incoming call was anonymous. Returns an error if
// the incoming call was authenticated by non-forwardable credentials.
func forwardedCreds(c context.Context) (*oauth2.Token, error) {
	switch s := GetState(c); {
	case s == nil:
		return nil, ErrNotConfigured
	case s.User().Identity == identity.AnonymousIdentity:
		return nil, nil // nothing to forward if the call is anonymous
	default:
		// Grab the end user credentials (or an error) from the auth state, as
		// put there by Authenticate(...).
		return s.UserCredentials()
	}
}

// asActorHeaders returns a map of authentication headers to add to outbound
// RPC requests done in AsActor mode.
//
// This will be called by the transport layer on each request.
func asActorHeaders(c context.Context, uri string, opts *rpcOptions) (*oauth2.Token, map[string]string, error) {
	mintTokenCall := MintAccessTokenForServiceAccount
	if opts.rpcMocks != nil && opts.rpcMocks.MintAccessTokenForServiceAccount != nil {
		mintTokenCall = opts.rpcMocks.MintAccessTokenForServiceAccount
	}
	oauthTok, err := mintTokenCall(c, MintAccessTokenParams{
		ServiceAccount: opts.serviceAccount,
		Scopes:         opts.scopes,
		MinTTL:         2 * time.Minute,
	})
	return oauthTok, nil, err
}
