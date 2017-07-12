// Copyright 2017 The LUCI Authors.
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
	"fmt"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"

	"github.com/luci/luci-go/common/clock"
	. "github.com/smartystreets/goconvey/convey"
)

func TestMemoryTokenCache(t *testing.T) {
	t.Parallel()

	Convey("MemoryTokenCache works", t, func() {
		testCacheSemantics(context.Background(), &MemoryTokenCache{})
	})

	Convey("MemoryTokenCache works (parallel)", t, func() {
		testCacheInParallel(context.Background(), &MemoryTokenCache{})
	})
}

func testCacheSemantics(ctx context.Context, cache TokenCache) {
	// Missing is fine.
	tok, err := cache.GetToken(&CacheKey{Key: "missing"})
	So(err, ShouldBeNil)
	So(tok, ShouldBeNil)

	// Deleting missing is fine.
	So(cache.DeleteToken(&CacheKey{Key: "missing"}), ShouldBeNil)

	// Put -> Get.
	tok1 := &oauth2.Token{
		AccessToken: "abc",
		Expiry:      clock.Now(ctx).Add(time.Hour).UTC(),
	}
	So(cache.PutToken(&CacheKey{Key: "k"}, tok1), ShouldBeNil)
	tok, err = cache.GetToken(&CacheKey{Key: "k"})
	So(err, ShouldBeNil)
	So(tok, ShouldResemble, tok1)

	// Put a bunch more.
	for i := 0; i < 5; i++ {
		So(cache.PutToken(&CacheKey{Key: fmt.Sprintf("k%d", i)}, tok1), ShouldBeNil)
	}

	// Overwrite -> Get.
	tok2 := &oauth2.Token{
		AccessToken: "def",
		Expiry:      clock.Now(ctx).Add(2 * time.Hour).UTC(),
	}
	So(cache.PutToken(&CacheKey{Key: "k"}, tok2), ShouldBeNil)
	tok, err = cache.GetToken(&CacheKey{Key: "k"})
	So(err, ShouldBeNil)
	So(tok, ShouldResemble, tok2)

	// Delete -> Get.
	So(cache.DeleteToken(&CacheKey{Key: "k"}), ShouldBeNil)
	tok, err = cache.GetToken(&CacheKey{Key: "k"})
	So(err, ShouldBeNil)
	So(tok, ShouldBeNil)

	// The rest is still there. Remove it.
	for i := 0; i < 5; i++ {
		k := &CacheKey{Key: fmt.Sprintf("k%d", i)}
		tok, err = cache.GetToken(k)
		So(err, ShouldBeNil)
		So(tok, ShouldResemble, tok1)
		So(cache.DeleteToken(k), ShouldBeNil)
	}
}

func testCacheInParallel(ctx context.Context, cache TokenCache) {
	tok := &oauth2.Token{
		AccessToken: "zzz",
		Expiry:      clock.Now(ctx).Add(365 * 24 * time.Hour),
	}

	const (
		threads  = 30
		attempts = 15
	)

	// There are no guarantees in general that parallel writes to the cache will
	// not lose some data (see DiskTokenCache comments). But each op should be
	// successful, even if its results are later silently discarded.
	wg := sync.WaitGroup{}
	errs := make(chan error, threads*attempts)
	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < attempts; j++ {
				err := cache.PutToken(&CacheKey{Key: fmt.Sprintf("%d", j)}, tok)
				if err != nil {
					errs <- err
				}
			}
		}()
	}
	wg.Wait()

	// The channel with errors should be empty.
	select {
	case err := <-errs:
		So(err, ShouldBeNil)
	default:
	}
}
