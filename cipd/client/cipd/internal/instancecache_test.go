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

package internal

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/check"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipd/client/cipd/builder"
	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/internal/messages"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/cipd/client/cipd/reader"
	"go.chromium.org/luci/cipd/common"
)

// trackingFetcher is a stateful tracker which counts the number of calls made,
// and also synthesizes data using `fakeData`.
type trackingFetcher struct {
	errFn func(fetchID int32) error

	calls atomic.Int32
}

// fetch can be used as InstanceCache.Fetcher.
func (f *trackingFetcher) fetch(ctx context.Context, service string, pin common.Pin, w io.WriteSeeker) error {
	fetchID := f.calls.Add(1)
	if f.errFn != nil {
		if err := f.errFn(fetchID); err != nil {
			return err
		}
	}
	data, err := fakeData(ctx, pin)
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(data))
	return err
}

// pin generates a common.Pin for "pkg" with an instance ID of `aaaa...{i}`.
//
// This pin will not pass hash verification, but is only syntactically valid.
func pin(i int) common.Pin {
	pin := common.Pin{PackageName: "pkg", InstanceID: fmt.Sprintf("%d", i)}
	pin.InstanceID = strings.Repeat("a", 40-len(pin.InstanceID)) + pin.InstanceID
	return pin
}

// mkContentPin generates a content-addressed pin for `pkg/content/{data}`.
//
// `data` must, itself, be valid as a PackageName. The instance ID will be
// an instance zip for this package name with a single file named `file` whose
// content is `data`.
//
// Returns both the pin and the synthesized instance file.
func mkContentPin(ctx context.Context, data string) (common.Pin, string, error) {
	buf := bytes.NewBuffer(nil)
	pin, err := builder.BuildInstance(ctx, builder.Options{
		Input:       []fs.File{fs.NewTestFile("file", data, fs.TestFileOpts{})},
		Output:      buf,
		PackageName: fmt.Sprintf("pkg/content/%s", data),
	})

	return pin, buf.String(), err
}

// fakeData returns synthetic data for the given `p` pin.
//
// If p.PackageName starts with `pkg/content/`, then this will use
// `mkContentPin` to return the actual matching instance zip file data.
//
// Otherwise this returns `data:{p.InstanceID}`.
func fakeData(ctx context.Context, p common.Pin) (string, error) {
	if data, ok := strings.CutPrefix(p.PackageName, "pkg/content/"); ok {
		actualPin, inst, err := mkContentPin(ctx, data)
		if err != nil {
			return "", err
		}
		if p != actualPin {
			return "", errors.Fmt("TESTBUG: fakeData: %q != %q", p, actualPin)
		}
		return inst, nil
	}

	return "data:" + p.InstanceID, nil
}

// access does a OpenAsSource(), checks that it does not return an error, then
// calls the callback.
//
// Once the callback ends, it closes the source.
//
// Asserts that InstanceResult does not have an error, and passes to the
// callback a boolean indicated if this instance was created as a result of
// this `access` or not.
func access(t testing.TB, ctx context.Context, cache *InstanceCache, pin common.Pin, cb func(created bool, src pkg.Source) (corrupted bool)) {
	t.Helper()

	src, fetched, err := cache.OpenAsSource(ctx, "", pin)
	assert.NoErr(t, err)
	var corrupted bool
	defer func() {
		src.Close(ctx, corrupted)
	}()

	corrupted = cb(fetched, src)
}

// putNew ensures that `pin` is not in `cache`, then accesses it via
// OpenAsSource.
func putNew(t testing.TB, ctx context.Context, cache *InstanceCache, pin common.Pin) {
	t.Helper()

	_, existed := accessTime(ctx, cache, pin)
	assert.That(t, existed, should.BeFalse, truth.LineContext(), truth.Explain(
		"putNew: cannot create new instance %q: instance already exists", pin,
	))

	src, fetched, err := cache.OpenAsSource(ctx, "", pin)
	assert.NoErr(t, err, truth.LineContext())
	assert.That(t, fetched, should.BeTrue, truth.LineContext(), truth.Explain(
		"putNew: unexpectedly did not fetch",
	))
	assert.NoErr(t, src.Close(ctx, false), truth.LineContext())
}

