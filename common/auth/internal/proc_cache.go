// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package internal

import (
	"sync"

	"golang.org/x/oauth2"
)

var (
	// ProcTokenCache is shared in-process cache to use if disk cache is disabled.
	ProcTokenCache TokenCache = &MemoryTokenCache{}
)

// MemoryTokenCache implements TokenCache on top of in-process memory.
type MemoryTokenCache struct {
	lock  sync.RWMutex
	cache map[string]oauth2.Token
}

// GetToken reads the token from cache.
func (c *MemoryTokenCache) GetToken(key *CacheKey) (*oauth2.Token, error) {
	k := key.ToMapKey()
	c.lock.RLock()
	tok, ok := c.cache[k]
	c.lock.RUnlock()
	if ok {
		return &tok, nil
	}
	return nil, nil
}

// PutToken writes the token to cache.
func (c *MemoryTokenCache) PutToken(key *CacheKey, tok *oauth2.Token) error {
	k := key.ToMapKey()
	c.lock.Lock()
	if c.cache == nil {
		c.cache = make(map[string]oauth2.Token, 1)
	}
	c.cache[k] = *tok
	c.lock.Unlock()
	return nil
}

// DeleteToken removes the token from cache.
func (c *MemoryTokenCache) DeleteToken(key *CacheKey) error {
	k := key.ToMapKey()
	c.lock.Lock()
	delete(c.cache, k)
	c.lock.Unlock()
	return nil
}
