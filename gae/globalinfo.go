// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gae

import (
	"time"

	"golang.org/x/net/context"
)

// GlobalInfo is the interface for all of the package methods which normally
// would be in the 'appengine' package.
type GlobalInfo interface {
	AppID() string
	Datacenter() string
	DefaultVersionHostname() string
	InstanceID() string
	IsDevAppServer() bool
	IsOverQuota(err error) bool
	IsTimeoutError(err error) bool
	ModuleHostname(module, version, instance string) (string, error)
	ModuleName() string
	RequestID() string
	ServerSoftware() string
	ServiceAccount() (string, error)
	VersionID() string

	Namespace(namespace string) (context.Context, error)

	AccessToken(scopes ...string) (token string, expiry time.Time, err error)
	PublicCertificates() ([]GICertificate, error)
	SignBytes(bytes []byte) (keyName string, signature []byte, err error)
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