// readSrc reads the content of `src`
func readSrc(t testing.TB, src pkg.Source) string {
	t.Helper()

	buf, err := io.ReadAll(io.NewSectionReader(src, 0, src.Size()))
	assert.NoErr(t, err, truth.LineContext())
	return string(buf)
}

// testHas checks that the pin exists in this cache.
func testHas(t testing.TB, ctx context.Context, cache *InstanceCache, pin common.Pin) {
	t.Helper()

	_, existed := accessTime(ctx, cache, pin)
	assert.That(t, existed, should.BeTrue, truth.LineContext(), truth.Explain(
		"testHas: cache does not contain %q", pin,
	))
}

// accessTime returns the last access time of `pin` in the cache.
//
// If the cache does not contain `pin`, returns ok == false.
func accessTime(ctx context.Context, cache *InstanceCache, pin common.Pin) (lastAccess time.Time, ok bool) {
	cache.withState(ctx, clock.Now(ctx), true, func(s *messages.InstanceCache) (save bool) {
		var entry *messages.InstanceCache_Entry
		if entry, ok = s.Entries[pin.InstanceID]; ok {
			lastAccess = entry.LastAccess.AsTime()
		}
		return false
	})
	return
}

// countTempFiles returns the number of files in the given cache's FS.
//
// Note that this includes state.db (if present).
func countTempFiles(t testing.TB, cache *InstanceCache) int {
	t.Helper()

	tempDirFile, err := os.Open(cache.FS.Root())
	assert.NoErr(t, err, truth.LineContext())
	defer tempDirFile.Close()

	files, err := tempDirFile.Readdirnames(0)
	assert.NoErr(t, err, truth.LineContext())

	return len(files)
}

type instCacheTestEnv struct {
	cache   *InstanceCache
	tempDir string
	fetcher *trackingFetcher
	tc      testclock.TestClock
}

func instanceCacheTestSetup(t testing.TB) (*InstanceCache, *instCacheTestEnv, context.Context) {
	ctx, tc := testclock.UseTime(t.Context(), testclock.TestTimeLocal)

	tempDir := t.TempDir()

	fetcher := &trackingFetcher{}
	cache := &InstanceCache{
		FS:      fs.NewFileSystem(tempDir, ""),
		Fetcher: fetcher.fetch,
	}
	t.Cleanup(func() {
		cache.Close(ctx)
	})

	return cache, &instCacheTestEnv{cache, tempDir, fetcher, tc}, ctx
}

