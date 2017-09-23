// Copyright 2017 The LUCI Authors.
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

package internal

import (
	"sync"
)

var (
	// ProcTokenCache is shared in-process cache to use if disk cache is disabled.
	ProcTokenCache TokenCache = &MemoryTokenCache{}
)

// MemoryTokenCache implements TokenCache on top of in-process memory.
type MemoryTokenCache struct {
	lock  sync.RWMutex
	cache map[string]Token
}

// GetToken reads the token from cache.
func (c *MemoryTokenCache) GetToken(key *CacheKey) (*Token, error) {
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
func (c *MemoryTokenCache) PutToken(key *CacheKey, tok *Token) error {
	k := key.ToMapKey()
	c.lock.Lock()
	if c.cache == nil {
		c.cache = make(map[string]Token, 1)
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
