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

			cache.RequestInstances([]*InstanceRequest{
				{Context: ctx, Pin: pin},
			})
			res := cache.WaitInstance()
			So(res.Err, ShouldBeNil)

			fetchM.Lock()
			created = fetchCalls > before
			fetchM.Unlock()

			return created, res.Source
		}

		putNew := func(cache *InstanceCache, pin common.Pin) {
			created, src := access(cache, pin)
			So(created, ShouldBeTrue)
			So(src.Close(ctx, false), ShouldBeNil)
		}

		readSrc := func(src pkg.Source) string {
			buf, err := io.ReadAll(io.NewSectionReader(src, 0, src.Size()))
			So(err, ShouldBeNil)
			return string(buf)
		}

		testHas := func(cache *InstanceCache, pin common.Pin) {
			created, src := access(cache, pin)
			So(created, ShouldBeFalse)
			So(readSrc(src), ShouldEqual, fakeData(pin))
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
			cache2 := &InstanceCache{FS: fs, Fetcher: fetcher}
			cache2.Launch(ctx)
			defer cache2.Close(ctx)

			// Add new.
			putNew(cache, pin(0))

			// Check it can be seen even through another InstanceCache object.
			testHas(cache, pin(0))
			testHas(cache2, pin(0))
		})

		Convey("Temp cache removes files", func() {
			cache := &InstanceCache{
				FS:      fs,
				Tmp:     true,
				Fetcher: fetcher,
			}
			cache.Launch(ctx)
			defer cache.Close(ctx)

			So(countTempFiles(), ShouldEqual, 0)

			cache.RequestInstances([]*InstanceRequest{
				{Context: ctx, Pin: pin(0)},
			})
			res := cache.WaitInstance()
			So(res.Err, ShouldBeNil)

			So(countTempFiles(), ShouldEqual, 1)
			So(res.Source.Close(ctx, false), ShouldBeNil)
			So(countTempFiles(), ShouldEqual, 0)
		})

		Convey("Redownloads corrupted files", func() {
			So(countTempFiles(), ShouldEqual, 0)

			// Download the first time.
			cache.RequestInstances([]*InstanceRequest{
				{Context: ctx, Pin: pin(0)},
			})
			res := cache.WaitInstance()
			So(res.Err, ShouldBeNil)
			So(fetchCalls, ShouldEqual, 1)
			So(res.Source.Close(ctx, false), ShouldBeNil)

			// Stored in the cache (plus state.db file).
			So(countTempFiles(), ShouldEqual, 2)

			// The second call grabs it from the cache.
			cache.RequestInstances([]*InstanceRequest{
				{Context: ctx, Pin: pin(0)},
			})
			res = cache.WaitInstance()
			So(res.Err, ShouldBeNil)
			So(fetchCalls, ShouldEqual, 1)

			// Close as corrupted. Should be removed from the cache.
			So(res.Source.Close(ctx, true), ShouldBeNil)

			// Only state.db file left.
			So(countTempFiles(), ShouldEqual, 1)

			// Download the second time.
			cache.RequestInstances([]*InstanceRequest{
				{Context: ctx, Pin: pin(0)},
			})
			res = cache.WaitInstance()
			So(res.Err, ShouldBeNil)
			So(fetchCalls, ShouldEqual, 2)
			So(res.Source.Close(ctx, false), ShouldBeNil)
		})

		Convey("Concurrency", func() {
			cache := &InstanceCache{
				FS:      fs,
				Fetcher: fetcher,
			}
			defer cache.Close(ctx)

			var reqs []*InstanceRequest
			for i := 0; i < 100; i++ {
				reqs = append(reqs, &InstanceRequest{
					Context: ctx,
					Pin:     pin(i),
					State:   i,
				})
			}

			Convey("Preserves the order when using single stream", func() {
				cache.ParallelDownloads = 1
				cache.Launch(ctx)

				cache.RequestInstances(reqs)
				for i := 0; i < len(reqs); i++ {
					res := cache.WaitInstance()
					So(res.Err, ShouldBeNil)
					So(res.State.(int), ShouldEqual, i)
					So(readSrc(res.Source), ShouldEqual, fakeData(pin(i)))
					So(res.Source.Close(ctx, false), ShouldBeNil)
				}
			})

			Convey("Doesn't deadlock", func() {
				cache.ParallelDownloads = 4
				cache.Launch(ctx)

				seen := map[int]struct{}{}

				cache.RequestInstances(reqs)
				for i := 0; i < len(reqs); i++ {
					res := cache.WaitInstance()
					So(res.Err, ShouldBeNil)
					So(res.Source.Close(ctx, false), ShouldBeNil)
					seen[res.State.(int)] = struct{}{}
				}

				So(len(seen), ShouldEqual, len(reqs))
			})

			Convey("Handles errors", func() {
				fetchErr = make(chan error, len(reqs))

				cache.ParallelDownloads = 4
				cache.Launch(ctx)

				cache.RequestInstances(reqs)

				// Make errCount fetches fail and rest succeed.
				const errCount = 10
				for i := 0; i < errCount; i++ {
					fetchErr <- fmt.Errorf("boom %d", i)
				}
				for i := errCount; i < len(reqs); i++ {
					fetchErr <- nil
				}

				errs := 0
				for i := 0; i < len(reqs); i++ {
					res := cache.WaitInstance()
					if res.Source != nil {
						So(res.Source.Close(ctx, false), ShouldBeNil)
					}
					if res.Err != nil {
						errs++
					}
				}
				So(errs, ShouldEqual, errCount)
			})
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
