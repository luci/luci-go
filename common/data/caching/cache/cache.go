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
	"context"
	"crypto"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/trace"
	"sync"
	"time"

	"go.chromium.org/luci/common/data/text/units"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/filesystem"
)

// Cache is a cache of objects holding content in disk.
//
// All implementations must be thread-safe.
type Cache struct {
	// Immutable.
	policies Policies
	path     string
	h        crypto.Hash

	freeSpaceWarningOnce sync.Once

	// Lock protected.
	mu  sync.Mutex // This protects modification of cached entries under |path| too.
	lru lruDict    // Implements LRU based eviction.

	// TODO(crbug.com/1231726): remove after debug.
	log bytes.Buffer

	statsMu sync.Mutex // Protects the stats below
	// TODO(tikuta): Add stats about: # removed.
	// TODO(tikuta): stateFile
	added []int64
	used  []int64
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

// AddFlags adds flags for cache policy parameters.
func (p *Policies) AddFlags(f *flag.FlagSet) {
	f.Var(&p.MaxSize, "cache-max-size", "Cache is trimmed if the cache gets larger than this value. If 0, the cache is effectively a leak.")
	f.IntVar(&p.MaxItems, "cache-max-items", 0, "Maximum number of items to keep in the cache.")
	f.Var(&p.MinFreeSpace, "cache-min-free-space", "Cache is trimmed if disk free space becomes lower than this value.")
}

// IsDefault returns whether some flags are set or not.
func (p *Policies) IsDefault() bool {
	return p.MaxSize == 0 && p.MaxItems == 0 && p.MinFreeSpace == 0
}

func (p *Policies) fitsCacheSize(s units.Size) bool {
	return p.MaxSize == 0 || s <= p.MaxSize
}

// ErrInvalidHash indicates invalid hash is specified.
var ErrInvalidHash = errors.New("invalid hash")

// New creates a disk based cache.
//
// It may return both a valid Cache and an error if it failed to load the
// previous cache metadata. It is safe to ignore this error. This creates
// cache directory if it doesn't exist.
func New(policies Policies, path string, h crypto.Hash) (*Cache, error) {
	var err error
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, errors.Annotate(err, "failed to call Abs(%s)", path).Err()
	}
	err = os.MkdirAll(path, 0700)
	if err != nil {
		return nil, errors.Annotate(err, "failed to call MkdirAll(%s)", path).Err()
	}

	d := &Cache{
		policies: policies,
		path:     path,
		h:        h,
		lru:      makeLRUDict(h),
	}
	p := d.statePath()

	err = func() error {
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
		files, err := os.ReadDir(path)
		if err != nil {
			return nil, errors.Annotate(err, "failed to call os.ReadDir(%s)", path).Err()
		}

		for _, file := range files {
			p := filepath.Join(path, file.Name())
			if err := os.RemoveAll(p); err != nil {
				return nil, errors.Annotate(err, "failed to call os.RemoveAll(%s)", p).Err()
			}
		}

		d.lru = makeLRUDict(h)
	}

	if json, err := d.lru.MarshalJSON(); err != nil {
		return nil, err
	} else {
		fmt.Fprintf(&d.log, "initial json: %s\n", string(json))
	}

	return d, err
}

