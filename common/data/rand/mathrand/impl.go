// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package mathrand

import (
	"math/rand"
	"sync"
)

// Rand is a random number generator interface.
//
// A Rand instance is not necessarily safe for concurrent use. In order to
// ensure that it is, wrap it in Locking or obtain it from a method that
// provides this guarantee (e.g., Get).
type Rand interface {
	// Int63 returns a non-negative pseudo-random 63-bit integer as an int64
	// from the source in the context or the shared global source.
	Int63() int64

	// Uint32 returns a pseudo-random 32-bit value as a uint32 from the source in
	// the context or the shared global source.
	Uint32() uint32

	// Int31 returns a non-negative pseudo-random 31-bit integer as an int32 from
	// the source in the context or the shared global source.
	Int31() int32

	// Int returns a non-negative pseudo-random int from the source in the context
	// or the shared global source.
	Int() int

	// Int63n returns, as an int64, a non-negative pseudo-random number in [0,n)
	// from the source in the context or the shared global source.
	//
	// It panics if n <= 0.
	Int63n(n int64) int64

	// Int31n returns, as an int32, a non-negative pseudo-random number in [0,n)
	// from the source in the context or the shared global source.
	//
	// It panics if n <= 0.
	Int31n(n int32) int32

	// Intn returns, as an int, a non-negative pseudo-random number in [0,n) from
	// the source in the context or the shared global source.
	//
	// It panics if n <= 0.
	Intn(n int) int

	// Float64 returns, as a float64, a pseudo-random number in [0.0,1.0) from
	// the source in the context or the shared global source.
	Float64() float64

	// Float32 returns, as a float32, a pseudo-random number in [0.0,1.0) from
	// the source in the context or the shared global source.
	Float32() float32

	// Perm returns, as a slice of n ints, a pseudo-random permutation of the
	// integers [0,n) from the source in the context or the shared global source.
	Perm(n int) []int

	// Read generates len(p) random bytes from the source in the context or
	// the shared global source and writes them into p. It always returns len(p)
	// and a nil error.
	Read(p []byte) (n int, err error)

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
	NormFloat64() float64

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
	ExpFloat64() float64

	// WithGoRand invokes the supplied "fn" while holding an exclusive lock
	// for it. This can be used by callers to pull and use a *rand.Rand instance
	// out of the Context safely.
	//
	// The callback's r must not be retained or used outside of hte scope of the
	// callback.
	WithGoRand(fn func(r *rand.Rand) error) error
}

// Locking wraps a Rand instance in a layer that locks around all of its
// methods.
//
// A user must hold Locking's Mutex if the want to directly access and use
// Locking's R member safely.
//
// By default, a Rand instance is not safe for concurrent use. A ocking Rand
// instance is.
type Locking struct {
	sync.Mutex
	R Rand
}

func wrapLocking(r Rand) Rand {
	if _, ok := r.(*Locking); ok {
		return r
	}
	return &Locking{R: r}
}

// Int63 returns a non-negative pseudo-random 63-bit integer as an int64
// from the source in the context or the shared global source.
func (l *Locking) Int63() int64 {
	l.Lock()
	defer l.Unlock()
	return l.R.Int63()
}

// Uint32 returns a pseudo-random 32-bit value as a uint32 from the source in
// the context or the shared global source.
func (l *Locking) Uint32() uint32 {
	l.Lock()
	defer l.Unlock()
	return l.R.Uint32()
}

// Int31 returns a non-negative pseudo-random 31-bit integer as an int32 from
// the source in the context or the shared global source.
func (l *Locking) Int31() int32 {
	l.Lock()
	defer l.Unlock()
	return l.R.Int31()
}

// Int returns a non-negative pseudo-random int from the source in the context
// or the shared global source.
func (l *Locking) Int() int {
	l.Lock()
	defer l.Unlock()
	return l.R.Int()
}

// Int63n returns, as an int64, a non-negative pseudo-random number in [0,n)
// from the source in the context or the shared global source.
//
// It panics if n <= 0.
func (l *Locking) Int63n(n int64) int64 {
	l.Lock()
	defer l.Unlock()
	return l.R.Int63n(n)
}

// Int31n returns, as an int32, a non-negative pseudo-random number in [0,n)
// from the source in the context or the shared global source.
//
// It panics if n <= 0.
func (l *Locking) Int31n(n int32) int32 {
	l.Lock()
	defer l.Unlock()
	return l.R.Int31n(n)
}

// Intn returns, as an int, a non-negative pseudo-random number in [0,n) from
// the source in the context or the shared global source.
//
// It panics if n <= 0.
func (l *Locking) Intn(n int) int {
	l.Lock()
	defer l.Unlock()
	return l.R.Intn(n)
}

// Float64 returns, as a float64, a pseudo-random number in [0.0,1.0) from
// the source in the context or the shared global source.
func (l *Locking) Float64() float64 {
	l.Lock()
	defer l.Unlock()
	return l.R.Float64()
}

// Float32 returns, as a float32, a pseudo-random number in [0.0,1.0) from
// the source in the context or the shared global source.
func (l *Locking) Float32() float32 {
	l.Lock()
	defer l.Unlock()
	return l.R.Float32()
}

// Perm returns, as a slice of n ints, a pseudo-random permutation of the
// integers [0,n) from the source in the context or the shared global source.
func (l *Locking) Perm(n int) []int {
	l.Lock()
	defer l.Unlock()
	return l.R.Perm(n)
}

// Read generates len(p) random bytes from the source in the context or
// the shared global source and writes them into p. It always returns len(p)
// and a nil error.
func (l *Locking) Read(p []byte) (n int, err error) {
	l.Lock()
	defer l.Unlock()
	return l.R.Read(p)
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
func (l *Locking) NormFloat64() float64 {
	l.Lock()
	defer l.Unlock()
	return l.R.NormFloat64()
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
func (l *Locking) ExpFloat64() float64 {
	l.Lock()
	defer l.Unlock()
	return l.R.ExpFloat64()
}

// WithGoRand invokes the supplied "fn" while holding an exclusive lock
// for it. This can be used by callers to pull and use a *rand.Rand instance
// out of the Context safely.
//
// The callback's r must not be retained or used outside of hte scope of the
// callback.
func (l *Locking) WithGoRand(fn func(r *rand.Rand) error) error {
	l.Lock()
	defer l.Unlock()
	return l.R.WithGoRand(fn)
}

// wrapped is a simple wrapper to a *math.Rand instance.
type wrapped struct {
	*rand.Rand
}

// Wrap wraps a Rand instance, allowing it to satisfy the Rand interface.
func wrapRand(r *rand.Rand) Rand { return wrapped{r} }

func (w wrapped) WithGoRand(fn func(r *rand.Rand) error) error { return fn(w.Rand) }
