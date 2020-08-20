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
	"strings"
	"time"

	"golang.org/x/net/context"
)

// RawInterface is the interface for all of the package methods which normally
// would be in the 'appengine' package.
type RawInterface interface {
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

	AccessToken(scopes ...string) (token string, expiry time.Time, err error)
	PublicCertificates() ([]Certificate, error)
	SignBytes(bytes []byte) (keyName string, signature []byte, err error)

	// Testable returns this RawInterface's Testing interface. Testing will
	// return nil if testing is not supported in this implementation.
	GetTestable() Testable
}

// Testable is an additional set of functions for testing instrumentation.
type Testable interface {
	SetVersionID(string) context.Context
	SetRequestID(string) context.Context
}

// AppID returns the current App ID.
func AppID(c context.Context) string {
	return Raw(c).AppID()
}

// TrimmedAppID gets the 'appid' portion of "foo.com:appid". This form can
// occur if you use
func TrimmedAppID(c context.Context) string {
	toks := strings.Split(AppID(c), ":")
	return toks[len(toks)-1]
}

// FullyQualifiedAppID returns the fully-qualified App ID.
func FullyQualifiedAppID(c context.Context) string {
	return Raw(c).FullyQualifiedAppID()
}

// GetNamespace returns the current namespace. If the current namespace is the
// default namespace, GetNamespace will return an empty string.
func GetNamespace(c context.Context) string {
	return Raw(c).GetNamespace()
}

// Datacenter returns the current datacenter.
func Datacenter(c context.Context) string {
	return Raw(c).Datacenter()
}

// DefaultVersionHostname returns the default version hostname.
func DefaultVersionHostname(c context.Context) string {
	return Raw(c).DefaultVersionHostname()
}

// InstanceID returns the current instance ID.
func InstanceID(c context.Context) string {
	return Raw(c).InstanceID()
}

// IsDevAppServer returns true if running on a development server.
func IsDevAppServer(c context.Context) bool {
	return Raw(c).IsDevAppServer()
}

// IsOverQuota returns true if the supplied error is an over quota error.
func IsOverQuota(c context.Context, err error) bool {
	return Raw(c).IsOverQuota(err)
}

// IsTimeoutError returns true if the supplied error indicates a timeout.
func IsTimeoutError(c context.Context, err error) bool {
	return Raw(c).IsTimeoutError(err)
}

// ModuleHostname returns the hostname of a module instance.
func ModuleHostname(c context.Context, module, version, instance string) (string, error) {
	return Raw(c).ModuleHostname(module, version, instance)
}

// ModuleName returns the current module name.
func ModuleName(c context.Context) string {
	return Raw(c).ModuleName()
}

// RequestID returns the current request ID.
func RequestID(c context.Context) string {
	return Raw(c).RequestID()
}

// ServerSoftware returns the AppEngine release version.
func ServerSoftware(c context.Context) string {
	return Raw(c).ServerSoftware()
}

// ServiceAccount returns the current service account name, in the form of an
// e-mail address.
func ServiceAccount(c context.Context) (string, error) {
	return Raw(c).ServiceAccount()
}

// VersionID returns the version ID for the current application, in the form
// "X.Y".
func VersionID(c context.Context) string {
	return Raw(c).VersionID()
}

// Namespace sets the current namespace. If the namespace is invalid or could
// not be set, an error will be returned.
func Namespace(c context.Context, namespace string) (context.Context, error) {
	return Raw(c).Namespace(namespace)
}

// MustNamespace is the same as Namespace, but will panic if there's an error.
// Since an error can only occur if namespace doesn't match the a regex this
// is valid to use if the namespace you're using is statically known, or known
// to conform to the regex. The regex in question is:
//
//   ^[0-9A-Za-z._-]{0,100}$
func MustNamespace(c context.Context, namespace string) context.Context {
	ret, err := Namespace(c, namespace)
	if err != nil {
		panic(err)
	}
	return ret
}

// AccessToken generates an OAuth2 access token for the specified scopes
// on behalf of the current ServiceAccount.
func AccessToken(c context.Context, scopes ...string) (token string, expiry time.Time, err error) {
	return Raw(c).AccessToken(scopes...)
}

// PublicCertificates retrieves the public certificates of the app.
func PublicCertificates(c context.Context) ([]Certificate, error) {
	return Raw(c).PublicCertificates()
}

// SignBytes signs bytes using the application's unique private key.
func SignBytes(c context.Context, bytes []byte) (keyName string, signature []byte, err error) {
	return Raw(c).SignBytes(bytes)
}

// GetTestable returns this Interface's Testing interface. Testing will return
// nil if testing is not supported in this implementation.
func GetTestable(c context.Context) Testable {
	return Raw(c).GetTestable()
}
