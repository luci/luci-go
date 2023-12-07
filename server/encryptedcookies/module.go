// Copyright 2021 The LUCI Authors.
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

package encryptedcookies

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/google/tink/go/tink"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag/stringlistflag"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/encryptedcookies/internal"
	"go.chromium.org/luci/server/encryptedcookies/internal/fakecookies"
	"go.chromium.org/luci/server/encryptedcookies/session"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/warmup"
)

// ModuleName can be used to refer to this module when declaring dependencies.
var ModuleName = module.RegisterName("go.chromium.org/luci/server/encryptedcookies")

// ModuleOptions contain configuration of the encryptedcookies server module.
type ModuleOptions struct {
	// TinkAEADKey is a "sm://..." reference to a Tink AEAD keyset to use.
	//
	// If empty, will use the primary keyset via secrets.PrimaryTinkAEAD().
	TinkAEADKey string

	// DiscoveryURL is an URL of the discovery document with provider's config.
	DiscoveryURL string

	// ClientID identifies OAuth2 Web client representing the application.
	ClientID string

	// ClientSecret is a "sm://..." reference to OAuth2 client secret.
	ClientSecret string

	// RedirectURL must be `https://<host>/auth/openid/callback`.
	RedirectURL string

	// SessionStoreKind can be used to pick a concrete implementation of a store.
	SessionStoreKind string

	// SessionStoreNamespace can be used to namespace sessions in the store.
	SessionStoreNamespace string

	// RequiredScopes is a list of required OAuth scopes that will be requested
	// when making the OAuth authorization request, in addition to the default
	// scopes (openid email profile) and the OptionalScopes.
	//
	// Existing sessions that don't have the required scopes will be closed. All
	// scopes in the RequiredScopes must be in the RequiredScopes or
	// OptionalScopes of other running instances of the app. Otherwise a session
	// opened by other running instances could be closed immediately.
	RequiredScopes stringlistflag.Flag

	// OptionalScopes is a list of optional OAuth scopes that will be requested
	// when making the OAuth authorization request, in addition to the default
	// scopes (openid email profile) and the RequiredScopes.
	//
	// Existing sessions that don't have the optional scopes will not be closed.
	// This is useful for rolling out changes incrementally. Once the new version
	// takes over all the traffic, promote the optional scopes to RequiredScopes.
	OptionalScopes stringlistflag.Flag

	// ExposeStateEndpoint controls whether "/auth/openid/state" endpoint should
	// be exposed.
	//
	// See auth.StateEndpointResponse struct for details.
	//
	// It is off by default since it can potentially make XSS vulnerabilities more
	// severe by exposing OAuth and ID tokens to malicious injected code. It
	// should be enabled only if the frontend code needs it and it is aware of
	// XSS risks.
	ExposeStateEndpoint bool

	// LimitCookieExposure, if set, limits the cookie to be set only on
	// "/auth/openid/" HTTP path and makes it `SameSite: strict`.
	//
	// This is useful for SPAs that exchange cookies for authentication tokens via
	// fetch(...) requests to "/auth/openid/state". In this case the cookie is
	// not normally used by any other HTTP handler and it makes no sense to send
	// it in every request.
	LimitCookieExposure bool
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.StringVar(
		&o.TinkAEADKey,
		"encrypted-cookies-tink-aead-key",
		o.TinkAEADKey,
		`An optional reference (e.g. "sm://...") to a secret with Tink AEAD keyset `+
			`to use to encrypt cookies instead of the -primary-tink-aead-key.`,
	)
	f.StringVar(
		&o.DiscoveryURL,
		"encrypted-cookies-discovery-url",
		o.DiscoveryURL,
		`URL of the discovery document with OpenID provider's config.`,
	)
	f.StringVar(
		&o.ClientID,
		"encrypted-cookies-client-id",
		o.ClientID,
		`OAuth2 web client ID representing the application.`,
	)
	f.StringVar(
		&o.ClientSecret,
		"encrypted-cookies-client-secret",
		o.ClientSecret,
		`Reference (e.g. "sm://...") to a secret with OAuth2 client secret.`,
	)
	f.StringVar(
		&o.RedirectURL,
		"encrypted-cookies-redirect-url",
		o.RedirectURL,
		fmt.Sprintf(`A redirect URL registered with the OpenID provider, must end with %q.`, callbackURL),
	)
	f.StringVar(
		&o.SessionStoreKind,
		"encrypted-cookies-session-store-kind",
		o.SessionStoreKind,
		`Defines what sort of a session store to use if there's more than one available.`,
	)
	f.StringVar(
		&o.SessionStoreNamespace,
		"encrypted-cookies-session-store-namespace",
		o.SessionStoreNamespace,
		`Namespace for the sessions in the store.`,
	)
	f.Var(
		&o.RequiredScopes,
		`encrypted-cookies-required-scopes`, `Required OAuth scopes that will be requested when `+
			`making the OAuth authorization request, in addition to the default `+
			`scopes (openid email profile) and the optional-scopes. Existing `+
			`sessions without the required scopes will be closed.`,
	)
	f.Var(
		&o.OptionalScopes,
		`encrypted-cookies-optional-scopes`, `Optional OAuth scopes that will be requested when `+
			`making the OAuth authorization request, in addition to the default `+
			`scopes (openid email profile) and the required-scopes. Existing `+
			`sessions without the optional scopes will NOT be closed.`,
	)
	f.BoolVar(&o.ExposeStateEndpoint,
		"encrypted-cookies-expose-state-endpoint",
		o.ExposeStateEndpoint,
		`Controls whether to expose "/auth/openid/state" endpoint.`,
	)
	f.BoolVar(&o.LimitCookieExposure,
		"encrypted-cookies-limit-cookie-exposure",
		o.LimitCookieExposure,
		`If set, assign the cookie only to "/auth/openid/" HTTP path and make it "SameSite: strict".`,
	)
}

