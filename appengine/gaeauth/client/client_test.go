// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package client

import (
	"math/rand"
	"net/http"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/info"
	"github.com/luci/gae/service/urlfetch"

	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/mathrand"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestTokenMinter(t *testing.T) {
	Convey("works", t, func() {
		c := testContext()
		scopes := []string{"a", "b", "c"}
		expiry := time.Unix(1442540000, 0)
		c = mockAccessTokenRPC(c, scopes, "token", expiry)
		tok, err := (tokenMinter{c}).MintToken(scopes)
		So(err, ShouldBeNil)
		So(tok, ShouldResemble, auth.Token{
			AccessToken: "token",
			Expiry:      expiry,
		})
	})
}

func TestTokenCache(t *testing.T) {
	Convey("with mocked time", t, func() {
		now := time.Unix(1442540000, 0)
		c := memory.Use(context.Background())
		c, clk := testclock.UseTime(c, now)
		cache := tokenCache{c, "key"}

		Convey("normal flow works", func() {
			// Empty initially.
			tok, err := cache.Read()
			So(err, ShouldBeNil)
			So(tok, ShouldBeNil)

			// Add something.
			err = cache.Write([]byte("token"), now.Add(60*time.Minute))
			So(err, ShouldBeNil)

			// Valid now.
			tok, err = cache.Read()
			So(err, ShouldBeNil)
			So(string(tok), ShouldEqual, "token")

			// Still valid 30 min later.
			clk.Add(30 * time.Minute)
			tok, err = cache.Read()
			So(err, ShouldBeNil)
			So(string(tok), ShouldEqual, "token")

			// Empty when close to expiration.
			clk.Add(29*time.Minute + 500*time.Millisecond)
			tok, err = cache.Read()
			So(err, ShouldBeNil)
			So(tok, ShouldBeNil)
		})

		Convey("token that expires too soon is rejected", func() {
			err := cache.Write([]byte("token"), now.Add(time.Second))
			So(err, ShouldErrLike, "token expires too soon")
		})

		Convey("Clear works", func() {
			err := cache.Write([]byte("token"), now.Add(60*time.Minute))
			So(err, ShouldBeNil)

			So(cache.Clear(), ShouldBeNil)

			tok, err := cache.Read()
			So(err, ShouldBeNil)
			So(tok, ShouldBeNil)
		})
	})
}

func TestFetchStoreServiceAccountJSON(t *testing.T) {
	Convey("works", t, func() {
		c := testContext()

		sa, err := FetchServiceAccountJSON(c, "account")
		So(err, ShouldBeNil)
		So(sa, ShouldBeNil)

		So(StoreServiceAccountJSON(c, "account", []byte("blah")), ShouldBeNil)

		sa, err = FetchServiceAccountJSON(c, "account")
		So(err, ShouldBeNil)
		So(string(sa), ShouldEqual, "blah")
	})
}

func TestAuthenticatorAndTransport(t *testing.T) {
	Convey("with mocked context", t, func() {
		c := testContext()
		scopes := []string{"a", "b", "c"}
		expiry := time.Unix(1442540000, 0)
		c = mockAccessTokenRPC(c, scopes, "token", expiry)
		c = urlfetch.Set(c, http.DefaultTransport)

		Convey("works with Service Account key", func() {
			// Just ensure doesn't crash.
			t, err := Transport(c, scopes, []byte("{}"))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
		})

		Convey("works with devserver service account", func() {
			// Just ensure doesn't crash.
			c = mockIsDevAppServer(c, true)
			_, err := Transport(c, scopes, nil)
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
		})

		Convey("works with GAE access token", func() {
			// Just ensure doesn't crash.
			c = mockIsDevAppServer(c, false)
			_, err := Transport(c, scopes, nil)
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
		})
	})
}

///

type mockedInfo struct {
	info.Interface

	scopes []string
	tok    string
	exp    time.Time
}

func (m *mockedInfo) AccessToken(scopes ...string) (string, time.Time, error) {
	So(scopes, ShouldResemble, m.scopes)
	return m.tok, m.exp, nil
}

type mockedDevAppServer struct {
	info.Interface

	value bool
}

func (m *mockedDevAppServer) IsDevAppServer() bool {
	return m.value
}

func testContext() context.Context {
	return mathrand.Set(memory.Use(context.Background()), rand.New(rand.NewSource(1000)))
}

func mockAccessTokenRPC(c context.Context, scopes []string, tok string, exp time.Time) context.Context {
	return info.AddFilters(c, func(ci context.Context, i info.Interface) info.Interface {
		return &mockedInfo{i, scopes, tok, exp}
	})
}

func mockIsDevAppServer(c context.Context, value bool) context.Context {
	return info.AddFilters(c, func(ci context.Context, i info.Interface) info.Interface {
		return &mockedDevAppServer{i, value}
	})
}
