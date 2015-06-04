// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wrapper

import (
	"math/rand"

	"golang.org/x/net/context"
	"infra/libs/clock"
)

// MathRandFactory is the function signature for factory methods compatible with
// SetMathRandFactory.
type MathRandFactory func(context.Context) *rand.Rand

// GetMathRand gets a *"math/rand".Rand from the context. If one hasn't been
// set, this creates a new Rand object with a Source initialized from the
// current time clock.Now(c).UnixNano().
func GetMathRand(c context.Context) *rand.Rand {
	if f, ok := c.Value(mathRandKey).(MathRandFactory); ok && f != nil {
		return f(c)
	}
	return rand.New(rand.NewSource(clock.Now(c).UnixNano()))
}

// SetMathRandFactory sets the function to produce *"math/rand".Rand instances,
// as returned by the GetMathRand method.
func SetMathRandFactory(c context.Context, mrf MathRandFactory) context.Context {
	return context.WithValue(c, mathRandKey, mrf)
}

// SetMathRand sets the current *"math/rand".Rand object in the context. Useful
// for testing with a quick mock. This is just a shorthand SetMathRandFactory
// invocation to set a factory which always returns the same object.
func SetMathRand(c context.Context, r *rand.Rand) context.Context {
	if r == nil {
		return SetMathRandFactory(c, nil)
	}
	return SetMathRandFactory(c, func(context.Context) *rand.Rand { return r })
}