// NewModule returns a server module that configures an authentication method
// based on encrypted cookies.
func NewModule(opts *ModuleOptions) module.Module {
	if opts == nil {
		opts = &ModuleOptions{}
	}
	return &serverModule{opts: opts}
}

// NewModuleFromFlags is a variant of NewModule that initializes options through
// command line flags.
//
// Calling this function registers flags in flag.CommandLine. They are usually
// parsed in server.Main(...).
func NewModuleFromFlags() module.Module {
	opts := &ModuleOptions{}
	opts.Register(flag.CommandLine)
	return NewModule(opts)
}

// serverModule implements module.Module.
type serverModule struct {
	opts *ModuleOptions
}

// Name is part of module.Module interface.
func (*serverModule) Name() module.Name {
	return ModuleName
}

// Dependencies is part of module.Module interface.
func (*serverModule) Dependencies() []module.Dependency {
	deps := []module.Dependency{
		module.RequiredDependency(secrets.ModuleName),
	}
	for _, impl := range internal.StoreImpls() {
		deps = append(deps, impl.Deps...)
	}
	return deps
}

// Initialize is part of module.Module interface.
func (m *serverModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	// If in the dev mode and have no configuration, use a fake implementation.
	if !opts.Prod && m.opts.ClientID == "" {
		return ctx, m.initInDevMode(ctx, host)
	}

	// Fill in defaults.
	if m.opts.DiscoveryURL == "" {
		m.opts.DiscoveryURL = openid.GoogleDiscoveryURL
	}

	// Check required flags.
	if m.opts.ClientID == "" {
		return nil, errors.Reason("client ID is required").Err()
	}
	if m.opts.ClientSecret == "" {
		return nil, errors.Reason("client secret is required").Err()
	}
	if m.opts.RedirectURL == "" {
		return nil, errors.Reason("redirect URL is required").Err()
	}
	if !strings.HasSuffix(m.opts.RedirectURL, callbackURL) {
		return nil, errors.Reason("redirect URL should end with %q", callbackURL).Err()
	}

	// Figure out what AEAD key to use.
	var aead *secrets.AEADHandle
	if m.opts.TinkAEADKey != "" {
		var err error
		if aead, err = secrets.LoadTinkAEAD(ctx, m.opts.TinkAEADKey); err != nil {
			return nil, err
		}
	} else {
		aead = secrets.PrimaryTinkAEAD(ctx)
		if aead == nil {
			return nil, errors.Reason("no AEAD key is configured, use either -primary-tink-aead-key or -encrypted-cookies-tink-aead-key").Err()
		}
	}

	// Construct the session store based on a link time config and CLI flags.
	sessions, err := m.initSessionStore(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "failed to initialize the session store").Err()
	}

	// Load initial values of secrets to verify they are correct. This also
	// subscribes to their rotations.
	cfg, err := m.loadOpenIDConfig(ctx)
	if err != nil {
		return nil, err
	}

	// Have enough configuration to create the AuthMethod.
	method := &AuthMethod{
		OpenIDConfig:        func(context.Context) (*OpenIDConfig, error) { return cfg.Load().(*OpenIDConfig), nil },
		AEADProvider:        func(context.Context) tink.AEAD { return aead.Unwrap() },
		Sessions:            sessions,
		Insecure:            !opts.Prod,
		OptionalScopes:      m.opts.OptionalScopes,
		RequiredScopes:      m.opts.RequiredScopes,
		ExposeStateEndpoint: m.opts.ExposeStateEndpoint,
		LimitCookieExposure: m.opts.LimitCookieExposure,
	}

	// Register it with the server guts.
	host.RegisterCookieAuth(method)
	warmup.Register("server/encryptedcookies", method.Warmup)
	method.InstallHandlers(host.Routes(), nil)

	return ctx, nil
}

