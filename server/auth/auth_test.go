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
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/service/protocol"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/router"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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
		req := makeRequest()
		req.FakeRemoteAddr = "1.2.3.4"
		c, err := auth.Authenticate(c, req)
		So(err, ShouldBeNil)

		So(CurrentUser(c), ShouldResemble, &User{
			Identity: "user:abc@example.com",
			Email:    "abc@example.com",
			ClientID: "some_client_id",
		})

		So(GetState(c).PeerIP().String(), ShouldEqual, "1.2.3.4")

		url, err := LoginURL(c, "login")
		So(err, ShouldBeNil)
		So(url, ShouldEqual, "http://fake.login.url/login")

		url, err = LogoutURL(c, "logout")
		So(err, ShouldBeNil)
		So(url, ShouldEqual, "http://fake.logout.url/logout")

		tok, extra, err := GetState(c).UserCredentials()
		So(err, ShouldBeNil)
		So(tok, ShouldResemble, &oauth2.Token{AccessToken: "token-abc@example.com"})
		So(extra, ShouldHaveLength, 0)
	})

	Convey("Custom EndUserIP implementation", t, func() {
		req := makeRequest()
		req.FakeHeader.Add("X-Custom-IP", "4.5.6.7")

		c := injectTestDB(context.Background(), &fakeDB{})
		c = ModifyConfig(c, func(cfg Config) Config {
			cfg.EndUserIP = func(r RequestMetadata) string { return r.Header("X-Custom-IP") }
			return cfg
		})

		auth := Authenticator{
			Methods: []Method{fakeAuthMethod{email: "zzz@example.com"}},
		}
		c, err := auth.Authenticate(c, req)
		So(err, ShouldBeNil)
		So(GetState(c).PeerIP().String(), ShouldEqual, "4.5.6.7")
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
		c = injectFrontendClientID(c, "frontend_client_id")
		auth := Authenticator{
			Methods: []Method{fakeAuthMethod{clientID: "another_client_id"}},
		}
		_, err := auth.Authenticate(c, makeRequest())
		So(err, ShouldEqual, ErrBadClientID)
	})

	Convey("IsAllowedOAuthClientID with frontend client_id", t, func() {
		c := injectTestDB(context.Background(), &fakeDB{
			allowedClientID: "some_client_id",
		})
		c = injectFrontendClientID(c, "frontend_client_id")
		auth := Authenticator{
			Methods: []Method{fakeAuthMethod{clientID: "frontend_client_id"}},
		}
		_, err := auth.Authenticate(c, makeRequest())
		So(err, ShouldBeNil) // success!
	})

	Convey("IP allowlist restriction works", t, func() {
		db, err := authdb.NewSnapshotDB(&protocol.AuthDB{
			IpWhitelistAssignments: []*protocol.AuthIPWhitelistAssignment{
				{
					Identity:    "user:abc@example.com",
					IpWhitelist: "allowlist",
				},
			},
			IpWhitelists: []*protocol.AuthIPWhitelist{
				{
					Name: "allowlist",
					Subnets: []string{
						"1.2.3.4/32",
					},
				},
			},
		}, "http://auth-service", 1234, false)
		So(err, ShouldBeNil)

		c := injectTestDB(context.Background(), db)

		Convey("User is using IP allowlist and IP is in the allowlist.", func() {
			auth := Authenticator{
				Methods: []Method{fakeAuthMethod{email: "abc@example.com"}},
			}
			req := makeRequest()
			req.FakeRemoteAddr = "1.2.3.4"
			c, err := auth.Authenticate(c, req)
			So(err, ShouldBeNil)
			So(CurrentIdentity(c), ShouldEqual, identity.Identity("user:abc@example.com"))
		})

		Convey("User is using IP allowlist and IP is NOT in the allowlist.", func() {
			auth := Authenticator{
				Methods: []Method{fakeAuthMethod{email: "abc@example.com"}},
			}
			req := makeRequest()
			req.FakeRemoteAddr = "1.2.3.5"
			_, err := auth.Authenticate(c, req)
			So(err, ShouldEqual, ErrForbiddenIP)
		})

		Convey("User is not using IP allowlist.", func() {
			auth := Authenticator{
				Methods: []Method{fakeAuthMethod{email: "def@example.com"}},
			}
			req := makeRequest()
			req.FakeRemoteAddr = "1.2.3.5"
			c, err := auth.Authenticate(c, req)
			So(err, ShouldBeNil)
			So(CurrentIdentity(c), ShouldEqual, identity.Identity("user:def@example.com"))
		})
	})

	Convey("X-Luci-Project works", t, func() {
		c := injectTestDB(context.Background(), &fakeDB{
			groups: map[string][]identity.Identity{
				InternalServicesGroup: {"user:allowed@example.com"},
			},
		})

		Convey("Allowed", func() {
			auth := Authenticator{
				Methods: []Method{fakeAuthMethod{email: "allowed@example.com"}},
			}
			req := makeRequest()
			req.FakeHeader.Set(XLUCIProjectHeader, "test-proj")
			c, err := auth.Authenticate(c, req)
			So(err, ShouldBeNil)
			So(CurrentIdentity(c), ShouldEqual, identity.Identity("project:test-proj"))

			tok, extra, err := GetState(c).UserCredentials()
			So(err, ShouldBeNil)
			So(tok, ShouldResemble, &oauth2.Token{AccessToken: "token-allowed@example.com"})
			So(extra, ShouldResemble, map[string]string{XLUCIProjectHeader: "test-proj"})
		})

		Convey("Forbidden", func() {
			auth := Authenticator{
				Methods: []Method{fakeAuthMethod{email: "unknown@example.com"}},
			}
			req := makeRequest()
			req.FakeHeader.Set(XLUCIProjectHeader, "test-proj")
			_, err := auth.Authenticate(c, req)
			So(err, ShouldEqual, ErrProjectHeaderForbidden)
		})

		Convey("Bad project ID", func() {
			auth := Authenticator{
				Methods: []Method{fakeAuthMethod{email: "allowed@example.com"}},
			}
			req := makeRequest()
			req.FakeHeader.Set(XLUCIProjectHeader, "?????")
			_, err := auth.Authenticate(c, req)
			So(err, ShouldErrLike, "bad value")
		})
	})
}

