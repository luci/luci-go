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
	"regexp"
	"sort"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/oauth2"
	"google.golang.org/grpc/credentials"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/errors"
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
	// RPC requests done in this mode will have 'Authorization' header set to
	// either an OAuth2 access token or an ID token, depending on a presence of
	// WithIDTokenAudience option.
	//
	// If WithIDTokenAudience is not given, RPCs will be authenticated with
	// an OAuth2 access token of the service's own service account. The set of
	// OAuth scopes can be customized via WithScopes option, and by default it
	// is ["https://www.googleapis.com/auth/userinfo.email"].
	//
	// If WithIDTokenAudience is given, RPCs will be authenticated with an ID
	// token that has `aud` claim set to the supplied value. WithScopes can't be
	// used in this case, providing it will cause an error.
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

	// AsSessionUser is used for outbound RPCs that inherit the authority of
	// an end-user by using credentials stored in the current auth session.
	//
	// Works only if the method used to authenticate the incoming request supports
	// this mechanism. Currently this is only go.chromium.org/luci/server/encryptedcookies.
	//
	// Unlike deprecated AsUser, which uses LUCI delegation tokens, AsSessionUser
	// authenticates outbound RPCs using standard OAuth2 or ID tokens, making this
	// mechanism more widely applicable.
	//
	// On a flip side, the implementation relies on OpenID Connect refresh tokens,
	// which limits it only to real human accounts that can click buttons in the
	// browser to go through the OpenID Connect sign in flow to get a refresh
	// token and establish a session (i.e. service accounts are not supported).
	// Thus this mechanism is primarily useful when implementing Web UIs that use
	// session cookies for authentication and want to call other services on
	// user's behalf from the backend side.
	//
	// By default RPCs performed with AsSessionUser use email-scoped OAuth2 access
	// tokens with the client ID matching the current service OAuth2 client ID.
	// There's no way to ask for more scopes (using WithScopes option would result
	// in an error).
	//
	// If WithIDToken option is specified, RPCs use ID tokens with the audience
	// matching the current service OAuth2 client ID. There's no way to customize
	// the audience.
	AsSessionUser

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
	// either an OAuth2 access token or an ID token of the service account
	// specified by WithServiceAccount option.
	//
	// What kind of token is used depends on a presence of WithIDTokenAudience
	// option and it follows the rules described in AsSelf comment.
	//
	// TODO(crbug.com/1081932): Implement WithIDTokenAudience mode.
	AsActor

	// AsProject is used for outbounds RPCs sent with the authority of some LUCI
	// project (specified via WithProject option).
	//
	// When used to call external services (anything that is not a part of the
	// current LUCI deployment), uses 'Authorization' header with either an OAuth2
	// access token or an ID token of the project-specific service account
	// (specified in the LUCI project definition in 'projects.cfg' deployment
	// configuration file).
	//
	// What kind of token is used in this case depends on a presence of
	// WithIDTokenAudience option and it follows the rules described in AsSelf
	// comment.
	//
	// When used to call LUCI services belonging the same LUCI deployment (per
	// 'internal_service_regexp' setting in 'security.cfg' deployment
	// configuration file) uses the current service's OAuth2 access token plus
	// 'X-Luci-Project' header with the project name. Such calls are authenticated
	// by the peer as coming from 'project:<name>' identity. Options WithScopes
	// and WithIDTokenAudience are ignored in this case.
	//
	// TODO(crbug.com/1081932): Implement WithIDTokenAudience mode.
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

// WithIDToken indicates to use ID tokens instead of OAuth2 tokens.
//
// If no audience is given via WithIDTokenAudience, uses "https://${host}"
// by default.
func WithIDToken() RPCOption {
	return rpcOption(func(opts *rpcOptions) {
		opts.idToken = true
	})
}

// WithIDTokenAudience indicates to use ID tokens with a specific audience
// instead of OAuth2 tokens.
//
// Implies WithIDToken.
//
// The token's `aud` claim will be set to the given value. It can be customized
// per-request by using `${host}` which will be substituted with a host name of
// the request URI.
//
// Usage example:
//
//	tr, err := auth.GetRPCTransport(ctx,
//	  auth.AsSelf,
//	  auth.WithIDTokenAudience("https://${host}"),
//	)
//	if err != nil {
//	  return err
//	}
//	client := &http.Client{Transport: tr}
//	...
//
// Not compatible with WithScopes.
func WithIDTokenAudience(aud string) RPCOption {
	return rpcOption(func(opts *rpcOptions) {
		opts.idToken = true
		opts.idTokenAud = aud
	})
}