// initSessionStore makes a store based on a link time configuration and flags.
func (m *serverModule) initSessionStore(ctx context.Context) (session.Store, error) {
	impls := internal.StoreImpls()

	var ids []string
	for _, impl := range impls {
		ids = append(ids, impl.ID)
	}
	idsStr := strings.Join(ids, ", ")

	var impl internal.StoreImpl
	switch {
	case len(impls) == 0:
		return nil, errors.Reason("no session store implementations are linked into the binary, " +
			"use nameless imports to link to some").Err()
	case len(impls) == 1 && m.opts.SessionStoreKind == "":
		impl = impls[0] // have only one and can use it by default
	case len(impls) > 1 && m.opts.SessionStoreKind == "":
		return nil, errors.Reason(
			"multiple session store implementations are linked into the binary, "+
				"pick one explicitly: %s", idsStr).Err()
	default:
		found := false
		for _, impl = range impls {
			if impl.ID == m.opts.SessionStoreKind {
				found = true
				break
			}
		}
		if !found {
			return nil, errors.Reason("session store implementation %q is not linked into the binary, "+
				"linked implementations: %s", m.opts.SessionStoreKind, idsStr).Err()
		}
	}

	return impl.Factory(ctx, m.opts.SessionStoreNamespace)
}

// loadOpenIDConfig loads the client secret and constructs OpenIDConfig with it.
//
// Subscribes to its rotation. Returns an atomic with the current value of
// the OpenID config (as *OpenIDConfig). It will be updated when the secret is
// rotated.
func (m *serverModule) loadOpenIDConfig(ctx context.Context) (*atomic.Value, error) {
	secret, err := secrets.StoredSecret(ctx, m.opts.ClientSecret)
	if err != nil {
		return nil, errors.Annotate(err, "failed to load OAuth2 client secret").Err()
	}

	openIDConfig := func(s *secrets.Secret) *OpenIDConfig {
		return &OpenIDConfig{
			DiscoveryURL: m.opts.DiscoveryURL,
			ClientID:     m.opts.ClientID,
			ClientSecret: string(s.Active),
			RedirectURI:  m.opts.RedirectURL,
		}
	}

	val := &atomic.Value{}
	val.Store(openIDConfig(&secret))

	secrets.AddRotationHandler(ctx, m.opts.ClientSecret, func(ctx context.Context, secret secrets.Secret) {
		logging.Infof(ctx, "OAuth2 client secret was rotated")
		val.Store(openIDConfig(&secret))
	})

	return val, nil
}

// initInDevMode initializes a primitive fake cookie-based auth method.
//
// Can be used on the localhost during the development as a replacement for the
// real thing.
func (m *serverModule) initInDevMode(ctx context.Context, host module.Host) error {
	method := &fakecookies.AuthMethod{LimitCookieExposure: m.opts.LimitCookieExposure}
	host.RegisterCookieAuth(method)
	method.InstallHandlers(host.Routes(), nil)

	// fakecookies.AuthMethod can't register the state handler itself since it
	// introduces module import cycle, so do it here instead. fakecookies is
	// internal API of this package.
	if m.opts.ExposeStateEndpoint {
		authenticator := auth.Authenticator{Methods: []auth.Method{method}}
		host.Routes().GET(stateURL, []router.Middleware{authenticator.GetMiddleware()}, func(ctx *router.Context) {
			stateHandlerImpl(ctx, fakecookies.IsFakeCookiesSession)
		})
		method.ExposedStateEndpoint = stateURL
	}

	return nil
}
