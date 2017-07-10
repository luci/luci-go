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

// RawInterface provides access to the "appengine/users" API methods.
type RawInterface interface {
	Current() *User
	CurrentOAuth(scopes ...string) (*User, error)

	IsAdmin() bool

	LoginURL(dest string) (string, error)
	LoginURLFederated(dest, identity string) (string, error)
	LogoutURL(dest string) (string, error)

	OAuthConsumerKey() (string, error)

	// If this implementation supports it, this will return an instance of the
	// Testable object for this service, which will let you 'log in' virtual users
	// in your test cases. If the implementation doesn't support it, it will
	// return nil.
	GetTestable() Testable
}

// Current returns the currently logged-in user, or nil if the user is not
// signed in.
func Current(c context.Context) *User {
	return Raw(c).Current()
}

// CurrentOAuth returns the user associated with the OAuth consumer making this
// request.
//
// If the OAuth consumer did not make a valid OAuth request, or the scopes is
// non-empty and the current user does not have at least one of the scopes, this
// method will return an error.
func CurrentOAuth(c context.Context, scopes ...string) (*User, error) {
	return Raw(c).CurrentOAuth(scopes...)
}

// IsAdmin returns true if the current user is an administrator for this
// AppEngine project.
func IsAdmin(c context.Context) bool {
	return Raw(c).IsAdmin()
}

// LoginURL returns a URL that, when visited, prompts the user to sign in, then
// redirects the user to the URL specified by dest.
func LoginURL(c context.Context, dest string) (string, error) {
	return Raw(c).LoginURL(dest)
}

// LoginURLFederated is like LoginURL but accepts a user's OpenID identifier.
func LoginURLFederated(c context.Context, dest, identity string) (string, error) {
	return Raw(c).LoginURLFederated(dest, identity)
}

// LogoutURL returns a URL that, when visited, signs the user out, then redirects
// the user to the URL specified by dest.
func LogoutURL(c context.Context, dest string) (string, error) {
	return Raw(c).LogoutURL(dest)
}

// OAuthConsumerKey returns the OAuth consumer key provided with the current
// request.
//
// This method will return an error if the OAuth request was invalid.
func OAuthConsumerKey(c context.Context) (string, error) {
	return Raw(c).OAuthConsumerKey()
}

// GetTestable returns a Testable for the current task queue service in c, or
// nil if it does not offer one.
//
// The Testable instance will let you 'log in' virtual users in your test cases.
func GetTestable(c context.Context) Testable {
	return Raw(c).GetTestable()
}
