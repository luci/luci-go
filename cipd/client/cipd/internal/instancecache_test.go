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
		cache := NewInstanceCache(fs)
		cache.maxSize = testInstanceCacheMaxSize

		pin := func(i int) common.Pin {
			pin := common.Pin{"pkg", fmt.Sprintf("%d", i)}
			pin.InstanceID = strings.Repeat("a", 40-len(pin.InstanceID)) + pin.InstanceID
			return pin
		}

		access := func(cache *InstanceCache, pin common.Pin, data string) (created bool, src pkg.Source) {
			alloc := cache.Allocate(ctx, pin)
			defer alloc.Release(ctx)

			var err error
			src, err = alloc.Realize(ctx, func(f *os.File) error {
				created = true
				_, err := f.WriteString(data)
				return err
			})
			So(err, ShouldBeNil)

			return
		}

		putNew := func(cache *InstanceCache, pin common.Pin, data string) {
			created, src := access(cache, pin, data)
			So(created, ShouldBeTrue)
			So(src.Close(ctx, false), ShouldBeNil)
		}

		testHas := func(cache *InstanceCache, pin common.Pin, data string) {
			created, src := access(cache, pin, data)
			So(created, ShouldBeFalse)
			buf, err := ioutil.ReadAll(io.NewSectionReader(src, 0, src.Size()))
			So(err, ShouldBeNil)
			So(string(buf), ShouldEqual, data)
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

		Convey("Works in general", func() {
			cache2 := NewInstanceCache(fs)
			cache2.maxSize = testInstanceCacheMaxSize

			// Add new.
			putNew(cache, pin(0), "blah")

			// Check it can be seen even through another InstanceCache object.
			testHas(cache, pin(0), "blah")
			testHas(cache2, pin(0), "blah")
		})

		Convey("Concurrent access", func() {
			const concurrency = 20
			const fakeData = "zzz"

			p := pin(0)

			state := make([]struct {
				alloc  *Allocation
				src    pkg.Source
				err    error
				called bool
			}, concurrency)

			// Try to "collide" concurrent Realize calls.
			wg := sync.WaitGroup{}
			wg.Add(concurrency)
			realizeErrCh := make(chan error)
			for i := 0; i < concurrency; i++ {
				i := i
				alloc := cache.Allocate(ctx, p)
				state[i].alloc = alloc
				go func() {
					defer wg.Done()
					called := false
					src, err := alloc.Realize(ctx, func(f *os.File) error {
						called = true
						if err := <-realizeErrCh; err != nil {
							return err
						}
						_, err := f.WriteString("zzz")
						return err
					})
					state[i].src, state[i].err, state[i].called = src, err, called
				}()
			}

			// Created only one alloc.
			So(cache.allocs, ShouldHaveLength, 1)

			execute := func(realizeErr error) {
				realizeErrCh <- realizeErr
				close(realizeErrCh)
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
				called := 0
				for _, s := range state {
					So(s.err, ShouldBeNil)
					if s.called {
						called++
					}
					buf, err := ioutil.ReadAll(io.NewSectionReader(s.src, 0, s.src.Size()))
					So(err, ShouldBeNil)
					So(string(buf), ShouldEqual, fakeData)
				}
				So(called, ShouldEqual, 1)

				// Double-close is fine.
				So(state[0].src.Close(ctx, false), ShouldBeNil)
				So(state[0].src.Close(ctx, false), ShouldEqual, os.ErrClosed)

				closeAll()

				// The file is still in the cache.
				testHas(cache, pin(0), fakeData)
			})

			Convey("Error", func() {
				var boomErr = fmt.Errorf("boom")

				execute(boomErr)
				defer closeAll()

				// All failed, but only one actually was executing the fetch.
				called := 0
				for _, s := range state {
					So(s.err, ShouldEqual, boomErr)
					So(s.src, ShouldBeNil)
					if s.called {
						called++
					}
				}
				So(called, ShouldEqual, 1)
			})

			Convey("Corruption", func() {
				execute(nil)

				// Close the source as corrupted at least through one allocation.
				So(state[0].src.Close(ctx, true), ShouldBeNil)
				closeAll()

				// The file was removed from the cache and we can recreate it now.
				putNew(cache, pin(0), fakeData)
			})
		})

		Convey("GC respects MaxSize", func() {
			// Add twice more the limit.
			for i := 0; i < testInstanceCacheMaxSize*2; i++ {
				putNew(cache, pin(i), "blah")
				tc.Add(time.Second)
			}

			// Check the number of actual files.
			tempDirFile, err := os.Open(tempDir)
			So(err, ShouldBeNil)

			files, err := tempDirFile.Readdirnames(0)
			So(err, ShouldBeNil)
			So(files, ShouldHaveLength, testInstanceCacheMaxSize+1) // 1 for state.db

			// Only last testInstanceCacheMaxSize instances are still in the cache.
			for i := testInstanceCacheMaxSize; i < testInstanceCacheMaxSize*2; i++ {
				testHas(cache, pin(i), "blah")
			}
			// The rest are missing and can be recreated.
			for i := 0; i < testInstanceCacheMaxSize; i++ {
				putNew(cache, pin(i), "blah")
			}
		})

		Convey("GC respects MaxAge", func() {
			cache.maxAge = 2500 * time.Millisecond
			for i := 0; i < 8; i++ {
				if i != 0 {
					tc.Add(time.Second)
				}
				putNew(cache, pin(i), "blah")
			}

			// Age of last added item (i == 7) is 0 => age of i'th item is 7-i.
			//
			// Condition for survival: age < cache.maxAge, e.g 7-i<2.5 => i >= 5.
			//
			// Thus we expect {5, 6, 7} to still be in the cache after the GC.
			cache.GC(ctx)
			testHas(cache, pin(5), "blah")
			testHas(cache, pin(6), "blah")
			testHas(cache, pin(7), "blah")

			// The rest are missing and can be recreated.
			for i := 0; i < 5; i++ {
				putNew(cache, pin(i), "blah")
			}
		})

		Convey("Sync", func() {
			stateDbPath := filepath.Join(tempDir, instanceCacheStateFilename)
			const count = 10

			testSync := func(causeResync func()) {
				// Add instances.
				for i := 0; i < count; i++ {
					putNew(cache, pin(i), "blah")
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