func TestMiddleware(t *testing.T) {
	t.Parallel()

	handler := func(c *router.Context) {
		fmt.Fprintf(c.Writer, "%s", CurrentIdentity(c.Request.Context()))
	}

	call := func(a *Authenticator) *httptest.ResponseRecorder {
		req, err := http.NewRequest("GET", "http://example.com/foo", nil)
		So(err, ShouldBeNil)
		w := httptest.NewRecorder()
		router.RunMiddleware(&router.Context{
			Writer: w,
			Request: req.WithContext(injectTestDB(context.Background(), &fakeDB{
				allowedClientID: "some_client_id",
			})),
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
		So(rr.Code, ShouldEqual, 403)
		So(rr.Body.String(), ShouldEqual, ErrBadClientID.Error()+"\n")
	})

	Convey("Transient error", t, func() {
		rr := call(&Authenticator{
			Methods: []Method{fakeAuthMethod{err: errors.New("boo", transient.Tag)}},
		})
		So(rr.Code, ShouldEqual, 500)
		So(rr.Body.String(), ShouldEqual, "Internal Server Error\n")
	})
}

///

type fakeRequest struct {
	FakeRemoteAddr string
	FakeHost       string
	FakeHeader     http.Header
}

func (r *fakeRequest) Header(key string) string                { return r.FakeHeader.Get(key) }
func (r *fakeRequest) Cookie(key string) (*http.Cookie, error) { return nil, fmt.Errorf("no cookie") }
func (r *fakeRequest) RemoteAddr() string                      { return r.FakeRemoteAddr }
func (r *fakeRequest) Host() string                            { return r.FakeHost }

func makeRequest() *fakeRequest {
	return &fakeRequest{
		FakeRemoteAddr: "127.0.0.1",
		FakeHost:       "some-url",
		FakeHeader:     map[string][]string{},
	}
}

///

// fakeAuthMethod implements Method.
type fakeAuthMethod struct {
	err      error
	clientID string
	email    string
	observe  func(RequestMetadata)
}

func (m fakeAuthMethod) Authenticate(_ context.Context, r RequestMetadata) (*User, Session, error) {
	if m.observe != nil {
		m.observe(r)
	}
	if m.err != nil {
		return nil, nil, m.err
	}
	email := m.email
	if email == "" {
		email = "abc@example.com"
	}
	return &User{
		Identity: identity.Identity("user:" + email),
		Email:    email,
		ClientID: m.clientID,
	}, nil, nil
}

func (m fakeAuthMethod) LoginURL(ctx context.Context, dest string) (string, error) {
	return "http://fake.login.url/" + dest, nil
}

func (m fakeAuthMethod) LogoutURL(ctx context.Context, dest string) (string, error) {
	return "http://fake.logout.url/" + dest, nil
}

func (m fakeAuthMethod) GetUserCredentials(context.Context, RequestMetadata) (*oauth2.Token, error) {
	email := m.email
	if email == "" {
		email = "abc@example.com"
	}
	return &oauth2.Token{AccessToken: "token-" + email}, nil
}

func injectTestDB(ctx context.Context, d authdb.DB) context.Context {
	return ModifyConfig(ctx, func(cfg Config) Config {
		cfg.DBProvider = func(ctx context.Context) (authdb.DB, error) {
			return d, nil
		}
		return cfg
	})
}

func injectFrontendClientID(ctx context.Context, clientID string) context.Context {
	return ModifyConfig(ctx, func(cfg Config) Config {
		cfg.FrontendClientID = func(context.Context) (string, error) {
			return clientID, nil
		}
		return cfg
	})
}

///

// fakeDB implements DB.
type fakeDB struct {
	allowedClientID string
	internalService string
	authServiceURL  string
	tokenServiceURL string
	groups          map[string][]identity.Identity
	realmData       map[string]*protocol.RealmData
}

func (db *fakeDB) IsAllowedOAuthClientID(ctx context.Context, email, clientID string) (bool, error) {
	return clientID == db.allowedClientID, nil
}

func (db *fakeDB) IsInternalService(ctx context.Context, hostname string) (bool, error) {
	return hostname == db.internalService, nil
}

func (db *fakeDB) IsMember(ctx context.Context, id identity.Identity, groups []string) (bool, error) {
	for _, g := range groups {
		for _, member := range db.groups[g] {
			if id == member {
				return true, nil
			}
		}
	}
	return false, nil
}

func (db *fakeDB) CheckMembership(ctx context.Context, id identity.Identity, groups []string) ([]string, error) {
	panic("not implemented")
}

func (db *fakeDB) HasPermission(ctx context.Context, id identity.Identity, perm realms.Permission, realm string, attrs realms.Attrs) (bool, error) {
	return false, errors.New("fakeDB: HasPermission is not implemented")
}

func (db *fakeDB) QueryRealms(ctx context.Context, id identity.Identity, perm realms.Permission, project string, attrs realms.Attrs) ([]string, error) {
	return nil, errors.New("fakeDB: QueryRealms is not implemented")
}

func (db *fakeDB) FilterKnownGroups(ctx context.Context, groups []string) ([]string, error) {
	return nil, errors.New("fakeDB: FilterKnownGroups is not implemented")
}

func (db *fakeDB) GetCertificates(ctx context.Context, id identity.Identity) (*signing.PublicCertificates, error) {
	return nil, errors.New("fakeDB: GetCertificates is not implemented")
}

func (db *fakeDB) GetAllowlistForIdentity(ctx context.Context, ident identity.Identity) (string, error) {
	return "", nil
}

func (db *fakeDB) IsAllowedIP(ctx context.Context, ip net.IP, allowlist string) (bool, error) {
	return allowlist == "bots" && ip.String() == "1.2.3.4", nil
}

func (db *fakeDB) GetAuthServiceURL(ctx context.Context) (string, error) {
	if db.authServiceURL == "" {
		return "", errors.New("fakeDB: GetAuthServiceURL is not configured")
	}
	return db.authServiceURL, nil
}

func (db *fakeDB) GetTokenServiceURL(ctx context.Context) (string, error) {
	if db.tokenServiceURL == "" {
		return "", errors.New("fakeDB: GetTokenServiceURL is not configured")
	}
	return db.tokenServiceURL, nil
}

func (db *fakeDB) GetRealmData(ctx context.Context, realm string) (*protocol.RealmData, error) {
	return db.realmData[realm], nil
}
