// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolatedclient

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"sync"

	"github.com/luci/luci-go/common/isolated"
)

// LocalCache is a cache of objects.
//
// All implementations must be thread-safe.
type LocalCache interface {
	// CachedSet returns a set of all cached digests.
	CachedSet() []isolated.HexDigest

	// Touch ensures item is not corrupted and updates its LRU position.
	//
	// size is the expected size of this item.
	//
	// Returns true if item is in cache and not corrupted.
	Touch(digest isolated.HexDigest, size int64) bool

	// Evit removes item from cache if it's there.
	Evict(digest isolated.HexDigest)

	// Read returns contents of the cached item.
	Read(digest isolated.HexDigest) (io.ReadCloser, error)

	// Write reads data from src and stores it in cache.
	Write(digest isolated.HexDigest, src io.Reader) error

	// Hardlink ensures file at |dest| has same content as cached |digest|.
	Hardlink(digest isolated.HexDigest, dest string, perm os.FileMode) error
}

// memoryLocalCache implements LocalCache in memory.
type memoryLocalCache struct {
	// Lock protected.
	lock sync.Mutex
	data map[isolated.HexDigest][]byte
}

// MakeMemoryCache creates a purely in-memory cache.
func MakeMemoryCache() LocalCache {
	return &memoryLocalCache{
		data: map[isolated.HexDigest][]byte{},
	}
}

func (m *memoryLocalCache) CachedSet() []isolated.HexDigest {
	m.lock.Lock()
	defer m.lock.Unlock()
	out := make([]isolated.HexDigest, 0, len(m.data))
	for k := range m.data {
		out = append(out, k)
	}
	return out
}

func (m *memoryLocalCache) Touch(digest isolated.HexDigest, size int64) bool {
	if !digest.Validate() {
		return false
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	_, ok := m.data[digest]
	return ok
}

func (m *memoryLocalCache) Evict(digest isolated.HexDigest) {
	if !digest.Validate() {
		return
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	delete(m.data, digest)
}

func (m *memoryLocalCache) Read(digest isolated.HexDigest) (io.ReadCloser, error) {
	if !digest.Validate() {
		return nil, os.ErrInvalid
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	content, ok := m.data[digest]
	if !ok {
		return nil, os.ErrNotExist
	}
	return ioutil.NopCloser(bytes.NewBuffer(content)), nil
}

func (m *memoryLocalCache) Write(digest isolated.HexDigest, src io.Reader) error {
	if !digest.Validate() {
		return os.ErrInvalid
	}
	content, err := ioutil.ReadAll(src)
	if err != nil {
		return err
	}

	// TODO(maruel): This check is not required. It is kept to ease development
	// but should likely be removed in the near future since it is a waste of CPU.
	if isolated.HashBytes(content) != digest {
		return os.ErrInvalid
	}

	m.lock.Lock()
	defer m.lock.Unlock()
	m.data[digest] = content
	return nil
}

func (m *memoryLocalCache) Hardlink(digest isolated.HexDigest, dest string, perm os.FileMode) error {
	if !digest.Validate() {
		return os.ErrInvalid
	}
	m.lock.Lock()
	content, ok := m.data[digest]
	m.lock.Unlock()
	if !ok {
		return os.ErrNotExist
	}
	return ioutil.WriteFile(dest, content, perm)
}
