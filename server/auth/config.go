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
	"net/http"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"

	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/signing"
)

// DBProvider is a callback that returns most recent DB instance.
//
// DB represents a snapshot of user groups used for authorization checks.
type DBProvider func(c context.Context) (authdb.DB, error)

// AccessTokenProvider knows how to generate OAuth2 access token for the
// service account belonging to the server itself.
type AccessTokenProvider func(c context.Context, scopes []string) (*oauth2.Token, error)

// AnonymousTransportProvider returns http.RoundTriper that can make
// unauthenticated HTTP requests.
//
// The returned round tripper is assumed to be bound to the context and won't
// outlive it.
type AnonymousTransportProvider func(c context.Context) http.RoundTripper

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

	// AccessTokenProvider knows how to generate OAuth2 access token for the
	// service account belonging to the server itself.
	AccessTokenProvider AccessTokenProvider

	// AnonymousTransport returns http.RoundTriper that can make unauthenticated
	// HTTP requests.
	//
	// The returned round tripper is assumed to be bound to the context and won't
	// outlive it.
	AnonymousTransport AnonymousTransportProvider

	// Cache implements a strongly consistent cache.
	//
	// Usually backed by memcache. Should do namespacing itself (i.e. the auth
	// library assumes full ownership of the keyspace).
	Cache Cache

	// IsDevMode is true when running the server locally during development.
	//
	// Setting this to true changes default deadlines. For instance, GAE dev
	// server is known to be very slow and deadlines tuned for production
	// environment are too limiting.
	IsDevMode bool
}

// ModifyConfig makes a context with a derived configuration.
//
// It grabs current configuration from the context (if any), passes it to the
// callback, and puts whatever callback returns into a derived context.
func ModifyConfig(c context.Context, cb func(Config) Config) context.Context {
	var cfg Config
	if cur := getConfig(c); cur != nil {
		cfg = *cur
	}
	cfg = cb(cfg)
	return setConfig(c, &cfg)
}

func (cfg *Config) anonymousClient(ctx context.Context) *http.Client {
	return &http.Client{Transport: cfg.AnonymousTransport(ctx)}
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

var cfgContextKey = "auth.Config context key"

// setConfig replaces the configuration in the context.
func setConfig(c context.Context, cfg *Config) context.Context {
	return context.WithValue(c, &cfgContextKey, cfg)
}

// getConfig returns the config stored in the context (or nil if not there).
func getConfig(c context.Context) *Config {
	val, _ := c.Value(&cfgContextKey).(*Config)
	return val
}

////////////////////////////////////////////////////////////////////////////////
// Helpers that extract stuff from the config.

// GetDB returns most recent snapshot of authorization database using DBProvider
// installed in the context via 'ModifyConfig'.
//
// If no factory is installed, returns DB that forbids everything and logs
// errors. It is often good enough for unit tests that do not care about
// authorization, and still not horribly bad if accidentally used in production.
func GetDB(c context.Context) (authdb.DB, error) {
	if cfg := getConfig(c); cfg != nil && cfg.DBProvider != nil {
		return cfg.DBProvider(c)
	}
	return authdb.ErroringDB{Error: ErrNotConfigured}, nil
}
