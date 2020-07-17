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
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/grpc/credentials"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/trace"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/server/auth/delegation"
	"go.chromium.org/luci/server/auth/internal"
)

// CloudOAuthScopes is a list of OAuth scopes recommended to use when
// authenticating to Google Cloud services.
//
// Besides the actual cloud-platform scope also includes userinfo.email scope,
// so that it is possible to examine the token email.
//
// Note that it is preferable to use the exact same list of scopes in all
// Cloud API clients. That way when the server runs locally in a development
// mode, we need to go through the login flow only once. Using different scopes
// for different clients would require to "login" for each unique set of scopes.
var CloudOAuthScopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
}

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
	//
	// In LUCI services AsSelf should be used very sparingly, only for internal
	// "maintenance" RPCs that happen outside of the context of any LUCI project.
	// Using AsSelf to authorize RPCs that touch project data leads to "confused
	// deputy" problems. Prefer to use AsProject when possible.
	AsSelf

	// AsUser is used for outbound RPCs that inherit the authority of a user
	// that initiated the request that is currently being handled, regardless of
	// how exactly the user was authenticated.
	//
	// DEPRECATED.
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
	// authenticated via a forwardable token, e.g. an OAuth2 access token.
	//
	// If the current request was initiated by an anonymous caller, the RPC will
	// have no auth headers (just like in NoAuth mode).
	//
	// An attempt to use GetRPCTransport(ctx, AsCredentialsForwarder) with
	// unsupported credentials results in an error.
	AsCredentialsForwarder

	// AsActor is used for outbound RPCs sent with the authority of some service
	// account that the current service has "iam.serviceAccountTokenCreator" role
	// in.
	//
	// RPC requests done in this mode will have 'Authorization' header set to
	// the access token of the service account specified by WithServiceAccount()
	// option.
	//
	// By default uses "https://www.googleapis.com/auth/userinfo.email" API scope.
	// Can be customized with WithScopes() options.
	AsActor

	// AsProject is used for outbounds RPCs sent with the authority of some LUCI
	// project (specified via WithProject option).
	//
	// When used to call external services (anything that is not a part of the
	// current LUCI deployment), uses 'Authorization' header with OAuth2 access
	// token associated with the project-specific service account (specified with
	// the LUCI project definition in 'projects.cfg' deployment configuration
	// file).
	//
	// By default uses "https://www.googleapis.com/auth/userinfo.email" API scope
	// in this case. Can be customized with WithScopes() options.
	//
	// When used to call LUCI services belonging the same LUCI deployment (per
	// 'internal_service_regexp' setting in 'security.cfg' deployment
	// configuration file) uses the current service's OAuth2 access token plus
	// 'X-Luci-Project' header with the project name. Such calls are authenticated
	// by the peer as coming from 'project:<name>' identity. Any custom OAuth
	// scopes supplied via WithScopes() option are ignored in this case.
	AsProject
)

// XLUCIProjectHeader is a header with the current project for internal LUCI
// RPCs done via AsProject authority.
const XLUCIProjectHeader = "X-Luci-Project"

// RPCOption is an option for GetRPCTransport, GetPerRPCCredentials and
// GetTokenSource functions.
type RPCOption interface {
	apply(opts *rpcOptions)
}

type rpcOption func(opts *rpcOptions)

func (o rpcOption) apply(opts *rpcOptions) { o(opts) }

// WithProject can be used to generate an OAuth token with an identity of that
// particular LUCI project.
//
// See AsProject for more info.
func WithProject(project string) RPCOption {
	return rpcOption(func(opts *rpcOptions) {
		opts.project = project
	})
}

// WithScopes can be used to customize OAuth scopes for outbound RPC requests.
//
// If not used, the requests are made with "userinfo.email" scope.
func WithScopes(scopes ...string) RPCOption {
	return rpcOption(func(opts *rpcOptions) {
		opts.scopes = append(opts.scopes, scopes...)
	})
}

// WithServiceAccount option must be used with AsActor authority kind to specify
// what service account to act as.
func WithServiceAccount(email string) RPCOption {
	return rpcOption(func(opts *rpcOptions) {
		opts.serviceAccount = email
	})
}

// WithDelegationToken can be used to attach an existing delegation token to
// requests made in AsUser mode.
//
// DEPRECATED.
//
// The token can be obtained earlier via MintDelegationToken call. The transport
// doesn't attempt to validate it and just blindly sends it to the other side.
func WithDelegationToken(token string) RPCOption {
	return rpcOption(func(opts *rpcOptions) {
		opts.delegationToken = token
	})
}

