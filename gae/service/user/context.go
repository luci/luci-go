// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package user

import (
	"golang.org/x/net/context"
)

type key int

var serviceKey key

// Factory is the function signature for factory methods compatible with
// SetFactory.
type Factory func(context.Context) Interface

// Get pulls the user service implementation from context or nil if it
// wasn't set.
func Get(c context.Context) Interface {
	if f, ok := c.Value(serviceKey).(Factory); ok && f != nil {
		return f(c)
	}
	return nil
}

// SetFactory sets the function to produce user.Interface instances,
// as returned by the Get method.
func SetFactory(c context.Context, f Factory) context.Context {
	return context.WithValue(c, serviceKey, f)
}

// Set sets the user service in this context. Useful for testing with a quick
// mock. This is just a shorthand SetFactory invocation to set a factory which
// always returns the same object.
func Set(c context.Context, u Interface) context.Context {
	return SetFactory(c, func(context.Context) Interface { return u })
}