func TestInstanceCache(t *testing.T) {
	t.Parallel()

	t.Run("Works in general", func(t *testing.T) {
		t.Parallel()
		cache, env, ctx := instanceCacheTestSetup(t)

		cache2 := &InstanceCache{
			FS:      fs.NewFileSystem(env.tempDir, ""),
			Fetcher: (&trackingFetcher{}).fetch,
		}
		defer cache2.Close(ctx)

		// Add new.
		putNew(t, ctx, cache, pin(0))

		// Check it can be seen even through another InstanceCache object.
		testHas(t, ctx, cache, pin(0))
		testHas(t, ctx, cache2, pin(0))
	})

	t.Run("Temp cache removes files", func(t *testing.T) {
		t.Parallel()
		cache, _, ctx := instanceCacheTestSetup(t)
		cache.Tmp = true

		assert.That(t, countTempFiles(t, cache), should.Equal(0))
		access(t, ctx, cache, pin(0), func(created bool, src pkg.Source) (corrupted bool) {
			assert.That(t, countTempFiles(t, cache), should.Equal(1))
			return
		})
		assert.That(t, countTempFiles(t, cache), should.Equal(0))
	})

	t.Run("Redownloads corrupted files", func(t *testing.T) {
		t.Parallel()
		cache, env, ctx := instanceCacheTestSetup(t)
		assert.Loosely(t, countTempFiles(t, cache), should.BeZero)

		// Download the first time.
		access(t, ctx, cache, pin(0), func(created bool, src pkg.Source) (corrupted bool) {
			assert.That(t, created, should.BeTrue)
			return
		})

		// Stored in the cache (plus state.db file).
		assert.Loosely(t, countTempFiles(t, cache), should.Equal(2))

		// The second call grabs it from the cache.
		access(t, ctx, cache, pin(0), func(created bool, src pkg.Source) (corrupted bool) {
			assert.That(t, created, should.BeFalse)

			// Close as corrupted. Should be removed from the cache.
			corrupted = true
			return
		})

		// Only state.db file left.
		assert.Loosely(t, countTempFiles(t, cache), should.Equal(1))

		// Download the second time.
		access(t, ctx, cache, pin(0), func(created bool, src pkg.Source) (corrupted bool) {
			assert.Loosely(t, env.fetcher.calls.Load(), should.Equal(2))
			return
		})
	})

	t.Run("GC respects MaxAge vs launchTime", func(t *testing.T) {
		t.Parallel()
		cache, env, ctx := instanceCacheTestSetup(t)
		cache.GCMaxAge = 2500 * time.Millisecond
		t0 := clock.Now(ctx)
		for i := range 8 {
			if i != 0 {
				env.tc.Add(time.Second)
			}
			putNew(t, ctx, cache, pin(i))
		}
		assert.That(t, cache.launchTime, should.Match(t0))

		// This should remove *nothing* - all instances were added after our
		// launch time.
		cache.Close(ctx) // Does a GC pass.

		assert.That(t, countTempFiles(t, cache), should.Equal(9))

		// Relaunch the cache and close it to do another GC pass with launchTime
		// == clock.Now.
		cache.OpenAsSource(ctx, "", common.Pin{})
		assert.That(t, cache.launchTime, should.Match(clock.Now(ctx)))
		cache.Close(ctx)

		// At this point 0..4 have been collected, so 5..7 exist.
		assert.That(t, countTempFiles(t, cache), should.Equal(4)) // +1 for state.db.
		for i := range 3 {
			testHas(t, ctx, cache, pin(i+5))
		}

		// The rest are missing and can be recreated.
		for i := range 5 {
			putNew(t, ctx, cache, pin(i))
		}
	})

	t.Run(`ExactGC`, func(t *testing.T) {
		t.Parallel()
		cache, env, ctx := instanceCacheTestSetup(t)
		cache.ExactGC = true

		// Ensure the cache is launched.
		cache.OpenAsSource(ctx, "", common.Pin{})

		// Advance time a bit so all these pins initially start with a time >
		// launchTime.
		env.tc.Add(time.Minute)

		// Add a bunch of instances.
		putNew(t, ctx, cache, pin(0))
		putNew(t, ctx, cache, pin(1))
		putNew(t, ctx, cache, pin(2))
		putNew(t, ctx, cache, pin(3))

		// Delete the state file to force resync. All these instances should now
		// be marked as === launchTime.
		assert.NoErr(t, os.Remove(filepath.Join(env.tempDir, instanceCacheStateFilename)))

		// Touch some overlapping instances and add some new instances.
		cache.Touch(ctx, pin(2).InstanceID, pin(3).InstanceID)
		putNew(t, ctx, cache, pin(4))
		putNew(t, ctx, cache, pin(5))

		// Our explicit Close() should remove 0 and 1 because they were ==
		// launchTime, and we didn't actually touch them after the sync.
		cache.Close(ctx)

		want := make([]string, 0, 4)
		for pinNum := 2; pinNum < 6; pinNum++ {
			want = append(want, pin(pinNum).InstanceID)
		}
		slices.Sort(want)

		// `true` forces resync to actual on-disk state.
		got, err := cache.AllInstanceIDs(ctx, true)
		assert.NoErr(t, err)
		assert.That(t, got, should.Match(want))
	})

	t.Run(`nil Fetcher`, func(t *testing.T) {
		t.Parallel()
		cache, _, ctx := instanceCacheTestSetup(t)
		cache.Fetcher = nil

		_, _, err := cache.OpenAsSource(ctx, "", pin(0))
		assert.ErrIsLike(t, err, ErrNoFetcher)
	})

	t.Run(`DisableGC`, func(t *testing.T) {
		t.Parallel()
		cache, env, ctx := instanceCacheTestSetup(t)
		cache.ExactGC = true

		// Launch the cache.
		cache.OpenAsSource(ctx, "", common.Pin{})
		env.tc.Add(time.Millisecond)

		putNew(t, ctx, cache, pin(0))
		putNew(t, ctx, cache, pin(1))
		putNew(t, ctx, cache, pin(2))

		cache.Close(ctx)

		assert.That(t, countTempFiles(t, cache), should.Equal(4)) // 1+ for state.db.

		cache.PassiveWritePolicy = DisableGC

		env.tc.Add(time.Minute)

		// Relaunch + Close/GC again.
		cache.OpenAsSource(ctx, "", common.Pin{})
		cache.Close(ctx)

		// We still have all GC'able instances.
		assert.That(t, countTempFiles(t, cache), should.Equal(4)) // 1+ for state.db.

		cache.PassiveWritePolicy = 0

		// Relaunch + Close/GC again.
		cache.OpenAsSource(ctx, "", common.Pin{})
		cache.Close(ctx)

		assert.That(t, countTempFiles(t, cache), should.Equal(1)) // Everything is gone.
	})

	t.Run(`DisableStateWrite`, func(t *testing.T) {
		t.Parallel()
		cache, env, ctx := instanceCacheTestSetup(t)
		putNew(t, ctx, cache, pin(0))

		assert.That(t, countTempFiles(t, cache), should.Equal(2)) // +1 for state.db.

		assert.NoErr(t, os.Remove(filepath.Join(env.tempDir, instanceCacheStateFilename)))

		assert.That(t, countTempFiles(t, cache), should.Equal(1)) // Just the instance.

		// Now disable writing state.
		cache.PassiveWritePolicy = DisableStateWrite

		// We still have the instance.
		testHas(t, ctx, cache, pin(0))

		// But we didn't write the state.db as a side effect.
		assert.That(t, countTempFiles(t, cache), should.Equal(1))
	})

	t.Run(`AllInstanceIDs`, func(t *testing.T) {
		t.Parallel()
		cache, _, ctx := instanceCacheTestSetup(t)
		want := make([]string, 8)
		for i := range 8 {
			putNew(t, ctx, cache, pin(i))
			want[i] = pin(i).InstanceID
		}
		slices.Sort(want)

		allIDs, err := cache.AllInstanceIDs(ctx, false)
		assert.NoErr(t, err)
		assert.That(t, allIDs, should.Match(want))
	})

	t.Run(`AllInstanceIDs (sync)`, func(t *testing.T) {
		t.Parallel()
		cache, _, ctx := instanceCacheTestSetup(t)
		want := make([]string, 8)
		for i := range 4 {
			putNew(t, ctx, cache, pin(i))
			want[i] = pin(i).InstanceID
		}

		// Add some 'extra' instance IDs.
		for i := 4; i < 8; i++ {
			iid := pin(i).InstanceID
			want[i] = iid
			cache.FS.EnsureFile(ctx, filepath.Join(cache.FS.Root(), iid), func(f *os.File) error {
				return nil
			})
		}
		slices.Sort(want)

		allIDs, err := cache.AllInstanceIDs(ctx, false)
		assert.NoErr(t, err)
		assert.That(t, allIDs, should.Match(want[:4]))

		// Forcing a re-sync sees the extra IDs.
		allIDs, err = cache.AllInstanceIDs(ctx, true)
		assert.NoErr(t, err)
		assert.That(t, allIDs, should.Match(want))
	})
}

