// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"testing"

	userS "github.com/luci/gae/service/user"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestUser(t *testing.T) {
	t.Parallel()

	Convey("user", t, func() {
		c := Use(context.Background())
		user := userS.Get(c)

		Convey("default state is anonymous", func() {
			So(user.Current(), ShouldBeNil)

			usr, err := user.CurrentOAuth("something")
			So(err, ShouldBeNil)
			So(usr, ShouldBeNil)

			So(user.IsAdmin(), ShouldBeFalse)
		})

		Convey("can login (normal)", func() {
			user.Testable().Login("hello@world.com", "", false)
			So(user.Current(), ShouldResembleV, &userS.User{
				Email:      "hello@world.com",
				AuthDomain: "world.com",
				ID:         "14628837901535854097",
			})

			usr, err := user.CurrentOAuth("scope")
			So(usr, ShouldBeNil)
			So(err, ShouldBeNil)

			Convey("and logout", func() {
				user.Testable().Logout()
				So(user.Current(), ShouldBeNil)

				usr, err := user.CurrentOAuth("scope")
				So(usr, ShouldBeNil)
				So(err, ShouldBeNil)
			})
		})

		Convey("can be admin", func() {
			user.Testable().Login("hello@world.com", "", true)
			So(user.Current(), ShouldResembleV, &userS.User{
				Email:      "hello@world.com",
				AuthDomain: "world.com",
				ID:         "14628837901535854097",
				Admin:      true,
			})
			So(user.IsAdmin(), ShouldBeTrue)
		})

		Convey("can login (oauth)", func() {
			user.Testable().Login("hello@world.com", "clientID", false)
			usr, err := user.CurrentOAuth("scope")
			So(err, ShouldBeNil)
			So(usr, ShouldResembleV, &userS.User{
				Email:      "hello@world.com",
				AuthDomain: "world.com",
				ID:         "14628837901535854097",
				ClientID:   "clientID",
			})

			So(user.Current(), ShouldBeNil)

			Convey("and logout", func() {
				user.Testable().Logout()
				So(user.Current(), ShouldBeNil)

				usr, err := user.CurrentOAuth("scope")
				So(usr, ShouldBeNil)
				So(err, ShouldBeNil)
			})
		})

		Convey("panics on bad email", func() {
			So(func() {
				user.Testable().Login("bademail", "", false)
			}, ShouldPanicLike, `"bademail" doesn't seem to be a valid email`)

			So(func() {
				user.Testable().Login("too@many@ats.com", "", false)
			}, ShouldPanicLike, `"too@many@ats.com" doesn't seem to be a valid email`)
		})

		Convey("fake URLs", func() {
			url, err := user.LoginURL("https://funky.example.com")
			So(err, ShouldBeNil)
			So(url, ShouldEqual, "https://fakeapp.example.com/_ah/login?redirect=https%3A%2F%2Ffunky.example.com")

			url, err = user.LogoutURL("https://funky.example.com")
			So(err, ShouldBeNil)
			So(url, ShouldEqual, "https://fakeapp.example.com/_ah/logout?redirect=https%3A%2F%2Ffunky.example.com")
		})

		Convey("Some stuff is deprecated", func() {
			url, err := user.LoginURLFederated("https://something", "something")
			So(err, ShouldErrLike, "LoginURLFederated is deprecated")
			So(url, ShouldEqual, "")

			key, err := user.OAuthConsumerKey()
			So(err, ShouldErrLike, "OAuthConsumerKey is deprecated")
			So(key, ShouldEqual, "")
		})

	})
}
