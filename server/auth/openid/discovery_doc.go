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

package openid

import (
	"context"
	"time"

	"go.chromium.org/luci/server/auth/internal"
	"go.chromium.org/luci/server/caching"
)

// GoogleDiscoveryURL is an URL of the Google OpenID Connect discovery document.
const GoogleDiscoveryURL = "https://accounts.google.com/.well-known/openid-configuration"

var (
	discoveryDocCache = caching.RegisterLRUCache(8) // URL string => *DiscoveryDoc
	signingKeysCache  = caching.RegisterLRUCache(8) // URL string => *JSONWebKeySet
)

// DiscoveryDoc describes a subset of OpenID Discovery JSON document.
//
// See https://developers.google.com/identity/protocols/OpenIDConnect#discovery.
type DiscoveryDoc struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	RevocationEndpoint    string `json:"revocation_endpoint"`
	JwksURI               string `json:"jwks_uri"`
}

// SigningKeys returns a JSON Web Key set fetched from the location specified
// in the discovery document.
//
// It fetches them on the first use and then keeps them cached in the process
// cache for 6h.
//
// May return both fatal and transient errors.
func (d *DiscoveryDoc) SigningKeys(ctx context.Context) (*JSONWebKeySet, error) {
	fetcher := func() (any, time.Duration, error) {
		raw := &JSONWebKeySetStruct{}
		req := internal.Request{
			Method: "GET",
			URL:    d.JwksURI,
			Out:    raw,
		}
		if err := req.Do(ctx); err != nil {
			return nil, 0, err
		}
		keys, err := NewJSONWebKeySet(raw)
		if err != nil {
			return nil, 0, err
		}
		return keys, time.Hour * 6, nil
	}

	cached, err := signingKeysCache.LRU(ctx).GetOrCreate(ctx, d.JwksURI, fetcher)
	if err != nil {
		return nil, err
	}
	return cached.(*JSONWebKeySet), nil
}

// FetchDiscoveryDoc fetches the discovery document from the given URL.
//
// It is cached in the process cache for 24 hours.
func FetchDiscoveryDoc(ctx context.Context, url string) (*DiscoveryDoc, error) {
	fetcher := func() (any, time.Duration, error) {
		doc := &DiscoveryDoc{}
		req := internal.Request{
			Method: "GET",
			URL:    url,
			Out:    doc,
		}
		if err := req.Do(ctx); err != nil {
			return nil, 0, err
		}
		return doc, time.Hour * 24, nil
	}

	// Cache the document in the process cache.
	cached, err := discoveryDocCache.LRU(ctx).GetOrCreate(ctx, url, fetcher)
	if err != nil {
		return nil, err
	}
	return cached.(*DiscoveryDoc), nil
}
