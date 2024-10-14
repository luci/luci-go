// Copyright 2022 The LUCI Authors.
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

package gaeemulation

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gomodule/redigo/redis"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/dscache"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

type testEntity struct {
	ID  int `gae:"$id"`
	Val int
}

func TestRedisCacheSmoke(t *testing.T) {
	t.Parallel()

	// TODO(crbug.com/1416970): This test is flaky.
	t.Skip()

	ftt.Run("Smoke test", t, func(c *ftt.Test) {
		ctx := context.Background()
		ctx = memory.Use(ctx)

		s, err := miniredis.Run()
		assert.Loosely(c, err, should.BeNil)
		defer func() {
			t.Logf("Total connections: %d", s.TotalConnectionCount())
			t.Logf("Current connections: %d", s.CurrentConnectionCount())
			s.Close()
		}()

		pool := &redis.Pool{
			MaxIdle:   64,
			MaxActive: 512,
			Wait:      true, // if all connections are busy, wait for an available one
			Dial: func() (redis.Conn, error) {
				return redis.Dial("tcp", s.Addr(),
					redis.DialConnectTimeout(time.Minute),
					redis.DialReadTimeout(time.Minute),
					redis.DialWriteTimeout(time.Minute),
				)
			},
		}
		defer pool.Close()

		ctx = dscache.FilterRDS(ctx, &redisCache{pool: pool})

		wg := sync.WaitGroup{}
		defer wg.Wait()

		const entitiesCount = 3

		entities := func() []testEntity {
			ents := make([]testEntity, entitiesCount)
			for i := 0; i < entitiesCount; i++ {
				ents[i].ID = i + 1
			}
			return ents
		}
		assert.Loosely(c, datastore.Put(ctx, entities()), should.BeNil)

		type state struct {
			last [entitiesCount]int
		}

		runManyParallel := func(cb func(*state)) {
			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					s := &state{}
					for j := 0; j < 200; j++ {
						cb(s)
					}
				}()
			}
		}

		// Reading entities in a loop and observing their value increase
		// monotonically.
		runManyParallel(func(s *state) {
			ents := entities()
			if err := datastore.Get(ctx, ents); err != nil {
				panic(err)
			}
			for i := 0; i < entitiesCount; i++ {
				if ents[i].Val < s.last[i] {
					panic(fmt.Sprintf("%d: %d < %d", i, ents[i].Val, s.last[i]))
				}
				s.last[i] = ents[i].Val
			}
			randomSleep(3 * time.Millisecond)
		})

		// Monotonically increasing entities in a transaction.
		runManyParallel(func(*state) {
			err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				ents := entities()
				if err := datastore.Get(ctx, ents); err != nil {
					return err
				}
				for i := 0; i < entitiesCount; i++ {
					ents[i].Val++
				}
				randomSleep(2 * time.Millisecond)
				return datastore.Put(ctx, ents)
			}, nil)
			if err != nil && err != datastore.ErrConcurrentTransaction {
				panic(err)
			}
		})
	})
}

func randomSleep(dur time.Duration) {
	time.Sleep(time.Duration(rand.Int63n(int64(dur))))
}
