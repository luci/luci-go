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

package authtest

import (
	"errors"
	"net/http"
	"net/url"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/auth"
)

// ErrAuthenticationError is returned by FakeAuth.Authenticate.
var ErrAuthenticationError = errors.New("authtest: fake Authenticate error")

// FakeAuth implements auth.Method's Authenticate by returning predefined
// user.
type FakeAuth struct {
	User *auth.User // user to return in Authenticate or nil for error
}

// Authenticate returns predefined User object (if it is not nil) or error.
func (m FakeAuth) Authenticate(context.Context, *http.Request) (*auth.User, error) {
	if m.User == nil {
		return nil, ErrAuthenticationError
	}
	return m.User, nil
}

// LoginURL returns fake login URL.
func (m FakeAuth) LoginURL(c context.Context, dest string) (string, error) {
	return "http://fake.example.com/login?dest=" + url.QueryEscape(dest), nil
}

// LogoutURL returns fake logout URL.
func (m FakeAuth) LogoutURL(c context.Context, dest string) (string, error) {
	return "http://fake.example.com/logout?dest=" + url.QueryEscape(dest), nil
}
