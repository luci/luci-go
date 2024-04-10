// Copyright 2015 The LUCI Authors.
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

// Package internal contains code used internally by auth/integration.
package internal

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
)

// expiryRandInterval is used by TokenExpiresInRnd.
const expiryRandInterval = 30 * time.Second

const (
	// NoEmail indicates an OAuth2 token is not associated with an email.
	//
	// See Token below. We need this special value to distinguish "an email can
	// not possibly be fetched ever" from "the cached token doesn't have an email
	// yet" cases.
	NoEmail = "-"

	// UnknownEmail indicates an OAuth2 token may potentially be associated with
	// an email, but we haven't tried to fetch the email yet.
	UnknownEmail = ""

	// NoIDToken indicates it was impossible to obtain an ID token, e.g. no
	// "openid" scope in the refresh token or the provider doesn't support ID
	// tokens at all.
	NoIDToken = "-"

	// NoAccessToken indicates the access token was not returned by the provider.
	//
	// This can happen with providers that support only ID tokens.
	NoAccessToken = "-"
)

var (
	// ErrInsufficientAccess is returned by MintToken() if token can't be minted
	// for given OAuth scopes. For example, if GCE instance wasn't granted access
	// to requested scopes when it was created.
	ErrInsufficientAccess = errors.New("can't get access token for the given account and scopes")

	// ErrBadRefreshToken is returned by RefreshToken if refresh token was revoked
	// or otherwise invalid. It means MintToken must be used to get a new refresh
	// token.
	ErrBadRefreshToken = errors.New("refresh_token is not valid")

	// ErrBadCredentials is returned by MintToken or RefreshToken if provided
	// offline credentials (like service account key) are invalid.
	ErrBadCredentials = errors.New("invalid or unavailable service account credentials")
)

// Token is an oauth2.Token with an email and ID token that correspond to it.
//
// Email may be an empty string, in which case we assume the email hasn't been
// fetched yet. It can also be a special NoEmail string, which means the token
// is not associated with an email (happens for tokens without 'userinfo.email'
// scope).
type Token struct {
	oauth2.Token

	IDToken string // an ID token derived directly from the access token or NoIDToken
	Email   string // an email or NoEmail or empty string (aka UnknownEmail)
}

// TokenProvider knows how to mint new tokens or refresh existing ones.
type TokenProvider interface {
	// RequiresInteraction is true if provider may start user interaction
	// in MintToken.
	RequiresInteraction() bool

	// Lightweight is true if MintToken is very cheap to call.
	//
	// In this case the token is not being cached on disk (only in memory), since
	// it's easy to get a new one each time the process starts.
	//
	// By avoiding the disk cache, we reduce the chance of a leak.
	Lightweight() bool

	// Email is email associated with tokens produced by the provider, if known.
	//
	// May return UnknownEmail, which means the provider doesn't know the email
	// in advance and RefreshToken must be used to get the token and the email.
	// This happens, for example, for interactive providers before user has
	// logged in.
	//
	// It can also be NoEmail which means the email is not available, even if
	// caller is using RefreshToken.
	Email() string

	// CacheKey identifies a slot in the token cache to store the token in.
	//
	// Note: CacheKey MAY change during lifetime of a TokenProvider. It happens,
	// for example, for ServiceAccount token provider if the underlying service
	// account key is replaced while the process is still running.
	CacheKey(ctx context.Context) (*CacheKey, error)

	// MintToken launches authentication flow (possibly interactive) and returns
	// a new refreshable token (or error). It must never return (nil, nil).
	//
	// In actor mode 'base' is an IAM-scoped sufficiently fresh oauth token. It's
	// nil otherwise. Used by IAM-based token provider.
	MintToken(ctx context.Context, base *Token) (*Token, error)

	// RefreshToken takes existing token (probably expired, but not necessarily)
	// and returns a new refreshed token. It should never do any user interaction.
	// If a user interaction is required, a error should be returned instead.
	//
	// In actor mode 'base' is an IAM-scoped sufficiently fresh oauth token. It's
	// nil otherwise. Used by IAM-based token provider.
	RefreshToken(ctx context.Context, prev, base *Token) (*Token, error)
}

// TokenCache stores access and refresh tokens to avoid requesting them all
// the time.
type TokenCache interface {
	// GetToken reads the token from cache.
	//
	// Returns (nil, nil) if requested token is not in the cache.
	GetToken(key *CacheKey) (*Token, error)

	// PutToken writes the token to cache.
	PutToken(key *CacheKey, tok *Token) error

	// DeleteToken removes the token from cache.
	DeleteToken(key *CacheKey) error
}

// CacheKey identifies a slot in the token cache to store the token in.
type CacheKey struct {
	// Key identifies an auth method being used to get the token and its
	// parameters.
	//
	// Its exact form is not important, since it is used only for string matching
	// when searching for a token inside the cache.
	//
	// The following forms are being used currently:
	//  * user/<client_id> when using UserCredentialsMethod with some ClientID.
	//  * service_account/<email>/<key_id> when using ServiceAccountMethod.
	//  * gce/<account> when using GCEMetadataMethod.
	//  * iam/<account> when using IAM actor mode.
	//  * luci_ts/<account>/<host>/<realm> when using Token Server actor mode.
	//  * luci_ctx/<digest> when using LUCIContextMethod.
	Key string `json:"key"`

	// Scopes is the list of requested OAuth scopes or an ID token audience.
	//
	// The token audience is indicated by a fake scope that looks like
	// "audience:<value>". Cache keys are used only for map indexing, their exact
	// content doesn't matter. Adding a separate field (like `Audience`) to the
	// key causes complication with older binaries that read the token cache and
	// don't know about the new field, so we abuse `Scopes` field instead.
	Scopes []string `json:"scopes,omitempty"`
}

