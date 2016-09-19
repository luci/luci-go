// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package memory

import (
	"testing"

	"github.com/luci/gae/service/user"

	"golang.org/x/net/context"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestUser(t *testing.T) {
	t.Parallel()

	Convey("user", t, func() {
		c := Use(context.Background())

		Convey("default state is anonymous", func() {
			So(user.Current(c), ShouldBeNil)

			usr, err := user.CurrentOAuth(c, "something")
			So(err, ShouldBeNil)
			So(usr, ShouldBeNil)

			So(user.IsAdmin(c), ShouldBeFalse)
		})

		Convey("can login (normal)", func() {
			user.GetTestable(c).Login("hello@world.com", "", false)
			So(user.Current(c), ShouldResemble, &user.User{
				Email:      "hello@world.com",
				AuthDomain: "world.com",
				ID:         "14628837901535854097",
			})

			usr, err := user.CurrentOAuth(c, "scope")
			So(usr, ShouldBeNil)
			So(err, ShouldBeNil)

			Convey("and logout", func() {
				user.GetTestable(c).Logout()
				So(user.Current(c), ShouldBeNil)

				usr, err := user.CurrentOAuth(c, "scope")
				So(usr, ShouldBeNil)
				So(err, ShouldBeNil)
			})
		})

		Convey("can be admin", func() {
			user.GetTestable(c).Login("hello@world.com", "", true)
			So(user.Current(c), ShouldResemble, &user.User{
				Email:      "hello@world.com",
				AuthDomain: "world.com",
				ID:         "14628837901535854097",
				Admin:      true,
			})
			So(user.IsAdmin(c), ShouldBeTrue)
		})

		Convey("can login (oauth)", func() {
			user.GetTestable(c).Login("hello@world.com", "clientID", false)
			usr, err := user.CurrentOAuth(c, "scope")
			So(err, ShouldBeNil)
			So(usr, ShouldResemble, &user.User{
				Email:      "hello@world.com",
				AuthDomain: "world.com",
				ID:         "14628837901535854097",
				ClientID:   "clientID",
			})

			So(user.Current(c), ShouldBeNil)

			Convey("and logout", func() {
				user.GetTestable(c).Logout()
				So(user.Current(c), ShouldBeNil)

				usr, err := user.CurrentOAuth(c, "scope")
				So(usr, ShouldBeNil)
				So(err, ShouldBeNil)
			})
		})

		Convey("panics on bad email", func() {
			So(func() {
				user.GetTestable(c).Login("bademail", "", false)
			}, ShouldPanicLike, `mail: missing phrase`)
		})

		Convey("fake URLs", func() {
			url, err := user.LoginURL(c, "https://funky.example.com")
			So(err, ShouldBeNil)
			So(url, ShouldEqual, "https://fakeapp.example.com/_ah/login?redirect=https%3A%2F%2Ffunky.example.com")

			url, err = user.LogoutURL(c, "https://funky.example.com")
			So(err, ShouldBeNil)
			So(url, ShouldEqual, "https://fakeapp.example.com/_ah/logout?redirect=https%3A%2F%2Ffunky.example.com")
		})

		Convey("Some stuff is deprecated", func() {
			url, err := user.LoginURLFederated(c, "https://something", "something")
			So(err, ShouldErrLike, "LoginURLFederated is deprecated")
			So(url, ShouldEqual, "")

			key, err := user.OAuthConsumerKey(c)
			So(err, ShouldErrLike, "OAuthConsumerKey is deprecated")
			So(key, ShouldEqual, "")
		})

	})
}
