// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package gaeauth implements OAuth2 authentication for outbound connections
// from Appengine using service account keys. It supports native GAE service
// account credentials and external ones provided via JSON keys. It caches
// access tokens in memcache.
package gaeauth

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	"github.com/luci/gae/service/memcache"
	"github.com/luci/gae/service/urlfetch"

	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/mathrand"
)

// cacheSchema defines version of memcache structures.
const cacheSchema = "gaeauth/v1/"

const (
	// minExpirationTime defines minimum acceptable lifetime of a token.
	minExpirationTime = time.Minute
	// expirationRandomization defines how much to randomize expiration time.
	expirationRandomization = 5 * time.Minute
)

// Transport returns http.RoundTripper that injects Authorization headers into
// requests. It uses GAE service account tokens minted for given set of scopes,
// or 'email' scope if empty. If serviceAccountJSON is given, it must be a
// byte blob with service account JSON key to use instead of app's own account.
// If it is not given, and running on devserver, it will be read from
// the datastore via FetchServiceAccountJSON(c, "devserver").
//
// Prefer to use Transport(...) lazily (e.g. only when really going to send
// the request), since it does a bunch of datastore and memcache reads to
// initialize http.RoundTripper.
func Transport(c context.Context, scopes []string, serviceAccountJSON []byte) (http.RoundTripper, error) {
	if len(serviceAccountJSON) == 0 && info.Get(c).IsDevAppServer() {
		var err error
		if serviceAccountJSON, err = FetchServiceAccountJSON(c, "devserver"); err != nil {
			return nil, err
		}
		if len(serviceAccountJSON) == 0 {
			// Make an empty stub for user to fill in via dev server admin console.
			StoreServiceAccountJSON(c, "devserver", nil)
		}
	}
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
	return auth.NewAuthenticator(opts).Transport()
}

// FetchServiceAccountJSON returns service account JSON key blob by reading it
// from the datastore.
func FetchServiceAccountJSON(c context.Context, id string) ([]byte, error) {
	ent := serviceAccount{ID: id}
	err := datastore.Get(c).Get(&ent)
	if err == datastore.ErrNoSuchEntity {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return []byte(ent.ServiceAccount), nil
}

// StoreServiceAccountJSON puts service account JSON key blob in the datastore.
func StoreServiceAccountJSON(c context.Context, id string, blob []byte) error {
	// Store as a string so the entity is editable via devserver admin console.
	return datastore.Get(c).Put(&serviceAccount{
		ID:             id,
		ServiceAccount: string(blob),
	})
}

//// Internal stuff.

// serviceAccount is entity that stores service account JSON key.
type serviceAccount struct {
	_kind string `gae:"$kind,AuthServiceAccount"`

	ID             string `gae:"$id"`
	ServiceAccount string `gae:",noindex"`
}

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
	itm := mem.NewItem(c.key)
	err := mem.Get(itm)
	if err == memcache.ErrCacheMiss {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	buf := bytes.NewReader(itm.Value())

	// Randomize cache expiration time to avoid thundering herd effect when
	// token expires. Also start refreshing 1 min before real expiration time to
	// account for possible clock desync.
	expTs := int64(0)
	if err = binary.Read(buf, binary.LittleEndian, &expTs); err != nil {
		return nil, err
	}
	lifetime := time.Unix(expTs, 0).Sub(clock.Now(c.c))
	rnd := mathrand.Get(c.c).Int31n(int32(expirationRandomization / time.Second))
	if lifetime < minExpirationTime+time.Duration(rnd)*time.Second {
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
	return mem.Set(mem.NewItem(c.key).SetValue(buf.Bytes()).SetExpiration(lifetime))
}

func (c tokenCache) Clear() error {
	err := memcache.Get(c.c).Delete(c.key)
	if err != nil && err != memcache.ErrCacheMiss {
		return err
	}
	return nil
}
