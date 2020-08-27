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

package casviewer

import (
	"context"
	"sync"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
)

// clientCacheKey is a context key for ClientCache.
var clientCacheKey = "client factory key"

// ClientCache caches CAS clients, one per an instance.
type ClientCache struct {
	lock    *sync.RWMutex
	clients map[string]*client.Client
}

// NewClientCache initializes ClientCache.
func NewClientCache() *ClientCache {
	return &ClientCache{
		lock:    new(sync.RWMutex),
		clients: make(map[string]*client.Client),
	}
}

// ForInstance returns a Client by loading it from cache or creating a new one.
func (cc *ClientCache) ForInstance(c context.Context, instance string) (*client.Client, error) {
	// Load Client from cache.
	if cl := cc.get(instance); cl != nil {
		return cl, nil
	}

	// Defer write lock until completing NewClient successfully.
	// Competitors may create clients for the same instance, but it will be ok.
	cl, err := NewClient(c, instance)
	if err != nil {
		return nil, err
	}

	// Cache the client.
	cc.set(instance, cl)
	return cl, nil
}

// Clear closes Clients gracefully, and removes them from cache.
func (cc *ClientCache) Clear() {
	cc.lock.Lock()
	defer cc.lock.Unlock()
	for inst, cl := range cc.clients {
		cl.Close()
		delete(cc.clients, inst)
	}
}

// get loads a cached Client for the instance with read lock.
func (cc *ClientCache) get(instance string) *client.Client {
	cc.lock.RLock()
	defer cc.lock.RUnlock()
	return cc.clients[instance]
}

// set caches the Client with read lock.
func (cc *ClientCache) set(instance string, cl *client.Client) {
	cc.lock.Lock()
	defer cc.lock.Unlock()
	cc.clients[instance] = cl
}

// withClientCacheMW creates a middleware that injects the ClientCache to context.
func withClientCacheMW(cc *ClientCache) router.Middleware {
	return func(c *router.Context, next router.Handler) {
		c.Context = context.WithValue(c.Context, &clientCacheKey, cc)
		next(c)
	}
}

// clientCache returns ClientCache by retrieving it from the context.
func clientCache(c context.Context) *ClientCache {
	cc, ok := c.Value(&clientCacheKey).(*ClientCache)
	if !ok {
		panic(errors.New("ClientCache not intalled in the context"))
	}
	return cc
}

// NewClient connects to the instance of remote execution service, and returns a client.
func NewClient(ctx context.Context, instance string) (*client.Client, error) {
	creds, err := auth.GetPerRPCCredentials(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return nil, errors.Annotate(err, "failed to get credentials").Err()
	}

	c, err := client.NewClient(ctx, instance,
		client.DialParams{
			Service:            "remotebuildexecution.googleapis.com:443",
			TransportCredsOnly: true,
		}, &client.PerRPCCreds{Creds: creds})
	if err != nil {
		return nil, errors.Annotate(err, "failed to create client").Err()
	}

	return c, nil
}