// Close closes the Cache, writes the cache status file to cache dir.
func (d *Cache) Close() error {
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

// Keys returns the list of all cached digests in LRU order.
func (d *Cache) Keys() HexDigests {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.lru.keys()
}

// TotalSize returns the size of the contents maintained in the LRU cache.
func (d *Cache) TotalSize() units.Size {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.lru.sum
}

// Touch updates the LRU position of an item to ensure it is kept in the
// cache.
//
// Returns true if item is in cache.
func (d *Cache) Touch(digest HexDigest) bool {
	if !digest.Validate(d.h) {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.lru.touch(digest)
}

// Evict removes item from cache if it's there.
func (d *Cache) Evict(digest HexDigest) {
	if !digest.Validate(d.h) {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lru.pop(digest)
	_ = os.Remove(d.itemPath(digest))
}

// Read returns contents of the cached item.
func (d *Cache) Read(digest HexDigest) (io.ReadCloser, error) {
	if !digest.Validate(d.h) {
		return nil, os.ErrInvalid
	}

	d.mu.Lock()
	f, err := os.Open(d.itemPath(digest))
	if err != nil {
		d.mu.Unlock()
		return nil, err
	}
	d.lru.touch(digest)
	d.mu.Unlock()

	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, errors.Annotate(err, "failed to get stat for %s", digest).Err()
	}

	d.statsMu.Lock()
	defer d.statsMu.Unlock()
	d.used = append(d.used, fi.Size())
	return f, nil
}

// Add reads data from src and stores it in cache.
func (d *Cache) Add(ctx context.Context, digest HexDigest, src io.Reader) error {
	return d.add(ctx, digest, src, nil)
}

// AddFileWithoutValidation adds src as cache entry with hardlink.
// But this doesn't do any content validation.
//
// TODO(tikuta): make one function and control the behavior by option?
func (d *Cache) AddFileWithoutValidation(ctx context.Context, digest HexDigest, src string) error {
	ctx, task := trace.NewTask(ctx, "AddFileWithoutValidation")
	defer task.End()

	fi, err := os.Stat(src)
	if err != nil {
		return errors.Annotate(err, "failed to get stat: %s", src).Err()
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	start := time.Now()
	dest := d.itemPath(digest)
	if err := makeHardLinkOrClone(src, dest); err != nil && !errors.Contains(err, os.ErrExist) {
		terr := func() error {
			if runtime.GOOS == "darwin" {
				// TODO(crbug.com/1140864): Fallback to Copy in macOS, this is mitigation for strange `operation not permitted` error.
				if cerr := filesystem.Copy(dest, src, fi.Mode()); cerr != nil {
					err = errors.Annotate(err, "fallback copy failed: %v", cerr).Err()
				} else {
					return nil
				}
			}

			return errors.Annotate(err, "failed to link %s to %s", src, digest).Err()
		}()
		if terr != nil {
			return terr
		}
	}

	trace.Logf(ctx, "", "os.Link took %s", time.Since(start))

	d.lru.pushFront(digest, units.Size(fi.Size()))
	if err := d.respectPolicies(ctx); err != nil {
		d.lru.pop(digest)
		return err
	}

	d.statsMu.Lock()
	defer d.statsMu.Unlock()
	d.added = append(d.added, fi.Size())
	return nil
}

// AddWithHardlink reads data from src and stores it in cache and hardlink file.
// This is to avoid file removal by shrink in Add().
func (d *Cache) AddWithHardlink(ctx context.Context, digest HexDigest, src io.Reader, dest string, perm os.FileMode) error {
	return d.add(ctx, digest, src, func() error {
		if err := d.hardlinkUnlocked(digest, dest, perm); err != nil {
			_ = os.Remove(d.itemPath(digest))
			return errors.Annotate(err, "failed to call Hardlink(%s, %s)", digest, dest).Err()
		}
		return nil
	})
}

// Hardlink ensures file at |dest| has the same content as cached |digest|.
//
// Note that the behavior when dest already exists is undefined. It will work
// on all POSIX and may or may not fail on Windows depending on the
// implementation used. Do not rely on this behavior.
func (d *Cache) Hardlink(digest HexDigest, dest string, perm os.FileMode) error {
	if runtime.GOOS == "darwin" {
		// Accessing the path, which is being replaced, with os.Link
		// seems to cause flaky 'operation not permitted' failure on
		// macOS (https://crbug.com/1076468). So prevent that by holding
		// lock here.
		d.mu.Lock()
		defer d.mu.Unlock()
	}
	return d.hardlinkUnlocked(digest, dest, perm)
}

// Added returns a list of file size added to cache.
func (d *Cache) Added() []int64 {
	d.statsMu.Lock()
	defer d.statsMu.Unlock()
	return append([]int64{}, d.added...)
}

// Used returns a list of file size used from cache.
func (d *Cache) Used() []int64 {
	d.statsMu.Lock()
	defer d.statsMu.Unlock()
	return append([]int64{}, d.used...)
}

// Private details.

func (d *Cache) add(ctx context.Context, digest HexDigest, src io.Reader, cb func() error) error {
	if !digest.Validate(d.h) {
		return os.ErrInvalid
	}
	tmp, err := os.CreateTemp(d.path, string(digest)+".*.tmp")
	if err != nil {
		return errors.Annotate(err, "failed to create tempfile for %s", digest).Err()
	}
	// TODO(maruel): Use a LimitedReader flavor that fails when reaching limit.
	h := d.h.New()
	size, err := io.Copy(tmp, io.TeeReader(src, h))
	if err2 := tmp.Close(); err == nil {
		err = err2
	}
	fname := tmp.Name()
	if err != nil {
		_ = os.Remove(fname)
		return err
	}
	if d := Sum(h); d != digest {
		_ = os.Remove(fname)
		return errors.Annotate(ErrInvalidHash, "invalid hash, got=%s, want=%s", d, digest).Err()
	}
	if !d.policies.fitsCacheSize(units.Size(size)) {
		_ = os.Remove(fname)
		return errors.Reason("item too large, size=%d, limit=%d", size, d.policies.MaxSize).Err()
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// If the cache already exists, do not try os.Rename().
	if d.lru.touch(digest) {
		logging.Debugf(ctx, "cache already exists. path: %s, digest %s\n", d.path, digest)
		if err := os.Remove(fname); err != nil {
			return errors.Annotate(err, "failed to remove tmp file: %s", fname).Err()
		}
		if cb != nil {
			if err := cb(); err != nil {
				return err
			}
		}
		return nil
	}

	if err := os.Rename(fname, d.itemPath(digest)); err != nil {
		_ = os.Remove(fname)
		return errors.Annotate(err, "failed to rename %s -> %s", fname, d.itemPath(digest)).Err()
	}

	if cb != nil {
		if err := cb(); err != nil {
			return err
		}
	}

	d.lru.pushFront(digest, units.Size(size))
	if err := d.respectPolicies(ctx); err != nil {
		d.lru.pop(digest)
		return err
	}
	d.statsMu.Lock()
	defer d.statsMu.Unlock()
	d.added = append(d.added, size)
	return nil
}

func (d *Cache) hardlinkUnlocked(digest HexDigest, dest string, perm os.FileMode) error {
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
	if err := makeHardLinkOrClone(src, dest); err != nil {
		if _, serr := os.Stat(src); errors.Contains(serr, os.ErrNotExist) {
			// In Windows, os.Link may fail with access denied error even if |src| isn't there.
			// And this is to normalize returned error in such case.
			// https://crbug.com/1098265
			err = errors.Annotate(serr, "%s doesn't exist and os.Link failed: %v\nlogs:\n%s", src, err, d.log.String()).Err()
		}
		debugInfo := fmt.Sprintf("Stats:\n*  src: %s\n*  dest: %s\n*  destDir: %s\nUID=%d GID=%d", statsStr(src), statsStr(dest), statsStr(filepath.Dir(dest)), os.Getuid(), os.Getgid())
		return errors.Annotate(err, "failed to call makeHardLinkOrClone(%s, %s)\n%s", src, dest, debugInfo).Err()
	}

	if err := os.Chmod(dest, perm); err != nil {
		return errors.Annotate(err, "failed to call os.Chmod(%s, %#o)", dest, perm).Err()
	}

	fi, err := os.Stat(dest)
	if err != nil {
		return errors.Annotate(err, "failed to call os.Stat(%s)", dest).Err()
	}
	size := fi.Size()
	d.statsMu.Lock()
	defer d.statsMu.Unlock()
	// If this succeeds directly, it means the file is already cached on the
	// disk, so we put it into LRU.
	d.used = append(d.used, size)

	return nil
}

func (d *Cache) itemPath(digest HexDigest) string {
	return filepath.Join(d.path, string(digest))
}

func (d *Cache) statePath() string {
	return filepath.Join(d.path, "state.json")
}

func (d *Cache) respectPolicies(ctx context.Context) error {
	ctx, task := trace.NewTask(ctx, "respectPolicies")
	defer task.End()

	minFreeSpaceWanted := uint64(d.policies.MinFreeSpace)
	for {
		freeSpace, err := filesystem.GetFreeSpace(d.path)
		if err != nil {
			return errors.Annotate(err, "couldn't estimate the free space at %s", d.path).Err()
		}
		if (d.policies.MaxItems == 0 || d.lru.length() <= d.policies.MaxItems) && d.policies.fitsCacheSize(d.lru.sum) && freeSpace >= minFreeSpaceWanted {
			break
		}
		if d.lru.length() == 0 {
			d.freeSpaceWarningOnce.Do(func() {
				// TODO(crbug.com/chrome-operations/49): make this error again.
				logging.Warningf(ctx, "no more space to free in %s: current free space=%d policies.MinFreeSpace=%d", d.path, freeSpace, minFreeSpaceWanted)
			})

			break
		}
		k, _ := d.lru.popOldest()
		_ = os.Remove(d.itemPath(k))
	}
	return nil
}

func statsStr(path string) string {
	fi, err := os.Stat(path)
	return fmt.Sprintf("path=%s FileInfo=%+v err=%v", path, fi, err)
}
