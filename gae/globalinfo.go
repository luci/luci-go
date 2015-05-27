// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wrapper

import (
	"time"

	"appengine"

	"golang.org/x/net/context"
)

// GlobalInfo is the interface for all of the package methods which normally
// would be in the 'appengine' package.
type GlobalInfo interface {
	// methods usually requiring a Context

	AccessToken(scopes ...string) (token string, expiry time.Time, err error)
	AppID() string
	DefaultVersionHostname() string
	ModuleHostname(module, version, instance string) (string, error)
	ModuleName() string
	PublicCertificates() ([]appengine.Certificate, error)
	RequestID() string
	ServiceAccount() (string, error)
	SignBytes(bytes []byte) (keyName string, signature []byte, err error)
	VersionID() string

	// our tweaked interface

	// Namespace takes the new namespace as a string, and returns a context
	// set to use that namespace, or an error.
	// The appengine SDK doesn't document what errors you can see from this
	// method, or under what circumstances they might occur.
	Namespace(namespace string) (context.Context, error)

	// Really global functions... these don't normally even require context, but
	// for the purposes of testing+consistency, they're included here.

	Datacenter() string
	InstanceID() string
	IsDevAppserver() bool
	ServerSoftware() string

	IsCapabilityDisabled(err error) bool
	IsOverQuota(err error) bool
	IsTimeoutError(err error) bool

	// Backends are deprecated in favor of modules, so simplify this a bit by
	// omitting them from the interface.
	// BackendHostname(name string, index int) string
	// BackendInstance() (name string, index int)
}

// GIFactory is the function signature for factory methods compatible with
// SetGIFactory.
type GIFactory func(context.Context) GlobalInfo

// GetGI gets gets the GlobalInfo implementation from context.
func GetGI(c context.Context) GlobalInfo {
	if f, ok := c.Value(globalInfoKey).(GIFactory); ok && f != nil {
		return f(c)
	}
	return nil
}

// SetGIFactory sets the function to produce GlobalInfo instances, as returned
// by the GetGI method.
func SetGIFactory(c context.Context, gif GIFactory) context.Context {
	return context.WithValue(c, globalInfoKey, gif)
}

// SetGI sets the current GlobalInfo object in the context. Useful for testing
// with a quick mock. This is just a shorthand SetGIFactory invocation to set
// a factory which always returns the same object.
func SetGI(c context.Context, gi GlobalInfo) context.Context {
	return SetGIFactory(c, func(context.Context) GlobalInfo { return gi })
}
