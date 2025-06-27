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
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/internal/messages"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/cipd/common"
)

// No need to create a lot of files in tests.
const testInstanceCacheMaxSize = 10

func TestInstanceCache(t *testing.T) {
	t.Parallel()

	ftt.Run("InstanceCache", t, func(t *ftt.Test) {
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestTimeLocal)

		tempDir, err := os.MkdirTemp("", "instanceche_test")
		assert.Loosely(t, err, should.BeNil)
		defer os.RemoveAll(tempDir)
		fs := fs.NewFileSystem(tempDir, "")

		pin := func(i int) common.Pin {
			pin := common.Pin{"pkg", fmt.Sprintf("%d", i)}
			pin.InstanceID = strings.Repeat("a", 40-len(pin.InstanceID)) + pin.InstanceID
			return pin
		}

		fakeData := func(p common.Pin) string {
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
			FS:      fs,
			Fetcher: fetcher,
			maxSize: testInstanceCacheMaxSize,
		}
		cache.Launch(ctx)
		defer cache.Close(ctx)

		access := func(cache *InstanceCache, pin common.Pin) (created bool, src pkg.Source) {
			fetchM.Lock()
			before := fetchCalls
			fetchM.Unlock()

			cache.RequestInstances(ctx, []*InstanceRequest{
				{Context: ctx, Pin: pin},
			})
			res := cache.WaitInstance()
			assert.Loosely(t, res.Err, should.BeNil)

			fetchM.Lock()
			created = fetchCalls > before
			fetchM.Unlock()

			return created, res.Source
		}

		putNew := func(cache *InstanceCache, pin common.Pin) {
			created, src := access(cache, pin)
			assert.Loosely(t, created, should.BeTrue)
			assert.Loosely(t, src.Close(ctx, false), should.BeNil)
		}

		readSrc := func(src pkg.Source) string {
			buf, err := io.ReadAll(io.NewSectionReader(src, 0, src.Size()))
			assert.Loosely(t, err, should.BeNil)
			return string(buf)
		}

		testHas := func(cache *InstanceCache, pin common.Pin) {
			created, src := access(cache, pin)
			assert.Loosely(t, created, should.BeFalse)
			assert.Loosely(t, readSrc(src), should.Equal(fakeData(pin)))
			assert.Loosely(t, src.Close(ctx, false), should.BeNil)
		}

		accessTime := func(cache *InstanceCache, pin common.Pin) (lastAccess time.Time, ok bool) {
			cache.withState(ctx, clock.Now(ctx), func(s *messages.InstanceCache) (save bool) {
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
			cache2 := &InstanceCache{FS: fs, Fetcher: fetcher}
			cache2.Launch(ctx)
			defer cache2.Close(ctx)

			// Add new.
			putNew(cache, pin(0))

			// Check it can be seen even through another InstanceCache object.
			testHas(cache, pin(0))
			testHas(cache2, pin(0))
		})

		t.Run("Temp cache removes files", func(t *ftt.Test) {
			cache := &InstanceCache{
				FS:      fs,
				Tmp:     true,
				Fetcher: fetcher,
			}
			cache.Launch(ctx)
			defer cache.Close(ctx)

			assert.Loosely(t, countTempFiles(), should.BeZero)

			cache.RequestInstances(ctx, []*InstanceRequest{
				{Context: ctx, Pin: pin(0)},
			})
			res := cache.WaitInstance()
			assert.Loosely(t, res.Err, should.BeNil)

			assert.Loosely(t, countTempFiles(), should.Equal(1))
			assert.Loosely(t, res.Source.Close(ctx, false), should.BeNil)
			assert.Loosely(t, countTempFiles(), should.BeZero)
		})

		t.Run("Redownloads corrupted files", func(t *ftt.Test) {
			assert.Loosely(t, countTempFiles(), should.BeZero)

			// Download the first time.
			cache.RequestInstances(ctx, []*InstanceRequest{
				{Context: ctx, Pin: pin(0)},
			})
			res := cache.WaitInstance()
			assert.Loosely(t, res.Err, should.BeNil)
			assert.Loosely(t, fetchCalls, should.Equal(1))
			assert.Loosely(t, res.Source.Close(ctx, false), should.BeNil)

			// Stored in the cache (plus state.db file).
			assert.Loosely(t, countTempFiles(), should.Equal(2))

			// The second call grabs it from the cache.
			cache.RequestInstances(ctx, []*InstanceRequest{
				{Context: ctx, Pin: pin(0)},
			})
			res = cache.WaitInstance()
			assert.Loosely(t, res.Err, should.BeNil)
			assert.Loosely(t, fetchCalls, should.Equal(1))

			// Close as corrupted. Should be removed from the cache.
			assert.Loosely(t, res.Source.Close(ctx, true), should.BeNil)

			// Only state.db file left.
			assert.Loosely(t, countTempFiles(), should.Equal(1))

			// Download the second time.
			cache.RequestInstances(ctx, []*InstanceRequest{
				{Context: ctx, Pin: pin(0)},
			})
			res = cache.WaitInstance()
			assert.Loosely(t, res.Err, should.BeNil)
			assert.Loosely(t, fetchCalls, should.Equal(2))
			assert.Loosely(t, res.Source.Close(ctx, false), should.BeNil)
		})

		t.Run("Concurrency", func(t *ftt.Test) {
			cache := &InstanceCache{
				FS:      fs,
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
				cache.Launch(ctx)

				cache.RequestInstances(ctx, reqs)
				for i := 0; i < len(reqs); i++ {
					res := cache.WaitInstance()
					assert.Loosely(t, res.Err, should.BeNil)
					assert.Loosely(t, res.State.(int), should.Equal(i))
					assert.Loosely(t, readSrc(res.Source), should.Equal(fakeData(pin(i))))
					assert.Loosely(t, res.Source.Close(ctx, false), should.BeNil)
				}
			})

			t.Run("Doesn't deadlock", func(t *ftt.Test) {
				cache.ParallelDownloads = 4
				cache.Launch(ctx)

				seen := map[int]struct{}{}

				cache.RequestInstances(ctx, reqs)
				for i := 0; i < len(reqs); i++ {
					res := cache.WaitInstance()
					assert.Loosely(t, res.Err, should.BeNil)
					assert.Loosely(t, res.Source.Close(ctx, false), should.BeNil)
					seen[res.State.(int)] = struct{}{}
				}

				assert.Loosely(t, len(seen), should.Equal(len(reqs)))
			})

			t.Run("Handles errors", func(t *ftt.Test) {
				fetchErr = make(chan error, len(reqs))

				cache.ParallelDownloads = 4
				cache.Launch(ctx)

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
					res := cache.WaitInstance()
					if res.Source != nil {
						assert.Loosely(t, res.Source.Close(ctx, false), should.BeNil)
					}
					if res.Err != nil {
						errs++
					}
				}
				assert.Loosely(t, errs, should.Equal(errCount))
			})
		})

		t.Run("GC respects MaxSize", func(t *ftt.Test) {
			// Add twice more the limit.
			for i := range testInstanceCacheMaxSize * 2 {
				putNew(cache, pin(i))
				tc.Add(time.Second)
			}

			// Check the number of actual files.
			assert.Loosely(t, countTempFiles(), should.Equal(testInstanceCacheMaxSize+1)) // +1 for state.db

			// Only last testInstanceCacheMaxSize instances are still in the cache.
			for i := testInstanceCacheMaxSize; i < testInstanceCacheMaxSize*2; i++ {
				testHas(cache, pin(i))
			}
			// The rest are missing and can be recreated.
			for i := range testInstanceCacheMaxSize {
				putNew(cache, pin(i))
			}
		})

		t.Run("GC respects MaxAge", func(t *ftt.Test) {
			cache.maxAge = 2500 * time.Millisecond
			for i := range 8 {
				if i != 0 {
					tc.Add(time.Second)
				}
				putNew(cache, pin(i))
			}

			// Age of last added item (i == 7) is 0 => age of i'th item is 7-i.
			//
			// Condition for survival: age < cache.maxAge, e.g. 7-i<2.5 => i >= 5.
			//
			// Thus we expect {5, 6, 7} to still be in the cache after the GC.
			cache.GC(ctx)
			testHas(cache, pin(5))
			testHas(cache, pin(6))
			testHas(cache, pin(7))

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
				res := cache.WaitInstance()
				assert.Loosely(t, res.Err, should.BeNil)
				assert.Loosely(t, res.Source.Close(ctx, false), should.BeNil)
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
					assert.Loosely(t, lastAccess.UnixNano(), should.Equal(clock.Now(ctx).UnixNano()))
				}

				_, ok := accessTime(cache, common.Pin{"nonexistent", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"})
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
	})
}
