// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package datastore

import (
	"golang.org/x/net/context"
)

type key int

var (
	rawDatastoreKey       key
	rawDatastoreFilterKey key = 1
)

// RawFactory is the function signature for factory methods compatible with
// SetRawFactory.
type RawFactory func(context.Context) RawInterface

// RawFilter is the function signature for a filter RDS implementation. It
// gets the current RDS implementation, and returns a new RDS implementation
// backed by the one passed in.
type RawFilter func(context.Context, RawInterface) RawInterface

// getUnfiltered gets gets the RawInterface implementation from context without
// any of the filters applied.
func getUnfiltered(c context.Context) RawInterface {
	if f, ok := c.Value(rawDatastoreKey).(RawFactory); ok && f != nil {
		return f(c)
	}
	return nil
}

// GetRaw gets the RawInterface implementation from context.
func GetRaw(c context.Context) RawInterface {
	ret := getUnfiltered(c)
	if ret == nil {
		return nil
	}
	for _, f := range getCurFilters(c) {
		ret = f(c, ret)
	}
	return applyCheckFilter(c, ret)
}

// Get gets the Interface implementation from context.
func Get(c context.Context) Interface {
	return &datastoreImpl{GetRaw(c)}
}

// SetRawFactory sets the function to produce Datastore instances, as returned by
// the GetRaw method.
func SetRawFactory(c context.Context, rdsf RawFactory) context.Context {
	return context.WithValue(c, rawDatastoreKey, rdsf)
}

// SetRaw sets the current Datastore object in the context. Useful for testing with
// a quick mock. This is just a shorthand SetRawFactory invocation to set a factory
// which always returns the same object.
func SetRaw(c context.Context, rds RawInterface) context.Context {
	return SetRawFactory(c, func(context.Context) RawInterface { return rds })
}

func getCurFilters(c context.Context) []RawFilter {
	curFiltsI := c.Value(rawDatastoreFilterKey)
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
	return context.WithValue(c, rawDatastoreFilterKey, newFilts)
}
