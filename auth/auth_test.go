// Copyright 2014 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package auth

import (
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"infra/libs/auth/internal"
	"infra/libs/logging"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func ExampleDefaultAuthenticatedClient() {
	client, err := DefaultAuthenticatedClient(SilentLogin)
	if err == ErrLoginRequired {
		logging.Errorf("Run 'auth login' to login")
		return
	}
	if err != nil {
		logging.Errorf("Failed to login: %s", err)
		return
	}
	client.Get("https://some-server.appspot.com")
}

func mockSecretsDir() string {
	tempDir, err := ioutil.TempDir("", "auth_test")
	So(err, ShouldBeNil)

	prev := secretsDir
	secretsDir = func() string { return tempDir }
	Reset(func() {
		secretsDir = prev
		os.RemoveAll(tempDir)
	})
	So(SecretsDir(), ShouldEqual, tempDir)

	return tempDir
}

func mockTokenProvider(factory func() internal.TokenProvider) {
	prev := makeTokenProvider
	makeTokenProvider = func(*Options) (internal.TokenProvider, error) {
		return factory(), nil
	}
	Reset(func() {
		makeTokenProvider = prev
	})
}

func mockTerminal() {
	prev := logging.IsTerminal
	logging.IsTerminal = true
	Reset(func() {
		logging.IsTerminal = prev
	})
}

func TestAuthenticator(t *testing.T) {
	Convey("Given mocked secrets dir", t, func() {
		tempDir := mockSecretsDir()

		Convey("Check NewAuthenticator defaults", func() {
			clientID, clientSecret := DefaultClient()
			a := NewAuthenticator(Options{}).(*authenticatorImpl)
			So(a.opts, ShouldResemble, &Options{
				Method:                 AutoSelectMethod,
				Scopes:                 []string{OAuthScopeEmail},
				ClientID:               clientID,
				ClientSecret:           clientSecret,
				ServiceAccountJSONPath: filepath.Join(tempDir, "service_account.json"),
				GCEAccountName:         "default",
				Context:                context.TODO(),
				Log:                    logging.DefaultLogger,
			})
		})
	})
}

func TestPurgeCredentialsCache(t *testing.T) {
	Convey("Given mocked secrets dir", t, func() {
		tempDir := mockSecretsDir()

		makeFile := func(name string) {
			err := ioutil.WriteFile(filepath.Join(tempDir, name), []byte(""), 0600)
			So(err, ShouldBeNil)
		}

		makeFile("secret1.tok")
		makeFile("secret2.tok")
		makeFile("not-secret.txt")

		Convey("Check PurgeCredentialsCache", func() {
			err := PurgeCredentialsCache()
			So(err, ShouldBeNil)
			left, err := ioutil.ReadDir(tempDir)
			So(err, ShouldBeNil)
			So(len(left), ShouldEqual, 1)
			So(left[0].Name(), ShouldEqual, "not-secret.txt")
		})
	})
}

func TestAuthenticatedClient(t *testing.T) {
	Convey("Given mocked secrets dir", t, func() {
		var tokenProvider internal.TokenProvider

		mockTerminal()
		mockSecretsDir()
		mockTokenProvider(func() internal.TokenProvider { return tokenProvider })

		Convey("Test login required", func() {
			tokenProvider = &fakeTokenProvider{interactive: true}
			c, err := AuthenticatedClient(InteractiveLogin, NewAuthenticator(Options{}))
			So(err, ShouldBeNil)
			So(c, ShouldNotEqual, http.DefaultClient)
		})

		Convey("Test login not required", func() {
			tokenProvider = &fakeTokenProvider{interactive: true}
			c, err := AuthenticatedClient(OptionalLogin, NewAuthenticator(Options{}))
			So(err, ShouldBeNil)
			So(c, ShouldEqual, http.DefaultClient)
		})
	})
}

func TestRefreshToken(t *testing.T) {
	Convey("Given mocked secrets dir", t, func() {
		var tokenProvider *fakeTokenProvider

		mockTerminal()
		mockSecretsDir()
		mockTokenProvider(func() internal.TokenProvider { return tokenProvider })

		Convey("Test non interactive auth", func() {
			tokenProvider = &fakeTokenProvider{
				interactive: false,
				tokenToMint: &fakeToken{},
			}
			auth, ok := NewAuthenticator(Options{}).(*authenticatorImpl)
			So(ok, ShouldBeTrue)
			_, err := auth.Transport()
			So(err, ShouldBeNil)
			// No token yet. The token is minted on first refresh.
			So(auth.currentToken(), ShouldBeNil)
			tok, err := auth.refreshToken(nil)
			So(err, ShouldBeNil)
			So(tok, ShouldEqual, tokenProvider.tokenToMint)
		})

		Convey("Test interactive auth (cache expired)", func() {
			tokenProvider = &fakeTokenProvider{
				interactive:      true,
				tokenToMint:      &fakeToken{name: "minted"},
				tokenToRefresh:   &fakeToken{name: "refreshed"},
				tokenToUnmarshal: &fakeToken{name: "cached", expired: true},
			}
			auth, ok := NewAuthenticator(Options{}).(*authenticatorImpl)
			So(ok, ShouldBeTrue)
			_, err := auth.Transport()
			So(err, ShouldEqual, ErrLoginRequired)
			err = auth.Login()
			So(err, ShouldBeNil)
			_, err = auth.Transport()
			So(err, ShouldBeNil)
			// Minted initial token.
			So(auth.currentToken(), ShouldEqual, tokenProvider.tokenToMint)
			// Should return refreshed token.
			tok, err := auth.refreshToken(auth.currentToken())
			So(err, ShouldBeNil)
			So(tok, ShouldEqual, tokenProvider.tokenToRefresh)
		})

		Convey("Test interactive auth (cache non expired)", func() {
			tokenProvider = &fakeTokenProvider{
				interactive:      true,
				tokenToMint:      &fakeToken{name: "minted"},
				tokenToRefresh:   &fakeToken{name: "refreshed"},
				tokenToUnmarshal: &fakeToken{name: "cached", expired: false},
			}
			auth, ok := NewAuthenticator(Options{}).(*authenticatorImpl)
			So(ok, ShouldBeTrue)
			_, err := auth.Transport()
			So(err, ShouldEqual, ErrLoginRequired)
			err = auth.Login()
			So(err, ShouldBeNil)
			_, err = auth.Transport()
			So(err, ShouldBeNil)
			// Minted initial token.
			So(auth.currentToken(), ShouldEqual, tokenProvider.tokenToMint)
			// Should return token from cache (since it's not expired yet).
			tok, err := auth.refreshToken(auth.currentToken())
			So(err, ShouldBeNil)
			So(tok, ShouldEqual, tokenProvider.tokenToUnmarshal)
		})
	})
}

////////////////////////////////////////////////////////////////////////////////

type fakeTokenProvider struct {
	interactive      bool
	tokenToMint      internal.Token
	tokenToRefresh   internal.Token
	tokenToUnmarshal internal.Token
}

func (p *fakeTokenProvider) RequiresInteraction() bool {
	return p.interactive
}

func (p *fakeTokenProvider) MintToken() (internal.Token, error) {
	if p.tokenToMint != nil {
		return p.tokenToMint, nil
	}
	return &fakeToken{}, nil
}

func (p *fakeTokenProvider) RefreshToken(internal.Token) (internal.Token, error) {
	if p.tokenToRefresh != nil {
		return p.tokenToRefresh, nil
	}
	return &fakeToken{}, nil
}

func (p *fakeTokenProvider) MarshalToken(internal.Token) ([]byte, error) {
	return []byte("fake token"), nil
}

func (p *fakeTokenProvider) UnmarshalToken([]byte) (internal.Token, error) {
	if p.tokenToUnmarshal != nil {
		return p.tokenToUnmarshal, nil
	}
	return &fakeToken{}, nil
}

type fakeToken struct {
	name    string
	expired bool
}

func (t *fakeToken) Equals(another internal.Token) bool {
	casted, ok := another.(*fakeToken)
	return ok && casted == t
}

func (t *fakeToken) RequestHeaders() map[string]string { return make(map[string]string) }
func (t *fakeToken) Expired() bool                     { return t.expired }
