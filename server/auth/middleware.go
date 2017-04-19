// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package auth

import (
	"github.com/luci/luci-go/server/router"
)

// Authenticate returns a middleware that performs authentication.
//
// This is simplest form of this middleware that uses only one authentication
// method. It is sufficient in most cases.
//
// This middleware either updates the context by injecting the authentication
// state into it (enabling functions like CurrentIdentity and IsMember), or
// aborts the request with an HTTP 401 or HTTP 500 error.
//
// Note that it passes through anonymous requests. CurrentIdentity returns
// identity.AnonymousIdentity in this case. Use separate authorization layer to
// further restrict the access, if necessary.
func Authenticate(m Method) router.Middleware {
	a := &Authenticator{Methods: []Method{m}}
	return a.GetMiddleware()
}
