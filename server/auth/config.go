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
	"net/http"
	"time"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/signing"
)

var cfgContextKey = "auth.Config context key"

// DBProvider is a callback that returns most recent DB instance.
//
// DB represents a snapshot of user groups used for authorization checks.
type DBProvider func(ctx context.Context) (authdb.DB, error)

// AccessTokenProvider knows how to generate OAuth2 access tokens for the
// service account belonging to the server itself.
type AccessTokenProvider func(ctx context.Context, scopes []string) (*oauth2.Token, error)

// IDTokenProvider knows how to generate ID tokens for the service account
// belonging to the server itself.
type IDTokenProvider func(ctx context.Context, audience string) (*oauth2.Token, error)

// AnonymousTransportProvider returns http.RoundTriper that can make
// unauthenticated HTTP requests.
//
// The returned round tripper is assumed to be bound to the context and won't
// outlive it.
type AnonymousTransportProvider func(ctx context.Context) http.RoundTripper

// ActorTokensProvider knows how to produce OAuth and ID tokens for service
// accounts the server "acts as".
//
// `serviceAccount` and `delegates` should just be service account email
// addresses (no need to prefix them with `projects/-/serviceAccounts/`).
//
// Errors returned by its method may be tagged with transient.Tag to indicate
// they are transient. All other errors are assumed to be fatal.
type ActorTokensProvider interface {
	// GenerateAccessToken generates an access token for the given account.
	GenerateAccessToken(ctx context.Context, serviceAccount string, scopes, delegates []string) (*oauth2.Token, error)
	// GenerateIDToken generates an ID token for the given account.
	GenerateIDToken(ctx context.Context, serviceAccount, audience string, delegates []string) (string, error)
}

// Config contains global configuration of the auth library.
//
// This configuration adjusts the library to the particular execution
// environment (GAE, Flex, whatever). It contains concrete implementations of
// various interfaces used by the library.
//
// It lives in the context and must be installed there by some root middleware
// (via ModifyConfig call).
type Config struct {
	// DBProvider is a callback that returns most recent DB instance.
	DBProvider DBProvider

	// Signer possesses the service's private key and can sign blobs with it.
	//
	// It provides the bundle with corresponding public keys and information about
	// the service account they belong too (the service's own identity).
	//
	// Used to implement '/auth/api/v1/server/(certificates|info)' routes.
	Signer signing.Signer

	// AccessTokenProvider knows how to generate OAuth2 access tokens for the
	// service account belonging to the server itself.
	AccessTokenProvider AccessTokenProvider

	// IDTokenProvider knows how to generate ID tokens for the service account
	// belonging to the server itself.
	//
	// Optional. If not set the server will fall back to using the generateIdToken
	// IAM RPC targeting its own account. This fallback requires the service
	// account to have iam.serviceAccountTokenCreator role on *itself*, which is
	// a bit weird and not default.
	IDTokenProvider IDTokenProvider

	// ActorTokensProvider knows how to produce OAuth and ID tokens for service
	// accounts the server "act as".
	//
	// If nil, a default generic implementation based on HTTP POST request to
	// Cloud IAM Credentials service will be used.
	ActorTokensProvider ActorTokensProvider

	// AnonymousTransport returns http.RoundTriper that can make unauthenticated
	// HTTP requests.
	//
	// The returned round tripper is assumed to be bound to the context and won't
	// outlive it.
	AnonymousTransport AnonymousTransportProvider

	// EndUserIP takes a request and returns IP address of a user that sent it.
	//
	// If nil, a default implementation is used. It simply returns r.RemoteAddr().
	//
	// A custom implementation may parse X-Forwarded-For header (or other headers)
	// depending on how load balancers and proxies in front the server populate
	// them.
	EndUserIP func(r RequestMetadata) string

	// FrontendClientID returns an OAuth 2.0 client ID to use from the frontend.
	//
	// It will be served via /auth/api/v1/server/client_id endpoint (so that the
	// frontend can grab it), and the auth library will permit access tokens with
	// this client as an audience when authenticating requests.
	//
	// May return an empty string if the client ID is not configured. All errors
	// are treated as transient.
	//
	// Should be fast. Will be called each time a token needs to be authenticated.
	FrontendClientID func(context.Context) (string, error)

	// FrontendOAuthScopes returns the OAuth scopes to use from the frontend.
	//
	// May return an empty slice if the OAuth scopes are not configured. All
	// errors are treated as transient.
	//
	// Should be fast. Will be called each time a token needs to be authenticated.
	FrontendOAuthScopes func(context.Context) ([]string, error)

	// IsDevMode is true when running the server locally during development.
	//
	// Setting this to true changes default deadlines. For instance, GAE dev
	// server is known to be very slow and deadlines tuned for production
	// environment are too limiting.
	IsDevMode bool
}

