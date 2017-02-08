// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package auth

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/auth/delegation"
	"github.com/luci/luci-go/server/auth/identity"
	"github.com/luci/luci-go/server/auth/signing"
)

var (
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
	MaxDelegationTokenTTL = 12 * time.Hour
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
func MintDelegationToken(ctx context.Context, p DelegationTokenParams) (tok *delegation.Token, err error) {
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

	// The state carries ID of the current user and URL of an auth service.
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

	// Grab URL of the main auth service we are bound to. It will be the one to
	// sign the token, and thus its identity is indirectly defines the identity
	// of the generated token.
	authServiceURL, err := state.DB().GetAuthServiceURL(ctx)
	if err != nil {
		report(err, "ERROR_AUTH_DB")
		return nil, err
	}

	// TODO(vadimsh): Cache tokens in the request state, so that multiple outbound
	// HTTP calls to some remote service during lifetime of inbound request don't
	// hit memcache all the time.

	// Try to find an existing cached token and check that it lives long enough.
	cacheKey := string(userID) + "\n" + authServiceURL + "\n" + target
	now := clock.Now(ctx).UTC()
	switch cached, err := delegationTokenCache.Fetch(ctx, cacheKey); {
	case err != nil:
		report(err, "ERROR_CACHE")
		return nil, err
	case cached != nil && cached.Expiry.After(now.Add(p.MinTTL)):
		t := cached.Token.(delegation.Token) // let it panic on type mismatch
		report(nil, "SUCCESS_CACHE_HIT")
		return &t, nil
	}

	// Minting a new token involves RPCs to remote services that should be fast.
	// Abort the attempt if it gets stuck for longer than 10 sec, it's unlikely
	// it'll succeed. Note that we setup the new context only on slow code path
	// (on cache miss), since it involves some overhead we don't want to pay on
	// the fast path. We assume memcache RPCs don't get stuck for a long time
	// (unlike URL Fetch calls to GAE).
	cfg := GetConfig(ctx)
	if cfg == nil || cfg.Signer == nil {
		report(ErrNotConfigured, "ERROR_NOT_CONFIGURED")
		return nil, ErrNotConfigured
	}
	var cancel context.CancelFunc
	ctx, cancel = clock.WithTimeout(ctx, cfg.adjustedTimeout(10*time.Second))
	defer cancel()

	// Need to make a new token. Log parameters and its ID.
	ctx = logging.SetFields(ctx, logging.Fields{
		"intent": p.Intent,
		"target": target,
		"userID": userID,
	})
	logging.Debugf(ctx, "Minting delegation token")
	defer func() {
		if err != nil {
			logging.WithError(err).Errorf(ctx, "Failed to mint delegation token")
		} else {
			logging.Fields{
				"subtokenID": tok.SubtokenID,
				"expiry":     tok.Expiry,
			}.Debugf(ctx, "Minted new delegation token")
		}
	}()

	// Grab ID of the currently running service, to bind the token to it.
	us, err := cfg.Signer.ServiceInfo(ctx)
	if err != nil {
		report(err, "ERROR_SERVICE_INFO")
		return nil, err
	}
	ourOwnID, err := identity.MakeIdentity("user:" + us.ServiceAccountName)
	if err != nil {
		report(err, "ERROR_BROKEN_IDENTITY")
		return nil, err
	}

	// Grab service ID of the endpoint we are calling to make the token usable
	// only by this service. Most of the time this will hit the local memory
	// cache. Sometimes it will make a request to the remote service. This will
	// fail if 'rootURI' is not pointing to a LUCI service. We use service IDs
	// to identify services (instead of URIs), since same service may be
	// accessible via multiple URIs (e.g. <version>-dot-<service>.appspot.com URIs
	// on Appengine).

	var targetServices []identity.Identity
	if target != "*" {
		serviceID, err := signing.FetchLUCIServiceIdentity(ctx, target)
		if err != nil {
			report(err, "ERROR_FETCH_TARGET_ID")
			return nil, err
		}
		targetServices = append(targetServices, serviceID)
	}

	// Request a new token from the auth service.
	tokenReq := delegation.TokenRequest{
		AuthServiceURL:   authServiceURL,
		Audience:         []identity.Identity{ourOwnID},
		Impersonate:      userID,
		ValidityDuration: MaxDelegationTokenTTL,
		Intent:           p.Intent,
	}
	if target == "*" {
		tokenReq.Untargeted = true
	} else {
		tokenReq.TargetServices = targetServices
	}
	tok, err = delegation.CreateToken(ctx, tokenReq)
	if err != nil {
		if errors.IsTransient(err) {
			report(err, "ERROR_TRANSIENT_IN_MINTING")
		} else {
			report(err, "ERROR_MINTING")
		}
		return nil, err
	}

	// Cache the token. Ignore errors here, it's not big deal, we have the token.
	err = delegationTokenCache.Store(ctx, cachedToken{
		Key:     cacheKey,
		Token:   *tok,
		Created: now,
		Expiry:  tok.Expiry,
	})
	if err != nil {
		logging.Errorf(ctx, "Failed to store delegation token in the cache - %s", err)
	}

	report(nil, "SUCCESS_CACHE_MISS")
	return tok, nil
}