func TestManagedInstanceCache(t *testing.T) {
	t.Parallel()

	waitInstance := func(t testing.TB, cache *ManagedInstanceCache, cb func(res *InstanceResult) (corrupted bool)) {
		t.Helper()

		res := cache.WaitInstance()
		corrupted := false
		defer func() {
			if res.Source != nil {
				assert.Loosely(t, res.Source.Close(t.Context(), corrupted), should.BeNil, truth.LineContext())
			}
			if res.Instance != nil {
				assert.Loosely(t, res.Instance.Close(t.Context(), corrupted), should.BeNil, truth.LineContext())
			}
		}()
		corrupted = cb(res)
	}

	mkReqs := func(ctx context.Context, N int) []*InstanceRequest {
		var reqs []*InstanceRequest
		for i := range N {
			reqs = append(reqs, &InstanceRequest{
				Context: ctx,
				Pin:     pin(i),
				State:   i,
			})
		}
		return reqs
	}

	managedTestEnv := func(t *testing.T) (*ManagedInstanceCache, *instCacheTestEnv, context.Context) {
		cache, env, ctx := instanceCacheTestSetup(t)
		ret := &ManagedInstanceCache{Caches: []*InstanceCache{cache}}
		t.Cleanup(func() {
			ret.Close(ctx)
		})
		return ret, env, ctx
	}

	t.Run("Preserves the order when using single stream", func(t *testing.T) {
		t.Parallel()
		cache, _, ctx := managedTestEnv(t)
		cache.ParallelDownloads = 1

		reqs := mkReqs(ctx, 100)

		cache.RequestInstances(ctx, reqs)
		for i := range reqs {
			waitInstance(t, cache, func(res *InstanceResult) (corrupted bool) {
				assert.Loosely(t, res.Err, should.BeNil)
				assert.Loosely(t, res.State.(int), should.Equal(i))

				data, err := fakeData(ctx, pin(i))
				assert.NoErr(t, err)
				assert.Loosely(t, readSrc(t, res.Source), should.Equal(data))
				return
			})
		}
	})

	t.Run("Doesn't deadlock", func(t *testing.T) {
		t.Parallel()
		cache, _, ctx := managedTestEnv(t)
		cache.ParallelDownloads = 4

		reqs := mkReqs(ctx, 100)

		seen := map[int]struct{}{}

		cache.RequestInstances(ctx, reqs)
		for range reqs {
			waitInstance(t, cache, func(res *InstanceResult) (corrupted bool) {
				assert.Loosely(t, res.Err, should.BeNil)
				seen[res.State.(int)] = struct{}{}
				return
			})
		}

		assert.Loosely(t, len(seen), should.Equal(len(reqs)))
	})

	t.Run("Handles errors", func(t *testing.T) {
		t.Parallel()
		cache, env, ctx := managedTestEnv(t)
		cache.ParallelDownloads = 4

		reqs := mkReqs(ctx, 100)

		// Make errCount fetches fail and rest succeed.
		const errCount = 10
		env.fetcher.errFn = func(fetchID int32) error {
			if fetchID <= errCount {
				return fmt.Errorf("boom %d", fetchID)
			}
			return nil
		}

		cache.RequestInstances(ctx, reqs)

		errs := 0
		for range reqs {
			waitInstance(t, cache, func(res *InstanceResult) (corrupted bool) {
				if res.Err != nil {
					errs++
				}
				return
			})
		}
		assert.Loosely(t, errs, should.Equal(errCount))
	})

	t.Run("RequestInstancesDoesNotEvictEntriesToBeFetched", func(t *testing.T) {
		t.Parallel()
		cache, env, ctx := managedTestEnv(t)
		env.cache.GCMaxAge = 2 * time.Second
		for i := range 8 {
			putNew(t, ctx, env.cache, pin(i))
		}
		// At this point, 8 fetches have been done.
		assert.Loosely(t, env.fetcher.calls.Load(), should.Equal(8))

		env.tc.Add(3 * time.Second)

		// All of the cache entries are now older than cache.maxAge,
		// but we are fetching them again, so they should not be
		// evicted.
		var ir []*InstanceRequest
		// A new download will trigger gc
		ir = append(ir, &InstanceRequest{Context: ctx, Pin: pin(10)})
		for i := range 8 {
			ir = append(ir, &InstanceRequest{Context: ctx, Pin: pin(i)})
		}
		cache.RequestInstances(ctx, ir)

		for range 9 {
			waitInstance(t, cache, func(res *InstanceResult) (corrupted bool) {
				assert.Loosely(t, res.Err, should.BeNil)
				return
			})
		}

		// Only one new fetch should happen, for pin 10.
		assert.Loosely(t, env.fetcher.calls.Load(), should.Equal(9))
	})

	t.Run(`VerifyHash`, func(t *testing.T) {
		t.Parallel()
		cache, env, ctx := managedTestEnv(t)
		pin, _, err := mkContentPin(ctx, "hello")
		assert.NoErr(t, err)

		putNew(t, ctx, env.cache, pin)
		env.cache.Fetcher = nil

		cache.RequestInstances(ctx, []*InstanceRequest{
			{Context: ctx, Pin: pin, OpenAs: VerifiedInstance},
		})
		waitInstance(t, cache, func(res *InstanceResult) (corrupted bool) {
			assert.NoErr(t, res.Err)
			return
		})

		// Now corrupt the file.
		assert.NoErr(t, env.cache.FS.EnsureFile(ctx, filepath.Join(env.cache.FS.Root(), pin.InstanceID), func(f *os.File) error {
			_, err := fmt.Fprintf(f, "I AM A BANANA")
			return err
		}))

		// Now we will see the cache corruption.
		cache.RequestInstances(ctx, []*InstanceRequest{
			{Context: ctx, Pin: pin, OpenAs: VerifiedInstance},
		})
		waitInstance(t, cache, func(res *InstanceResult) (corrupted bool) {
			assert.ErrIsLike(t, res.Err, reader.ErrHashMismatch)
			return
		})

		// And the file has been deleted.
		assert.That(t, countTempFiles(t, env.cache), should.Equal(1)) // +1 for state.db.
	})

	t.Run(`DisableCorruptDeletion`, func(t *testing.T) {
		t.Parallel()
		cache, env, ctx := managedTestEnv(t)
		env.cache.PassiveWritePolicy = DisableCorruptDeletion
		pin, _, err := mkContentPin(ctx, "hello")
		assert.NoErr(t, err)

		putNew(t, ctx, env.cache, pin)
		env.cache.Fetcher = nil

		// Now corrupt the file.
		assert.NoErr(t, env.cache.FS.EnsureFile(ctx, filepath.Join(env.cache.FS.Root(), pin.InstanceID), func(f *os.File) error {
			_, err := fmt.Fprintf(f, "I AM A BANANA")
			return err
		}))

		// Now we will see the cache corruption.
		cache.RequestInstances(ctx, []*InstanceRequest{
			{Context: ctx, Pin: pin, OpenAs: VerifiedInstance},
		})
		waitInstance(t, cache, func(res *InstanceResult) (corrupted bool) {
			assert.ErrIsLike(t, res.Err, reader.ErrHashMismatch)
			return
		})

		// But the file is still there!
		assert.That(t, countTempFiles(t, env.cache), should.Equal(2)) // +1 for state.db.
	})

	t.Run(`Chained`, func(t *testing.T) {
		t.Parallel()

		cache, env, ctx := managedTestEnv(t)

		readOnlyCache := &InstanceCache{
			FS:      fs.NewFileSystem(t.TempDir(), ""),
			Fetcher: (&trackingFetcher{}).fetch,
		}

		putNew(t, ctx, readOnlyCache, pin(0))
		putNew(t, ctx, readOnlyCache, pin(3))
		readOnlyCache.Fetcher = nil
		readOnlyCache.PassiveWritePolicy = DisablePassiveWrites

		cache.Caches = []*InstanceCache{readOnlyCache, env.cache}

		// For now, also disable fetches in the default read/write cache.
		env.cache.Fetcher = nil

		cache.RequestInstances(ctx, []*InstanceRequest{
			{Context: ctx, Pin: pin(0)},
			{Context: ctx, Pin: pin(1)},
			{Context: ctx, Pin: pin(2)},
			{Context: ctx, Pin: pin(3)},
		})
		for range 4 {
			waitInstance(t, cache, func(res *InstanceResult) (corrupted bool) {
				if res.Pin == pin(0) || res.Pin == pin(3) {
					assert.NoErr(t, res.Err)
				} else {
					assert.ErrIsLike(t, res.Err, ErrNoFetcher)
				}
				return
			})
		}

		// Re-enable the fetcher, and request something not in the chained cache.
		env.cache.Fetcher = env.fetcher.fetch

		// We see it!
		cache.RequestInstances(ctx, []*InstanceRequest{
			{Context: ctx, Pin: pin(8)},
		})
		waitInstance(t, cache, func(res *InstanceResult) (corrupted bool) {
			assert.NoErr(t, res.Err)
			assert.That(t, res.Pin, should.Match(pin(8)))
			return
		})

		// But it is in `cache`, not `readOnlyCache`.
		testHas(t, ctx, env.cache, pin(8))

		_, exists := accessTime(ctx, readOnlyCache, pin(8))
		assert.That(t, exists, should.BeFalse, truth.Explain("pin %q exists in readOnlyCache", pin(8)))
	})
}

