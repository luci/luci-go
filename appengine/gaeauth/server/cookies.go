// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package server

import (
	"net/http"
	"strings"

	"golang.org/x/net/context"

	"github.com/luci/gae/service/info"
	"github.com/luci/gae/service/user"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/identity"
)

// UsersAPIAuthMethod implements auth.Method and auth.UsersAPI interfaces on top
// of GAE Users API (that uses HTTP cookies internally to track user sessions).
type UsersAPIAuthMethod struct{}

// Authenticate extracts peer's identity from the incoming request.
func (m UsersAPIAuthMethod) Authenticate(c context.Context, r *http.Request) (*auth.User, error) {
	u := user.Current(c)
	if u == nil {
		return nil, nil
	}
	id, err := identity.MakeIdentity("user:" + u.Email)
	if err != nil {
		return nil, err
	}
	return &auth.User{
		Identity:  id,
		Superuser: u.Admin,
		Email:     u.Email,
	}, nil
}

const (
	// serviceLoginURL is expected URL prefix for LoginURLs returned by prod GAE.
	serviceLoginURL = "https://www.google.com/accounts/ServiceLogin?"
	// accountChooserURL is what we use instead.
	accountChooserURL = "https://www.google.com/accounts/AccountChooser?"
)

// LoginURL returns a URL that, when visited, prompts the user to sign in,
// then redirects the user to the URL specified by dest.
func (m UsersAPIAuthMethod) LoginURL(c context.Context, dest string) (string, error) {
	url, err := user.LoginURL(c, dest)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(url, serviceLoginURL) {
		if !info.IsDevAppServer(c) {
			logging.Warningf(c, "Unexpected login URL: %q", url)
		}
		return url, nil
	}
	// Give the user a choice of existing accounts in their session or the option
	// to add an account, even if they are currently signed in to exactly one
	// account.
	return accountChooserURL + url[len(serviceLoginURL):], nil
}

// LogoutURL returns a URL that, when visited, signs the user out,
// then redirects the user to the URL specified by dest.
func (m UsersAPIAuthMethod) LogoutURL(c context.Context, dest string) (string, error) {
	return user.LogoutURL(c, dest)
}
