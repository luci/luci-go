// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolateserver

import (
	"bytes"
	"encoding/hex"
	"hash"
	"io"
	"io/ioutil"
	"os"
	"sync"
)

// LocalCache is a cache of objects.
//
// All implementations must be thread-safe.
type LocalCache interface {
	// CachedSet returns a set of all cached digests.
	CachedSet() []HexDigest

	// Touch ensures item is not corrupted and updates its LRU position.
	//
	// size is the expected size of this item.
	//
	// Returns true if item is in cache and not corrupted.
	Touch(digest HexDigest, size int64) bool

	// Evit removes item from cache if it's there.
	Evict(digest HexDigest)

	// Read returns contents of the cached item.
	Read(digest HexDigest) (io.ReadCloser, error)

	// Write reads data from src and stores it in cache.
	Write(digest HexDigest, src io.Reader) error

	// Hardlink ensures file at |dest| has same content as cached |digest|.
	Hardlink(digest HexDigest, dest string, perm os.FileMode) error
}

// HashFactory creates a new hash algo as needed.
type HashFactory func() hash.Hash

// memoryLocalCache implements LocalCache in memory.
type memoryLocalCache struct {
	// Immutable.
	algo    hash.Hash
	factory HashFactory

	// Lock protected.
	lock sync.Mutex
	data map[HexDigest][]byte
}

// MakeMemoryCache creates a purely in-memory cache.
func MakeMemoryCache(algo HashFactory) LocalCache {
	return &memoryLocalCache{
		algo:    algo(),
		factory: algo,
		data:    map[HexDigest][]byte{},
	}
}

func (m *memoryLocalCache) CachedSet() []HexDigest {
	m.lock.Lock()
	defer m.lock.Unlock()
	out := make([]HexDigest, 0, len(m.data))
	for k := range m.data {
		out = append(out, k)
	}
	return out
}

func (m *memoryLocalCache) Touch(digest HexDigest, size int64) bool {
	if !digest.Validate(m.algo) {
		return false
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	_, ok := m.data[digest]
	return ok
}

func (m *memoryLocalCache) Evict(digest HexDigest) {
	if !digest.Validate(m.algo) {
		return
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	delete(m.data, digest)
}

func (m *memoryLocalCache) Read(digest HexDigest) (io.ReadCloser, error) {
	if !digest.Validate(m.algo) {
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

func (m *memoryLocalCache) Write(digest HexDigest, src io.Reader) error {
	if !digest.Validate(m.algo) {
		return os.ErrInvalid
	}
	content, err := ioutil.ReadAll(src)
	if err != nil {
		return err
	}

	// TODO(maruel): This check is not required. It is kept to ease development
	// but should likely be removed in the near future since it is a waste of CPU.
	algo := m.factory()
	algo.Write(content)
	if HexDigest(hex.EncodeToString(algo.Sum(nil))) != digest {
		return os.ErrInvalid
	}

	m.lock.Lock()
	defer m.lock.Unlock()
	m.data[digest] = content
	return nil
}

func (m *memoryLocalCache) Hardlink(digest HexDigest, dest string, perm os.FileMode) error {
	if !digest.Validate(m.algo) {
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
