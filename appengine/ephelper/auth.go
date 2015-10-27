// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ephelper

import (
	gaeauth "github.com/luci/luci-go/appengine/gaeauth/server"
	"github.com/luci/luci-go/server/auth"
)

// NewAuthenticator returns a new Authenticator instance that authenticates
// using only Google Cloud Endpoints-specific methods.
func NewAuthenticator() *auth.Authenticator {
	return auth.NewAuthenticator([]auth.Method{
		&gaeauth.OAuth2Method{},
	})
}
