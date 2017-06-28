// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package client implements OAuth2 authentication for outbound connections
// from Appengine using the application services account.
package client

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"

	"github.com/luci/gae/service/info"

	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/data/caching/proccache"
	"github.com/luci/luci-go/common/data/rand/mathrand"
	"github.com/luci/luci-go/common/data/stringset"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry/transient"
)

// GetAccessToken returns an OAuth access token representing app's service
// account.
//
// If scopes is empty, uses auth.OAuthScopeEmail scope.
//
// Implements a caching layer on top of GAE's GetAccessToken RPC. May return
// transient errors.
func GetAccessToken(c context.Context, scopes []string) (*oauth2.Token, error) {
	scopes, cacheKey := normalizeScopes(scopes)

	// Try to find the token in the local memory first. If it expires soon,
	// refresh it earlier with some probability. That avoids a situation when
	// parallel requests that use access tokens suddenly see the cache expired
	// and rush to refresh the token all at once.
	pcache := proccache.GetCache(c)
	if entry := pcache.Get(cacheKey); entry != nil && !closeToExpRandomized(c, entry.Exp) {
		return entry.Value.(*oauth2.Token), nil
	}

	// The token needs to be refreshed.
	logging.Debugf(c, "Getting an access token for scopes %q", strings.Join(scopes, ", "))
	accessToken, exp, err := info.AccessToken(c, scopes...)
	if err != nil {
		return nil, transient.Tag.Apply(err)
	}
	logging.Debugf(c, "The token expires in %s", exp.Sub(clock.Now(c)))

	// Prematurely expire it to guarantee all returned token live for at least
	// 'expirationMinLifetime'.
	tok := &oauth2.Token{
		AccessToken: accessToken,
		Expiry:      exp.Add(-expirationMinLifetime),
		TokenType:   "Bearer",
	}

	// Store the new token in the cache (overriding what's already there).
	pcache.Put(cacheKey, tok, tok.Expiry)

	return tok, nil
}

// NewTokenSource makes oauth2.TokenSource implemented on top of GetAccessToken.
//
// It is bound to the given context.
func NewTokenSource(ctx context.Context, scopes []string) oauth2.TokenSource {
	return &tokenSource{ctx, scopes}
}

type tokenSource struct {
	ctx    context.Context
	scopes []string
}

func (ts *tokenSource) Token() (*oauth2.Token, error) {
	return GetAccessToken(ts.ctx, ts.scopes)
}

//// Internal stuff.

type cacheKey string

const (
	// expirationMinLifetime is minimal possible lifetime of a returned token.
	expirationMinLifetime = 2 * time.Minute
	// expirationRandomization defines how much to randomize expiration time.
	expirationRandomization = 3 * time.Minute
)

func normalizeScopes(scopes []string) ([]string, cacheKey) {
	if len(scopes) == 0 {
		scopes = []string{auth.OAuthScopeEmail}
	} else {
		set := stringset.New(len(scopes))
		for _, s := range scopes {
			if strings.ContainsRune(s, '\n') {
				panic(fmt.Errorf("invalid scope %q", s))
			}
			set.Add(s)
		}
		scopes = set.ToSlice()
		sort.Strings(scopes)
	}
	return scopes, cacheKey(strings.Join(scopes, "\n"))
}

func closeToExpRandomized(c context.Context, exp time.Time) bool {
	switch now := clock.Now(c); {
	case now.After(exp):
		return true // expired already
	case now.Add(expirationRandomization).Before(exp):
		return false // far from expiration
	default:
		// The expiration is close enough. Do the randomization.
		rnd := time.Duration(mathrand.Int63n(c, int64(expirationRandomization)))
		return now.Add(rnd).After(exp)
	}
}
