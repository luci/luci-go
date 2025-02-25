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

package memory

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/gae/service/user"
)

func TestUser(t *testing.T) {
	t.Parallel()

	ftt.Run("user", t, func(t *ftt.Test) {
		c := Use(context.Background())

		t.Run("default state is anonymous", func(t *ftt.Test) {
			assert.Loosely(t, user.Current(c), should.BeNil)

			usr, err := user.CurrentOAuth(c, "something")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, usr, should.BeNil)

			assert.Loosely(t, user.IsAdmin(c), should.BeFalse)
		})

		t.Run("can login (normal)", func(t *ftt.Test) {
			user.GetTestable(c).Login("hello@world.com", "", false)
			assert.Loosely(t, user.Current(c), should.Match(&user.User{
				Email:      "hello@world.com",
				AuthDomain: "world.com",
				ID:         "14628837901535854097",
			}))

			usr, err := user.CurrentOAuth(c, "scope")
			assert.Loosely(t, usr, should.BeNil)
			assert.Loosely(t, err, should.BeNil)

			t.Run("and logout", func(t *ftt.Test) {
				user.GetTestable(c).Logout()
				assert.Loosely(t, user.Current(c), should.BeNil)

				usr, err := user.CurrentOAuth(c, "scope")
				assert.Loosely(t, usr, should.BeNil)
				assert.Loosely(t, err, should.BeNil)
			})
		})

		t.Run("can be admin", func(t *ftt.Test) {
			user.GetTestable(c).Login("hello@world.com", "", true)
			assert.Loosely(t, user.Current(c), should.Match(&user.User{
				Email:      "hello@world.com",
				AuthDomain: "world.com",
				ID:         "14628837901535854097",
				Admin:      true,
			}))
			assert.Loosely(t, user.IsAdmin(c), should.BeTrue)
		})

		t.Run("can login (oauth)", func(t *ftt.Test) {
			user.GetTestable(c).Login("hello@world.com", "clientID", false)
			usr, err := user.CurrentOAuth(c, "scope")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, usr, should.Match(&user.User{
				Email:      "hello@world.com",
				AuthDomain: "world.com",
				ID:         "14628837901535854097",
				ClientID:   "clientID",
			}))

			assert.Loosely(t, user.Current(c), should.BeNil)

			t.Run("and logout", func(t *ftt.Test) {
				user.GetTestable(c).Logout()
				assert.Loosely(t, user.Current(c), should.BeNil)

				usr, err := user.CurrentOAuth(c, "scope")
				assert.Loosely(t, usr, should.BeNil)
				assert.Loosely(t, err, should.BeNil)
			})
		})

		t.Run("panics on bad email", func(t *ftt.Test) {
			assert.Loosely(t, func() {
				user.GetTestable(c).Login("bademail", "", false)
			}, should.PanicLike(`mail:`))
		})

		t.Run("fake URLs", func(t *ftt.Test) {
			url, err := user.LoginURL(c, "https://funky.example.com")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, url, should.Equal("https://fakeapp.example.com/_ah/login?redirect=https%3A%2F%2Ffunky.example.com"))

			url, err = user.LogoutURL(c, "https://funky.example.com")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, url, should.Equal("https://fakeapp.example.com/_ah/logout?redirect=https%3A%2F%2Ffunky.example.com"))
		})

		t.Run("Some stuff is deprecated", func(t *ftt.Test) {
			url, err := user.LoginURLFederated(c, "https://something", "something")
			assert.Loosely(t, err, should.ErrLike("LoginURLFederated is deprecated"))
			assert.Loosely(t, url, should.BeEmpty)

			key, err := user.OAuthConsumerKey(c)
			assert.Loosely(t, err, should.ErrLike("OAuthConsumerKey is deprecated"))
			assert.Loosely(t, key, should.BeEmpty)
		})

	})
}
