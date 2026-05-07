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
	"strings"
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipd/client/cipd/builder"
	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/internal/messages"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/cipd/client/cipd/reader"
	"go.chromium.org/luci/cipd/common"
)

func TestInstanceCache(t *testing.T) {
	t.Parallel()

	ftt.Run("InstanceCache", t, func(t *ftt.Test) {
		ctx, tc := testclock.UseTime(t.Context(), testclock.TestTimeLocal)

		tempDir, err := os.MkdirTemp("", "instanceche_test")
		assert.Loosely(t, err, should.BeNil)
		defer os.RemoveAll(tempDir)
		fsInst := fs.NewFileSystem(tempDir, "")

		pin := func(i int) common.Pin {
			pin := common.Pin{PackageName: "pkg", InstanceID: fmt.Sprintf("%d", i)}
			pin.InstanceID = strings.Repeat("a", 40-len(pin.InstanceID)) + pin.InstanceID
			return pin
		}

		mkContent := func(data string) (common.Pin, string) {
			buf := bytes.NewBuffer(nil)
			pin, err := builder.BuildInstance(ctx, builder.Options{
				Input:       []fs.File{fs.NewTestFile("file", data, fs.TestFileOpts{})},
				Output:      buf,
				PackageName: fmt.Sprintf("pkg/content/%s", data),
			})
			assert.NoErr(t, err)
			return pin, buf.String()
		}

		contentPin := func(c string) common.Pin {
			ret, _ := mkContent(c)
			return ret
		}

		fakeData := func(p common.Pin) string {
			if data, ok := strings.CutPrefix(p.PackageName, "pkg/content/"); ok {
				actualPin, inst := mkContent(data)
				assert.That(t, p, should.Match(actualPin))
				return inst
			}
			return "data:" + p.InstanceID
		}

		var fetchM sync.Mutex
		var fetchErr chan error
		var fetchCalls int

		fetcher := func(ctx context.Context, pin common.Pin, w io.WriteSeeker) error {
			fetchM.Lock()
			fetchCalls++
			fetchErrCh := fetchErr
			fetchM.Unlock()
			if fetchErrCh != nil {
				if err := <-fetchErrCh; err != nil {
					return err
				}
			}
			_, err := w.Write([]byte(fakeData(pin)))
			return err
		}

		cache := &InstanceCache{
			FS:      fsInst,
			Fetcher: fetcher,
		}
		defer cache.Close(ctx)

		// withInstance waits for an instance, and calls the callback with it.
		//
		// Closes the source/instance in the result after the callback, using
		// `corrupted` when calling Close.
		withInstance := func(cache *InstanceCache, cb func(res *InstanceResult) (corrupted bool)) {
			res := cache.WaitInstance()
			corrupted := false
			defer func() {
				if res.Source != nil {
					assert.Loosely(t, res.Source.Close(ctx, corrupted), should.BeNil)
				}
				if res.Instance != nil {
					assert.Loosely(t, res.Instance.Close(ctx, corrupted), should.BeNil)
				}
			}()
			corrupted = cb(res)
		}

		// access requests the instance, waits for it, asserts that the result has
		// no error, then calls `cb`.
		//
		// If `created` is true, it means the fetcher was called between the
		// request and the response.
		//
		// Closes the source/instance in the result after the callback, using
		// `corrupted` when calling Close.
		access := func(cache *InstanceCache, pin common.Pin, cb func(created bool, res *InstanceResult) (corrupted bool)) {
			t.Helper()

			fetchM.Lock()
			before := fetchCalls
			fetchM.Unlock()

			cache.RequestInstances(ctx, []*InstanceRequest{
				{Context: ctx, Pin: pin},
			})

			withInstance(cache, func(res *InstanceResult) (corrupted bool) {
				fetchM.Lock()
				created := fetchCalls > before
				fetchM.Unlock()

				assert.NoErr(t, res.Err, truth.LineContext())
				return cb(created, res)
			})
		}

		putNew := func(cache *InstanceCache, pin common.Pin) {
			t.Helper()

			access(cache, pin, func(created bool, res *InstanceResult) (corrupted bool) {
				assert.Loosely(t, created, should.BeTrue, truth.LineContext(), truth.Explain(
					"failed to create %v", pin))
				return
			})
		}

		readSrc := func(src pkg.Source) string {
			t.Helper()

			buf, err := io.ReadAll(io.NewSectionReader(src, 0, src.Size()))
			assert.NoErr(t, err, truth.LineContext())
			return string(buf)
		}

		testHas := func(cache *InstanceCache, pin common.Pin) {
			t.Helper()

			access(cache, pin, func(created bool, res *InstanceResult) (corrupted bool) {
				assert.Loosely(t, created, should.BeFalse, truth.LineContext())
				assert.Loosely(t, readSrc(res.Source), should.Equal(fakeData(pin)), truth.LineContext())
				return
			})
		}

		accessTime := func(cache *InstanceCache, pin common.Pin) (lastAccess time.Time, ok bool) {
			cache.withState(ctx, clock.Now(ctx), false, func(s *messages.InstanceCache) (save bool) {
				var entry *messages.InstanceCache_Entry
				if entry, ok = s.Entries[pin.InstanceID]; ok {
					lastAccess = entry.LastAccess.AsTime()
				}
				return false
			})
			return
		}

		countTempFiles := func() int {
			tempDirFile, err := os.Open(tempDir)
			assert.Loosely(t, err, should.BeNil)
			defer tempDirFile.Close()
			files, err := tempDirFile.Readdirnames(0)
			assert.Loosely(t, err, should.BeNil)
			return len(files)
		}

		t.Run("Works in general", func(t *ftt.Test) {
			cache2 := &InstanceCache{FS: fsInst, Fetcher: fetcher}
			defer cache2.Close(ctx)

			// Add new.
			putNew(cache, pin(0))

			// Check it can be seen even through another InstanceCache object.
			testHas(cache, pin(0))
			testHas(cache2, pin(0))
		})

		t.Run("Temp cache removes files", func(t *ftt.Test) {
			cache := &InstanceCache{
				FS:      fsInst,
				Tmp:     true,
				Fetcher: fetcher,
			}
			defer cache.Close(ctx)

			assert.That(t, countTempFiles(), should.Equal(0))
			access(cache, pin(0), func(created bool, res *InstanceResult) (corrupted bool) {
				assert.That(t, countTempFiles(), should.Equal(1))
				return
			})
			assert.That(t, countTempFiles(), should.Equal(0))
		})

		t.Run("Redownloads corrupted files", func(t *ftt.Test) {
			assert.Loosely(t, countTempFiles(), should.BeZero)

			// Download the first time.
			access(cache, pin(0), func(created bool, res *InstanceResult) (corrupted bool) {
				assert.That(t, created, should.BeTrue)
				return
			})

			// Stored in the cache (plus state.db file).
			assert.Loosely(t, countTempFiles(), should.Equal(2))

			// The second call grabs it from the cache.
			access(cache, pin(0), func(created bool, res *InstanceResult) (corrupted bool) {
				assert.That(t, created, should.BeFalse)

				// Close as corrupted. Should be removed from the cache.
				corrupted = true
				return
			})

			// Only state.db file left.
			assert.Loosely(t, countTempFiles(), should.Equal(1))

			// Download the second time.
			access(cache, pin(0), func(created bool, res *InstanceResult) (corrupted bool) {
				assert.Loosely(t, fetchCalls, should.Equal(2))
				return
			})
		})

		t.Run("Concurrency", func(t *ftt.Test) {
			cache := &InstanceCache{
				FS:      fsInst,
				Fetcher: fetcher,
			}
			defer cache.Close(ctx)

			var reqs []*InstanceRequest
			for i := range 100 {
				reqs = append(reqs, &InstanceRequest{
					Context: ctx,
					Pin:     pin(i),
					State:   i,
				})
			}

			t.Run("Preserves the order when using single stream", func(t *ftt.Test) {
				cache.ParallelDownloads = 1

				cache.RequestInstances(ctx, reqs)
				for i := 0; i < len(reqs); i++ {
					withInstance(cache, func(res *InstanceResult) (corrupted bool) {
						assert.Loosely(t, res.Err, should.BeNil)
						assert.Loosely(t, res.State.(int), should.Equal(i))
						assert.Loosely(t, readSrc(res.Source), should.Equal(fakeData(pin(i))))
						return
					})
				}
			})

			t.Run("Doesn't deadlock", func(t *ftt.Test) {
				cache.ParallelDownloads = 4

				seen := map[int]struct{}{}

				cache.RequestInstances(ctx, reqs)
				for i := 0; i < len(reqs); i++ {
					withInstance(cache, func(res *InstanceResult) (corrupted bool) {
						assert.Loosely(t, res.Err, should.BeNil)
						seen[res.State.(int)] = struct{}{}
						return
					})
				}

				assert.Loosely(t, len(seen), should.Equal(len(reqs)))
			})

			t.Run("Handles errors", func(t *ftt.Test) {
				fetchErr = make(chan error, len(reqs))

				cache.ParallelDownloads = 4

				cache.RequestInstances(ctx, reqs)

				// Make errCount fetches fail and rest succeed.
				const errCount = 10
				for i := range errCount {
					fetchErr <- fmt.Errorf("boom %d", i)
				}
				for i := errCount; i < len(reqs); i++ {
					fetchErr <- nil
				}

				errs := 0
				for i := 0; i < len(reqs); i++ {
					withInstance(cache, func(res *InstanceResult) (corrupted bool) {
						if res.Err != nil {
							errs++
						}
						return
					})
				}
				assert.Loosely(t, errs, should.Equal(errCount))
			})
		})

		t.Run("GC respects MaxAge vs launchTime", func(t *ftt.Test) {
			cache.maxAge = 2500 * time.Millisecond
			t0 := clock.Now(ctx)
			for i := range 8 {
				if i != 0 {
					tc.Add(time.Second)
				}
				putNew(cache, pin(i))
			}
			assert.That(t, cache.launchTime, should.Match(t0))

			// This should remove *nothing* - all instances were added after our
			// launch time.
			cache.Close(ctx) // Does a GC pass.

			// We use cache.withState directly to ensure it has all entries, but to
			// avoid 'touching' files, which testHas() would do.
			cache.withState(ctx, tc.Now(), false, func(ic *messages.InstanceCache) (save bool) {
				assert.Loosely(t, ic.Entries, should.HaveLength(8))
				return false
			})

			// Relaunch the cache and close it to do another GC pass with launchTime
			// == clock.Now.
			cache.RequestInstances(ctx, nil)
			assert.That(t, cache.launchTime, should.Match(clock.Now(ctx)))
			cache.Close(ctx)

			// At this point 0..4 have been collected, so 5..7 exist.
			assert.That(t, countTempFiles(), should.Equal(4)) // +1 for state.db.
			for i := range 3 {
				testHas(cache, pin(i+5))
			}

			// The rest are missing and can be recreated.
			for i := range 5 {
				putNew(cache, pin(i))
			}
		})

		t.Run("RequestInstancesDoesNotEvictEntriesToBeFetched", func(t *ftt.Test) {
			cache.maxAge = 2 * time.Second
			for i := range 8 {
				putNew(cache, pin(i))
			}
			// At this point, 8 fetches have been done.
			assert.Loosely(t, fetchCalls, should.Equal(8))

			tc.Add(3 * time.Second)

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
				withInstance(cache, func(res *InstanceResult) (corrupted bool) {
					assert.Loosely(t, res.Err, should.BeNil)
					return
				})
			}

			// Only one new fetch should happen, for pin 10.
			assert.Loosely(t, fetchCalls, should.Equal(9))
		})

		t.Run("Sync", func(t *ftt.Test) {
			stateDbPath := filepath.Join(tempDir, instanceCacheStateFilename)
			const count = 10

			testSync := func(causeResync func()) {
				// Add instances.
				for i := range count {
					putNew(cache, pin(i))
				}

				causeResync()

				// state.db must be restored.
				for i := range count {
					lastAccess, ok := accessTime(cache, pin(i))
					assert.Loosely(t, ok, should.BeTrue)
					assert.Loosely(t, lastAccess, should.Match(clock.Now(ctx)))
				}

				_, ok := accessTime(cache, common.Pin{
					PackageName: "nonexistent",
					InstanceID:  "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				})
				assert.Loosely(t, ok, should.BeFalse)
			}

			t.Run("state.db disappeared", func(t *ftt.Test) {
				testSync(func() {
					err := os.Remove(stateDbPath)
					assert.Loosely(t, err, should.BeNil)
				})
			})

			t.Run("state.db corrupted", func(t *ftt.Test) {
				testSync(func() {
					f, err := os.Create(stateDbPath)
					assert.Loosely(t, err, should.BeNil)
					f.WriteString("blah")
					defer f.Close()
				})
			})
		})

		t.Run(`ExactGC`, func(t *ftt.Test) {
			cache.ExactGC = true

			// Ensure the cache is launched.
			cache.RequestInstances(ctx, nil)

			// Advance time a bit so all these pins initially start with a time >
			// launchTime.
			tc.Add(time.Minute)

			// Add a bunch of instances.
			putNew(cache, pin(0))
			putNew(cache, pin(1))
			putNew(cache, pin(2))
			putNew(cache, pin(3))

			// Delete the state file to force resync. All these instances should now
			// be marked as === launchTime.
			assert.NoErr(t, os.Remove(filepath.Join(tempDir, instanceCacheStateFilename)))

			// Access some overlapping instances and add some new instances.
			testHas(cache, pin(2))
			testHas(cache, pin(3))
			putNew(cache, pin(4))
			putNew(cache, pin(5))

			// Our explicit Close() should remove 0 and 1 because they were ==
			// launchTime, and we didn't actually touch them after the sync.
			cache.Close(ctx)

			want := map[string]struct{}{}
			for pinNum := 2; pinNum < 6; pinNum++ {
				want[pin(pinNum).InstanceID] = struct{}{}
			}

			// The only instances left will be the overlapping + new instances.
			// We use cache.withState directly to ensure it has all entries, but to
			// avoid 'touching' files, which testHas() would do.
			cache.withState(ctx, tc.Now(), false, func(ic *messages.InstanceCache) (save bool) {
				assert.Loosely(t, ic.Entries, should.HaveLength(4))
				got := map[string]struct{}{}
				for iid := range ic.Entries {
					got[iid] = struct{}{}
				}
				assert.That(t, got, should.Match(want))
				return false
			})
		})

		t.Run(`nil Fetcher`, func(t *ftt.Test) {
			cache.Fetcher = nil

			cache.RequestInstances(ctx, []*InstanceRequest{
				{Context: ctx, Pin: pin(0)},
			})

			withInstance(cache, func(res *InstanceResult) (corrupted bool) {
				assert.ErrIsLike(t, res.Err, ErrNoFetcher)
				return
			})
		})

		t.Run(`VerifyHash`, func(t *ftt.Test) {
			pin := contentPin("hello")

			putNew(cache, pin)
			cache.Fetcher = nil

			cache.RequestInstances(ctx, []*InstanceRequest{
				{Context: ctx, Pin: pin, OpenAs: VerifiedInstance},
			})
			withInstance(cache, func(res *InstanceResult) (corrupted bool) {
				assert.NoErr(t, res.Err)
				return
			})

			// Now corrupt the file.
			assert.NoErr(t, cache.FS.EnsureFile(ctx, filepath.Join(cache.FS.Root(), pin.InstanceID), func(f *os.File) error {
				_, err := fmt.Fprintf(f, "I AM A BANANA")
				return err
			}))

			// Now we will see the cache corruption.
			cache.RequestInstances(ctx, []*InstanceRequest{
				{Context: ctx, Pin: pin, OpenAs: VerifiedInstance},
			})
			withInstance(cache, func(res *InstanceResult) (corrupted bool) {
				assert.ErrIsLike(t, res.Err, reader.ErrHashMismatch)
				return
			})
		})
	})
}
