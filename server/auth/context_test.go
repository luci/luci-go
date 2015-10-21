// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/server/auth/identity"
	"github.com/luci/luci-go/server/middleware"
	. "github.com/smartystreets/goconvey/convey"
)

func TestContext(t *testing.T) {
	Convey("Works", t, func() {
		c := context.Background()

		So(GetAuthenticator(c), ShouldBeNil)
		_, err := LoginURL(c, "dest")
		So(err, ShouldEqual, ErrNoUsersAPI)
		_, err = LogoutURL(c, "dest")
		So(err, ShouldEqual, ErrNoUsersAPI)

		// Authenticator without UsersAPI.
		c = SetAuthenticator(c, &Authenticator{})

		So(GetAuthenticator(c), ShouldNotBeNil)
		_, err = LoginURL(c, "dest")
		So(err, ShouldEqual, ErrNoUsersAPI)
		_, err = LogoutURL(c, "dest")
		So(err, ShouldEqual, ErrNoUsersAPI)

		// Authenticator with UsersAPI.
		c = SetAuthenticator(c, makeAuthenticator(fakeMethod{}))

		So(GetAuthenticator(c), ShouldNotBeNil)
		dest, err := LoginURL(c, "dest")
		So(err, ShouldBeNil)
		So(dest, ShouldEqual, "http://login_url?r=dest")
		dest, err = LogoutURL(c, "dest")
		So(err, ShouldBeNil)
		So(dest, ShouldEqual, "http://logout_url?r=dest")
	})

}

func TestAuthenticate(t *testing.T) {
	call := func(c context.Context, h middleware.Handler) *httptest.ResponseRecorder {
		req, err := http.NewRequest("GET", "http://example.com/foo", nil)
		So(err, ShouldBeNil)
		w := httptest.NewRecorder()
		h(c, w, req, nil)
		return w
	}

	handler := func(c context.Context, rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		fmt.Fprintf(rw, "%s", CurrentIdentity(c))
	}

	Convey("Not configured", t, func() {
		rr := call(context.Background(), Authenticate(handler, nil))
		So(rr.Code, ShouldEqual, 500)
		So(rr.Body.String(), ShouldEqual, "Authentication middleware is not configured\n")
	})

	Convey("Transient error", t, func() {
		rr := call(context.Background(), Authenticate(handler, makeAuthenticator(fakeMethod{
			authError: errors.WrapTransient(errors.New("boo")),
		})))
		So(rr.Code, ShouldEqual, 500)
		So(rr.Body.String(), ShouldEqual, "Transient error during authentication - boo\n")
	})

	Convey("Fatal error", t, func() {
		rr := call(context.Background(), Authenticate(handler, makeAuthenticator(fakeMethod{
			authError: errors.New("boo"),
		})))
		So(rr.Code, ShouldEqual, 401)
		So(rr.Body.String(), ShouldEqual, "Authentication error - boo\n")
	})

	Convey("Works", t, func() {
		rr := call(context.Background(), Authenticate(handler, makeAuthenticator(fakeMethod{
			userID: "user:abc@example.com",
		})))
		So(rr.Code, ShouldEqual, 200)
		So(rr.Body.String(), ShouldEqual, "user:abc@example.com")
	})

	Convey("Anonymous works", t, func() {
		rr := call(context.Background(), Authenticate(handler, &Authenticator{}))
		So(rr.Code, ShouldEqual, 200)
		So(rr.Body.String(), ShouldEqual, "anonymous:anonymous")
	})

	Convey("Broken ID is rejected", t, func() {
		rr := call(context.Background(), Authenticate(handler, makeAuthenticator(fakeMethod{
			userID: "???",
		})))
		So(rr.Code, ShouldEqual, 401)
		So(rr.Body.String(), ShouldEqual, "Authentication error - auth: bad identity string \"???\"\n")
	})
}

func TestAutologin(t *testing.T) {
	call := func(c context.Context, h middleware.Handler) *httptest.ResponseRecorder {
		req, err := http.NewRequest("GET", "http://example.com/foo", nil)
		So(err, ShouldBeNil)
		w := httptest.NewRecorder()
		h(c, w, req, nil)
		return w
	}

	handler := func(c context.Context, rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		fmt.Fprintf(rw, "%s", CurrentIdentity(c))
	}

	Convey("Not configured", t, func() {
		rr := call(context.Background(), Autologin(handler, nil))
		So(rr.Code, ShouldEqual, 500)
		So(rr.Body.String(), ShouldEqual, "Authentication middleware is not configured\n")
	})

	Convey("Transient error", t, func() {
		rr := call(context.Background(), Autologin(handler, makeAuthenticator(fakeMethod{
			authError: errors.WrapTransient(errors.New("boo")),
		})))
		So(rr.Code, ShouldEqual, 500)
		So(rr.Body.String(), ShouldEqual, "Transient error during authentication - boo\n")
	})

	Convey("Fatal error", t, func() {
		rr := call(context.Background(), Autologin(handler, makeAuthenticator(fakeMethod{
			authError: errors.New("boo"),
		})))
		So(rr.Code, ShouldEqual, 302)
		So(rr.Header().Get("Location"), ShouldEqual, "http://login_url?r=%2Ffoo")
	})

	Convey("Anonymous is redirected to login if has UsersAPI", t, func() {
		rr := call(context.Background(), Autologin(handler, makeAuthenticator(fakeMethod{})))
		So(rr.Code, ShouldEqual, 302)
		So(rr.Header().Get("Location"), ShouldEqual, "http://login_url?r=%2Ffoo")
	})

	Convey("Anonymous is rejected if no UsersAPI", t, func() {
		rr := call(context.Background(), Autologin(handler, &Authenticator{}))
		So(rr.Code, ShouldEqual, 401)
		So(rr.Body.String(), ShouldEqual, "Authentication error - auth: methods do not support login or logout URL\n")
	})

	Convey("Handles transient error in LoginURL", t, func() {
		rr := call(context.Background(), Autologin(handler, makeAuthenticator(fakeMethod{
			loginURLError: errors.WrapTransient(errors.New("boo")),
		})))
		So(rr.Code, ShouldEqual, 500)
		So(rr.Body.String(), ShouldEqual, "Transient error during authentication - boo\n")
	})

	Convey("Passes authenticated user through", t, func() {
		rr := call(context.Background(), Autologin(handler, makeAuthenticator(fakeMethod{
			userID: "user:abc@example.com",
		})))
		So(rr.Code, ShouldEqual, 200)
		So(rr.Body.String(), ShouldEqual, "user:abc@example.com")
	})
}

func makeAuthenticator(m fakeMethod) *Authenticator {
	return &Authenticator{
		Methods: []Method{&m},
	}
}

type fakeMethod struct {
	authError     error
	loginURLError error
	userID        identity.Identity
}

func (m *fakeMethod) Authenticate(context.Context, *http.Request) (*User, error) {
	if m.authError != nil {
		return nil, m.authError
	}
	return &User{Identity: m.userID}, nil
}

func (m *fakeMethod) LoginURL(c context.Context, dest string) (string, error) {
	if m.loginURLError != nil {
		return "", m.loginURLError
	}
	v := url.Values{}
	v.Set("r", dest)
	return "http://login_url?" + v.Encode(), nil
}

func (m *fakeMethod) LogoutURL(c context.Context, dest string) (string, error) {
	v := url.Values{}
	v.Set("r", dest)
	return "http://logout_url?" + v.Encode(), nil
}