// WithScopes can be used to customize OAuth scopes for outbound RPC requests.
//
// Not compatible with WithIDTokenAudience.
func WithScopes(scopes ...string) RPCOption {
	return rpcOption(func(opts *rpcOptions) {
		opts.scopes = append(opts.scopes, scopes...)
	})
}

// WithProject can be used to generate an OAuth token with an identity of that
// particular LUCI project.
//
// See AsProject for more info.
func WithProject(project string) RPCOption {
	return rpcOption(func(opts *rpcOptions) {
		opts.project = project
	})
}

// WithProjectNoFallback is like WithProject, but disables fallback on the
// service global account in case the project doesn't have a project scoped
// account associated with it.
//
// See AsProject for more info.
func WithProjectNoFallback(project string) RPCOption {
	return rpcOption(func(opts *rpcOptions) {
		opts.project = project
		opts.noFallback = true
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
//
//	tr, err := auth.GetRPCTransport(c, auth.AsSelf, auth.WithScopes("..."))
//	if err != nil {
//	  return err
//	}
//	client := &http.Client{Transport: tr}
//	...
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

	baseTransport := otelhttp.NewTransport(
		// Wrap with tsmon metrics.
		metric.InstrumentTransport(ctx,
			config.AnonymousTransport(ctx),
			options.monitoringClient,
		),
		// Further tweak OpenTelemetry tracing wrapper.
		otelhttp.WithSpanNameFormatter(func(op string, r *http.Request) string {
			return r.URL.Path
		}),
	)
	if options.kind == NoAuth {
		return baseTransport, nil
	}

	return auth.NewModifyingTransport(baseTransport, func(req *http.Request) error {
		tok, extra, err := getRPCHeaders(req.Context(), ctx, options, req)
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
func GetPerRPCCredentials(ctx context.Context, kind RPCAuthorityKind, opts ...RPCOption) (credentials.PerRPCCredentials, error) {
	options, err := makeRPCOptions(kind, opts)
	if err != nil {
		return nil, err
	}
	if options.checkCtx != nil {
		if err := options.checkCtx(ctx); err != nil {
			return nil, err
		}
	}
	return perRPCCreds{ctx, options}, nil
}

type perRPCCreds struct {
	ctx     context.Context
	options *rpcOptions
}

func (creds perRPCCreds) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	// Don't transfer tokens in clear text.
	ri, _ := credentials.RequestInfoFromContext(ctx)
	if err := credentials.CheckSecurityLevel(ri.AuthInfo, credentials.PrivacyAndIntegrity); err != nil {
		return nil, errors.Fmt("can't use per RPC credentials: %w", err)
	}

	// URI is needed for some auth modes to "lock" tokens to a concrete audience.
	if len(uri) == 0 {
		panic("perRPCCreds: no URI given")
	}
	u, err := url.Parse(uri[0])
	if err != nil {
		return nil, errors.Fmt("malformed URI %q: %w", uri[0], err)
	}

	tok, extra, err := getRPCHeaders(ctx, creds.ctx, creds.options, &http.Request{URL: u})
	switch {
	case err != nil:
		return nil, err
	case tok == nil && len(extra) == 0:
		return nil, nil
	}

	// gRPC metadata uses lower case keys by convention.
	metadata := make(map[string]string, 1+len(extra))
	if tok != nil {
		metadata["authorization"] = tok.TokenType + " " + tok.AccessToken
	}
	for k, v := range extra {
		metadata[strings.ToLower(k)] = v
	}
	return metadata, nil
}

func (creds perRPCCreds) RequireTransportSecurity() bool {
	return true
}

// GetTokenSource returns an oauth2.TokenSource bound to the supplied Context.
//
// Supports only AsSelf, AsCredentialsForwarder and AsActor authority kinds,
// since they are the only ones that exclusively use only Authorization header.
//
// While GetPerRPCCredentials is preferred, this can be used by packages that
// cannot or do not properly handle this gRPC option.
func GetTokenSource(ctx context.Context, kind RPCAuthorityKind, opts ...RPCOption) (oauth2.TokenSource, error) {
	if kind != AsSelf && kind != AsCredentialsForwarder && kind != AsActor {
		return nil, errors.New("GetTokenSource can only be used with AsSelf, AsCredentialsForwarder or AsActor authority kind")
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
	if options.idTokenAudGen != nil {
		// There's no access to an URI in oauth2.TokenSource.Token() method, can't
		// use patterned audiences there.
		return nil, errors.New("WithIDTokenAudience with patterned audience is not supported by GetTokenSource, " +
			"use GetRPCTransport or GetPerRPCCredentials instead")
	}
	return &tokenSource{ctx, options}, nil
}

type tokenSource struct {
	ctx     context.Context
	options *rpcOptions
}

func (ts *tokenSource) Token() (*oauth2.Token, error) {
	tok, extra, err := ts.options.getRPCHeaders(ts.ctx, ts.options, nil)
	switch {
	case err != nil:
		return nil, err
	case tok == nil:
		return nil, errors.New("using non-OAuth2 based credentials in TokenSource")
	case len(extra) != 0:
		keys := make([]string, 0, len(extra))
		for k := range extra {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return nil, errors.Fmt("extra headers %q with credentials are not supported in TokenSource", keys)
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

var defaultOAuthScopes = scopes.DefaultScopeSet()

// headersGetter returns a main Authorization token and optional additional
// headers.
//
// `req` is an outbound request if known. May be nil. May not be fully
// initialized for the gRPC case.
type headersGetter func(ctx context.Context, opts *rpcOptions, req *http.Request) (*oauth2.Token, map[string]string, error)

// audGenerator takes a request and returns an audience string derived from it.
type audGenerator func(r *http.Request) (string, error)

type rpcOptions struct {
	kind             RPCAuthorityKind
	project          string       // for AsProject
	noFallback       bool         // for AsProject
	idToken          bool         // for AsSelf, AsProject, AsActor and AsSessionUser
	idTokenAud       string       // for AsSelf, AsProject and AsActor
	idTokenAudGen    audGenerator // non-nil iff idTokenAud is a pattern
	scopes           []string     // for AsSelf, AsProject and AsActor
	serviceAccount   string       // for AsActor
	delegationToken  string       // for AsUser
	delegationTags   []string     // for AsUser
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

	asSelfOrActorOrProject := options.kind == AsSelf ||
		options.kind == AsActor ||
		options.kind == AsProject

	// Set default scopes.
	if asSelfOrActorOrProject && !options.idToken && len(options.scopes) == 0 {
		options.scopes = defaultOAuthScopes
	}
	// Set the default audience.
	if options.kind != AsSessionUser && options.idToken && options.idTokenAud == "" {
		options.idTokenAud = "https://${host}"
	}

	// Validate options.
	if !asSelfOrActorOrProject && options.kind != AsSessionUser && options.idToken {
		return nil, errors.New("WithIDToken can only be used with AsSelf, AsActor, AsProject or AsSessionUser authority kind")
	}
	if !asSelfOrActorOrProject && options.idTokenAud != "" {
		return nil, errors.New("WithIDTokenAudience can only be used with AsSelf, AsActor or AsProject authority kind")
	}
	if !asSelfOrActorOrProject && len(options.scopes) != 0 {
		return nil, errors.New("WithScopes can only be used with AsSelf, AsActor or AsProject authority kind")
	}
	if options.idToken && len(options.scopes) != 0 {
		return nil, errors.New("WithIDToken and WithScopes cannot be used together")
	}
	if options.serviceAccount != "" && options.kind != AsActor {
		return nil, errors.New("WithServiceAccount can only be used with AsActor authority kind")
	}
	if options.serviceAccount == "" && options.kind == AsActor {
		return nil, errors.New("AsActor authority kind requires WithServiceAccount option")
	}
	if options.delegationToken != "" && options.kind != AsUser {
		return nil, errors.New("WithDelegationToken can only be used with AsUser authority kind")
	}
	if len(options.delegationTags) != 0 && options.kind != AsUser {
		return nil, errors.New("WithDelegationTags can only be used with AsUser authority kind")
	}
	if len(options.delegationTags) != 0 && options.delegationToken != "" {
		return nil, errors.New("WithDelegationTags and WithDelegationToken cannot be used together")
	}
	if options.project == "" && options.kind == AsProject {
		return nil, errors.New("AsProject authority kind requires WithProject option")
	}

	// Temporarily not supported combinations of options.
	//
	// TODO(crbug.com/1081932): Support.
	if options.idToken && (options.kind == AsActor || options.kind == AsProject) {
		return nil, errors.New("WithIDToken is not supported here yet")
	}

	// Convert `idTokenAud` into a callback {http.Request => aud}. This is needed
	// to support "${host}" substitution.
	if options.idTokenAud != "" {
		gen, err := parseAudPattern(options.idTokenAud)
		if err != nil {
			return nil, errors.Fmt("bad WithIDTokenAudience value: %w", err)
		}
		options.idTokenAudGen = gen // this is nil if idTokenAud is not a pattern
	}

	// Validate 'kind' and pick correct implementation of getRPCHeaders.
	switch options.kind {
	case NoAuth:
		options.getRPCHeaders = noAuthHeaders
	case AsSelf:
		if options.idTokenAud != "" {
			options.getRPCHeaders = asSelfIDTokenHeaders
		} else {
			options.getRPCHeaders = asSelfOAuthHeaders
		}
	case AsUser:
		options.getRPCHeaders = asUserHeaders
	case AsSessionUser:
		options.checkCtx = func(ctx context.Context) error {
			_, err := currentSession(ctx)
			return err
		}
		options.getRPCHeaders = asSessionUserHeaders
	case AsCredentialsForwarder:
		options.checkCtx = func(ctx context.Context) error {
			_, _, err := forwardedCreds(ctx)
			return err
		}
		options.getRPCHeaders = func(ctx context.Context, _ *rpcOptions, _ *http.Request) (*oauth2.Token, map[string]string, error) {
			return forwardedCreds(ctx)
		}
	case AsActor:
		options.getRPCHeaders = asActorHeaders
	case AsProject:
		options.getRPCHeaders = asProjectHeaders
	default:
		return nil, errors.Fmt("unknown RPCAuthorityKind %d", options.kind)
	}

	// Default value for "client" field in monitoring metrics.
	if options.monitoringClient == "" {
		options.monitoringClient = "luci-go-server"
	}

	return options, nil
}

// noAuthHeaders is getRPCHeaders for NoAuth mode.
func noAuthHeaders(ctx context.Context, opts *rpcOptions, req *http.Request) (*oauth2.Token, map[string]string, error) {
	return nil, nil, nil
}

// asSelfOAuthHeaders returns a map of authentication headers to add to outbound
// RPC requests done in AsSelf mode when using OAuth2 access tokens.
//
// This will be called by the transport layer on each request.
func asSelfOAuthHeaders(ctx context.Context, opts *rpcOptions, req *http.Request) (*oauth2.Token, map[string]string, error) {
	cfg := getConfig(ctx)
	if cfg == nil || cfg.AccessTokenProvider == nil {
		return nil, nil, ErrNotConfigured
	}
	tok, err := cfg.AccessTokenProvider(ctx, opts.scopes)
	if err != nil {
		return nil, nil, errors.Fmt("failed to get AsSelf access token: %w", err)
	}
	return tok, nil, nil
}

// asSelfIDTokenHeaders returns a map of authentication headers to add to
// outbound RPC requests done in AsSelf mode when using ID tokens.
//
// This will be called by the transport layer on each request.
func asSelfIDTokenHeaders(ctx context.Context, opts *rpcOptions, req *http.Request) (*oauth2.Token, map[string]string, error) {
	cfg := getConfig(ctx)
	if cfg == nil || cfg.Signer == nil {
		return nil, nil, ErrNotConfigured
	}

	// Derive the audience string. It may have "${host}" var that is replaced
	// based on the hostname in the `req`.
	var aud string
	if opts.idTokenAudGen != nil {
		var err error
		if aud, err = opts.idTokenAudGen(req); err != nil {
			return nil, nil, errors.Fmt("can't derive audience for ID token: %w", err)
		}
	} else {
		// Using a static audience, not a pattern.
		aud = opts.idTokenAud
	}

	// First try the environment-specific method of getting an ID token (e.g.
	// querying it from the GCE metadata server). It may not be available (e.g.
	// on GAE v1). We'll fall back to a more expensive generic method below.
	if cfg.IDTokenProvider != nil {
		tok, err := cfg.IDTokenProvider(ctx, aud)
		return tok, nil, err
	}

	// The method below works almost everywhere, but it requires the service
	// account to have iam.serviceAccountTokenCreator role on itself, which is
	// a bit weird and not default.

	// Discover our own service account name to use it as a target.
	info, err := cfg.Signer.ServiceInfo(ctx)
	switch {
	case err != nil:
		return nil, nil, errors.Fmt("failed to get our own service info: %w", err)
	case info.ServiceAccountName == "":
		return nil, nil, errors.New("no service account name in our own service info")
	}

	// Grab ID token for our own account. This uses our own IAM-scoped access
	// token internally and also implements heavy caching of the result, so its
	// fine to call it often.
	mintTokenCall := MintIDTokenForServiceAccount
	if opts.rpcMocks != nil && opts.rpcMocks.MintIDTokenForServiceAccount != nil {
		mintTokenCall = opts.rpcMocks.MintIDTokenForServiceAccount
	}
	tok, err := mintTokenCall(ctx, MintIDTokenParams{
		ServiceAccount: info.ServiceAccountName,
		Audience:       aud,
		MinTTL:         2 * time.Minute,
	})
	if err != nil {
		return nil, nil, errors.Fmt("failed to get our own ID token for %q with aud %q: %w", info.ServiceAccountName, aud, err)
	}

	return &oauth2.Token{
		AccessToken: tok.Token,
		TokenType:   "Bearer",
		Expiry:      tok.Expiry,
	}, nil, nil
}

// asUserHeaders returns a map of authentication headers to add to outbound
// RPC requests done in AsUser mode.
//
// This will be called by the transport layer on each request.
func asUserHeaders(ctx context.Context, opts *rpcOptions, req *http.Request) (*oauth2.Token, map[string]string, error) {
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

		// Only https:// are allowed, can't send bearer tokens in clear text.
		if req.URL.Scheme != "https" {
			return nil, nil, errors.New("refusing to use delegation tokens with non-https URL")
		}

		// Grab a token that's good enough for at least 10 min. Outbound RPCs
		// shouldn't last longer than that.
		mintTokenCall := MintDelegationToken
		if opts.rpcMocks != nil && opts.rpcMocks.MintDelegationToken != nil {
			mintTokenCall = opts.rpcMocks.MintDelegationToken
		}
		tok, err := mintTokenCall(ctx, DelegationTokenParams{
			TargetHost: req.URL.Hostname(),
			Tags:       opts.delegationTags,
			MinTTL:     10 * time.Minute,
		})
		if err != nil {
			return nil, nil, errors.Fmt("failed to mint AsUser delegation token: %w", err)
		}
		delegationToken = tok.Token
	}

	// Use our own OAuth token too, since the delegation token is bound to us.
	oauthTok, err := cfg.AccessTokenProvider(ctx, scopes.DefaultScopeSet())
	if err != nil {
		return nil, nil, errors.Fmt("failed to get own access token: %w", err)
	}

	logging.Fields{
		"fingerprint": tokenFingerprint(delegationToken),
	}.Debugf(ctx, "auth: Sending delegation token")
	return oauthTok, map[string]string{delegation.HTTPHeaderName: delegationToken}, nil
}

// forwardedCreds returns the end user token and any extra authentication
// headers as they were received by the service.
//
// Returns (nil, nil, nil) if the incoming call was anonymous. Returns an error
// if the incoming call was authenticated by non-forwardable credentials.
func forwardedCreds(ctx context.Context) (*oauth2.Token, map[string]string, error) {
	switch s := GetState(ctx); {
	case s == nil:
		return nil, nil, ErrNotConfigured
	case s.User().Identity == identity.AnonymousIdentity:
		return nil, nil, nil // nothing to forward if the call is anonymous
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
func asActorHeaders(ctx context.Context, opts *rpcOptions, req *http.Request) (*oauth2.Token, map[string]string, error) {
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
		return nil, nil, errors.Fmt("failed to mint AsActor access token: %w", err)
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
func asProjectHeaders(ctx context.Context, opts *rpcOptions, req *http.Request) (*oauth2.Token, map[string]string, error) {
	internal, err := isInternalURL(ctx, req.URL)
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
		tok, _, err := asSelfOAuthHeaders(ctx, opts, req)
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
		return nil, nil, errors.Fmt("failed to mint AsProject access token: %w", err)
	}

	if tok == nil {
		if opts.noFallback {
			logging.Errorf(ctx, "Project %q doesn't have a project-scoped account, but it is required", opts.project)
			return nil, nil, errors.Fmt("project %q doesn't have a project-scoped account, can't act as this project", opts.project)
		}
		logging.Infof(ctx, "Project %q doesn't have a project-scoped account, fallback to the service identity", opts.project)
		return asSelfOAuthHeaders(ctx, opts, req)
	}

	return &oauth2.Token{
		AccessToken: tok.Token,
		TokenType:   "Bearer",
		Expiry:      tok.Expiry,
	}, nil, nil
}

// currentSession either returns the current session or ErrNotConfigured.
func currentSession(ctx context.Context) (Session, error) {
	if state := GetState(ctx); state != nil {
		return state.Session(), nil
	}
	return nil, ErrNotConfigured
}

// asSessionUserHeaders returns a map of authentication headers to add to
// outbound RPC requests done in AsSessionUser mode.
//
// This will be called by the transport layer on each request.
func asSessionUserHeaders(ctx context.Context, opts *rpcOptions, _ *http.Request) (tok *oauth2.Token, _ map[string]string, err error) {
	s, err := currentSession(ctx)
	if err != nil {
		return nil, nil, err
	}
	if s == nil {
		return nil, nil, nil
	}
	if opts.idToken {
		tok, err = s.IDToken(ctx)
	} else {
		tok, err = s.AccessToken(ctx)
	}
	return
}

// isInternalURL returns true if the URL points to a LUCI microservice belonging
// to the same LUCI deployment as us.
//
// Returns an error if the URL is not https:// or there were errors accessing
// the AuthDB to compare the URL against the list of LUCI services.
func isInternalURL(ctx context.Context, u *url.URL) (bool, error) {
	if u.Scheme != "https" {
		return false, errors.Fmt("AsProject can be used only with https:// targets, got %s", u)
	}
	state := GetState(ctx)
	if state == nil {
		return false, ErrNotConfigured
	}
	return state.DB().IsInternalService(ctx, u.Hostname())
}

var placeholderRe = regexp.MustCompile(`\${[^}]*}`)

// parseAudPattern takes a pattern like "https://${host}" and produces
// a callback that knows how to fill it in given a *http.Request.
//
// Returns (nil, nil) if `pat` is not really a pattern but just a static string.
// Returns an error if `pat` looks like a malformed or unsupported pattern.
func parseAudPattern(pat string) (audGenerator, error) {
	// Recognized static string, use a cheesy check for mismatched curly braces.
	if !placeholderRe.MatchString(pat) {
		if strings.Contains(pat, "${") {
			return nil, errors.Fmt("%q looks like a malformed pattern", pat)
		}
		return nil, nil
	}

	renderPat := func(req *http.Request) (out string, err error) {
		out = placeholderRe.ReplaceAllStringFunc(pat, func(match string) string {
			if err == nil {
				switch match {
				case "${host}":
					return renderAudHost(req)
				default:
					err = errors.Fmt("unknown var %s", match)
				}
			}
			return ""
		})
		return
	}

	// Verify all referenced vars are known by interpreting a phony request. That
	// way a set of supported vars is neatly referenced only in `renderPat`.
	_, err := renderPat(&http.Request{
		URL: &url.URL{
			Scheme: "https",
			Host:   "example.com",
			Path:   "/example",
		},
	})
	if err != nil {
		return nil, errors.Fmt("bad pattern %q: %w", pat, err)
	}

	return renderPat, nil
}

// renderAudHost renders "${host}" variable used in the audience pattern string.
func renderAudHost(req *http.Request) string {
	// Prefer a value of `Host` header when given.
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	// Strip the default port number. This is mostly useful when calling Cloud Run
	// which doesn't like audiences with "...:443".
	switch req.URL.Scheme {
	case "http":
		return strings.TrimSuffix(host, ":80")
	case "https":
		return strings.TrimSuffix(host, ":443")
	default:
		return host
	}
}

// getRPCHeaders calls opts.getRPCHeaders callback, passing it correct context.
//
// Some libraries (in particular Spanner), use very bare bones `ctx` as a
// request context (essentially context.Background() with gRPC metadata on top).
// Such contexts are not sufficient to call getRPCHeaders, so we merge it with
// the context used to create the RPC transport or credentials provider to get
// a full-featured LUCI context that at the same time has the same deadline and
// cancellation as `ctx`.
func getRPCHeaders(ctx, transportCtx context.Context, opts *rpcOptions, req *http.Request) (*oauth2.Token, map[string]string, error) {
	merged := &internal.MergedContext{
		Root:     ctx,
		Fallback: transportCtx,
	}

	// Forbid a case when a transport is created with one user context, but then
	// reused with another. This is likely a bug and can lead to leakage of
	// user credentials (when such transport is e.g. AsCredentialsForwarder).
	if tstate := GetState(transportCtx); !isBackgroundState(tstate) {
		if rstate := GetState(merged); !isBackgroundState(rstate) {
			if tstate != rstate {
				return nil, nil, errors.New("a transport or credentials provider created within a context of one user request is used within another user request, this is dangerous")
			}
		}
	}

	return opts.getRPCHeaders(merged, opts, req)
}
