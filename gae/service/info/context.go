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

package info

import (
	"golang.org/x/net/context"
)

type key int

var (
	infoKey       key
	infoFilterKey key = 1
)

// Factory is the function signature for factory methods compatible with
// SetFactory.
type Factory func(context.Context) RawInterface

// Filter is the function signature for a filter GI implementation. It
// gets the current GI implementation, and returns a new GI implementation
// backed by the one passed in.
type Filter func(context.Context, RawInterface) RawInterface

// getUnfiltered gets gets the RawInterface implementation from context without
// any of the filters applied.
func getUnfiltered(c context.Context) RawInterface {
	if f, ok := c.Value(infoKey).(Factory); ok && f != nil {
		return f(c)
	}
	return nil
}

// Raw returns the current filtered RawInterface installed in the Context.
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

// SetFactory sets the function to produce RawInterface instances, as returned
// by the Get method.
func SetFactory(c context.Context, gif Factory) context.Context {
	return context.WithValue(c, infoKey, gif)
}

// Set sets the current RawInterface object in the context. Useful for testing
// with a quick mock. This is just a shorthand SetFactory invocation to set
// a factory which always returns the same object.
func Set(c context.Context, gi RawInterface) context.Context {
	return SetFactory(c, func(context.Context) RawInterface { return gi })
}

func getCurFilters(c context.Context) []Filter {
	curFiltsI := c.Value(infoFilterKey)
	if curFiltsI != nil {
		return curFiltsI.([]Filter)
	}
	return nil
}

// AddFilters adds RawInterface filters to the context.
func AddFilters(c context.Context, filts ...Filter) context.Context {
	if len(filts) == 0 {
		return c
	}
	cur := getCurFilters(c)
	newFilts := make([]Filter, 0, len(cur)+len(filts))
	newFilts = append(newFilts, getCurFilters(c)...)
	newFilts = append(newFilts, filts...)
	return context.WithValue(c, infoFilterKey, newFilts)
}
