// Copyright 2020 The LUCI Authors.
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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/trace"
)

// MintIDTokenParams is passed to MintIDTokenForServiceAccount.
type MintIDTokenParams struct {
	// ServiceAccount is an email of a service account to mint a token for.
	ServiceAccount string

	// Audience is a target audience of the token.
	Audience string

	// MinTTL defines an acceptable token lifetime.
	//
	// The returned token will be valid for at least MinTTL, but no longer than
	// one hour.
	//
	// Default is 2 min.
	MinTTL time.Duration
}

// actorIDTokenCache is used to store ID tokens of service accounts the current
// service has "iam.serviceAccountTokenCreator" role in.
//
// The underlying token type is IDToken string.
var actorIDTokenCache = newTokenCache(tokenCacheConfig{
	Kind:                         "as_actor_id_tok",
	Version:                      1,
	ProcessCacheCapacity:         8192,
	ExpiryRandomizationThreshold: 5 * time.Minute, // ~10% of regular 1h expiration
})

// MintIDTokenForServiceAccount produces an ID token for some service account
// that the current service has "iam.serviceAccountTokenCreator" role in.
//
// Used to implement AsActor authorization kind, but can also be used directly,
// if needed. The token is cached internally. Same token may be returned by
// multiple calls, if its lifetime allows.
//
// Recognizes transient errors and marks them, but does not automatically
// retry. Has internal timeout of 10 sec.
func MintIDTokenForServiceAccount(ctx context.Context, params MintIDTokenParams) (_ *Token, err error) {
	ctx, span := trace.StartSpan(ctx, "go.chromium.org/luci/server/auth.MintIDTokenForServiceAccount")
	span.Attribute("cr.dev/account", params.ServiceAccount)
	defer func() { span.End(err) }()

	report := durationReporter(ctx, mintIDTokenDuration)

	cfg := getConfig(ctx)
	if cfg == nil || cfg.AccessTokenProvider == nil {
		report(ErrNotConfigured, "ERROR_NOT_CONFIGURED")
		return nil, ErrNotConfigured
	}

	// Check required inputs. Also the cache key uses '\n' as a separator, make
	// sure inputs don't have it.
	if params.ServiceAccount == "" || params.Audience == "" ||
		strings.ContainsRune(params.ServiceAccount, '\n') || strings.ContainsRune(params.Audience, '\n') {
		err := fmt.Errorf("invalid parameters")
		report(err, "ERROR_BAD_ARGUMENTS")
		return nil, err
	}

	if params.MinTTL == 0 {
		params.MinTTL = 2 * time.Minute
	}

	ctx = logging.SetFields(ctx, logging.Fields{
		"token":    "actor",
		"account":  params.ServiceAccount,
		"audience": params.Audience,
	})

	cached, err, label := actorIDTokenCache.fetchOrMintToken(ctx, &fetchOrMintTokenOp{
		CacheKey:    params.ServiceAccount + "\n" + params.Audience,
		MinTTL:      params.MinTTL,
		MintTimeout: cfg.adjustedTimeout(10 * time.Second),

		// Mint is called on cache miss, under the lock.
		Mint: func(ctx context.Context) (t *cachedToken, err error, label string) {
			idToken, err := cfg.actorTokensProvider().GenerateIDToken(ctx, params.ServiceAccount, params.Audience)
			if err != nil {
				if transient.Tag.In(err) {
					return nil, err, "ERROR_TRANSIENT_IN_MINTING"
				}
				return nil, err, "ERROR_FATAL_IN_MINTING"
			}

			expiry, err := extractExpiryFromIDToken(idToken)
			if err != nil {
				return nil, fmt.Errorf("got malformed ID token: %s", err), "ERROR_MALFORMED_ID_TOKEN"
			}

			now := clock.Now(ctx).UTC()
			logging.Fields{
				"fingerprint": tokenFingerprint(idToken),
				"validity":    expiry.Sub(now),
			}.Debugf(ctx, "Minted new actor ID token")

			return &cachedToken{
				Created: now,
				Expiry:  expiry,
				IDToken: idToken,
			}, nil, "SUCCESS_CACHE_MISS"
		},
	})

	report(err, label)
	if err != nil {
		return nil, err
	}
	return &Token{
		Token:  cached.IDToken,
		Expiry: cached.Expiry,
	}, nil
}

// extractExpiryFromIDToken extracts token's expiration time by parsing JWT.
//
// Doesn't verify the token in any way, assuming it has come from a trusted
// place.
func extractExpiryFromIDToken(jwt string) (time.Time, error) {
	// Extract base64-encoded payload.
	chunks := strings.Split(jwt, ".")
	if len(chunks) != 3 {
		return time.Time{}, fmt.Errorf("bad JWT - expected 3 components separated by '.'")
	}
	payload := chunks[1]

	// Decode base64.
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return time.Time{}, fmt.Errorf("token payload is not base64 - %s", err)
	}

	// Parse just enough JSON to get the expiration Unix timestamp.
	var parsed struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return time.Time{}, fmt.Errorf("token payload is not JSON - %s", err)
	}

	// It must be set.
	if parsed.Exp == 0 {
		return time.Time{}, fmt.Errorf("the token has no `exp` field")
	}
	return time.Unix(parsed.Exp, 0).UTC(), nil
}
