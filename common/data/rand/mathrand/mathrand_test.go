// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package mathrand

import (
	"math"
	"math/rand"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func Test(t *testing.T) {
	t.Parallel()

	Convey("test mathrand", t, func() {
		c := context.Background()

		Convey("unset", func() {
			// Just ensure doesn't crash.
			So(Get(c).Int()+1 > 0, ShouldBeTrue)
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
		mean, dev := calcStats(10000, func() float64 {
			return Get(ctx).Float64()
		})

		// For ideal uniform [0, 1) distribution it should be:
		// Average: 0.500000
		// Standard deviation: 0.288675
		So(mean, ShouldBeBetween, 0.495, 0.505)
		So(dev, ShouldBeBetween, 0.284, 0.29)
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

// BenchmarkStdlibDefaultSource-8           	50000000	        32.0 ns/op
// BenchmarkOurDefaultSourceViaCtx-8        	  200000	       10893 ns/op
// BenchmarkOurDefaultSourceViaFunc-8       	30000000	        38.7 ns/op
// BenchmarkOurInitializedSourceViaCtx-8    	50000000	        30.2 ns/op
// BenchmarkOurInitializedSourceViaFunc-8   	50000000	        29.1 ns/op
