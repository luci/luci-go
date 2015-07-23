// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package mathrand implements a mockable interface for "math/rand"
// functionality which is controllable through context.Context. You should use
// this instead of math/rand directly, to allow you to make deterministic tests.
package mathrand

import (
	"math/rand"

	"github.com/luci/luci-go/common/clock"
	"golang.org/x/net/context"
)

type key int

var mathRandKey key

// Factory is the function signature for factory methods compatible with
// SetFactory.
type Factory func(context.Context) *rand.Rand

// Get gets a *"math/rand".Rand from the context. If one hasn't been
// set, this creates a new Rand object with a Source initialized from the
// current time clock.Now(c).UnixNano().
func Get(c context.Context) *rand.Rand {
	if f, ok := c.Value(mathRandKey).(Factory); ok && f != nil {
		return f(c)
	}
	return rand.New(rand.NewSource(clock.Now(c).UnixNano()))
}

// SetFactory sets the function to produce *"math/rand".Rand instances,
// as returned by the Get method.
func SetFactory(c context.Context, mrf Factory) context.Context {
	return context.WithValue(c, mathRandKey, mrf)
}

// Set sets the current *"math/rand".Rand object in the context. Useful
// for testing with a quick mock. This is just a shorthand SetFactory
// invocation to set a factory which always returns the same object.
func Set(c context.Context, r *rand.Rand) context.Context {
	if r == nil {
		return SetFactory(c, nil)
	}
	return SetFactory(c, func(context.Context) *rand.Rand { return r })
}
