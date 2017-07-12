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

package auth

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/router"

	"github.com/luci/luci-go/server/auth/authdb"
	"github.com/luci/luci-go/server/auth/identity"
	"github.com/luci/luci-go/server/auth/service/protocol"
	"github.com/luci/luci-go/server/auth/signing"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/retry/transient"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestAuthenticate(t *testing.T) {
	t.Parallel()

	Convey("Happy path", t, func() {
		c := injectTestDB(context.Background(), &fakeDB{
			allowedClientID: "some_client_id",
		})
		auth := Authenticator{
			Methods: []Method{fakeAuthMethod{clientID: "some_client_id"}},
		}
		c, err := auth.Authenticate(c, makeRequest())
		So(err, ShouldBeNil)

		So(CurrentUser(c), ShouldResemble, &User{
			Identity: "user:abc@example.com",
			Email:    "abc@example.com",
			ClientID: "some_client_id",
		})

		url, err := LoginURL(c, "login")
		So(err, ShouldBeNil)
		So(url, ShouldEqual, "http://fake.login.url/login")

		url, err = LogoutURL(c, "logout")
		So(err, ShouldBeNil)
		So(url, ShouldEqual, "http://fake.logout.url/logout")
	})

	Convey("No methods given", t, func() {
		c := injectTestDB(context.Background(), &fakeDB{
			allowedClientID: "some_client_id",
		})
		auth := Authenticator{}
		_, err := auth.Authenticate(c, makeRequest())
		So(err, ShouldEqual, ErrNotConfigured)
	})

	Convey("IsAllowedOAuthClientID on default DB", t, func() {
		c := context.Background()
		auth := Authenticator{
			Methods: []Method{fakeAuthMethod{clientID: "some_client_id"}},
		}
		_, err := auth.Authenticate(c, makeRequest())
		So(err, ShouldErrLike, "the library is not properly configured")
	})

	Convey("IsAllowedOAuthClientID with invalid client_id", t, func() {
		c := injectTestDB(context.Background(), &fakeDB{
			allowedClientID: "some_client_id",
		})
		auth := Authenticator{
			Methods: []Method{fakeAuthMethod{clientID: "another_client_id"}},
		}
		_, err := auth.Authenticate(c, makeRequest())
		So(err, ShouldEqual, ErrBadClientID)
	})

	Convey("IP whitelist restriction works", t, func() {
		db, err := authdb.NewSnapshotDB(&protocol.AuthDB{
			IpWhitelistAssignments: []*protocol.AuthIPWhitelistAssignment{
				{
					Identity:    strPtr("user:abc@example.com"),
					IpWhitelist: strPtr("whitelist"),
				},
			},
			IpWhitelists: []*protocol.AuthIPWhitelist{
				{
					Name: strPtr("whitelist"),
					Subnets: []string{
						"1.2.3.4/32",
					},
				},
			},
		}, "http://auth-service", 1234)
		So(err, ShouldBeNil)

		c := injectTestDB(context.Background(), db)

		Convey("User is using IP whitelist and IP is in the whitelist.", func() {
			auth := Authenticator{
				Methods: []Method{fakeAuthMethod{email: "abc@example.com"}},
			}
			req := makeRequest()
			req.RemoteAddr = "1.2.3.4"
			c, err := auth.Authenticate(c, req)
			So(err, ShouldBeNil)
			So(CurrentIdentity(c), ShouldEqual, identity.Identity("user:abc@example.com"))
		})

		Convey("User is using IP whitelist and IP is NOT in the whitelist.", func() {
			auth := Authenticator{
				Methods: []Method{fakeAuthMethod{email: "abc@example.com"}},
			}
			req := makeRequest()
			req.RemoteAddr = "1.2.3.5"
			_, err := auth.Authenticate(c, req)
			So(err, ShouldEqual, ErrIPNotWhitelisted)
		})

		Convey("User is not using IP whitelist.", func() {
			auth := Authenticator{
				Methods: []Method{fakeAuthMethod{email: "def@example.com"}},
			}
			req := makeRequest()
			req.RemoteAddr = "1.2.3.5"
			c, err := auth.Authenticate(c, req)
			So(err, ShouldBeNil)
			So(CurrentIdentity(c), ShouldEqual, identity.Identity("user:def@example.com"))
		})
	})
}

