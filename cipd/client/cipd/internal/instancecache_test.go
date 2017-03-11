// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package internal

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/cipd/client/cipd/common"
	"github.com/luci/luci-go/cipd/client/cipd/local"

	. "github.com/smartystreets/goconvey/convey"
)

// No need to create a lot of files in tests.
const testInstanceCacheMaxSize = 10

func TestInstanceCache(t *testing.T) {
	ctx := context.Background()

	Convey("InstanceCache", t, func(c C) {
		tempDir, err := ioutil.TempDir("", "instanceche_test")
		So(err, ShouldBeNil)
		defer os.RemoveAll(tempDir)

		now := time.Date(2016, 1, 2, 3, 4, 5, 6, time.UTC)

		fs := local.NewFileSystem(tempDir, "")
		cache := NewInstanceCache(fs)
		cache.maxSize = testInstanceCacheMaxSize

		put := func(cache *InstanceCache, pin common.Pin, data string) {
			err = cache.Put(ctx, pin, now, func(f *os.File) error {
				_, err := f.WriteString(data)
				So(err, ShouldBeNil)
				return nil
			})
			So(err, ShouldBeNil)
		}

		testHas := func(cache *InstanceCache, pin common.Pin, data string) {
			r, err := cache.Get(ctx, pin, now)
			So(err, ShouldBeNil)
			buf, err := ioutil.ReadAll(r)
			So(err, ShouldBeNil)
			So(string(buf), ShouldEqual, data)
			So(r.Close(), ShouldBeNil)
		}

		Convey("Works", func() {
			cache2 := NewInstanceCache(fs)
			cache2.maxSize = testInstanceCacheMaxSize

			pin := common.Pin{"pkg", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
			r, err := cache.Get(ctx, pin, now)
			So(os.IsNotExist(err), ShouldBeTrue)
			So(r, ShouldBeNil)

			// Add new.
			put(cache, pin, "blah")
			testHas(cache, pin, "blah")
			testHas(cache2, pin, "blah")

			// Replace existing.
			put(cache, pin, "huh")
			testHas(cache, pin, "huh")
			testHas(cache2, pin, "huh")
		})

		pini := func(i int) common.Pin {
			pin := common.Pin{"pkg", fmt.Sprintf("%d", i)}
			pin.InstanceID = strings.Repeat("a", 40-len(pin.InstanceID)) + pin.InstanceID
			return pin
		}

		Convey("GC respects MaxSize", func() {
			// Add twice more the limit.
			for i := 0; i < testInstanceCacheMaxSize*2; i++ {
				put(cache, pini(i), "blah")
				now = now.Add(time.Second)
			}

			// Check the number of actual files.
			tempDirFile, err := os.Open(tempDir)
			So(err, ShouldBeNil)

			files, err := tempDirFile.Readdirnames(0)
			So(err, ShouldBeNil)
			So(files, ShouldHaveLength, testInstanceCacheMaxSize+1) // 1 for state.db

			// Try to get.
			for i := 0; i < testInstanceCacheMaxSize*2; i++ {
				r, err := cache.Get(ctx, pini(i), now)
				if r != nil {
					r.Close()
				}
				So(os.IsNotExist(err), ShouldEqual, i < testInstanceCacheMaxSize)
			}
		})

		Convey("GC respects MaxAge", func() {
			cache.maxAge = 2500 * time.Millisecond
			for i := 0; i < 8; i++ {
				if i != 0 {
					now = now.Add(time.Second)
				}
				put(cache, pini(i), "blah")
			}

			// Age of last added item (i == 7) is 0 => age of i'th item is 7-i.
			//
			// Condition for survival: age < cache.maxAge, e.g 7-i<2.5 => i >= 5.
			//
			// Thus we expect {5, 6, 7} to still be in the cache after the GC.
			cache.GC(ctx, now)

			alive := []int{}
			for i := 0; i < 8; i++ {
				r, _ := cache.Get(ctx, pini(i), now)
				if r != nil {
					r.Close()
					alive = append(alive, i)
				}
			}
			So(alive, ShouldResemble, []int{5, 6, 7})
		})

		Convey("Sync", func() {
			stateDbPath := filepath.Join(tempDir, instanceCacheStateFilename)
			const count = 10

			testSync := func(causeResync func()) {
				// Add instances.
				for i := 0; i < count; i++ {
					put(cache, pini(i), "blah")
				}

				causeResync()

				// state.db must be restored.
				for i := 0; i < count; i++ {
					lastAccess, ok := cache.getAccessTime(ctx, now, pini(i))
					So(ok, ShouldBeTrue)
					So(lastAccess, ShouldResemble, now)
				}

				_, ok := cache.getAccessTime(
					ctx, now, common.Pin{"nonexistent", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"})
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
