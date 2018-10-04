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

// Package mathrand implements a mockable interface for math/rand.Rand.
//
// It is controllable through context.Context. You should use this instead of
// math/rand directly, to allow you to make deterministic tests.
package mathrand

import (
	"context"
	"math/rand"
	"sync"
)

var key = "holds a rand.Rand for mathrand"

var (
	// globalOnce performs one-time global random variable initialization.
	globalOnce sync.Once

	// globalRandBase is the gloal *rand.Rand instance. It MUST NOT BE USED
	// without holding globalRand's lock.
	//
	// globalRandBase must not be accessed directly; instead, it must be obtained
	// through getGlobalRand to ensure that it is initialized.
	globalRandBase *rand.Rand

	// globalRand is the locking Rand implementation that wraps globalRandBase.
	//
	// globalRand must not be accessed directly; instead, it must be obtained
	// through getGlobalRand to ensure that it is initialized.
	globalRand *Locking
)

// getGlobalRand returns globalRand and its Locking wrapper. This must be used
// instead of direct variable access in order to ensure that everything is
// initialized.
//
// We use a Once to perform this initialization so that we can enable
// applications to set the seed via rand.Seed if they wish.
func getGlobalRand() (*Locking, *rand.Rand) {
	globalOnce.Do(func() {
		globalRandBase = newRand()
		globalRand = &Locking{R: wrapped{globalRandBase}}
	})
	return globalRand, globalRandBase
}

func newRand() *rand.Rand { return rand.New(rand.NewSource(rand.Int63())) }

// getRand returns the Rand installed in c, or nil if no Rand is installed.
func getRand(c context.Context) Rand {
	if r, ok := c.Value(&key).(Rand); ok {
		return r
	}
	return nil
}

// Get gets a Rand from the Context. The resulting Rand is safe for concurrent
// use.
//
// If one hasn't been set, this will return a global Rand object backed by a
// shared rand.Rand singleton. Just like in "math/rand", rand.Seed can be called
// prior to using Get to set the seed used by this singleton.
func Get(c context.Context) Rand {
	if r := getRand(c); r != nil {
		return r
	}

	// Use the global instance.
	gr, _ := getGlobalRand()
	return gr
}

// Set sets the current *"math/rand".Rand object in the context.
//
// Useful for testing with a quick mock. The supplied *rand.Rand will be wrapped
// in a *Locking if necessary such that when it is returned from Get, it is
// safe for concurrent use.
func Set(c context.Context, mr *rand.Rand) context.Context {
	var r Rand
	if mr != nil {
		r = wrapRand(mr)
	}
	return SetRand(c, r)
}

// SetRand sets the current Rand object in the context.
//
// Useful for testing with a quick mock. The supplied Rand will be wrapped
// in a *Locking if necessary such that when it is returned from Get, it is
// safe for concurrent use.
func SetRand(c context.Context, r Rand) context.Context {
	if r != nil {
		r = wrapLocking(r)
	}
	return context.WithValue(c, &key, r)
}

////////////////////////////////////////////////////////////////////////////////
// Top-level convenience functions mirroring math/rand package API.
//
// This makes mathrand API more similar to math/rand API.

// Int63 returns a non-negative pseudo-random 63-bit integer as an int64
// from the source in the context or the shared global source.
func Int63(c context.Context) int64 { return Get(c).Int63() }

// Uint32 returns a pseudo-random 32-bit value as a uint32 from the source in
// the context or the shared global source.
func Uint32(c context.Context) uint32 { return Get(c).Uint32() }

// Int31 returns a non-negative pseudo-random 31-bit integer as an int32 from
// the source in the context or the shared global source.
func Int31(c context.Context) int32 { return Get(c).Int31() }

// Int returns a non-negative pseudo-random int from the source in the context
// or the shared global source.
func Int(c context.Context) int { return Get(c).Int() }

// Int63n returns, as an int64, a non-negative pseudo-random number in [0,n)
// from the source in the context or the shared global source.
//
// It panics if n <= 0.
func Int63n(c context.Context, n int64) int64 { return Get(c).Int63n(n) }

// Int31n returns, as an int32, a non-negative pseudo-random number in [0,n)
// from the source in the context or the shared global source.
//
// It panics if n <= 0.
func Int31n(c context.Context, n int32) int32 { return Get(c).Int31n(n) }

// Intn returns, as an int, a non-negative pseudo-random number in [0,n) from
// the source in the context or the shared global source.
//
// It panics if n <= 0.
func Intn(c context.Context, n int) int { return Get(c).Intn(n) }

// Float64 returns, as a float64, a pseudo-random number in [0.0,1.0) from
// the source in the context or the shared global source.
func Float64(c context.Context) float64 { return Get(c).Float64() }

// Float32 returns, as a float32, a pseudo-random number in [0.0,1.0) from
// the source in the context or the shared global source.
func Float32(c context.Context) float32 { return Get(c).Float32() }

// Perm returns, as a slice of n ints, a pseudo-random permutation of the
// integers [0,n) from the source in the context or the shared global source.
func Perm(c context.Context, n int) []int { return Get(c).Perm(n) }

// Read generates len(p) random bytes from the source in the context or
// the shared global source and writes them into p. It always returns len(p)
// and a nil error.
func Read(c context.Context, p []byte) (n int, err error) { return Get(c).Read(p) }

// NormFloat64 returns a normally distributed float64 in the range
// [-math.MaxFloat64, +math.MaxFloat64] with standard normal distribution
// (mean = 0, stddev = 1) from the source in the context or the shared global
// source.
//
// To produce a different normal distribution, callers can adjust the output
// using:
//
//  sample = NormFloat64(ctx) * desiredStdDev + desiredMean
//
func NormFloat64(c context.Context) float64 { return Get(c).NormFloat64() }

// ExpFloat64 returns an exponentially distributed float64 in the range
// (0, +math.MaxFloat64] with an exponential distribution whose rate parameter
// (lambda) is 1 and whose mean is 1/lambda (1) from the source in the context
// or the shared global source.
//
// To produce a distribution with a different rate parameter, callers can adjust
// the output using:
//
//  sample = ExpFloat64(ctx) / desiredRateParameter
//
func ExpFloat64(c context.Context) float64 { return Get(c).ExpFloat64() }

// WithGoRand invokes the supplied "fn" while holding an exclusive lock
// for it. This can be used by callers to pull and use a *rand.Rand instance
// out of the Context safely.
//
// The callback's r must not be retained or used outside of the scope of the
// callback.
func WithGoRand(c context.Context, fn func(r *rand.Rand) error) error {
	if r := getRand(c); r != nil {
		return r.WithGoRand(fn)
	}

	// Return our globalRandBase. We MUST hold globalRand's lock in order for this
	// to be safe.
	l, base := getGlobalRand()
	l.Lock()
	defer l.Unlock()
	return fn(base)
}
