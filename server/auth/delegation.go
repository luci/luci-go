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
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/tokenserver/api/minter/v1"

	"go.chromium.org/luci/server/auth/delegation/messages"
	"go.chromium.org/luci/server/auth/internal/tracing"
)

var (
	// ErrTokenServiceNotConfigured is returned by MintDelegationToken if the
	// token service URL is not configured. This usually means the corresponding
	// auth service is not paired with a token server.
	ErrTokenServiceNotConfigured = fmt.Errorf("auth: token service URL is not configured")

	// ErrBrokenTokenService is returned by MintDelegationToken if the RPC to the
	// token service succeeded, but response doesn't make sense. This should not
	// generally happen.
	ErrBrokenTokenService = fmt.Errorf("auth: unrecognized response from the token service")

	// ErrAnonymousDelegation is returned by MintDelegationToken if it is used in
	// a context of handling of an anonymous call.
	//
	// There's no identity to delegate in this case.
	ErrAnonymousDelegation = fmt.Errorf("auth: can't get delegation token for anonymous user")

	// ErrBadTargetHost is returned by MintDelegationToken if it receives invalid
	// TargetHost parameter.
	ErrBadTargetHost = fmt.Errorf("auth: invalid TargetHost (doesn't look like a hostname:port pair)")

	// ErrBadTokenTTL is returned by MintDelegationToken and MintProjectToken if requested
	// token lifetime is outside of the allowed range.
	ErrBadTokenTTL = fmt.Errorf("auth: requested token TTL is invalid")

	// ErrBadDelegationTag is returned by MintDelegationToken if some of the
	// passed tags are malformed.
	ErrBadDelegationTag = fmt.Errorf("auth: provided delegation tags are invalid")
)

const (
	// MaxDelegationTokenTTL is maximum allowed token lifetime that can be
	// requested via MintDelegationToken.
	MaxDelegationTokenTTL = 3 * time.Hour
)

// DelegationTokenParams is passed to MintDelegationToken.
type DelegationTokenParams struct {
	// TargetHost, if given, is hostname (with, possibly, ":port") of a service
	// that the token will be sent to.
	//
	// If this parameter is used, the resulting delegation token is scoped
	// only to the service at TargetHost. All other services will reject it.
	//
	// Must be set if Untargeted is false. Ignored if Untargeted is true.
	TargetHost string

	// Untargeted, if true, indicates that the caller is requesting a token that
	// is not scoped to any particular service.
	//
	// Such token can be sent to any supported LUCI service. Only allowlisted set
	// of callers have such superpower.
	//
	// If Untargeted is true, TargetHost is ignored.
	Untargeted bool

	// MinTTL defines an acceptable token lifetime.
	//
	// The returned token will be valid for at least MinTTL, but no longer than
	// MaxDelegationTokenTTL (which is 3h).
	//
	// Default is 10 min.
	MinTTL time.Duration

	// Intent is a reason why the token is created.
	//
	// Used only for logging purposes on the auth service, will be indexed. Should
	// be a short identifier-like string.
	//
	// Optional.
	Intent string

	// Tags are optional arbitrary key:value pairs embedded into the token.
	//
	// They convey circumstance of why the token is created.
	//
	// Services that accept the token may use them for additional authorization
	// decisions. Please use extremely carefully, only when you control both sides
	// of the delegation link and can guarantee that services involved understand
	// the tags.
	Tags []string

	// rpcClient is token server RPC client to use.
	//
	// Mocked in tests.
	rpcClient delegationTokenMinterClient
}

// delegationTokenMinterClient is subset of minter.TokenMinterClient we use.
type delegationTokenMinterClient interface {
	MintDelegationToken(context.Context, *minter.MintDelegationTokenRequest, ...grpc.CallOption) (*minter.MintDelegationTokenResponse, error)
}

// delegationTokenCache is used to store delegation tokens in the cache.
//
// The token is stored in DelegationToken field.
var delegationTokenCache = newTokenCache(tokenCacheConfig{
	Kind:                         "delegation",
	Version:                      7,
	ProcessCacheCapacity:         8192,
	ExpiryRandomizationThreshold: MaxDelegationTokenTTL / 10, // 10%
})

