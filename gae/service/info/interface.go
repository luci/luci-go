// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package info

import (
	"time"

	"golang.org/x/net/context"
)

// Interface is the interface for all of the package methods which normally
// would be in the 'appengine' package.
type Interface interface {
	AppID() string
	FullyQualifiedAppID() string
	GetNamespace() string

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
	MustNamespace(namespace string) context.Context

	AccessToken(scopes ...string) (token string, expiry time.Time, err error)
	PublicCertificates() ([]Certificate, error)
	SignBytes(bytes []byte) (keyName string, signature []byte, err error)
}
