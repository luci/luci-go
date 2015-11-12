// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package auth

import (
	"errors"
	"net/http"
	"testing"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/auth/identity"
	"github.com/luci/luci-go/server/secrets"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestAuthenticate(t *testing.T) {
	Convey("IsAllowedOAuthClientID on default DB", t, func() {
		c := context.Background()
		auth := Authenticator{fakeOAuthMethod{"some_client_id"}}
		_, err := auth.Authenticate(c, makeRequest())
		So(err, ShouldErrLike, "using default auth.DB")
	})

	Convey("IsAllowedOAuthClientID with valid client_id", t, func() {
		c := context.Background()
		c = UseDB(c, func(c context.Context) (DB, error) {
			return &fakeDB{
				allowedClientID: "some_client_id",
			}, nil
		})
		auth := Authenticator{fakeOAuthMethod{"some_client_id"}}
		_, err := auth.Authenticate(c, makeRequest())
		So(err, ShouldBeNil)
	})

	Convey("IsAllowedOAuthClientID with invalid client_id", t, func() {
		c := context.Background()
		c = UseDB(c, func(c context.Context) (DB, error) {
			return &fakeDB{
				allowedClientID: "some_client_id",
			}, nil
		})
		auth := Authenticator{fakeOAuthMethod{"another_client_id"}}
		_, err := auth.Authenticate(c, makeRequest())
		So(err, ShouldEqual, ErrBadClientID)
	})
}

///

func makeRequest() *http.Request {
	req, _ := http.NewRequest("GET", "http://some-url", nil)
	return req
}

///

// fakeOAuthMethod implements Method.
type fakeOAuthMethod struct {
	clientID string
}

func (m fakeOAuthMethod) Authenticate(context.Context, *http.Request) (*User, error) {
	return &User{
		Identity: identity.Identity("user:abc@example.com"),
		Email:    "abc@example.com",
		ClientID: m.clientID,
	}, nil
}

///

// fakeDB implements DB.
type fakeDB struct {
	allowedClientID string
}

func (db *fakeDB) IsAllowedOAuthClientID(c context.Context, email, clientID string) (bool, error) {
	return clientID == db.allowedClientID, nil
}

func (db *fakeDB) IsMember(c context.Context, id identity.Identity, group string) (bool, error) {
	return true, nil
}

func (db *fakeDB) SharedSecrets(c context.Context) (secrets.Store, error) {
	return nil, errors.New("fakeDB: SharedSecrets is not implemented")
}
