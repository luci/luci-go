// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package internal contains code used internally by common/auth.
package internal

import (
	"bytes"
	"reflect"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/data/rand/mathrand"
	"github.com/luci/luci-go/common/errors"
)

// expiryRandInterval is used by TokenExpiresInRnd.
const expiryRandInterval = 30 * time.Second

var (
	// ErrInsufficientAccess is returned by MintToken() if token can't be minted
	// for given OAuth scopes. For example, if GCE instance wasn't granted access
	// to requested scopes when it was created.
	ErrInsufficientAccess = errors.New("can't get access token for given scopes")

	// ErrBadRefreshToken is returned by RefreshToken if refresh token was revoked
	// or otherwise invalid. It means MintToken must be used to get a new refresh
	// token.
	ErrBadRefreshToken = errors.New("refresh_token is not valid")

	// ErrBadCredentials is returned by MintToken or RefreshToken if provided
	// offline credentials (like service account key) are invalid.
	ErrBadCredentials = errors.New("invalid service account credentials")
)

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
	MintToken(ctx context.Context, base *oauth2.Token) (*oauth2.Token, error)

	// RefreshToken takes existing token (probably expired, but not necessarily)
	// and returns a new refreshed token. It should never do any user interaction.
	// If a user interaction is required, a error should be returned instead.
	//
	// In actor mode 'base' is an IAM-scoped sufficiently fresh oauth token. It's
	// nil otherwise. Used by IAM-based token provider.
	RefreshToken(ctx context.Context, prev, base *oauth2.Token) (*oauth2.Token, error)
}

// TokenCache stores access and refresh tokens to avoid requesting them all
// the time.
type TokenCache interface {
	// GetToken reads the token from cache.
	//
	// Returns (nil, nil) if requested token is not in the cache.
	GetToken(key *CacheKey) (*oauth2.Token, error)

	// PutToken writes the token to cache.
	PutToken(key *CacheKey, tok *oauth2.Token) error

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
	//  * iam/<account> when using actor mode with ActAsServiceAccount != "".
	Key string `json:"key"`

	// Scopes is the list of requested OAuth scopes.
	Scopes []string `json:"scopes"`
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

// EqualCacheKeys returns true if keys are equal.
func EqualCacheKeys(a, b *CacheKey) bool {
	return reflect.DeepEqual(a, b)
}

// TokenExpiresIn returns True if the token is not valid or expires within given
// duration.
//
// The function returns True in any of the following conditions:
//  * The token is not valid.
//  * The token expires before now+lifetime.
//
// In all other cases it returns False.
func TokenExpiresIn(ctx context.Context, t *oauth2.Token, lifetime time.Duration) bool {
	if t == nil || t.AccessToken == "" {
		return true
	}
	if t.Expiry.IsZero() {
		return false
	}
	return t.Expiry.Before(clock.Now(ctx).Add(lifetime))
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
//  * The token is not valid.
//  * The token expires before now+lifetime.
//  * The token expiration time is between (now+lifetime, now+lifetime+rnd),
//    where rnd is a uniformly distributed random number between 0 and
//    expiryRandInterval sec (which is set to 30 sec).
//
// This is useful for processes that use multiple service account keys at
// around the same time. Without randomization, access tokens for such keys
// expire at the same time (strictly 1h after process startup, where 1h is
// the default token lifetime). This causes unnecessary contention on the token
// cache file.
func TokenExpiresInRnd(ctx context.Context, t *oauth2.Token, lifetime time.Duration) bool {
	if t == nil || t.AccessToken == "" {
		return true
	}
	if t.Expiry.IsZero() {
		return false
	}
	deadline := clock.Now(ctx).Add(lifetime)
	if t.Expiry.Before(deadline) {
		// Definitely expires within 'lifetime'.
		return true
	}
	if t.Expiry.After(deadline.Add(expiryRandInterval)) {
		// Definitely expires much later than 'lifetime', no need to involve RNG.
		return false
	}
	// Semi-randomly declare it as expired.
	rnd := time.Duration(mathrand.Int63n(ctx, int64(expiryRandInterval)))
	return t.Expiry.Before(deadline.Add(rnd))
}

// EqualTokens returns true if both token object have same access token.
//
// 'nil' token corresponds to an empty access token.
func EqualTokens(a, b *oauth2.Token) bool {
	if a == b {
		return true
	}
	aTok := ""
	if a != nil {
		aTok = a.AccessToken
	}
	bTok := ""
	if b != nil {
		bTok = b.AccessToken
	}
	return aTok == bTok
}

// isBadTokenError sniffs out HTTP 400/401 from token source errors.
func isBadTokenError(err error) bool {
	// See https://github.com/golang/oauth2/blob/master/internal/token.go.
	// Unfortunately, fmt.Errorf is used there, so there's no other way to
	// differentiate between bad tokens/creds and transient errors.
	// Note that mis-categorization is not a big deal: we'll unnecessarily retry
	// fatal error a bunch of times, thinking it is transient.
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "400 bad request") || strings.Contains(s, "401 unauthorized")
}

// isBadKeyError sniffs out errors related to malformed private keys.
func isBadKeyError(err error) bool {
	// See https://github.com/golang/oauth2/blob/master/internal/oauth2.go.
	// Unfortunately, if also uses fmt.Errorf.
	// Note that mis-categorization is not a big deal: we'll unnecessarily retry
	// fatal error a bunch of times, thinking it is transient.
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "private key should be a PEM or plain PKSC1 or PKCS8") || s == "private key is invalid"
}

// grabToken uses token source to create a new token.
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
		// errors, HTTP 500 responses, etc). It is difficult to categorize them,
		// since oauth2 library uses fmt.Errorf(...) for errors. Retrying a fatal
		// error a bunch of times is not very bad, so pick safer approach and assume
		// any error is transient. Revoked refresh token or bad credentials (most
		// common source of fatal errors) is already handled above.
		return nil, errors.WrapTransient(err)
	default:
		return tok, nil
	}
}
