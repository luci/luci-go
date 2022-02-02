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

package server

import (
	"context"
	"net/http"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/gae/service/user"
	"go.chromium.org/luci/server/auth"
)

// UsersAPIAuthMethod implements auth.Method and auth.UsersAPI interfaces on top
// of GAE Users API (that uses HTTP cookies internally to track user sessions).
//
// Deprecated: this method depends on Users API not available outside of the GAE
// first-gen runtime. Use go.chromium.org/luci/server/encryptedcookies instead.
type UsersAPIAuthMethod struct{}

// Authenticate extracts peer's identity from the incoming request.
func (m UsersAPIAuthMethod) Authenticate(ctx context.Context, r *http.Request) (*auth.User, auth.Session, error) {
	u := user.Current(ctx)
	if u == nil {
		return nil, nil, nil
	}
	id, err := identity.MakeIdentity("user:" + u.Email)
	if err != nil {
		return nil, nil, err
	}
	return &auth.User{
		Identity:  id,
		Superuser: u.Admin,
		Email:     u.Email,
	}, nil, nil
}

// LoginURL returns a URL that, when visited, prompts the user to sign in,
// then redirects the user to the URL specified by dest.
func (m UsersAPIAuthMethod) LoginURL(ctx context.Context, dest string) (string, error) {
	return user.LoginURL(ctx, dest)
}

// LogoutURL returns a URL that, when visited, signs the user out,
// then redirects the user to the URL specified by dest.
func (m UsersAPIAuthMethod) LogoutURL(ctx context.Context, dest string) (string, error) {
	return user.LogoutURL(ctx, dest)
}
