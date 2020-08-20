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

// Package urlfetch provides a way for an application to get http.RoundTripper
// that can make outbound HTTP requests. If used for https:// protocol, will
// always validate SSL certificates.
package urlfetch

import (
	"errors"
	"net/http"

	"golang.org/x/net/context"
)

type key int

var serviceKey key

// Factory is the function signature for factory methods compatible with
// SetFactory.
type Factory func(context.Context) http.RoundTripper

// Get pulls http.RoundTripper implementation from context or panics if it
// wasn't set. Use SetFactory(...) or Set(...) in unit tests to mock
// the round tripper.
func Get(c context.Context) http.RoundTripper {
	if f, ok := c.Value(serviceKey).(Factory); ok && f != nil {
		return f(c)
	}
	panic(errors.New("no http.RoundTripper is set in context"))
}

// SetFactory sets the function to produce http.RoundTripper instances,
// as returned by the Get method.
func SetFactory(c context.Context, f Factory) context.Context {
	return context.WithValue(c, serviceKey, f)
}

// Set sets the current http.RoundTripper object in the context. Useful for
// testing with a quick mock. This is just a shorthand SetFactory invocation
// to set a factory which always returns the same object.
func Set(c context.Context, r http.RoundTripper) context.Context {
	return SetFactory(c, func(context.Context) http.RoundTripper { return r })
}