var bufPool = sync.Pool{}

// ToMapKey returns a string that can be used as map[string] key.
//
// This string IS NOT PRINTABLE. It's a merely a string-looking []byte.
func (k *CacheKey) ToMapKey() string {
	b, _ := bufPool.Get().(*bytes.Buffer)
	if b == nil {
		b = &bytes.Buffer{}
	} else {
		b.Reset()
	}
	defer bufPool.Put(b)
	b.WriteString(k.Key)
	b.WriteByte(0)
	for _, s := range k.Scopes {
		b.WriteString(s)
		b.WriteByte(0)
	}
	return b.String()
}

// Function equalCacheKeys returns true if keys are equal.
func equalCacheKeys(a, b *CacheKey) bool {
	return reflect.DeepEqual(a, b)
}

// TokenExpiresIn returns True if the token is not valid or expires within given
// duration.
//
// The function returns True in any of the following conditions:
//   - The token is not valid.
//   - The token expires before now+lifetime.
//
// In all other cases it returns False.
func TokenExpiresIn(ctx context.Context, t *Token, lifetime time.Duration) bool {
	if t == nil || t.AccessToken == "" {
		return true
	}
	if t.Expiry.IsZero() {
		return false
	}
	return t.Expiry.Round(0).Before(clock.Now(ctx).Add(lifetime))
}

// TokenExpiresInRnd is like TokenExpiresIn, except it slightly randomizes the
// token expiration time.
//
// If the function returns False, the token expires past now+lifetime. In other
// words, it is totally safe to use the token until now+lifetime. The inverse of
// this statement is not correct though: if the function returns True, it
// doesn't necessarily imply the token will expire before now+lifetime.
//
// The function returns True in any of the following conditions:
//   - The token is not valid.
//   - The token expires before now+lifetime.
//   - The token expiration time is between (now+lifetime, now+lifetime+rnd),
//     where rnd is a uniformly distributed random number between 0 and
//     expiryRandInterval sec (which is set to 30 sec).
//
// This is useful for processes that use multiple service account keys at
// around the same time. Without randomization, access tokens for such keys
// expire at the same time (strictly 1h after process startup, where 1h is
// the default token lifetime). This causes unnecessary contention on the token
// cache file.
func TokenExpiresInRnd(ctx context.Context, t *Token, lifetime time.Duration) bool {
	if t == nil || t.AccessToken == "" {
		return true
	}
	if t.Expiry.IsZero() {
		return false
	}
	expiry := t.Expiry.Round(0) // force to use wall clock time
	deadline := clock.Now(ctx).Add(lifetime)
	if expiry.Before(deadline) {
		// Definitely expires within 'lifetime'.
		return true
	}
	if expiry.After(deadline.Add(expiryRandInterval)) {
		// Definitely expires much later than 'lifetime', no need to involve RNG.
		return false
	}
	// Semi-randomly declare it as expired.
	rnd := time.Duration(mathrand.Int63n(ctx, int64(expiryRandInterval)))
	return expiry.Before(deadline.Add(rnd))
}

// EqualTokens returns true if tokens are equal.
//
// 'nil' token corresponds to an empty access token.
func EqualTokens(a, b *Token) bool {
	if a == b {
		return true
	}
	if a == nil {
		a = &Token{}
	}
	if b == nil {
		b = &Token{}
	}
	return a.AccessToken == b.AccessToken &&
		a.Expiry.Equal(b.Expiry) &&
		a.IDToken == b.IDToken &&
		a.Email == b.Email
}

// isBadTokenError sniffs out HTTP 400/401 from token source errors.
func isBadTokenError(err error) bool {
	if rerr, _ := err.(*oauth2.RetrieveError); rerr != nil {
		return rerr.Response.StatusCode == 400 || rerr.Response.StatusCode == 401
	}
	return false
}

// isBadKeyError sniffs out errors related to malformed private keys.
func isBadKeyError(err error) bool {
	if err == nil {
		return false
	}
	// See https://go.googlesource.com/oauth2.git/+/197281d4/internal/oauth2.go#32
	// Unfortunately, if uses fmt.Errorf.
	s := err.Error()
	return strings.Contains(s, "private key should be a PEM") ||
		s == "private key is invalid"
}

// grabToken uses token source to create a new *oauth2.Token.
//
// It recognizes transient errors.
func grabToken(src oauth2.TokenSource) (*oauth2.Token, error) {
	switch tok, err := src.Token(); {
	case isBadTokenError(err):
		return nil, err
	case isBadKeyError(err):
		return nil, err
	case err != nil:
		// More often than not errors here are transient (network connectivity
		// errors, HTTP 500 responses, etc). Retrying a fatal error a bunch of times
		// is not very bad, so pick safer approach and assume any error is
		// transient. Revoked refresh token or bad credentials (most common source
		// of fatal errors) is already handled above.
		return nil, transient.Tag.Apply(err)
	default:
		return tok, nil
	}
}
