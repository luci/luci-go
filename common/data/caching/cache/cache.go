// Copyright 2015 The LUCI Authors.
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

package cache

import (
	"bytes"
	"crypto"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.chromium.org/luci/common/data/text/units"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/system/filesystem"
)

// Cache is a cache of objects.
//
// All implementations must be thread-safe.
type Cache interface {
	io.Closer

	// Keys returns the list of all cached digests in LRU order.
	Keys() isolated.HexDigests

	// Touch updates the LRU position of an item to ensure it is kept in the
	// cache.
	//
	// Returns true if item is in cache.
	Touch(digest isolated.HexDigest) bool

	// Evict removes item from cache if it's there.
	Evict(digest isolated.HexDigest)

	// Add reads data from src and stores it in cache.
	Add(digest isolated.HexDigest, src io.Reader) error

	// AddWithHardlink reads data from src and stores it in cache and hardlink file.
	// This is to avoid file removal by shrink in Add().
	AddWithHardlink(digest isolated.HexDigest, src io.Reader, dest string, perm os.FileMode) error

	// Read returns contents of the cached item.
	Read(digest isolated.HexDigest) (io.ReadCloser, error)

	// Hardlink ensures file at |dest| has the same content as cached |digest|.
	//
	// Note that the behavior when dest already exists is undefined. It will work
	// on all POSIX and may or may not fail on Windows depending on the
	// implementation used. Do not rely on this behavior.
	Hardlink(digest isolated.HexDigest, dest string, perm os.FileMode) error

	// GetAdded returns a list of file size added to cache.
	GetAdded() []int64

	// GetAdded returns a list of file size used from cache.
	GetUsed() []int64
}

// Policies is the policies to use on a cache to limit it's footprint.
//
// It's a cache, not a leak.
type Policies struct {
	// MaxSize trims if the cache gets larger than this value. If 0, the cache is
	// effectively a leak.
	MaxSize units.Size
	// MaxItems is the maximum number of items to keep in the cache. If 0, do not
	// enforce a limit.
	MaxItems int
	// MinFreeSpace trims if disk free space becomes lower than this value.
	// Only makes sense when using disk based cache.
	MinFreeSpace units.Size
}

// NewMemory creates a purely in-memory cache.
func NewMemory(policies Policies, namespace string) Cache {
	return &memory{
		policies: policies,
		h:        isolated.GetHash(namespace),
		data:     map[isolated.HexDigest][]byte{},
		lru:      makeLRUDict(namespace),
	}
}

// NewDisk creates a disk based cache.
//
// It may return both a valid Cache and an error if it failed to load the
// previous cache metadata. It is safe to ignore this error.
func NewDisk(policies Policies, path, namespace string) (Cache, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("must use absolute path")
	}
	d := &disk{
		policies: policies,
		path:     path,
		h:        isolated.GetHash(namespace),
		lru:      makeLRUDict(namespace),
	}
	p := d.statePath()

	err := func() error {
		f, err := os.Open(p)
		if err != nil && os.IsNotExist(err) {
			// The fact that the cache is new is not an error.
			return nil
		}
		if err != nil {
			return err
		}
		defer f.Close()
		return json.NewDecoder(f).Decode(&d.lru)
	}()

	if err != nil {
		// Do not use os.RemoveAll, due to strange 'Access Denied' error on windows
		// in os.MkDir after os.RemoveAll.
		// crbug.com/932396#c123
		files, err := ioutil.ReadDir(path)
		if err != nil {
			return nil, fmt.Errorf("failed to call ioutil.ReadDir(%s): %w", path, err)
		}

		for _, file := range files {
			p := filepath.Join(path, file.Name())
			if err := os.RemoveAll(p); err != nil {
				return nil, fmt.Errorf("failed to call os.RemoveAll(%s): %w", p, err)
			}
		}

		d.lru = makeLRUDict(namespace)
	}
	return d, err
}

// Private details.

