// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package auth

import (
	"golang.org/x/net/context"
)

// EmailScope is a scope used by OAuth2Method by default.
const EmailScope = "https://www.googleapis.com/auth/userinfo.email"

type oauthScopeKeyType int

var oauthScopeKey = oauthScopeKeyType(0)

// WithOAuthScopes installs the supplied OAuth scopes into the Context. They
// can be retrieved using OAuthScopes.
func WithOAuthScopes(c context.Context, scopes ...string) context.Context {
	return context.WithValue(c, oauthScopeKey, scopes)
}

// OAuthScopes returns the scopes installed in the Context by WithOAuthScopes.
//
// If no scopes are installed, it returns a default EmailScope.
func OAuthScopes(c context.Context) []string {
	scopes := []string(nil)
	if s, ok := c.Value(oauthScopeKey).([]string); ok {
		scopes = s
	}

	if len(scopes) > 0 {
		return scopes
	}
	return []string{EmailScope}
}
