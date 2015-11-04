// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package auth

import (
	"fmt"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"

	"github.com/luci/luci-go/server/auth/identity"
	"github.com/luci/luci-go/server/middleware"
)

type authenticatorKey int

// SetAuthenticator injects copy of Authenticator (list of auth methods) into
// the context to use by default in LoginURL, LogoutURL and Authenticate.
// Usually installed into the context by some base middleware.
func SetAuthenticator(c context.Context, a Authenticator) context.Context {
	return context.WithValue(c, authenticatorKey(0), append(Authenticator(nil), a...))
}

// GetAuthenticator extracts instance of Authenticator (list of auth methods)
// from the context. Returns nil if no authenticator is set.
func GetAuthenticator(c context.Context) Authenticator {
	if a, ok := c.Value(authenticatorKey(0)).(Authenticator); ok {
		return a
	}
	return nil
}

// Use is a middleware that simply puts given Authenticator into the context.
func Use(h middleware.Handler, a Authenticator) middleware.Handler {
	return func(c context.Context, rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		h(SetAuthenticator(c, a), rw, r, p)
	}
}

// LoginURL returns a URL that, when visited, prompts the user to sign in,
// then redirects the user to the URL specified by dest. It is wrapper around
// LoginURL method of Authenticator in the context.
func LoginURL(c context.Context, dest string) (string, error) {
	return GetAuthenticator(c).LoginURL(c, dest)
}

// LogoutURL returns a URL that, when visited, signs the user out,
// then redirects the user to the URL specified by dest. It is wrapper around
// LogoutURL method of Authenticator in the context.
func LogoutURL(c context.Context, dest string) (string, error) {
	return GetAuthenticator(c).LogoutURL(c, dest)
}

// Authenticate returns a wrapper around middleware.Handler that performs
// authentication (using Authenticator in the context) and calls `h`.
func Authenticate(h middleware.Handler) middleware.Handler {
	return func(c context.Context, rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		a := GetAuthenticator(c)
		if a == nil {
			replyError(c, rw, 500, "Authentication middleware is not configured")
			return
		}
		ctx, err := a.Authenticate(c, r)
		switch {
		case errors.IsTransient(err):
			replyError(c, rw, 500, fmt.Sprintf("Transient error during authentication - %s", err))
		case err != nil:
			replyError(c, rw, 401, fmt.Sprintf("Authentication error - %s", err))
		default:
			h(ctx, rw, r, p)
		}
	}
}

// Autologin is a middleware that redirects the user to login page if the user
// is not signed in yet or authentication methods do not recognize user
// credentials. Uses Authenticator instance in the context.
func Autologin(h middleware.Handler) middleware.Handler {
	return func(c context.Context, rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		a := GetAuthenticator(c)
		if a == nil {
			replyError(c, rw, 500, "Authentication middleware is not configured")
			return
		}
		ctx, err := a.Authenticate(c, r)

		switch {
		case errors.IsTransient(err):
			replyError(c, rw, 500, fmt.Sprintf("Transient error during authentication - %s", err))

		case err != nil:
			logging.Errorf(c, "Authentication failure - %s. Redirecting to login", err)
			fallthrough

		case err != nil || CurrentIdentity(ctx).Kind() == identity.Anonymous:
			dest := r.RequestURI
			if dest == "" {
				// Make r.URL relative.
				destURL := *r.URL
				destURL.Host = ""
				destURL.Scheme = ""
				dest = destURL.String()
			}
			url, err := a.LoginURL(c, dest)
			if err != nil {
				if errors.IsTransient(err) {
					replyError(c, rw, 500, fmt.Sprintf("Transient error during authentication - %s", err))
				} else {
					replyError(c, rw, 401, fmt.Sprintf("Authentication error - %s", err))
				}
				return
			}
			http.Redirect(rw, r, url, 302)

		default:
			h(ctx, rw, r, p)
		}
	}
}

// replyError logs the error and writes it to ResponseWriter.
func replyError(c context.Context, rw http.ResponseWriter, code int, msg string) {
	logging.Errorf(c, "HTTP %d: %s", code, msg)
	http.Error(rw, msg, code)
}
