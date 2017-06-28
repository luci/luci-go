// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package auth

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry"
	"github.com/luci/luci-go/common/retry/transient"
	"github.com/luci/luci-go/grpc/grpcutil"
	"github.com/luci/luci-go/grpc/prpc"
	"github.com/luci/luci-go/server/auth/delegation"
	"github.com/luci/luci-go/server/auth/delegation/messages"
	"github.com/luci/luci-go/server/auth/identity"
	"github.com/luci/luci-go/tokenserver/api/minter/v1"
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

	// ErrBadDelegationTokenTTL is returned by MintDelegationToken if requested
	// token lifetime is outside of the allowed range.
	ErrBadDelegationTokenTTL = fmt.Errorf("auth: requested delegation token TTL is invalid")
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
	// Such token can be sent to any supported LUCI service. Only whitelisted set
	// of callers have such superpower.
	//
	// If Untargeted is true, TargetHost is ignored.
	Untargeted bool

	// MinTTL defines an acceptable token lifetime.
	//
	// The returned token will be valid for at least MinTTL, but no longer than
	// MaxDelegationTokenTTL (which is 12h).
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

	// rpcClient is token server RPC client to use.
	//
	// Mocked in tests.
	rpcClient minter.TokenMinterClient
}

// delegationTokenCache is used to store delegation tokens in the cache.
//
// The underlying token type is delegation.Token.
var delegationTokenCache = tokenCache{
	Kind:                "delegation",
	Version:             3,
	ExpRandPercent:      10,
	MinAcceptedLifetime: 5 * time.Minute,
}

// MintDelegationToken returns a delegation token that can be used by the
// current service to "pretend" to be the current caller (as returned by
// CurrentIdentity(...)) when sending requests to some other LUCI service.
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
func MintDelegationToken(ctx context.Context, p DelegationTokenParams) (*delegation.Token, error) {
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
		report(ErrBadDelegationTokenTTL, "ERROR_BAD_TTL")
		return nil, ErrBadDelegationTokenTTL
	}

	// Config contains the cache implementation.
	cfg := getConfig(ctx)
	if cfg == nil || cfg.Cache == nil {
		report(ErrNotConfigured, "ERROR_NOT_CONFIGURED")
		return nil, ErrNotConfigured
	}

	// The state carries ID of the current user and URL of the token service.
	state := GetState(ctx)
	if state == nil {
		report(ErrNoAuthState, "ERROR_NO_AUTH_STATE")
		return nil, ErrNoAuthState
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

	cached, err, label := fetchOrMintToken(ctx, &fetchOrMintTokenOp{
		Config:   cfg,
		Cache:    &delegationTokenCache,
		CacheKey: string(userID) + "\n" + tokenServiceHost + "\n" + target,
		MinTTL:   p.MinTTL,

		// Mint is called on cache miss, under the lock.
		Mint: func(ctx context.Context) (*cachedToken, error, string) {
			// Grab a token server client (or its mock).
			rpcClient := p.rpcClient
			if rpcClient == nil {
				transport, err := GetRPCTransport(ctx, AsSelf)
				if err != nil {
					return nil, err, "ERROR_NO_TRANSPORT"
				}
				rpcClient = minter.NewTokenMinterPRPCClient(&prpc.Client{
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
				Token: delegation.Token{
					Token:  resp.Token,
					Expiry: exp,
				},
				Created: now,
				Expiry:  exp,
			}, nil, "SUCCESS_CACHE_MISS"
		},
	})

	if err != nil {
		report(err, label)
		return nil, err
	}

	t := cached.Token.(delegation.Token) // let it panic on type mismatch
	report(nil, label)
	return &t, nil
}
