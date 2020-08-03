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

package lhttp

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"golang.org/x/sync/semaphore"
	"golang.org/x/time/rate"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLimitRate(t *testing.T) {
	Convey(`LimitRate`, t, func() {
		count := 0
		var rt http.RoundTripper = RoundTripper(func(r *http.Request) (*http.Response, error) {
			count++
			return nil, nil
		})

		rt = LimitRate(rt, rate.NewLimiter(rate.Every(time.Millisecond), 1))

		start := time.Now()
		req := &http.Request{}
		for time.Since(start) < 100*time.Millisecond {
			rt.RoundTrip(req)
		}
		So(count, ShouldBeLessThanOrEqualTo, 120) // as opposed to a very large number.
	})
}

func TestLimitConcurrency(t *testing.T) {
	Convey(`LimitConcurrency`, t, func() {
		active := 0
		peak := 0
		var mu sync.Mutex
		var rt http.RoundTripper = RoundTripper(func(r *http.Request) (*http.Response, error) {
			mu.Lock()
			active++
			if active > peak {
				peak = active
			}
			mu.Unlock()

			time.Sleep(time.Millisecond)

			mu.Lock()
			active--
			mu.Unlock()

			return nil, nil
		})

		rt = LimitConcurrency(rt, semaphore.NewWeighted(10))
		var wg sync.WaitGroup
		start := time.Now()
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				req := &http.Request{}
				for time.Since(start) < 100*time.Millisecond {
					rt.RoundTrip(req)
				}
			}()
		}
		wg.Wait()
		So(peak, ShouldBeLessThanOrEqualTo, 10)
	})
}
