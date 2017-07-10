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
type Factory func(context.Context) RawInterface

// Filter is the function signature for a filter user implementation. It
// gets the current user implementation, and returns a new user implementation
// backed by the one passed in.
type Filter func(context.Context, RawInterface) RawInterface

// getUnfiltered gets gets the RawInterface implementation from context without
// any of the filters applied.
func getUnfiltered(c context.Context) RawInterface {
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

// Raw pulls the user service implementation from context or nil if it
// wasn't set.
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

// SetFactory sets the function to produce user.RawInterface instances,
// as returned by the Get method.
func SetFactory(c context.Context, f Factory) context.Context {
	return context.WithValue(c, serviceKey, f)
}

// Set sets the user service in this context. Useful for testing with a quick
// mock. This is just a shorthand SetFactory invocation to set a factory which
// always returns the same object.
func Set(c context.Context, u RawInterface) context.Context {
	return SetFactory(c, func(context.Context) RawInterface { return u })
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
	return context.WithValue(c, serviceFilterKey, newFilts)
}