// WithDelegationTags can be used to attach tags to the delegation token used
// internally in AsUser mode.
//
// DEPRECATED.
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
	return rpcOption(func(opts *rpcOptions) {
		opts.delegationTags = tags
	})
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
// This option has absolutely no effect when passed to GetPerRPCCredentials() or
// GetTokenSource(). It applies only to GetRPCTransport().
func WithMonitoringClient(client string) RPCOption {
	return rpcOption(func(opts *rpcOptions) {
		opts.monitoringClient = client
	})
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
func GetRPCTransport(ctx context.Context, kind RPCAuthorityKind, opts ...RPCOption) (http.RoundTripper, error) {
	options, err := makeRPCOptions(kind, opts)
	if err != nil {
		return nil, err
	}

	config := getConfig(ctx)
	if config == nil || config.AnonymousTransport == nil {
		return nil, ErrNotConfigured
	}

	if options.checkCtx != nil {
		if err := options.checkCtx(ctx); err != nil {
			return nil, err
		}
	}

	baseTransport := trace.InstrumentTransport(ctx,
		metric.InstrumentTransport(ctx,
			config.AnonymousTransport(ctx), options.monitoringClient,
		),
	)
	if options.kind == NoAuth {
		return baseTransport, nil
	}

	rootState := GetState(ctx)

	return auth.NewModifyingTransport(baseTransport, func(req *http.Request) error {
		// Prefer to use the request context to get authentication headers, if it is
		// set, to inherit its deadline and cancellation. Assert the request context
		// carries the same auth state (perhaps the background one) as the transport
		// context, otherwise there can be very weird side effects. Constructing
		// the transport with one auth state and then using it with another is not
		// allowed.
		reqCtx := req.Context()
		if reqCtx == context.Background() {
			reqCtx = ctx
		} else {
			switch reqState := GetState(reqCtx); {
			case reqState == rootState:
				// good, exact same state
			case isBackgroundState(reqState) && isBackgroundState(rootState):
				// good, both are background states
			default:
				panic(
					"the transport is shared between different auth contexts, this is not allowed: " +
						"requests passed to a transport created via GetRPCTransport(ctx, ...) " +
						"should either not have any context at all, or have a context that carries the same " +
						"authentication state as `ctx` (i.e. be derived from `ctx` itself or be derived from " +
						"an appropriate parent of `ctx`, like the root context associated with the incoming request)")
			}
		}
		tok, extra, err := options.getRPCHeaders(reqCtx, req.URL.String(), options)
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

func (creds perRPCCreds) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	if len(uri) == 0 {
		panic("perRPCCreds: no URI given")
	}
	tok, extra, err := creds.options.getRPCHeaders(ctx, uri[0], creds.options)
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
// Supports only AsSelf, AsCredentialsForwarder and AsActor authority kinds,
// since they are only ones that exclusively use OAuth2 tokens and no other
// extra headers.
//
// While GetPerRPCCredentials is preferred, this can be used by packages that
// cannot or do not properly handle this gRPC option.
func GetTokenSource(ctx context.Context, kind RPCAuthorityKind, opts ...RPCOption) (oauth2.TokenSource, error) {
	if kind != AsSelf && kind != AsCredentialsForwarder && kind != AsActor {
		return nil, errors.Reason("GetTokenSource can only be used with AsSelf, AsCredentialsForwarder or AsActor authority kind").Err()
	}
	options, err := makeRPCOptions(kind, opts)
	if err != nil {
		return nil, err
	}
	if options.checkCtx != nil {
		if err := options.checkCtx(ctx); err != nil {
			return nil, err
		}
	}
	return &tokenSource{ctx, options}, nil
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
		panic(keys)
	}
	return tok, nil
}

////////////////////////////////////////////////////////////////////////////////
// Internal stuff.

func init() {
	// This is needed to allow packages imported by 'server/auth' to make
	// authenticated calls. They can't use GetRPCTransport directly, since they
	// can't import 'server/auth' (it creates an import cycle).
	internal.RegisterClientFactory(func(ctx context.Context, scopes []string) (*http.Client, error) {
		var t http.RoundTripper
		var err error
		if len(scopes) == 0 {
			t, err = GetRPCTransport(ctx, NoAuth)
		} else {
			t, err = GetRPCTransport(ctx, AsSelf, WithScopes(scopes...))
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
	MintDelegationToken              func(context.Context, DelegationTokenParams) (*Token, error)
	MintAccessTokenForServiceAccount func(context.Context, MintAccessTokenParams) (*Token, error)
	MintIDTokenForServiceAccount     func(context.Context, MintIDTokenParams) (*Token, error)
	MintProjectToken                 func(context.Context, ProjectTokenParams) (*Token, error)
}

// apply implements RPCOption interface.
func (o *rpcMocks) apply(opts *rpcOptions) {
	opts.rpcMocks = o
}

var defaultOAuthScopes = []string{auth.OAuthScopeEmail}

// headersGetter returns a main Authorization token and optional additional
// headers.
type headersGetter func(ctx context.Context, uri string, opts *rpcOptions) (*oauth2.Token, map[string]string, error)

type rpcOptions struct {
	kind             RPCAuthorityKind
	project          string   // for AsProject
	scopes           []string // for AsSelf, AsProject and AsActor
	serviceAccount   string   // for AsActor
	delegationToken  string   // for AsUser
	delegationTags   []string // for AsUser
	monitoringClient string
	checkCtx         func(ctx context.Context) error // optional, may be skipped
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
	asSelfOrActorOrProject := options.kind == AsSelf || options.kind == AsActor || options.kind == AsProject
	if asSelfOrActorOrProject && len(options.scopes) == 0 {
		options.scopes = defaultOAuthScopes
	}

	// Validate options.
	if !asSelfOrActorOrProject && len(options.scopes) != 0 {
		return nil, errors.Reason("WithScopes can only be used with AsSelf, AsActor or AsProject authority kind").Err()
	}
	if options.serviceAccount != "" && options.kind != AsActor {
		return nil, errors.Reason("WithServiceAccount can only be used with AsActor authority kind").Err()
	}
	if options.serviceAccount == "" && options.kind == AsActor {
		return nil, errors.Reason("AsActor authority kind requires WithServiceAccount option").Err()
	}
	if options.delegationToken != "" && options.kind != AsUser {
		return nil, errors.Reason("WithDelegationToken can only be used with AsUser authority kind").Err()
	}
	if len(options.delegationTags) != 0 && options.kind != AsUser {
		return nil, errors.Reason("WithDelegationTags can only be used with AsUser authority kind").Err()
	}
	if len(options.delegationTags) != 0 && options.delegationToken != "" {
		return nil, errors.Reason("WithDelegationTags and WithDelegationToken cannot be used together").Err()
	}
	if options.project == "" && options.kind == AsProject {
		return nil, errors.Reason("AsProject authority kind requires WithProject option").Err()
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
		options.checkCtx = func(ctx context.Context) error {
			_, err := forwardedCreds(ctx)
			return err
		}
		options.getRPCHeaders = func(ctx context.Context, _ string, _ *rpcOptions) (*oauth2.Token, map[string]string, error) {
			tok, err := forwardedCreds(ctx)
			return tok, nil, err
		}
	case AsActor:
		options.getRPCHeaders = asActorHeaders
	case AsProject:
		options.getRPCHeaders = asProjectHeaders
	default:
		return nil, errors.Reason("unknown RPCAuthorityKind %d", options.kind).Err()
	}

	// Default value for "client" field in monitoring metrics.
	if options.monitoringClient == "" {
		options.monitoringClient = "luci-go-server"
	}

	return options, nil
}

// noAuthHeaders is getRPCHeaders for NoAuth mode.
func noAuthHeaders(ctx context.Context, uri string, opts *rpcOptions) (*oauth2.Token, map[string]string, error) {
	return nil, nil, nil
}

// asSelfHeaders returns a map of authentication headers to add to outbound
// RPC requests done in AsSelf mode.
//
// This will be called by the transport layer on each request.
func asSelfHeaders(ctx context.Context, uri string, opts *rpcOptions) (*oauth2.Token, map[string]string, error) {
	cfg := getConfig(ctx)
	if cfg == nil || cfg.AccessTokenProvider == nil {
		return nil, nil, ErrNotConfigured
	}
	tok, err := cfg.AccessTokenProvider(ctx, opts.scopes)
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to get AsSelf access token").Err()
	}
	return tok, nil, nil
}

// asUserHeaders returns a map of authentication headers to add to outbound
// RPC requests done in AsUser mode.
//
// This will be called by the transport layer on each request.
func asUserHeaders(ctx context.Context, uri string, opts *rpcOptions) (*oauth2.Token, map[string]string, error) {
	cfg := getConfig(ctx)
	if cfg == nil || cfg.AccessTokenProvider == nil {
		return nil, nil, ErrNotConfigured
	}

	delegationToken := ""
	if opts.delegationToken != "" {
		delegationToken = opts.delegationToken // WithDelegationToken was used
	} else {
		// Outbound RPC calls in the context of a request from anonymous caller are
		// anonymous too. No need to use any authentication headers.
		userIdent := CurrentIdentity(ctx)
		if userIdent == identity.AnonymousIdentity {
			return nil, nil, nil
		}

		// Grab root URL of the destination service. Only https:// are allowed.
		uri = strings.ToLower(uri)
		if !strings.HasPrefix(uri, "https://") {
			return nil, nil, errors.Reason("refusing to use delegation tokens with non-https URL").Err()
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
		tok, err := mintTokenCall(ctx, DelegationTokenParams{
			TargetHost: host,
			Tags:       opts.delegationTags,
			MinTTL:     10 * time.Minute,
		})
		if err != nil {
			return nil, nil, errors.Annotate(err, "failed to mint AsUser delegation token").Err()
		}
		delegationToken = tok.Token
	}

	// Use our own OAuth token too, since the delegation token is bound to us.
	oauthTok, err := cfg.AccessTokenProvider(ctx, []string{auth.OAuthScopeEmail})
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to get own access token").Err()
	}

	logging.Fields{
		"fingerprint": tokenFingerprint(delegationToken),
	}.Debugf(ctx, "auth: Sending delegation token")
	return oauthTok, map[string]string{delegation.HTTPHeaderName: delegationToken}, nil
}

// forwardedCreds returns the end user token, as it was received by the service.
//
// Returns (nil, nil) if the incoming call was anonymous. Returns an error if
// the incoming call was authenticated by non-forwardable credentials.
func forwardedCreds(ctx context.Context) (*oauth2.Token, error) {
	switch s := GetState(ctx); {
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
func asActorHeaders(ctx context.Context, uri string, opts *rpcOptions) (*oauth2.Token, map[string]string, error) {
	mintTokenCall := MintAccessTokenForServiceAccount
	if opts.rpcMocks != nil && opts.rpcMocks.MintAccessTokenForServiceAccount != nil {
		mintTokenCall = opts.rpcMocks.MintAccessTokenForServiceAccount
	}
	tok, err := mintTokenCall(ctx, MintAccessTokenParams{
		ServiceAccount: opts.serviceAccount,
		Scopes:         opts.scopes,
		MinTTL:         2 * time.Minute,
	})
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to mint AsActor access token").Err()
	}
	return &oauth2.Token{
		AccessToken: tok.Token,
		TokenType:   "Bearer",
		Expiry:      tok.Expiry,
	}, nil, nil
}

// asProjectHeaders returns a map of authentication headers to add to outbound
// RPC requests done in AsProject mode.
//
// This will be called by the transport layer on each request.
func asProjectHeaders(ctx context.Context, uri string, opts *rpcOptions) (*oauth2.Token, map[string]string, error) {
	internal, err := isInternalURI(ctx, uri)
	if err != nil {
		return nil, nil, err
	}

	// For calls within a single LUCI deployment use the service's own OAuth2
	// token and 'X-Luci-Project' header to convey the project identity to the
	// peer.
	if internal {
		// TODO(vadimsh): Always use userinfo.email scope here, not the original
		// one. The target of the call is a LUCI service, it generally doesn't care
		// about non-email scopes, but *requires* userinfo.email.
		tok, _, err := asSelfHeaders(ctx, uri, opts)
		return tok, map[string]string{XLUCIProjectHeader: opts.project}, err
	}

	// For calls to external (non-LUCI) services get an OAuth2 token of a project
	// scoped service account.
	mintTokenCall := MintProjectToken
	if opts.rpcMocks != nil && opts.rpcMocks.MintProjectToken != nil {
		mintTokenCall = opts.rpcMocks.MintProjectToken
	}
	mintParams := ProjectTokenParams{
		MinTTL:      2 * time.Minute,
		LuciProject: opts.project,
		OAuthScopes: opts.scopes,
	}

	tok, err := mintTokenCall(ctx, mintParams)
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to mint AsProject access token").Err()
	}

	// TODO(fmatenaar): This is only during migration and needs to be removed
	// eventually.
	if tok == nil {
		logging.Infof(ctx, "Project %s not found, fallback to service identity", opts.project)
		return asSelfHeaders(ctx, uri, opts)
	}

	return &oauth2.Token{
		AccessToken: tok.Token,
		TokenType:   "Bearer",
		Expiry:      tok.Expiry,
	}, nil, nil
}

// isInternalURI returns true if the URI points to a LUCI microservice belonging
// to the same LUCI deployment as us.
//
// Returns an error if the URI can't be parsed, not https:// or there were
// errors accessing AuthDB to compare the URI against the whitelist.
func isInternalURI(ctx context.Context, uri string) (bool, error) {
	switch u, err := url.Parse(uri); {
	case err != nil:
		return false, errors.Annotate(err, "could not parse URI %q", uri).Err()
	case u.Scheme != "https":
		return false, errors.Reason("AsProject can be used only with https:// targets, got %q", uri).Err()
	default:
		state := GetState(ctx)
		if state == nil {
			return false, ErrNotConfigured
		}
		return state.DB().IsInternalService(ctx, u.Host)
	}
}