type memory struct {
	// Immutable.
	policies Policies
	h        crypto.Hash

	// Lock protected.
	mu   sync.Mutex
	data map[isolated.HexDigest][]byte // Contains the actual content.
	lru  lruDict                       // Implements LRU based eviction.

	added []int64
	used  []int64
}

func (m *memory) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return nil
}

func (m *memory) Keys() isolated.HexDigests {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lru.keys()
}

func (m *memory) Touch(digest isolated.HexDigest) bool {
	if !digest.Validate(m.h) {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.data[digest]; !ok {
		return false
	}
	m.lru.touch(digest)
	return true
}

func (m *memory) Evict(digest isolated.HexDigest) {
	if !digest.Validate(m.h) {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, digest)
	m.lru.pop(digest)
}

func (m *memory) Read(digest isolated.HexDigest) (io.ReadCloser, error) {
	if !digest.Validate(m.h) {
		return nil, os.ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	content, ok := m.data[digest]
	if !ok {
		return nil, os.ErrNotExist
	}
	m.used = append(m.used, int64(len(content)))
	return ioutil.NopCloser(bytes.NewBuffer(content)), nil
}

func (m *memory) add(digest isolated.HexDigest, src io.Reader) ([]byte, error) {
	if !digest.Validate(m.h) {
		return nil, os.ErrInvalid
	}
	// TODO(maruel): Use a LimitedReader flavor that fails when reaching limit.
	content, err := ioutil.ReadAll(src)
	if err != nil {
		return nil, err
	}
	if isolated.HashBytes(m.h, content) != digest {
		return nil, errors.New("invalid hash")
	}
	if units.Size(len(content)) > m.policies.MaxSize {
		return nil, errors.New("item too large")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.added = append(m.added, int64(len(content)))
	m.data[digest] = content
	m.lru.pushFront(digest, units.Size(len(content)))
	m.respectPolicies()
	return content, nil
}

func (m *memory) Add(digest isolated.HexDigest, src io.Reader) error {
	_, err := m.add(digest, src)
	return err
}

func (m *memory) AddWithHardlink(digest isolated.HexDigest, src io.Reader, dest string, perm os.FileMode) error {
	content, err := m.add(digest, src)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(dest, content, perm)
}

func (m *memory) Hardlink(digest isolated.HexDigest, dest string, perm os.FileMode) error {
	if !digest.Validate(m.h) {
		return os.ErrInvalid
	}
	m.mu.Lock()
	content, ok := m.data[digest]
	m.mu.Unlock()
	if !ok {
		return os.ErrNotExist
	}
	return ioutil.WriteFile(dest, content, perm)
}

func (m *memory) respectPolicies() {
	for m.lru.length() > m.policies.MaxItems || m.lru.sum > m.policies.MaxSize {
		k, _ := m.lru.popOldest()
		delete(m.data, k)
	}
}

func (m *memory) GetAdded() []int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]int64{}, m.added...)
}

func (m *memory) GetUsed() []int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]int64{}, m.used...)
}

type disk struct {
	// Immutable.
	policies Policies
	path     string
	h        crypto.Hash

	// Lock protected.
	mu  sync.Mutex
	lru lruDict // Implements LRU based eviction.
	// TODO(maruel): Add stats about: # removed.
	// TODO(maruel): stateFile
	added []int64
	used  []int64
}

func (d *disk) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.lru.IsDirty() {
		return nil
	}
	f, err := os.Create(d.statePath())
	if err == nil {
		defer f.Close()
		err = json.NewEncoder(f).Encode(&d.lru)
	}
	return err
}

func (d *disk) Keys() isolated.HexDigests {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.lru.keys()
}

