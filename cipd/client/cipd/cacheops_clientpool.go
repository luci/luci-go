// Copyright 2026 The LUCI Authors.
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

package cipd

import (
	"context"
	"io"
	"sync"

	"go.chromium.org/luci/cipd/common"
)

// clientPoolEntry is either a Client or the error generated when constructing
// it.
type clientPoolEntry struct {
	c   Client
	err error
}

// clientPool acts as a factory for clients which vary by ServiceURL.
type clientPool struct {
	// The base options to use; ServiceURL will be overridden when
	// [clientPool.client] is called with a non-empty `service`.
	opts ClientOptions

	mu sync.Mutex
	// batchLevel is kind of a 'meta' batch level for the whole pool. when
	// beginBatch is called, this is incremented and all currently allocated
	// clients will BeginBatch.
	//
	// When a new client is generated, it will immediately have BeginBatch called
	// this many times.
	//
	// When endBatch is called this is decremented and all currently allocated
	// clients will EndBatch.
	batchLevel int

	// A mapping of ServiceURL -> Client/error
	clients map[string]clientPoolEntry
}

// client returns the client or error for the given ServiceURL, constructing it
// if necessary.
//
// If `service` is empty, this inherits the ServiceURL from c.opts.
func (c *clientPool) client(ctx context.Context, service string) (Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if service == "" {
		service = c.opts.ServiceURL
	}

	cur, ok := c.clients[service]
	if !ok {
		mopt := c.opts
		mopt.ServiceURL = service
		cur.c, cur.err = NewClient(mopt)
		if cur.err != nil {
			// Ignore; this will be logged later.
		} else {
			for range c.batchLevel {
				cur.c.BeginBatch(ctx)
			}
		}
		if c.clients == nil {
			c.clients = map[string]clientPoolEntry{}
		}
		c.clients[service] = cur
	}
	return cur.c, cur.err
}

// beginBatch is like Client.BeginBatch, but it applies to all clients in the
// pool, current or future.
func (c *clientPool) beginBatch(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.batchLevel++
	for _, entry := range c.clients {
		if c := entry.c; c != nil {
			c.BeginBatch(ctx)
		}
	}
}

// endBatch is like Client.EndBatch, but it applies to all clients in the
// pool.
func (c *clientPool) endBatch(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.batchLevel--
	for _, entry := range c.clients {
		if c := entry.c; c != nil {
			c.EndBatch(ctx)
		}
	}
}

// resolves pick the appropriate client and resolves the version.
func (c *clientPool) resolve(ctx context.Context, service, pkg, version string) (pin common.Pin, err error) {
	cli, err := c.client(ctx, service)
	if err == nil {
		pin, err = cli.ResolveVersion(ctx, pkg, version)
	}
	return
}

// close closes all of the clients in the pool.
//
// Resets the clientPool to its initial state.
func (c *clientPool) close(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, entry := range c.clients {
		if c := entry.c; c != nil {
			c.Close(ctx)
		}
	}
	c.clients = nil
}

// fetchInstanceTo forwards to [Client.FetchInstanceTo] for the correct client
// based on `service`.
func (c *clientPool) fetchInstanceTo(ctx context.Context, service string, pin common.Pin, f io.WriteSeeker) error {
	cli, err := c.client(ctx, service)
	if err != nil {
		return err
	}
	return cli.FetchInstanceTo(ctx, pin, f)
}
