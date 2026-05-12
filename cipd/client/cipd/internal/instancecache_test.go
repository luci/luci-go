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
func (f *trackingFetcher) fetch(ctx context.Context, pin common.Pin, w io.WriteSeeker) error {
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

func withInstance(t testing.TB, cache *ManagedInstanceCache, cb func(res *InstanceResult) (corrupted bool)) {
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

// access does a RequestInstances(pin) followed by a withInstance.
//
// Asserts that InstanceResult does not have an error, and passes to the
// callback a boolean indicated if this instance was created as a result of
// this `access` or not.
func access(t testing.TB, ctx context.Context, cache *ManagedInstanceCache, pin common.Pin, cb func(created bool, res *InstanceResult) (corrupted bool)) {
	t.Helper()

	_, existed := accessTime(ctx, cache.Cache, pin)

	cache.RequestInstances(ctx, []*InstanceRequest{
		{Context: ctx, Pin: pin},
	})

	withInstance(t, cache, func(res *InstanceResult) (corrupted bool) {
		t.Helper()

		assert.NoErr(t, res.Err, truth.LineContext())
		assert.That(t, res.Pin, should.Match(pin), truth.LineContext())
		return cb(!existed, res)
	})
}

// putNew is an `access` plus an assertion that `pin` was created.
func putNew(t testing.TB, ctx context.Context, cache *ManagedInstanceCache, pin common.Pin) {
	t.Helper()

	access(t, ctx, cache, pin, func(created bool, res *InstanceResult) (corrupted bool) {
		t.Helper()

		assert.Loosely(t, created, should.BeTrue, truth.Explain(
			"failed to create %v", pin), truth.LineContext())
		return
	})
}

// readSrc reads the content of `src`
func readSrc(t testing.TB, src pkg.Source) string {
	t.Helper()

	buf, err := io.ReadAll(io.NewSectionReader(src, 0, src.Size()))
	assert.NoErr(t, err, truth.LineContext())
	return string(buf)
}

// testHas is an `access` plus an assertion that this pin was NOT created.
//
// Note that this *modifies* the underlying cache.
func testHas(t testing.TB, ctx context.Context, cache *ManagedInstanceCache, pin common.Pin) {
	t.Helper()

	access(t, ctx, cache, pin, func(created bool, res *InstanceResult) (corrupted bool) {
		t.Helper()

		assert.Loosely(t, created, should.BeFalse, truth.LineContext())
		data, err := fakeData(ctx, pin)
		assert.NoErr(t, err, truth.LineContext())
		assert.Loosely(t, readSrc(t, res.Source), should.Equal(data), truth.LineContext())
		return
	})
}

// accessTime returns the last access time of `pin` in the cache.
//
// If the cache does not contain `pin`, returns ok == false.
func accessTime(ctx context.Context, cache *InstanceCache, pin common.Pin) (lastAccess time.Time, ok bool) {
	cache.withState(ctx, clock.Now(ctx), false, func(s *messages.InstanceCache) (save bool) {
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
	cache   *ManagedInstanceCache
	tempDir string
	fetcher *trackingFetcher
	tc      testclock.TestClock
}

func instanceCacheTestSetup(t testing.TB) (*ManagedInstanceCache, *instCacheTestEnv, context.Context) {
	ctx, tc := testclock.UseTime(t.Context(), testclock.TestTimeLocal)

	tempDir, err := os.MkdirTemp("", "instancecache_test")
	assert.Loosely(t, err, should.BeNil)
	fsInst := fs.NewFileSystem(tempDir, "")

	fetcher := &trackingFetcher{}
	cache := &ManagedInstanceCache{
		Cache: &InstanceCache{
			FS:      fsInst,
			Fetcher: fetcher.fetch,
		},
	}
	t.Cleanup(func() {
		cache.Close(ctx)
		os.RemoveAll(tempDir)
	})

	return cache, &instCacheTestEnv{cache, tempDir, fetcher, tc}, ctx
}

func TestManagedInstanceCache(t *testing.T) {
	t.Parallel()

	t.Run("Works in general", func(t *testing.T) {
		cache, env, ctx := instanceCacheTestSetup(t)

		cache2 := &ManagedInstanceCache{Cache: &InstanceCache{
			FS:      fs.NewFileSystem(env.tempDir, ""),
			Fetcher: (&trackingFetcher{}).fetch,
		}}
		defer cache2.Close(ctx)

		// Add new.
		putNew(t, ctx, cache, pin(0))

		// Check it can be seen even through another ManagedInstanceCache object.
		testHas(t, ctx, cache, pin(0))
		testHas(t, ctx, cache2, pin(0))
	})

	t.Run("Temp cache removes files", func(t *testing.T) {
		cache, _, ctx := instanceCacheTestSetup(t)
		cache.Cache.Tmp = true

		assert.That(t, countTempFiles(t, cache.Cache), should.Equal(0))
		access(t, ctx, cache, pin(0), func(created bool, res *InstanceResult) (corrupted bool) {
			assert.That(t, countTempFiles(t, cache.Cache), should.Equal(1))
			return
		})
		assert.That(t, countTempFiles(t, cache.Cache), should.Equal(0))
	})

	t.Run("Redownloads corrupted files", func(t *testing.T) {
		cache, env, ctx := instanceCacheTestSetup(t)
		assert.Loosely(t, countTempFiles(t, cache.Cache), should.BeZero)

		// Download the first time.
		access(t, ctx, cache, pin(0), func(created bool, res *InstanceResult) (corrupted bool) {
			assert.That(t, created, should.BeTrue)
			return
		})

		// Stored in the cache (plus state.db file).
		assert.Loosely(t, countTempFiles(t, cache.Cache), should.Equal(2))

		// The second call grabs it from the cache.
		access(t, ctx, cache, pin(0), func(created bool, res *InstanceResult) (corrupted bool) {
			assert.That(t, created, should.BeFalse)

			// Close as corrupted. Should be removed from the cache.
			corrupted = true
			return
		})

		// Only state.db file left.
		assert.Loosely(t, countTempFiles(t, cache.Cache), should.Equal(1))

		// Download the second time.
		access(t, ctx, cache, pin(0), func(created bool, res *InstanceResult) (corrupted bool) {
			assert.Loosely(t, env.fetcher.calls.Load(), should.Equal(2))
			return
		})
	})

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

	t.Run("Preserves the order when using single stream", func(t *testing.T) {
		cache, _, ctx := instanceCacheTestSetup(t)
		cache.ParallelDownloads = 1

		reqs := mkReqs(ctx, 100)

		cache.RequestInstances(ctx, reqs)
		for i := 0; i < len(reqs); i++ {
			withInstance(t, cache, func(res *InstanceResult) (corrupted bool) {
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
		cache, _, ctx := instanceCacheTestSetup(t)
		cache.ParallelDownloads = 4

		reqs := mkReqs(ctx, 100)

		seen := map[int]struct{}{}

		cache.RequestInstances(ctx, reqs)
		for i := 0; i < len(reqs); i++ {
			withInstance(t, cache, func(res *InstanceResult) (corrupted bool) {
				assert.Loosely(t, res.Err, should.BeNil)
				seen[res.State.(int)] = struct{}{}
				return
			})
		}

		assert.Loosely(t, len(seen), should.Equal(len(reqs)))
	})

	t.Run("Handles errors", func(t *testing.T) {
		cache, env, ctx := instanceCacheTestSetup(t)
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
		for i := 0; i < len(reqs); i++ {
			withInstance(t, cache, func(res *InstanceResult) (corrupted bool) {
				if res.Err != nil {
					errs++
				}
				return
			})
		}
		assert.Loosely(t, errs, should.Equal(errCount))
	})

	t.Run("GC respects MaxAge vs launchTime", func(t *testing.T) {
		cache, env, ctx := instanceCacheTestSetup(t)
		cache.Cache.GCMaxAge = 2500 * time.Millisecond
		t0 := clock.Now(ctx)
		for i := range 8 {
			if i != 0 {
				env.tc.Add(time.Second)
			}
			putNew(t, ctx, cache, pin(i))
		}
		assert.That(t, cache.Cache.launchTime, should.Match(t0))

		// This should remove *nothing* - all instances were added after our
		// launch time.
		cache.Close(ctx) // Does a GC pass.

		// We use withState directly to ensure it has all entries, but to avoid
		// 'touching' files, which testHas() would do.
		cache.Cache.withState(ctx, env.tc.Now(), false, func(ic *messages.InstanceCache) (save bool) {
			assert.Loosely(t, ic.Entries, should.HaveLength(8))
			return false
		})

		// Relaunch the cache and close it to do another GC pass with launchTime
		// == clock.Now.
		cache.RequestInstances(ctx, nil)
		assert.That(t, cache.Cache.launchTime, should.Match(clock.Now(ctx)))
		cache.Close(ctx)

		// At this point 0..4 have been collected, so 5..7 exist.
		assert.That(t, countTempFiles(t, cache.Cache), should.Equal(4)) // +1 for state.db.
		for i := range 3 {
			testHas(t, ctx, cache, pin(i+5))
		}

		// The rest are missing and can be recreated.
		for i := range 5 {
			putNew(t, ctx, cache, pin(i))
		}
	})

	t.Run("RequestInstancesDoesNotEvictEntriesToBeFetched", func(t *testing.T) {
		cache, env, ctx := instanceCacheTestSetup(t)
		cache.Cache.GCMaxAge = 2 * time.Second
		for i := range 8 {
			putNew(t, ctx, cache, pin(i))
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
			withInstance(t, cache, func(res *InstanceResult) (corrupted bool) {
				assert.Loosely(t, res.Err, should.BeNil)
				return
			})
		}

		// Only one new fetch should happen, for pin 10.
		assert.Loosely(t, env.fetcher.calls.Load(), should.Equal(9))
	})

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
			lastAccess, ok := accessTime(ctx, cache.Cache, pin(i))
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, lastAccess, should.Match(clock.Now(ctx)))
		}

		_, ok := accessTime(ctx, cache.Cache, common.Pin{
			PackageName: "nonexistent",
			InstanceID:  "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		})
		assert.Loosely(t, ok, should.BeFalse)
	}

	t.Run("state.db disappeared", func(t *testing.T) {
		testSync(t, func(stateDbPath string) {
			err := os.Remove(stateDbPath)
			assert.Loosely(t, err, should.BeNil)
		})
	})

	t.Run("state.db corrupted", func(t *testing.T) {
		testSync(t, func(stateDbPath string) {
			f, err := os.Create(stateDbPath)
			assert.Loosely(t, err, should.BeNil)
			f.WriteString("blah")
			defer f.Close()
		})
	})

	t.Run(`ExactGC`, func(t *testing.T) {
		cache, env, ctx := instanceCacheTestSetup(t)
		cache.Cache.ExactGC = true

		// Ensure the cache is launched.
		cache.RequestInstances(ctx, nil)

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

		// Access some overlapping instances and add some new instances.
		testHas(t, ctx, cache, pin(2))
		testHas(t, ctx, cache, pin(3))
		putNew(t, ctx, cache, pin(4))
		putNew(t, ctx, cache, pin(5))

		// Our explicit Close() should remove 0 and 1 because they were ==
		// launchTime, and we didn't actually touch them after the sync.
		cache.Close(ctx)

		want := map[string]struct{}{}
		for pinNum := 2; pinNum < 6; pinNum++ {
			want[pin(pinNum).InstanceID] = struct{}{}
		}

		// The only instances left will be the overlapping + new instances. We
		// use withState directly to ensure it has all entries, but to avoid
		// 'touching' files, which testHas() would do.
		cache.Cache.withState(ctx, env.tc.Now(), false, func(ic *messages.InstanceCache) (save bool) {
			assert.Loosely(t, ic.Entries, should.HaveLength(4))
			got := map[string]struct{}{}
			for iid := range ic.Entries {
				got[iid] = struct{}{}
			}
			assert.That(t, got, should.Match(want))
			return false
		})
	})

	t.Run(`nil Fetcher`, func(t *testing.T) {
		cache, _, ctx := instanceCacheTestSetup(t)
		cache.Cache.Fetcher = nil

		cache.RequestInstances(ctx, []*InstanceRequest{
			{Context: ctx, Pin: pin(0)},
		})

		withInstance(t, cache, func(res *InstanceResult) (corrupted bool) {
			assert.ErrIsLike(t, res.Err, ErrNoFetcher)
			return
		})
	})

	t.Run(`VerifyHash`, func(t *testing.T) {
		cache, _, ctx := instanceCacheTestSetup(t)
		pin, _, err := mkContentPin(ctx, "hello")
		assert.NoErr(t, err)

		putNew(t, ctx, cache, pin)
		cache.Cache.Fetcher = nil

		cache.RequestInstances(ctx, []*InstanceRequest{
			{Context: ctx, Pin: pin, OpenAs: VerifiedInstance},
		})
		withInstance(t, cache, func(res *InstanceResult) (corrupted bool) {
			assert.NoErr(t, res.Err)
			return
		})

		// Now corrupt the file.
		assert.NoErr(t, cache.Cache.FS.EnsureFile(ctx, filepath.Join(cache.Cache.FS.Root(), pin.InstanceID), func(f *os.File) error {
			_, err := fmt.Fprintf(f, "I AM A BANANA")
			return err
		}))

		// Now we will see the cache corruption.
		cache.RequestInstances(ctx, []*InstanceRequest{
			{Context: ctx, Pin: pin, OpenAs: VerifiedInstance},
		})
		withInstance(t, cache, func(res *InstanceResult) (corrupted bool) {
			assert.ErrIsLike(t, res.Err, reader.ErrHashMismatch)
			return
		})

		// And the file has been deleted.
		assert.That(t, countTempFiles(t, cache.Cache), should.Equal(1)) // +1 for state.db.
	})

	t.Run(`DisableGC`, func(t *testing.T) {
		cache, env, ctx := instanceCacheTestSetup(t)
		cache.Cache.ExactGC = true

		// Launch the cache.
		cache.RequestInstances(ctx, nil)
		env.tc.Add(time.Millisecond)

		putNew(t, ctx, cache, pin(0))
		putNew(t, ctx, cache, pin(1))
		putNew(t, ctx, cache, pin(2))

		cache.Close(ctx)

		assert.That(t, countTempFiles(t, cache.Cache), should.Equal(4)) // 1+ for state.db.

		cache.Cache.PassiveWritePolicy = DisableGC

		env.tc.Add(time.Minute)

		// Relaunch + Close/GC again.
		cache.RequestInstances(ctx, nil) // Relaunch cache at `now`.
		cache.Close(ctx)

		// We still have all GC'able instances.
		assert.That(t, countTempFiles(t, cache.Cache), should.Equal(4)) // 1+ for state.db.

		cache.Cache.PassiveWritePolicy = 0

		// Relaunch + Close/GC again.
		cache.RequestInstances(ctx, nil) // Relaunch cache at `now`.
		cache.Close(ctx)

		assert.That(t, countTempFiles(t, cache.Cache), should.Equal(1)) // Everything is gone.
	})

	t.Run(`DisableCorruptDeletion`, func(t *testing.T) {
		cache, _, ctx := instanceCacheTestSetup(t)
		cache.Cache.PassiveWritePolicy = DisableCorruptDeletion
		pin, _, err := mkContentPin(ctx, "hello")
		assert.NoErr(t, err)

		putNew(t, ctx, cache, pin)
		cache.Cache.Fetcher = nil

		// Now corrupt the file.
		assert.NoErr(t, cache.Cache.FS.EnsureFile(ctx, filepath.Join(cache.Cache.FS.Root(), pin.InstanceID), func(f *os.File) error {
			_, err := fmt.Fprintf(f, "I AM A BANANA")
			return err
		}))

		// Now we will see the cache corruption.
		cache.RequestInstances(ctx, []*InstanceRequest{
			{Context: ctx, Pin: pin, OpenAs: VerifiedInstance},
		})
		withInstance(t, cache, func(res *InstanceResult) (corrupted bool) {
			assert.ErrIsLike(t, res.Err, reader.ErrHashMismatch)
			return
		})

		// But the file is still there!
		assert.That(t, countTempFiles(t, cache.Cache), should.Equal(2)) // +1 for state.db.
	})

	t.Run(`DisableStateWrite`, func(t *testing.T) {
		cache, env, ctx := instanceCacheTestSetup(t)
		putNew(t, ctx, cache, pin(0))

		assert.That(t, countTempFiles(t, cache.Cache), should.Equal(2)) // +1 for state.db.

		assert.NoErr(t, os.Remove(filepath.Join(env.tempDir, instanceCacheStateFilename)))

		assert.That(t, countTempFiles(t, cache.Cache), should.Equal(1)) // Just the instance.

		// Now disable writing state.
		cache.Cache.PassiveWritePolicy = DisableStateWrite

		// We still have the instance.
		testHas(t, ctx, cache, pin(0))

		// But we didn't write the state.db as a side effect.
		assert.That(t, countTempFiles(t, cache.Cache), should.Equal(1))
	})

	t.Run(`AllInstanceIDs`, func(t *testing.T) {
		cache, _, ctx := instanceCacheTestSetup(t)
		want := make([]string, 8)
		for i := range 8 {
			putNew(t, ctx, cache, pin(i))
			want[i] = pin(i).InstanceID
		}
		slices.Sort(want)

		allIDs, err := cache.Cache.AllInstanceIDs(ctx, false)
		assert.NoErr(t, err)
		assert.That(t, allIDs, should.Match(want))
	})

	t.Run(`AllInstanceIDs (sync)`, func(t *testing.T) {
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
			cache.Cache.FS.EnsureFile(ctx, filepath.Join(cache.Cache.FS.Root(), iid), func(f *os.File) error {
				return nil
			})
		}
		slices.Sort(want)

		allIDs, err := cache.Cache.AllInstanceIDs(ctx, false)
		assert.NoErr(t, err)
		assert.That(t, allIDs, should.Match(want[:4]))

		// Forcing a re-sync sees the extra IDs.
		allIDs, err = cache.Cache.AllInstanceIDs(ctx, true)
		assert.NoErr(t, err)
		assert.That(t, allIDs, should.Match(want))
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
