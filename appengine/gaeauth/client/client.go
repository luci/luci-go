// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package client implements OAuth2 authentication for outbound connections
// from Appengine using service account keys. It supports native GAE service
// account credentials and external ones provided via JSON keys. It caches
// access tokens in memcache.
package client

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/gae/service/info"
	"github.com/luci/gae/service/memcache"
	"github.com/luci/gae/service/urlfetch"

	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/mathrand"
	"github.com/luci/luci-go/common/transport"
)

// cacheSchema defines version of memcache structures.
const cacheSchema = "gaeauth/v1/"

const (
	// minExpirationTime defines minimum acceptable lifetime of a token.
	minExpirationTime = time.Minute
	// maxExpirationTime defines maximum acceptable lifetime of a token.
	maxExpirationTime = 5 * time.Hour
	// expirationRandomization defines how much to randomize expiration time.
	expirationRandomization = 5 * time.Minute
)

// Authenticator returns an auth.Authenticator using GAE service account
// tokens minted for a given set of scopes, or 'email' scope if empty.
//
// If serviceAccountJSON is given, it must be a byte blob with service account
// JSON key to use instead of app's own account.
func Authenticator(c context.Context, scopes []string, serviceAccountJSON []byte) (*auth.Authenticator, error) {
	opts := auth.Options{
		Context:   c,
		Transport: urlfetch.Get(c),
		Scopes:    scopes,
		TokenCacheFactory: func(entryName string) (auth.TokenCache, error) {
			return tokenCache{c, cacheSchema + entryName}, nil
		},
	}
	if len(serviceAccountJSON) != 0 {
		opts.Method = auth.ServiceAccountMethod
		opts.ServiceAccountJSON = serviceAccountJSON
	} else {
		opts.Method = auth.CustomMethod
		opts.CustomTokenMinter = tokenMinter{c}
	}
	return auth.NewAuthenticator(auth.SilentLogin, opts), nil
}

// Transport returns http.RoundTripper that injects Authorization headers into
// requests. It uses an authenticator returned by Authenticator.
func Transport(c context.Context, scopes []string, serviceAccountJSON []byte) (http.RoundTripper, error) {
	a, err := Authenticator(c, scopes, serviceAccountJSON)
	if err != nil {
		return nil, err
	}
	return a.Transport()
}

// UseServiceAccountTransport injects authenticating transport into
// context.Context. It can be extracted back via transport.Get(c). It will be
// lazy-initialized on a first use.
func UseServiceAccountTransport(c context.Context, scopes []string, serviceAccountJSON []byte) context.Context {
	var cached http.RoundTripper
	var once sync.Once
	return transport.SetFactory(c, func(ic context.Context) http.RoundTripper {
		once.Do(func() {
			t, err := Transport(ic, scopes, serviceAccountJSON)
			if err != nil {
				cached = failTransport{err}
			} else {
				cached = t
			}
		})
		return cached
	})
}

// UseAnonymousTransport injects non-authenticating GAE transport into
// context.Context. It can be extracted back via transport.Get(c). Use it with
// libraries that search for transport in the context (e.g. common/config),
// since by default they revert to http.DefaultTransport that doesn't work in
// GAE environment.
func UseAnonymousTransport(c context.Context) context.Context {
	return transport.SetFactory(c, func(ic context.Context) http.RoundTripper {
		return urlfetch.Get(ic)
	})
}

type failTransport struct {
	err error
}

func (f failTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	// http.RoundTripper contract states: "RoundTrip should not modify the
	// request, except for consuming and closing the Body, including on errors"
	if r.Body != nil {
		_, _ = io.Copy(ioutil.Discard, r.Body)
		r.Body.Close()
	}
	return nil, f.err
}

//// Internal stuff.

// tokenMinter implements auth.TokenMinter on top of GAE API.
type tokenMinter struct {
	c context.Context
}

func (m tokenMinter) MintToken(scopes []string) (auth.Token, error) {
	tok, exp, err := info.Get(m.c).AccessToken(scopes...)
	if err != nil {
		return auth.Token{}, err
	}
	return auth.Token{AccessToken: tok, Expiry: exp}, nil
}

func (m tokenMinter) CacheSeed() []byte {
	return []byte(cacheSchema)
}

// tokenCache implement auth.TokenCache on top of memcache. Value in the cache
// is |<little endian int64 expiration time><byte blob with token>|.
type tokenCache struct {
	c   context.Context
	key string
}

func (c tokenCache) Read() ([]byte, error) {
	mem := memcache.Get(c.c)
	itm, err := mem.Get(c.key)
	if err == memcache.ErrCacheMiss {
		return nil, nil
	}
	if err != nil {
		return nil, errors.WrapTransient(err)
	}
	buf := bytes.NewReader(itm.Value())

	// Skip broken token (logging the error first).
	expTs := int64(0)
	if err = binary.Read(buf, binary.LittleEndian, &expTs); err != nil {
		logging.Warningf(c.c, "Could not deserialize cached token - %s", err)
		return nil, nil
	}

	// Randomize cache expiration time to avoid thundering herd effect when
	// token expires. Also start refreshing 1 min before real expiration time to
	// account for possible clock desync.
	lifetime := time.Unix(expTs, 0).Sub(clock.Now(c.c))
	rnd := mathrand.Get(c.c).Int31n(int32(expirationRandomization.Seconds()))
	if lifetime < minExpirationTime+time.Duration(rnd)*time.Second {
		return nil, nil
	}

	// Sanity check for broken entries. Lifetime should be bounded.
	if lifetime > maxExpirationTime {
		logging.Warningf(c.c, "Token lifetime is unexpectedly huge (%v), probably the token cache is broken", lifetime)
		return nil, nil
	}

	// Expiration time is ok. Read the actual token.
	return ioutil.ReadAll(buf)
}

func (c tokenCache) Write(tok []byte, expiry time.Time) error {
	lifetime := expiry.Sub(clock.Now(c.c))
	if lifetime < minExpirationTime {
		return fmt.Errorf("token expires too soon (%s sec), skipping cache", lifetime)
	}
	buf := bytes.NewBuffer(nil)
	binary.Write(buf, binary.LittleEndian, expiry.Unix())
	buf.Write(tok)
	mem := memcache.Get(c.c)
	err := mem.Set(mem.NewItem(c.key).SetValue(buf.Bytes()).SetExpiration(lifetime))
	return errors.WrapTransient(err)
}

func (c tokenCache) Clear() error {
	err := memcache.Get(c.c).Delete(c.key)
	if err != nil && err != memcache.ErrCacheMiss {
		return errors.WrapTransient(err)
	}
	return nil
}
