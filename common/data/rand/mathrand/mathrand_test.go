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

package mathrand

import (
	"context"
	"math"
	"math/rand"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func Test(t *testing.T) {
	t.Parallel()

	Convey("test mathrand", t, func() {
		c := context.Background()

		Convey("unset", func() {
			// Just ensure doesn't crash.
			So(Get(c).Int()+1 > 0, ShouldBeTrue)
			So(WithGoRand(c, func(r *rand.Rand) error {
				So(r.Int(), ShouldBeGreaterThanOrEqualTo, 0)
				return nil
			}), ShouldBeNil)
		})

		Convey("set persistance", func() {
			c = Set(c, rand.New(rand.NewSource(12345)))
			r := rand.New(rand.NewSource(12345))
			So(Get(c).Int(), ShouldEqual, r.Int())
			So(Get(c).Int(), ShouldEqual, r.Int())
		})

		Convey("nil set", func() {
			c = Set(c, nil)
			// Just ensure doesn't crash.
			So(Get(c).Int()+1 > 0, ShouldBeTrue)
		})
	})

	Convey("fairness of uninitialized source", t, func() {
		// We do some ugly stuff in Get(...) if context doesn't have math.Rand set,
		// check that the produced RNG sequence matches the uniform distribution
		// at least at first two moments.
		ctx := context.Background()
		mean, dev := calcStats(20000, func() float64 {
			return Get(ctx).Float64()
		})

		// For ideal uniform [0, 1) distribution it should be:
		// Average: 0.500000
		// Standard deviation: 0.288675
		So(mean, ShouldBeBetween, 0.495, 0.505)
		So(dev, ShouldBeBetween, 0.284, 0.29)
	})
}

func testConcurrentAccess(t *testing.T, r *rand.Rand) {
	const goroutines = 16
	const rounds = 1024

	Convey(`Concurrent access does not produce a race or deadlock.`, func() {
		c := context.Background()
		if r != nil {
			c = Set(c, r)
		}

		startC := make(chan struct{})
		doneC := make(chan struct{}, goroutines)
		for g := 0; g < goroutines; g++ {
			go func() {
				defer func() {
					doneC <- struct{}{}
				}()

				<-startC
				for i := 0; i < rounds; i++ {
					Int(c)
				}
			}()
		}

		close(startC)
		for reap := 0; reap < goroutines; reap++ {
			<-doneC
		}
	})

}

// TestConcurrentGlobalAccess is intentionally NOT Parallel, since we want to
// have exclusive access to the global instance.
func TestConcurrentGlobalAccess(t *testing.T) {
	Convey(`Testing concurrent global access`, t, func() {
		testConcurrentAccess(t, nil)
	})
}

func TestConcurrentAccess(t *testing.T) {
	t.Parallel()

	Convey(`Testing concurrent non-global access`, t, func() {
		testConcurrentAccess(t, newRand())
	})
}

func calcStats(n int, gen func() float64) (avg float64, std float64) {
	var m1 float64
	var m2 float64

	for i := 0; i < n; i++ {
		x := gen()
		m1 += x
		m2 += x * x
	}

	avg = m1 / float64(n)
	std = math.Sqrt(m2/float64(n) - avg*avg)
	return
}

func BenchmarkStdlibDefaultSource(b *testing.B) {
	calcStats(b.N, rand.Float64)
}

func BenchmarkOurDefaultSourceViaCtx(b *testing.B) {
	ctx := context.Background()
	calcStats(b.N, func() float64 {
		return Get(ctx).Float64()
	})
}

func BenchmarkOurDefaultSourceViaFunc(b *testing.B) {
	ctx := context.Background()
	calcStats(b.N, func() float64 {
		return Float64(ctx)
	})
}

func BenchmarkOurInitializedSourceViaCtx(b *testing.B) {
	ctx := context.Background()
	WithGoRand(ctx, func(r *rand.Rand) error {
		ctx = Set(ctx, r)
		calcStats(b.N, func() float64 {
			return Get(ctx).Float64()
		})
		return nil
	})
}

func BenchmarkOurInitializedSourceViaFunc(b *testing.B) {
	ctx := context.Background()
	WithGoRand(ctx, func(r *rand.Rand) error {
		ctx = Set(ctx, r)
		calcStats(b.N, func() float64 {
			return Float64(ctx)
		})
		return nil
	})
}

func BenchmarkGlobalSource(b *testing.B) {
	r, _ := getGlobalRand()
	calcStats(b.N, func() float64 {
		return r.Float64()
	})
}

// BenchmarkStdlibDefaultSource-32               30000000        35.6 ns/op
// BenchmarkOurDefaultSourceViaCtx-32            20000000        77.8 ns/op
// BenchmarkOurDefaultSourceViaFunc-32           20000000        78.6 ns/op
// BenchmarkOurInitializedSourceViaCtx-32        20000000        86.8 ns/op
// BenchmarkOurInitializedSourceViaFunc-32       20000000        81.9 ns/op
// BenchmarkGlobalSource-32                      30000000        43.8 ns/op
