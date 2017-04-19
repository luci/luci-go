// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"google.golang.org/grpc/credentials"

	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/auth/delegation"
	"github.com/luci/luci-go/server/auth/identity"
	"github.com/luci/luci-go/server/auth/internal"
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
	// that initiated the request that is currently being handled.
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
	//
	// The implementation is based on LUCI-specific protocol that uses special
	// delegation tokens. Only LUCI backends can understand them.
	AsUser

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
	baseTransport := config.AnonymousTransport(c)
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
// Supports only AsSelf and AsActor authorization kinds, since they are only
// ones that exclusively use OAuth2 tokens and no other extra headers.
//
// While GetPerRPCCredentials is preferred, this can be used by packages that
// cannot or do not properly handle this gRPC option.
func GetTokenSource(c context.Context, kind RPCAuthorityKind, opts ...RPCOption) (oauth2.TokenSource, error) {
	if kind != AsSelf && kind != AsActor {
		return nil, fmt.Errorf("auth: GetTokenSource can only be used with AsSelf or AsActor authorization kind")
	}
	options, err := makeRPCOptions(kind, opts)
	if err != nil {
		return nil, err
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
	kind            RPCAuthorityKind
	scopes          []string // for AsSelf and AsActor
	serviceAccount  string   // for AsActor
	delegationToken string   // for AsUser
	getRPCHeaders   headersGetter
	rpcMocks        *rpcMocks
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

	// Validate 'kind' and pick correct implementation of getRPCHeaders.
	switch options.kind {
	case NoAuth:
		options.getRPCHeaders = noAuthHeaders
	case AsSelf:
		options.getRPCHeaders = asSelfHeaders
	case AsUser:
		options.getRPCHeaders = asUserHeaders
	case AsActor:
		options.getRPCHeaders = asActorHeaders
	default:
		return nil, fmt.Errorf("auth: unknown RPCAuthorityKind %d", options.kind)
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
