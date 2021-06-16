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
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/internal/messages"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
)

// No need to create a lot of files in tests.
const testInstanceCacheMaxSize = 10

func TestInstanceCache(t *testing.T) {
	t.Parallel()

	Convey("InstanceCache", t, func() {
		ctx := context.Background()
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestTimeLocal)

		tempDir, err := ioutil.TempDir("", "instanceche_test")
		So(err, ShouldBeNil)
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

		cache := NewInstanceCache(fs, false, func(ctx context.Context, pin common.Pin, w io.WriteSeeker) error {
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
		})
		cache.maxSize = testInstanceCacheMaxSize

		access := func(cache *InstanceCache, pin common.Pin) (created bool, src pkg.Source) {
			alloc := cache.Allocate(ctx, pin)
			defer alloc.Release(ctx)

			fetchM.Lock()
			before := fetchCalls
			fetchM.Unlock()

			src, err := alloc.Realize(ctx)
			So(err, ShouldBeNil)

			fetchM.Lock()
			created = fetchCalls > before
			fetchM.Unlock()

			return created, src
		}

		putNew := func(cache *InstanceCache, pin common.Pin) {
			created, src := access(cache, pin)
			So(created, ShouldBeTrue)
			So(src.Close(ctx, false), ShouldBeNil)
		}

		testHas := func(cache *InstanceCache, pin common.Pin) {
			created, src := access(cache, pin)
			So(created, ShouldBeFalse)
			buf, err := ioutil.ReadAll(io.NewSectionReader(src, 0, src.Size()))
			So(err, ShouldBeNil)
			So(string(buf), ShouldEqual, fakeData(pin))
			So(src.Close(ctx, false), ShouldBeNil)
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
			So(err, ShouldBeNil)
			defer tempDirFile.Close()
			files, err := tempDirFile.Readdirnames(0)
			So(err, ShouldBeNil)
			return len(files)
		}

		Convey("Works in general", func() {
			cache2 := NewInstanceCache(fs, false, nil)
			cache2.maxSize = testInstanceCacheMaxSize

			// Add new.
			putNew(cache, pin(0))

			// Check it can be seen even through another InstanceCache object.
			testHas(cache, pin(0))
			testHas(cache2, pin(0))
		})

		Convey("Concurrent access", func() {
			const concurrency = 20

			p := pin(0)

			state := make([]struct {
				alloc *Allocation
				src   pkg.Source
				err   error
			}, concurrency)

			// Try to "collide" concurrent Realize calls.
			wg := sync.WaitGroup{}
			wg.Add(concurrency)
			fetchErr = make(chan error)
			for i := 0; i < concurrency; i++ {
				i := i
				alloc := cache.Allocate(ctx, p)
				state[i].alloc = alloc
				go func() {
					defer wg.Done()
					state[i].src, state[i].err = alloc.Realize(ctx)
				}()
			}

			// Created only one alloc.
			So(cache.allocs, ShouldHaveLength, 1)

			execute := func(realizeErr error) {
				fetchErr <- realizeErr
				close(fetchErr)
				wg.Wait()
			}

			closeAll := func() {
				// Close everything, but do not release allocations yet.
				for _, s := range state {
					if s.src != nil {
						s.src.Close(ctx, false)
					}
				}
				// Still holding to the alloc since have references.
				So(cache.allocs, ShouldHaveLength, 1)
				// Release all references.
				for _, s := range state {
					s.alloc.Release(ctx)
				}
				// The alloc is deleted for real.
				So(cache.allocs, ShouldHaveLength, 0)
			}

			Convey("Success", func() {
				execute(nil)

				// None failed, but only one was actually creating the file.
				So(fetchCalls, ShouldEqual, 1)
				for _, s := range state {
					So(s.err, ShouldBeNil)
					buf, err := ioutil.ReadAll(io.NewSectionReader(s.src, 0, s.src.Size()))
					So(err, ShouldBeNil)
					So(string(buf), ShouldEqual, fakeData(p))
				}

				// Double-close is fine.
				So(state[0].src.Close(ctx, false), ShouldBeNil)
				So(state[0].src.Close(ctx, false), ShouldEqual, os.ErrClosed)

				closeAll()

				// The file is still in the cache.
				testHas(cache, pin(0))
			})

			Convey("Error", func() {
				var boomErr = fmt.Errorf("boom")

				execute(boomErr)
				defer closeAll()

				// All failed, but only one actually was executing the fetch.
				So(fetchCalls, ShouldEqual, 1)
				for _, s := range state {
					So(s.err, ShouldEqual, boomErr)
					So(s.src, ShouldBeNil)
				}
			})

			Convey("Corruption", func() {
				execute(nil)

				// Close the source as corrupted at least through one allocation.
				So(state[0].src.Close(ctx, true), ShouldBeNil)
				closeAll()

				// The file was removed from the cache and we can recreate it now.
				putNew(cache, pin(0))
			})
		})

		Convey("Temp cache removes files", func() {
			cache := NewInstanceCache(fs, true, func(ctx context.Context, pin common.Pin, w io.WriteSeeker) error {
				_, err := w.Write([]byte("blah"))
				return err
			})

			alloc1 := cache.Allocate(ctx, pin(0))
			alloc2 := cache.Allocate(ctx, pin(0))

			// Nothing yet.
			So(countTempFiles(), ShouldEqual, 0)

			src, err := alloc1.Realize(ctx)
			So(err, ShouldBeNil)

			// A cached file appeared.
			So(countTempFiles(), ShouldEqual, 1)

			// Closing the file doesn't delete it yet, since we have allocs for it.
			So(src.Close(ctx, false), ShouldBeNil)
			So(countTempFiles(), ShouldEqual, 1)

			// Still have the file since alloc2 may still be wanting it.
			alloc1.Release(ctx)
			So(countTempFiles(), ShouldEqual, 1)

			// The cached file is gone after alloc2 referring it is closed.
			alloc2.Release(ctx)
			So(countTempFiles(), ShouldEqual, 0)
		})

		Convey("GC respects MaxSize", func() {
			// Add twice more the limit.
			for i := 0; i < testInstanceCacheMaxSize*2; i++ {
				putNew(cache, pin(i))
				tc.Add(time.Second)
			}

			// Check the number of actual files.
			So(countTempFiles(), ShouldEqual, testInstanceCacheMaxSize+1) // +1 for state.db

			// Only last testInstanceCacheMaxSize instances are still in the cache.
			for i := testInstanceCacheMaxSize; i < testInstanceCacheMaxSize*2; i++ {
				testHas(cache, pin(i))
			}
			// The rest are missing and can be recreated.
			for i := 0; i < testInstanceCacheMaxSize; i++ {
				putNew(cache, pin(i))
			}
		})

		Convey("GC respects MaxAge", func() {
			cache.maxAge = 2500 * time.Millisecond
			for i := 0; i < 8; i++ {
				if i != 0 {
					tc.Add(time.Second)
				}
				putNew(cache, pin(i))
			}

			// Age of last added item (i == 7) is 0 => age of i'th item is 7-i.
			//
			// Condition for survival: age < cache.maxAge, e.g 7-i<2.5 => i >= 5.
			//
			// Thus we expect {5, 6, 7} to still be in the cache after the GC.
			cache.GC(ctx)
			testHas(cache, pin(5))
			testHas(cache, pin(6))
			testHas(cache, pin(7))

			// The rest are missing and can be recreated.
			for i := 0; i < 5; i++ {
				putNew(cache, pin(i))
			}
		})

		Convey("Sync", func() {
			stateDbPath := filepath.Join(tempDir, instanceCacheStateFilename)
			const count = 10

			testSync := func(causeResync func()) {
				// Add instances.
				for i := 0; i < count; i++ {
					putNew(cache, pin(i))
				}

				causeResync()

				// state.db must be restored.
				for i := 0; i < count; i++ {
					lastAccess, ok := accessTime(cache, pin(i))
					So(ok, ShouldBeTrue)
					So(lastAccess.UnixNano(), ShouldEqual, clock.Now(ctx).UnixNano())
				}

				_, ok := accessTime(cache, common.Pin{"nonexistent", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"})
				So(ok, ShouldBeFalse)
			}

			Convey("state.db disappeared", func() {
				testSync(func() {
					err := os.Remove(stateDbPath)
					So(err, ShouldBeNil)
				})
			})

			Convey("state.db corrupted", func() {
				testSync(func() {
					f, err := os.Create(stateDbPath)
					So(err, ShouldBeNil)
					f.WriteString("blah")
					defer f.Close()
				})
			})
		})
	})
}
