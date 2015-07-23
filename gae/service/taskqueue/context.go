// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package taskqueue

import (
	"golang.org/x/net/context"
)

type key int

var (
	taskQueueKey       key
	taskQueueFilterKey key = 1
)

// Factory is the function signature for factory methods compatible with
// SetFactory.
type Factory func(context.Context) Interface

// Filter is the function signature for a filter TQ implementation. It
// gets the current TQ implementation, and returns a new TQ implementation
// backed by the one passed in.
type Filter func(context.Context, Interface) Interface

// getUnfiltered gets gets the Interface implementation from context without
// any of the filters applied.
func getUnfiltered(c context.Context) Interface {
	if f, ok := c.Value(taskQueueKey).(Factory); ok && f != nil {
		return f(c)
	}
	return nil
}

// Get gets the Interface implementation from context.
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

// SetFactory sets the function to produce Interface instances, as returned by
// the Get method.
func SetFactory(c context.Context, tqf Factory) context.Context {
	return context.WithValue(c, taskQueueKey, tqf)
}

// Set sets the current Interface object in the context. Useful for testing
// with a quick mock. This is just a shorthand SetFactory invocation to set
// a factory which always returns the same object.
func Set(c context.Context, tq Interface) context.Context {
	return SetFactory(c, func(context.Context) Interface { return tq })
}

func getCurFilters(c context.Context) []Filter {
	curFiltsI := c.Value(taskQueueFilterKey)
	if curFiltsI != nil {
		return curFiltsI.([]Filter)
	}
	return nil
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
	return context.WithValue(c, taskQueueFilterKey, newFilts)
}
