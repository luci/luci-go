// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package info

import (
	"time"

	"golang.org/x/net/context"
)

// RawInterface is the interface for all of the package methods which normally
// would be in the 'appengine' package.
type RawInterface interface {
	AppID() string
	FullyQualifiedAppID() string
	GetNamespace() (string, bool)

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
	PublicCertificates() ([]Certificate, error)
	SignBytes(bytes []byte) (keyName string, signature []byte, err error)

	// Testable returns this Interface's Testable interface. Testing will return
	// nil if testing is not supported in this implementation.
	Testable() Testable
}

// Interface is the interface for all of the package methods which normally
// would be in the 'appengine' package. This version adds a couple helper
// functions on top of the RawInterface.
type Interface interface {
	RawInterface

	// TrimmedAppID gets the 'appid' portion of "foo.com:appid". This form can
	// occur if you use
	TrimmedAppID() string

	// MustNamespace is the same as Namespace, but will panic if there's an error.
	// Since an error can only occur if namespace doesn't match the a regex this
	// is valid to use if the namespace you're using is statically known, or known
	// to conform to the regex. The regex in question is:
	//
	//   ^[0-9A-Za-z._-]{0,100}$
	MustNamespace(namespace string) context.Context
}

// Testable is an additional set of functions for testing instrumentation.
type Testable interface {
	SetVersionID(string) context.Context
	SetRequestID(string) context.Context
}