// MintDelegationToken returns a delegation token that can be used by the
// current service to "pretend" to be the current caller (as returned by
// CurrentIdentity(...)) when sending requests to some other LUCI service.
//
// DEPRECATED.
//
// The delegation token is essentially a signed assertion that the current
// service is allowed to access some other service on behalf of the current
// user.
//
// A token can be targeted to some single specific service or usable by any
// allowed LUCI service (aka 'untargeted'). See TargetHost and Untargeted
// fields in DelegationTokenParams.
//
// The token is cached internally. Same token may be returned by multiple calls,
// if its lifetime allows.
func MintDelegationToken(ctx context.Context, p DelegationTokenParams) (_ *Token, err error) {
	ctx, span := tracing.Start(ctx, "go.chromium.org/luci/server/auth.MintDelegationToken",
		attribute.String("cr.dev.target", p.TargetHost),
	)
	defer func() { tracing.End(span, err) }()

	report := durationReporter(ctx, mintDelegationTokenDuration)

	// Validate TargetHost.
	target := ""
	if p.Untargeted {
		target = "*"
	} else {
		p.TargetHost = strings.ToLower(p.TargetHost)
		if strings.IndexRune(p.TargetHost, '/') != -1 {
			report(ErrBadTargetHost, "ERROR_BAD_HOST")
			return nil, ErrBadTargetHost
		}
		target = "https://" + p.TargetHost
	}

	// Validate TTL is sane.
	if p.MinTTL == 0 {
		p.MinTTL = 10 * time.Minute
	}
	if p.MinTTL < 30*time.Second || p.MinTTL > MaxDelegationTokenTTL {
		report(ErrBadTokenTTL, "ERROR_BAD_TTL")
		return nil, ErrBadTokenTTL
	}

	// Validate tags are sane, sort them. Don't be very pedantic with validation,
	// the server will apply its more precise validation rules anyway.
	tags := append([]string(nil), p.Tags...)
	for _, t := range tags {
		parts := strings.SplitN(t, ":", 2)
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			report(ErrBadDelegationTag, "ERROR_BAD_TAG")
			return nil, ErrBadDelegationTag
		}
	}
	sort.Strings(tags)

	// The state carries ID of the current user and URL of the token service.
	state := GetState(ctx)
	if state == nil {
		report(ErrNotConfigured, "ERROR_NOT_CONFIGURED")
		return nil, ErrNotConfigured
	}

	// Identity we want to impersonate.
	userID := state.User().Identity
	if userID == identity.AnonymousIdentity {
		report(ErrAnonymousDelegation, "ERROR_NO_IDENTITY")
		return nil, ErrAnonymousDelegation
	}

	// Grab hostname of the token service we received from the auth service. It
	// will sign the token, and thus its identity is indirectly defines the
	// identity of the generated token. For that reason we use it as part of the
	// cache key.
	tokenServiceURL, err := state.DB().GetTokenServiceURL(ctx)
	switch {
	case err != nil:
		report(err, "ERROR_AUTH_DB")
		return nil, err
	case tokenServiceURL == "":
		report(ErrTokenServiceNotConfigured, "ERROR_NO_TOKEN_SERVICE")
		return nil, ErrTokenServiceNotConfigured
	case !strings.HasPrefix(tokenServiceURL, "https://"):
		// Note: this never actually happens.
		logging.Errorf(ctx, "Bad token service URL: %s", tokenServiceURL)
		report(ErrTokenServiceNotConfigured, "ERROR_NOT_HTTPS_TOKEN_SERVICE")
		return nil, ErrTokenServiceNotConfigured
	}
	tokenServiceHost := tokenServiceURL[len("https://"):]

	ctx = logging.SetFields(ctx, logging.Fields{
		"token":  "delegation",
		"target": target,
		"userID": userID,
	})

	cacheKey := fmt.Sprintf("%s\n%s\n%s\n%d\n%s",
		userID, tokenServiceHost, target, len(tags), strings.Join(tags, "\n"))

	cached, err, label := delegationTokenCache.fetchOrMintToken(ctx, &fetchOrMintTokenOp{
		CacheKey:    cacheKey,
		MinTTL:      p.MinTTL,
		MintTimeout: getConfig(ctx).adjustedTimeout(10 * time.Second),

		// Mint is called on cache miss, under the lock.
		Mint: func(ctx context.Context) (t *cachedToken, err error, label string) {
			// Grab a token server client (or its mock).
			rpcClient := p.rpcClient
			if rpcClient == nil {
				transport, err := GetRPCTransport(ctx, AsSelf)
				if err != nil {
					return nil, err, "ERROR_NO_TRANSPORT"
				}
				rpcClient = minter.NewTokenMinterClient(&prpc.Client{
					C:    &http.Client{Transport: transport},
					Host: tokenServiceHost,
					Options: &prpc.Options{
						Retry: func() retry.Iterator {
							return &retry.ExponentialBackoff{
								Limited: retry.Limited{
									Delay:   50 * time.Millisecond,
									Retries: 5,
								},
							}
						},
					},
				})
			}

			// The actual RPC call.
			resp, err := rpcClient.MintDelegationToken(ctx, &minter.MintDelegationTokenRequest{
				DelegatedIdentity: string(userID),
				ValidityDuration:  int64(MaxDelegationTokenTTL.Seconds()),
				Audience:          []string{"REQUESTOR"}, // make the token usable only by the calling service
				Services:          []string{target},
				Intent:            p.Intent,
				Tags:              tags,
			})
			if err != nil {
				err = grpcutil.WrapIfTransient(err)
				if transient.Tag.In(err) {
					return nil, err, "ERROR_TRANSIENT_IN_MINTING"
				}
				return nil, err, "ERROR_MINTING"
			}

			// Sanity checks. A correctly working token server should not trigger them.
			subtoken := resp.DelegationSubtoken
			good := false
			switch {
			case subtoken == nil:
				logging.Errorf(ctx, "No delegation_subtoken in the response")
			case subtoken.Kind != messages.Subtoken_BEARER_DELEGATION_TOKEN:
				logging.Errorf(ctx, "Invalid token kind: %s", subtoken.Kind)
			case subtoken.ValidityDuration <= 0:
				logging.Errorf(ctx, "Zero or negative validity_duration in the response")
			default:
				good = true
			}
			if !good {
				return nil, ErrBrokenTokenService, "ERROR_BROKEN_TOKEN_SERVICE"
			}

			// Log details about the new token.
			logging.Fields{
				"fingerprint": tokenFingerprint(resp.Token),
				"subtokenID":  subtoken.SubtokenId,
				"validity":    time.Duration(subtoken.ValidityDuration) * time.Second,
			}.Debugf(ctx, "Minted new delegation token")

			now := clock.Now(ctx).UTC()
			exp := now.Add(time.Duration(subtoken.ValidityDuration) * time.Second)
			return &cachedToken{
				Created:         now,
				Expiry:          exp,
				DelegationToken: resp.Token,
			}, nil, "SUCCESS_CACHE_MISS"
		},
	})

	report(err, label)
	if err != nil {
		return nil, err
	}
	return &Token{
		Token:  cached.DelegationToken,
		Expiry: cached.Expiry,
	}, nil
}
