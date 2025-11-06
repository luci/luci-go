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
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/client/casclient"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
)

// clientCacheKey is a context key type for ClientCache value.
type clientCacheKey struct{}

// ccKey is a context key for ClientCache value.
var ccKey = &clientCacheKey{}

// ClientCache caches CAS clients, one per an instance.
type ClientCache struct {
	lock    sync.RWMutex
	clients map[string]*client.Client
	ctx     context.Context
}

// NewClientCache initializes ClientCache.
//
// The given context will be used to create gRPC clients and their per-RPC
// credentials.
func NewClientCache(ctx context.Context) *ClientCache {
	return &ClientCache{
		clients: make(map[string]*client.Client),
		ctx:     ctx,
	}
}

// Get returns a Client by loading it from cache or creating a new one.
func (cc *ClientCache) Get(instance string) (*client.Client, error) {
	// Load Client from cache.
	cc.lock.RLock()
	cl, ok := cc.clients[instance]
	cc.lock.RUnlock()

	if ok {
		return cl, nil
	}

	cc.lock.Lock()
	defer cc.lock.Unlock()

	// Somebody may have already set a client for the same instance.
	cl, ok = cc.clients[instance]
	if ok {
		return cl, nil
	}

	cl, err := newClient(cc.ctx, instance)
	if err != nil {
		return nil, err
	}

	// Cache the client.
	cc.clients[instance] = cl
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

// withClientCacheMW creates a middleware that injects the ClientCache into
// the context.
func withClientCacheMW(cc *ClientCache) router.Middleware {
	return func(c *router.Context, next router.Handler) {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ccKey, cc))
		next(c)
	}
}

// GetClient returns a Client by loading it from cache or creating a new one.
func GetClient(ctx context.Context, instance string) (*client.Client, error) {
	cc, err := clientCache(ctx)
	if err != nil {
		return nil, err
	}
	return cc.Get(instance)
}

// clientCache returns ClientCache by retrieving it from the context.
func clientCache(c context.Context) (*ClientCache, error) {
	cc, ok := c.Value(ccKey).(*ClientCache)
	if !ok {
		return nil, errors.New("ClientCache not installed in the context")
	}
	return cc, nil
}

// newClient connects to the instance of remote execution service, and returns
// a client.
func newClient(ctx context.Context, instance string) (*client.Client, error) {
	creds, err := auth.GetPerRPCCredentials(ctx, auth.AsSelf, auth.WithScopes(scopes.CloudScopeSet()...))
	if err != nil {
		return nil, errors.Fmt("failed to get credentials: %w", err)
	}

	c, err := client.NewClient(ctx, instance,
		client.DialParams{
			Service:            casclient.AddrProd,
			TransportCredsOnly: true,
		},
		&client.PerRPCCreds{Creds: creds},
		client.StartupCapabilities(false),
	)
	if err != nil {
		// convert gRPC code to LUCI errors tag.
		t := grpcutil.Tag.WithDefault(status.Code(err))
		return nil, t.Apply(errors.Fmt("failed to create client: %w", err))
	}

	return c, nil
}
