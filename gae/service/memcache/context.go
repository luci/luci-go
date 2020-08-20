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

package memcache

import (
	"golang.org/x/net/context"
)

type key int

var (
	memcacheKey       key
	memcacheFilterKey key = 1
)

// RawFactory is the function signature for RawFactory methods compatible with
// SetRawFactory.
type RawFactory func(context.Context) RawInterface

// RawFilter is the function signature for a RawFilter MC implementation. It
// gets the current MC implementation, and returns a new MC implementation
// backed by the one passed in.
type RawFilter func(context.Context, RawInterface) RawInterface

// getUnfiltered gets gets the RawInterface implementation from context without
// any of the filters applied.
func getUnfiltered(c context.Context) RawInterface {
	if f, ok := c.Value(memcacheKey).(RawFactory); ok && f != nil {
		return f(c)
	}
	return nil
}

// Raw gets the current memcache implementation from the context.
func Raw(c context.Context) RawInterface {
	ret := getUnfiltered(c)
	if ret == nil {
		return nil
	}
	for _, f := range getCurFilters(c) {
		ret = f(c, ret)
	}
	return ret
}

// SetRawFactory sets the function to produce RawInterface instances, as returned by
// the Get method.
func SetRawFactory(c context.Context, mcf RawFactory) context.Context {
	return context.WithValue(c, memcacheKey, mcf)
}

// SetRaw sets the current RawInterface object in the context. Useful for testing
// with a quick mock. This is just a shorthand SetRawFactory invocation to SetRaw
// a RawFactory which always returns the same object.
func SetRaw(c context.Context, mc RawInterface) context.Context {
	return SetRawFactory(c, func(context.Context) RawInterface { return mc })
}

func getCurFilters(c context.Context) []RawFilter {
	curFiltsI := c.Value(memcacheFilterKey)
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
	return context.WithValue(c, memcacheFilterKey, newFilts)
}