// Initialize inserts authentication configuration into the context.
//
// An initialized context is required by any other function in the package. They
// return ErrNotConfigured otherwise.
//
// Calling Initialize twice causes a panic.
func Initialize(ctx context.Context, cfg *Config) context.Context {
	if getConfig(ctx) != nil {
		panic("auth.Initialize is called twice on same context")
	}
	return setConfig(ctx, cfg)
}

// ModifyConfig makes a context with a derived configuration.
//
// It grabs current configuration from the context (if any), passes it to the
// callback, and puts whatever callback returns into a derived context.
func ModifyConfig(ctx context.Context, cb func(Config) Config) context.Context {
	var cfg Config
	if cur := getConfig(ctx); cur != nil {
		cfg = *cur
	}
	cfg = cb(cfg)
	return setConfig(ctx, &cfg)
}

// adjustedTimeout returns `t` if IsDevMode is false or >=1 min if true.
func (cfg *Config) adjustedTimeout(t time.Duration) time.Duration {
	if !cfg.IsDevMode {
		return t
	}
	if t > time.Minute {
		return t
	}
	return time.Minute
}

// actorTokensProvider returns an actual ActorTokensProvider implementation.
func (cfg *Config) actorTokensProvider() ActorTokensProvider {
	if cfg.ActorTokensProvider != nil {
		return cfg.ActorTokensProvider
	}
	return defaultActorTokensProviderImpl
}

// setConfig completely replaces the configuration in the context.
func setConfig(ctx context.Context, cfg *Config) context.Context {
	return context.WithValue(ctx, &cfgContextKey, cfg)
}

// getConfig returns the config stored in the context (or nil if not there).
func getConfig(ctx context.Context) *Config {
	val, _ := ctx.Value(&cfgContextKey).(*Config)
	return val
}

////////////////////////////////////////////////////////////////////////////////
// Helpers that extract stuff from the config.

// GetDB returns most recent snapshot of authorization database using DBProvider
// installed in the context via 'Initialize' or 'ModifyConfig'.
//
// If no factory is installed, returns DB that forbids everything and logs
// errors. It is often good enough for unit tests that do not care about
// authorization, and still not horribly bad if accidentally used in production.
func GetDB(ctx context.Context) (authdb.DB, error) {
	if cfg := getConfig(ctx); cfg != nil && cfg.DBProvider != nil {
		return cfg.DBProvider(ctx)
	}
	return authdb.ErroringDB{Error: ErrNotConfigured}, nil
}

// GetSigner returns the signing.Signer instance representing the service.
//
// It is injected into the context as part of Config. It can be used to grab
// a service account the service is running as or its current public
// certificates.
//
// Returns nil if the signer is not available.
func GetSigner(ctx context.Context) signing.Signer {
	if cfg := getConfig(ctx); cfg != nil {
		return cfg.Signer
	}
	return nil
}

// GetFrontendClientID returns an OAuth 2.0 client ID to use from the frontend.
//
// If it is not configured returns an empty string. May return an error if
// it could not be fetched. Such error should be treated as transient.
//
// This client ID is served via /auth/api/v1/server/client_id endpoint (so that
// the frontend can grab it), and the auth library permits access tokens with
// this client as an audience when authenticating requests.
func GetFrontendClientID(ctx context.Context) (string, error) {
	if cfg := getConfig(ctx); cfg != nil && cfg.FrontendClientID != nil {
		return cfg.FrontendClientID(ctx)
	}
	return "", nil
}

// GetFrontendOAuthScope returns the OAuth 2.0 scope to use from the frontend.
//
// If it is not configured returns an empty string. May return an error if
// it could not be fetched. Such error should be treated as transient.
func GetFrontendOAuthScope(ctx context.Context) ([]string, error) {
	if cfg := getConfig(ctx); cfg != nil && cfg.FrontendOAuthScopes != nil {
		return cfg.FrontendOAuthScopes(ctx)
	}
	return nil, nil
}