func TestInstanceCacheSync(t *testing.T) {
	t.Parallel()

	testSync := func(t testing.TB, causeResync func(stateDbPath string)) {
		cache, env, ctx := instanceCacheTestSetup(t)
		const count = 10

		// Add instances.
		for i := range count {
			putNew(t, ctx, cache, pin(i))
		}

		causeResync(filepath.Join(env.tempDir, instanceCacheStateFilename))

		// state.db must be restored.
		for i := range count {
			lastAccess, ok := accessTime(ctx, cache, pin(i))
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, lastAccess, should.Match(clock.Now(ctx)))
		}

		_, ok := accessTime(ctx, cache, common.Pin{
			PackageName: "nonexistent",
			InstanceID:  "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		})
		assert.Loosely(t, ok, should.BeFalse)
	}

	t.Run("state.db disappeared", func(t *testing.T) {
		t.Parallel()
		testSync(t, func(stateDbPath string) {
			err := os.Remove(stateDbPath)
			assert.Loosely(t, err, should.BeNil)
		})
	})

	t.Run("state.db corrupted", func(t *testing.T) {
		t.Parallel()
		testSync(t, func(stateDbPath string) {
			f, err := os.Create(stateDbPath)
			assert.Loosely(t, err, should.BeNil)
			f.WriteString("blah")
			defer f.Close()
		})
	})
}

func TestPassiveWritePolicyMisc(t *testing.T) {
	t.Parallel()

	all := []PassiveWritePolicy{DisableGC, DisableCorruptDeletion, DisableStateWrite}

	cases := []struct {
		name    string
		policy  PassiveWritePolicy
		wantStr string
		has     []PassiveWritePolicy
	}{
		{
			name: "empty",
		},
		{
			name:    "DisableGC",
			policy:  DisableGC,
			wantStr: "no-gc",
			has:     []PassiveWritePolicy{DisableGC},
		},
		{
			name:    "DisableCorruptDeletion",
			policy:  DisableCorruptDeletion,
			wantStr: "no-corrupt-cleanup",
			has:     []PassiveWritePolicy{DisableCorruptDeletion},
		},
		{
			name:    "DisableStateWrite",
			policy:  DisableStateWrite,
			wantStr: "no-state-write",
			has:     []PassiveWritePolicy{DisableStateWrite},
		},
		{
			name:    "all",
			policy:  DisablePassiveWrites,
			wantStr: "no-gc|no-corrupt-cleanup|no-state-write",
			has:     all,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			check.That(t, tc.policy.String(), should.Equal(tc.wantStr))
			for _, has := range tc.has {
				check.That(t, tc.policy.has(has), should.BeTrue, truth.Explain("missing %q", has))
			}
			for _, notHas := range all {
				if !slices.Contains(tc.has, notHas) {
					check.That(t, tc.policy.has(notHas), should.BeFalse, truth.Explain("has extra %q", notHas))
				}
			}
		})
	}
}
