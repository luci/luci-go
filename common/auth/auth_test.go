// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package auth

import (
	"io/ioutil"
	"net/http"
	"os"
	"testing"
	"time"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/luci/luci-go/common/auth/internal"
	"github.com/luci/luci-go/common/logging"
)

var (
	ctx    = context.Background()
	log    = logging.Null()
	past   = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
	future = time.Now().Add(24 * time.Hour)
)

func ExampleDefaultAuthenticatedClient() {
	client, err := AuthenticatedClient(SilentLogin, NewAuthenticator(Options{}))
	if err == ErrLoginRequired {
		log.Errorf("Run 'auth login' to login")
		return
	}
	if err != nil {
		log.Errorf("Failed to login: %s", err)
		return
	}
	_, _ = client.Get("https://some-server.appspot.com")
}

func mockSecretsDir() string {
	tempDir, err := ioutil.TempDir("", "auth_test")
	So(err, ShouldBeNil)

	prev := secretsDir
	secretsDir = func() string { return tempDir }
	Reset(func() {
		secretsDir = prev
		So(os.RemoveAll(tempDir), ShouldBeNil)
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

func TestAuthenticator(t *testing.T) {
	Convey("Given mocked secrets dir", t, func() {
		Convey("Check NewAuthenticator defaults", func() {
			clientID, clientSecret := DefaultClient()
			ctx := context.Background()
			a := NewAuthenticator(Options{Context: ctx})
			So(a.opts, ShouldResemble, &Options{
				ClientID:               clientID,
				ClientSecret:           clientSecret,
				Context:                ctx,
				GCEAccountName:         "default",
				Logger:                 logging.Get(ctx),
				Method:                 AutoSelectMethod,
				Scopes:                 []string{OAuthScopeEmail},
				ServiceAccountJSONPath: "",
				Transport:              http.DefaultTransport,
			})
		})
	})
}

func TestAuthenticatedClient(t *testing.T) {
	Convey("Given mocked secrets dir", t, func() {
		var tokenProvider internal.TokenProvider

		mockSecretsDir()
		mockTokenProvider(func() internal.TokenProvider { return tokenProvider })

		Convey("Test login required", func() {
			tokenProvider = &fakeTokenProvider{interactive: true}
			t, err := AuthenticatedTransport(InteractiveLogin, NewAuthenticator(Options{}))
			So(err, ShouldBeNil)
			So(t, ShouldNotEqual, http.DefaultTransport)
		})

		Convey("Test login not required", func() {
			tokenProvider = &fakeTokenProvider{interactive: true}
			t, err := AuthenticatedTransport(OptionalLogin, NewAuthenticator(Options{}))
			So(err, ShouldBeNil)
			So(t, ShouldEqual, http.DefaultTransport)
		})
	})
}

func TestRefreshToken(t *testing.T) {
	Convey("Given mocked secrets dir", t, func() {
		var tokenProvider *fakeTokenProvider

		mockSecretsDir()
		mockTokenProvider(func() internal.TokenProvider { return tokenProvider })

		Convey("Test non interactive auth", func() {
			tokenProvider = &fakeTokenProvider{
				interactive: false,
				tokenToMint: &fakeToken{},
			}
			auth := NewAuthenticator(Options{})
			_, err := auth.Transport()
			So(err, ShouldBeNil)
			// No token yet. The token is minted on first refresh.
			So(auth.currentToken(), ShouldBeNil)
			tok, err := auth.refreshToken(nil, time.Minute, false)
			So(err, ShouldBeNil)
			So(tok, ShouldEqual, tokenProvider.tokenToMint)
		})

		Convey("Test interactive auth (cache expired)", func() {
			tokenProvider = &fakeTokenProvider{
				interactive:      true,
				tokenToMint:      &fakeToken{name: "minted"},
				tokenToRefresh:   &fakeToken{name: "refreshed"},
				tokenToUnmarshal: &fakeToken{name: "cached", expiry: past},
			}
			auth := NewAuthenticator(Options{})
			_, err := auth.Transport()
			So(err, ShouldEqual, ErrLoginRequired)
			err = auth.Login()
			So(err, ShouldBeNil)
			_, err = auth.Transport()
			So(err, ShouldBeNil)
			// Minted initial token.
			So(auth.currentToken(), ShouldEqual, tokenProvider.tokenToMint)
			// Should return refreshed token.
			tok, err := auth.refreshToken(auth.currentToken(), time.Minute, false)
			So(err, ShouldBeNil)
			So(tok, ShouldEqual, tokenProvider.tokenToRefresh)
		})

		Convey("Test interactive auth (cache non expired)", func() {
			tokenProvider = &fakeTokenProvider{
				interactive:      true,
				tokenToMint:      &fakeToken{name: "minted"},
				tokenToRefresh:   &fakeToken{name: "refreshed"},
				tokenToUnmarshal: &fakeToken{name: "cached", expiry: future},
			}
			auth := NewAuthenticator(Options{})
			_, err := auth.Transport()
			So(err, ShouldEqual, ErrLoginRequired)
			err = auth.Login()
			So(err, ShouldBeNil)
			_, err = auth.Transport()
			So(err, ShouldBeNil)
			// Minted initial token.
			So(auth.currentToken(), ShouldEqual, tokenProvider.tokenToMint)
			// Should return token from cache (since it's not expired yet).
			tok, err := auth.refreshToken(auth.currentToken(), time.Minute, false)
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

func (p *fakeTokenProvider) CacheSeed() []byte {
	return nil
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
	name   string
	expiry time.Time
}

func (t *fakeToken) Equals(another internal.Token) bool {
	casted, ok := another.(*fakeToken)
	return ok && casted == t
}

func (t *fakeToken) RequestHeaders() map[string]string { return make(map[string]string) }
func (t *fakeToken) Expiry() time.Time                 { return t.expiry }
func (t *fakeToken) AccessToken() string               { return "token" }