func TestMiddleware(t *testing.T) {
	t.Parallel()

	handler := func(c *router.Context) {
		fmt.Fprintf(c.Writer, "%s", CurrentIdentity(c.Context))
	}

	call := func(a *Authenticator) *httptest.ResponseRecorder {
		req, err := http.NewRequest("GET", "http://example.com/foo", nil)
		So(err, ShouldBeNil)
		w := httptest.NewRecorder()
		router.RunMiddleware(&router.Context{
			Context: injectTestDB(context.Background(), &fakeDB{
				allowedClientID: "some_client_id",
			}),
			Writer:  w,
			Request: req,
		}, router.NewMiddlewareChain(a.GetMiddleware()), handler)
		return w
	}

	Convey("Happy path", t, func() {
		rr := call(&Authenticator{
			Methods: []Method{fakeAuthMethod{clientID: "some_client_id"}},
		})
		So(rr.Code, ShouldEqual, 200)
		So(rr.Body.String(), ShouldEqual, "user:abc@example.com")
	})

	Convey("Fatal error", t, func() {
		rr := call(&Authenticator{
			Methods: []Method{fakeAuthMethod{clientID: "another_client_id"}},
		})
		So(rr.Code, ShouldEqual, 401)
		So(rr.Body.String(), ShouldEqual, "Authentication error\n")
	})

	Convey("Transient error", t, func() {
		rr := call(&Authenticator{
			Methods: []Method{fakeAuthMethod{err: errors.New("boo", transient.Tag)}},
		})
		So(rr.Code, ShouldEqual, 500)
		So(rr.Body.String(), ShouldEqual, "Transient error during authentication\n")
	})
}

///

func makeRequest() *http.Request {
	req, _ := http.NewRequest("GET", "http://some-url", nil)
	return req
}

///

// fakeAuthMethod implements Method.
type fakeAuthMethod struct {
	err      error
	clientID string
	email    string
}

func (m fakeAuthMethod) Authenticate(context.Context, *http.Request) (*User, error) {
	if m.err != nil {
		return nil, m.err
	}
	email := m.email
	if email == "" {
		email = "abc@example.com"
	}
	return &User{
		Identity: identity.Identity("user:" + email),
		Email:    email,
		ClientID: m.clientID,
	}, nil
}

func (m fakeAuthMethod) LoginURL(c context.Context, dest string) (string, error) {
	return "http://fake.login.url/" + dest, nil
}

func (m fakeAuthMethod) LogoutURL(c context.Context, dest string) (string, error) {
	return "http://fake.logout.url/" + dest, nil
}

func injectTestDB(c context.Context, d authdb.DB) context.Context {
	return setConfig(c, &Config{
		DBProvider: func(c context.Context) (authdb.DB, error) {
			return d, nil
		},
	})
}

func strPtr(s string) *string { return &s }

///

// fakeDB implements DB.
type fakeDB struct {
	allowedClientID string
	authServiceURL  string
	tokenServiceURL string
}

func (db *fakeDB) IsAllowedOAuthClientID(c context.Context, email, clientID string) (bool, error) {
	return clientID == db.allowedClientID, nil
}

func (db *fakeDB) IsMember(c context.Context, id identity.Identity, groups ...string) (bool, error) {
	return len(groups) != 0, nil
}

func (db *fakeDB) GetCertificates(c context.Context, id identity.Identity) (*signing.PublicCertificates, error) {
	return nil, errors.New("fakeDB: GetCertificates is not implemented")
}

func (db *fakeDB) GetWhitelistForIdentity(c context.Context, ident identity.Identity) (string, error) {
	return "", nil
}

func (db *fakeDB) IsInWhitelist(c context.Context, ip net.IP, whitelist string) (bool, error) {
	return whitelist == "bots" && ip.String() == "1.2.3.4", nil
}

func (db *fakeDB) GetAuthServiceURL(c context.Context) (string, error) {
	if db.authServiceURL == "" {
		return "", errors.New("fakeDB: GetAuthServiceURL is not configured")
	}
	return db.authServiceURL, nil
}

func (db *fakeDB) GetTokenServiceURL(c context.Context) (string, error) {
	if db.tokenServiceURL == "" {
		return "", errors.New("fakeDB: GetTokenServiceURL is not configured")
	}
	return db.tokenServiceURL, nil
}
