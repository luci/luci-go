// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package transport allows to inject http.RoundTripper implementation into
// a context.Context.
package transport

import (
	"net/http"

	"golang.org/x/net/context"
)

type key int

var contextKey key

// Factory is the function signature for factory methods compatible with
// SetFactory.
type Factory func(context.Context) http.RoundTripper

// Get extracts http.RoundTripper from the context. If one hasn't been
// set, it returns http.DefaultTransport.
func Get(c context.Context) http.RoundTripper {
	if f, ok := c.Value(contextKey).(Factory); ok && f != nil {
		return f(c)
	}
	return http.DefaultTransport
}

// GetClient returns *http.Client built around round tripper in the context.
func GetClient(c context.Context) *http.Client {
	return &http.Client{Transport: Get(c)}
}

// SetFactory sets the function to produce http.RoundTripper instances,
// as returned by the Get method.
func SetFactory(c context.Context, f Factory) context.Context {
	return context.WithValue(c, contextKey, f)
}

// Set sets the current http.RoundTripper object in the context. Useful
// for testing with a quick mock. This is just a shorthand SetFactory
// invocation to set a factory which always returns the same object.
func Set(c context.Context, r http.RoundTripper) context.Context {
	if r == nil {
		return SetFactory(c, nil)
	}
	return SetFactory(c, func(context.Context) http.RoundTripper { return r })
}
