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

// RawFactory is the function signature for RawFactory methods compatible with
// SetRawFactory. wantTxn is true if the Factory should return the taskqueue in
// the current transaction, and false if the Factory should return the
// non-transactional (root) taskqueue service.
type RawFactory func(c context.Context, wantTxn bool) RawInterface

// RawFilter is the function signature for a RawFilter TQ implementation. It
// gets the current TQ implementation, and returns a new TQ implementation
// backed by the one passed in.
type RawFilter func(context.Context, RawInterface) RawInterface

// getUnfiltered gets gets the RawInterface implementation from context without
// any of the filters applied.
func getUnfiltered(c context.Context, wantTxn bool) RawInterface {
	if f, ok := c.Value(taskQueueKey).(RawFactory); ok && f != nil {
		return f(c, wantTxn)
	}
	return nil
}

// getFiltered gets the taskqueue (transactional or not), and applies all of
// the currently installed filters to it.
func getFiltered(c context.Context, wantTxn bool) RawInterface {
	ret := getUnfiltered(c, wantTxn)
	if ret == nil {
		return nil
	}
	for _, f := range getCurFilters(c) {
		ret = f(c, ret)
	}
	return ret
}

// GetRaw gets the RawInterface implementation from context.
func GetRaw(c context.Context) RawInterface {
	return getFiltered(c, true)
}

// GetRawNoTxn gets the RawInterface implementation from context. If there's a
// currently active transaction, this will return a non-transactional connection
// to the taskqueue, otherwise this is the same as GetRaw.
func GetRawNoTxn(c context.Context) RawInterface {
	return getFiltered(c, false)
}

// Get gets the Interface implementation from context.
func Get(c context.Context) Interface {
	return &taskqueueImpl{GetRaw(c)}
}

// GetNoTxn gets the Interface implementation from context.
func GetNoTxn(c context.Context) Interface {
	return &taskqueueImpl{GetRawNoTxn(c)}
}

// SetRawFactory sets the function to produce RawInterface instances, as returned by
// the GetRaw method.
func SetRawFactory(c context.Context, tqf RawFactory) context.Context {
	return context.WithValue(c, taskQueueKey, tqf)
}

// SetRaw sets the current RawInterface object in the context. Useful for testing
// with a quick mock. This is just a shorthand SetRawFactory invocation to SetRaw
// a RawFactory which always returns the same object.
func SetRaw(c context.Context, tq RawInterface) context.Context {
	return SetRawFactory(c, func(context.Context, bool) RawInterface { return tq })
}

func getCurFilters(c context.Context) []RawFilter {
	curFiltsI := c.Value(taskQueueFilterKey)
	if curFiltsI != nil {
		return curFiltsI.([]RawFilter)
	}
	return nil
}

// AddRawFilters adds RawInterface filters to the context.
func AddRawFilters(c context.Context, filts ...RawFilter) context.Context {
	if len(filts) == 0 {
		return c
	}
	cur := getCurFilters(c)
	newFilts := make([]RawFilter, 0, len(cur)+len(filts))
	newFilts = append(newFilts, getCurFilters(c)...)
	newFilts = append(newFilts, filts...)
	return context.WithValue(c, taskQueueFilterKey, newFilts)
}
