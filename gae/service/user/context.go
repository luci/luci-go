// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package user

import (
	"golang.org/x/net/context"
)

type key int

var (
	serviceKey       key
	serviceFilterKey key = 1
)

// Factory is the function signature for factory methods compatible with
// SetFactory.
type Factory func(context.Context) Interface

// Filter is the function signature for a filter user implementation. It
// gets the current user implementation, and returns a new user implementation
// backed by the one passed in.
type Filter func(context.Context, Interface) Interface

// getUnfiltered gets gets the Interface implementation from context without
// any of the filters applied.
func getUnfiltered(c context.Context) Interface {
	if f, ok := c.Value(serviceKey).(Factory); ok && f != nil {
		return f(c)
	}
	return nil
}

func getCurFilters(c context.Context) []Filter {
	curFiltsI := c.Value(serviceFilterKey)
	if curFiltsI != nil {
		return curFiltsI.([]Filter)
	}
	return nil
}

// Get pulls the user service implementation from context or nil if it
// wasn't set.
func Get(c context.Context) Interface {
	ret := getUnfiltered(c)
	if ret == nil {
		return nil
	}
	for _, f := range getCurFilters(c) {
		ret = f(c, ret)
	}
	return ret
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

// AddFilters adds Interface filters to the context.
func AddFilters(c context.Context, filts ...Filter) context.Context {
	if len(filts) == 0 {
		return c
	}
	cur := getCurFilters(c)
	newFilts := make([]Filter, 0, len(cur)+len(filts))
	newFilts = append(newFilts, getCurFilters(c)...)
	newFilts = append(newFilts, filts...)
	return context.WithValue(c, serviceFilterKey, newFilts)
}
