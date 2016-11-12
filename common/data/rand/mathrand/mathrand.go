// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package mathrand implements a mockable interface for math/rand.Rand.
//
// It is controllable through context.Context. You should use this instead of
// math/rand directly, to allow you to make deterministic tests.
package mathrand

import (
	"math/rand"

	"golang.org/x/net/context"
)

var key = "holds a rand.Rand for mathrand"

func newRand() *rand.Rand {
	return rand.New(rand.NewSource(rand.Int63()))
}

func getRand(c context.Context) Rand {
	if r, ok := c.Value(&key).(Rand); ok {
		return r
	}
	return nil
}

// Get gets a Rand from the Context. The resulting Rand is safe for concurrent
// use.
//
// If one hasn't been set, this creates a new Rand object with a Source
// initialized from the global randomness source provided by stdlib.
//
// If you want to get just a single random number, prefer to use a corresponding
// global function instead: they know how to use math/rand global RNG directly
// and thus are much faster in case the context doesn't have a rand.Rand
// installed.
//
// Use 'Get' only if you plan to obtain a large series of random numbers.
func Get(c context.Context) Rand {
	if r := getRand(c); r != nil {
		return r
	}

	// Generate a new Rand instance and return it. Our callers expect this to be
	// concurrency-safe.
	return wrapLocking(wrapRand(newRand()))
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
// They are here to optimize the case when the context doesn't have math.Rand
// installed. Using Get(ctx).<function> in this case is semantically equivalent
// to using global RNG, but much slower (up to 400x slower), because it creates
// and seeds new math.Rand object on each call.
//
// Unfortunately since math.Rand is a struct, and its implementation is not
// thread-safe, we can't just return some global math.Rand instance. The stdlib
// has one, but it is private, and we can't reimplement it because stdlib does
// some disgusting type casts to private types in math.Rand implementation, e.g:
// https://github.com/golang/go/blob/fb3cf5c/src/math/rand/rand.go#L183
//
// This also makes mathrand API more similar to math/rand API.

// Int63 returns a non-negative pseudo-random 63-bit integer as an int64
// from the source in the context or the shared global source.
func Int63(c context.Context) int64 {
	if r := getRand(c); r != nil {
		return r.Int63()
	}
	return rand.Int63()
}

// Uint32 returns a pseudo-random 32-bit value as a uint32 from the source in
// the context or the shared global source.
func Uint32(c context.Context) uint32 {
	if r := getRand(c); r != nil {
		return r.Uint32()
	}
	return rand.Uint32()
}

// Int31 returns a non-negative pseudo-random 31-bit integer as an int32 from
// the source in the context or the shared global source.
func Int31(c context.Context) int32 {
	if r := getRand(c); r != nil {
		return r.Int31()
	}
	return rand.Int31()
}

// Int returns a non-negative pseudo-random int from the source in the context
// or the shared global source.
func Int(c context.Context) int {
	if r := getRand(c); r != nil {
		return r.Int()
	}
	return rand.Int()
}

// Int63n returns, as an int64, a non-negative pseudo-random number in [0,n)
// from the source in the context or the shared global source.
//
// It panics if n <= 0.
func Int63n(c context.Context, n int64) int64 {
	if r := getRand(c); r != nil {
		return r.Int63n(n)
	}
	return rand.Int63n(n)
}

// Int31n returns, as an int32, a non-negative pseudo-random number in [0,n)
// from the source in the context or the shared global source.
//
// It panics if n <= 0.
func Int31n(c context.Context, n int32) int32 {
	if r := getRand(c); r != nil {
		return r.Int31n(n)
	}
	return rand.Int31n(n)
}

// Intn returns, as an int, a non-negative pseudo-random number in [0,n) from
// the source in the context or the shared global source.
//
// It panics if n <= 0.
func Intn(c context.Context, n int) int {
	if r := getRand(c); r != nil {
		return r.Intn(n)
	}
	return rand.Intn(n)
}

// Float64 returns, as a float64, a pseudo-random number in [0.0,1.0) from
// the source in the context or the shared global source.
func Float64(c context.Context) float64 {
	if r := getRand(c); r != nil {
		return r.Float64()
	}
	return rand.Float64()
}

// Float32 returns, as a float32, a pseudo-random number in [0.0,1.0) from
// the source in the context or the shared global source.
func Float32(c context.Context) float32 {
	if r := getRand(c); r != nil {
		return r.Float32()
	}
	return rand.Float32()
}

// Perm returns, as a slice of n ints, a pseudo-random permutation of the
// integers [0,n) from the source in the context or the shared global source.
func Perm(c context.Context, n int) []int {
	if r := getRand(c); r != nil {
		return r.Perm(n)
	}
	return rand.Perm(n)
}

// Read generates len(p) random bytes from the source in the context or
// the shared global source and writes them into p. It always returns len(p)
// and a nil error.
func Read(c context.Context, p []byte) (n int, err error) {
	if r := getRand(c); r != nil {
		return r.Read(p)
	}
	return rand.Read(p)
}

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
func NormFloat64(c context.Context) float64 {
	if r := getRand(c); r != nil {
		return r.NormFloat64()
	}
	return rand.NormFloat64()
}

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
func ExpFloat64(c context.Context) float64 {
	if r := getRand(c); r != nil {
		return r.ExpFloat64()
	}
	return rand.ExpFloat64()
}

// WithGoRand invokes the supplied "fn" while holding an exclusive lock
// for it. This can be used by callers to pull and use a *rand.Rand instance
// out of the Context safely.
//
// The callback's r must not be retained or used outside of hte scope of the
// callback.
func WithGoRand(c context.Context, fn func(r *rand.Rand) error) error {
	if r := getRand(c); r != nil {
		return r.WithGoRand(fn)
	}

	// No Rand is installed in our Context. Generate a single-use Rand instance.
	// We don't need to wrap this at all, since the premise of this method is
	// that the result is not safe for concurrent use.
	return fn(newRand())
}
