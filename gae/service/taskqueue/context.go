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
// SetRawFactory.
type RawFactory func(c context.Context) RawInterface

// RawFilter is the function signature for a RawFilter TQ implementation. It
// gets the current TQ implementation, and returns a new TQ implementation
// backed by the one passed in.
type RawFilter func(context.Context, RawInterface) RawInterface

// rawUnfiltered gets gets the RawInterface implementation from context without
// any of the filters applied.
func rawUnfiltered(c context.Context) RawInterface {
	if f, ok := c.Value(taskQueueKey).(RawFactory); ok && f != nil {
		return f(c)
	}
	return nil
}

// rawWithFilters gets the taskqueue (transactional or not), and applies all of
// the currently installed filters to it.
func rawWithFilters(c context.Context, filters ...RawFilter) RawInterface {
	ret := rawUnfiltered(c)
	if ret == nil {
		return nil
	}
	for _, f := range getCurFilters(c) {
		ret = f(c, ret)
	}
	for _, f := range filters {
		ret = f(c, ret)
	}
	return ret
}

// Raw gets the RawInterface implementation from context.
func Raw(c context.Context) RawInterface {
	return rawWithFilters(c)
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
	return SetRawFactory(c, func(context.Context) RawInterface { return tq })
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
