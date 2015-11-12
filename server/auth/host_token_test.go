// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package auth

import (
	"testing"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/secrets"
	"github.com/luci/luci-go/server/secrets/testsecrets"
	. "github.com/smartystreets/goconvey/convey"
)

func TestValidateHostToken(t *testing.T) {
	Convey("validateHostToken works", t, func() {
		store := &testsecrets.Store{}
		c := context.Background()

		valid, err := hostToken.Generate(secrets.Set(c, store), nil, map[string]string{"h": "hostname"}, 0)
		So(err, ShouldBeNil)

		host, err := validateHostToken(c, store, valid)
		So(err, ShouldBeNil)
		So(host, ShouldEqual, "hostname")
	})

	Convey("validateHostToken skips bad token", t, func() {
		store := &testsecrets.Store{}
		c := context.Background()

		host, err := validateHostToken(c, store, "blablabla")
		So(err, ShouldBeNil)
		So(host, ShouldEqual, "")
	})

	Convey("validateHostToken skips bad host", t, func() {
		store := &testsecrets.Store{}
		c := context.Background()

		valid, err := hostToken.Generate(secrets.Set(c, store), nil, map[string]string{"h": "@@@bad@@@"}, 0)
		So(err, ShouldBeNil)

		host, err := validateHostToken(c, store, valid)
		So(err, ShouldBeNil)
		So(host, ShouldEqual, "")
	})
}