func (d *disk) Touch(digest isolated.HexDigest) bool {
	if !digest.Validate(d.h) {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	mtime := time.Now()
	if err := os.Chtimes(d.itemPath(digest), mtime, mtime); err != nil {
		return false
	}
	d.lru.touch(digest)
	return true
}

func (d *disk) Evict(digest isolated.HexDigest) {
	if !digest.Validate(d.h) {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lru.pop(digest)
	_ = os.Remove(d.itemPath(digest))
}

func (d *disk) Read(digest isolated.HexDigest) (io.ReadCloser, error) {
	if !digest.Validate(d.h) {
		return nil, os.ErrInvalid
	}
	f, err := os.Open(d.itemPath(digest))
	if err != nil {
		return nil, err
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	e := d.lru.items.entries[digest].Value.(*entry)
	d.used = append(d.used, int64(e.value))
	return f, nil
}

func (d *disk) add(digest isolated.HexDigest, src io.Reader, cb func() error) error {
	if !digest.Validate(d.h) {
		return os.ErrInvalid
	}
	p := d.itemPath(digest)
	dst, err := os.Create(p)
	if err != nil {
		return err
	}
	// TODO(maruel): Use a LimitedReader flavor that fails when reaching limit.
	h := d.h.New()
	size, err := io.Copy(dst, io.TeeReader(src, h))
	if err2 := dst.Close(); err == nil {
		err = err2
	}
	if err != nil {
		_ = os.Remove(p)
		return err
	}
	if isolated.Sum(h) != digest {
		_ = os.Remove(p)
		return errors.New("invalid hash")
	}
	if units.Size(size) > d.policies.MaxSize {
		_ = os.Remove(p)
		return errors.New("item too large")
	}

	if cb != nil {
		if err := cb(); err != nil {
			return err
		}
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.lru.pushFront(digest, units.Size(size))
	d.respectPolicies()
	d.added = append(d.added, size)
	return nil
}

func (d *disk) Add(digest isolated.HexDigest, src io.Reader) error {
	return d.add(digest, src, nil)
}

func (d *disk) AddWithHardlink(digest isolated.HexDigest, src io.Reader, dest string, perm os.FileMode) error {
	return d.add(digest, src, func() error {
		if err := d.Hardlink(digest, dest, perm); err != nil {
			_ = os.Remove(d.itemPath(digest))
			return fmt.Errorf("failed to call Hardlink(%s, %s): %w", digest, dest, err)
		}
		return nil
	})
}

func (d *disk) Hardlink(digest isolated.HexDigest, dest string, perm os.FileMode) error {
	if !digest.Validate(d.h) {
		return os.ErrInvalid
	}
	src := d.itemPath(digest)
	// - Windows, if dest exists, the call fails. In particular, trying to
	//   os.Remove() will fail if the file's ReadOnly bit is set. What's worse is
	//   that the ReadOnly bit is set on the file inode, shared on all hardlinks
	//   to this inode. This means that in the case of a file with the ReadOnly
	//   bit set, it would have to do:
	//   - If dest exists:
	//    - If dest has ReadOnly bit:
	//      - If file has any other inode:
	//        - Remove the ReadOnly bit.
	//        - Remove dest.
	//        - Set the ReadOnly bit on one of the inode found.
	//   - Call os.Link()
	//  In short, nobody ain't got time for that.
	//
	// - On any other (sane) OS, if dest exists, it is silently overwritten.
	if err := os.Link(src, dest); err != nil {
		return fmt.Errorf("failed to call os.Link(%s, %s): %w", src, dest, err)
	}

	if err := os.Chmod(dest, perm); err != nil {
		return fmt.Errorf("failed to call os.Chmod(%s, %#o): %w", dest, perm, err)
	}
	return nil
}

func (d *disk) GetAdded() []int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]int64{}, d.added...)
}

func (d *disk) GetUsed() []int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]int64{}, d.used...)
}

func (d *disk) itemPath(digest isolated.HexDigest) string {
	return filepath.Join(d.path, string(digest))
}

func (d *disk) statePath() string {
	return filepath.Join(d.path, "state.json")
}

func (d *disk) respectPolicies() {
	increaseFreeSpace := func() bool {
		size, err := filesystem.GetFreeSpace(d.path)
		if err != nil {
			return false
		}
		return size < uint64(d.policies.MinFreeSpace)
	}

	for d.lru.length() > d.policies.MaxItems || d.lru.sum > d.policies.MaxSize || increaseFreeSpace() {
		k, _ := d.lru.popOldest()
		_ = os.Remove(d.itemPath(k))
	}
}
