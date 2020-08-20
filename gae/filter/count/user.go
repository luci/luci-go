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

package count

import (
	"go.chromium.org/gae/service/user"
	"golang.org/x/net/context"
)

// UserCounter is the counter object for the User service.
type UserCounter struct {
	Current           Entry
	CurrentOAuth      Entry
	IsAdmin           Entry
	LoginURL          Entry
	LoginURLFederated Entry
	LogoutURL         Entry
	OAuthConsumerKey  Entry
}

type userCounter struct {
	c *UserCounter

	u user.RawInterface
}

var _ user.RawInterface = (*userCounter)(nil)

func (u *userCounter) Current() *user.User {
	u.c.Current.up()
	return u.u.Current()
}

func (u *userCounter) CurrentOAuth(scopes ...string) (*user.User, error) {
	ret, err := u.u.CurrentOAuth(scopes...)
	return ret, u.c.CurrentOAuth.up(err)
}

func (u *userCounter) IsAdmin() bool {
	u.c.IsAdmin.up()
	return u.u.IsAdmin()
}

func (u *userCounter) LoginURL(dest string) (string, error) {
	ret, err := u.u.LoginURL(dest)
	return ret, u.c.LoginURL.up(err)
}

func (u *userCounter) LoginURLFederated(dest, identity string) (string, error) {
	ret, err := u.u.LoginURLFederated(dest, identity)
	return ret, u.c.LoginURLFederated.up(err)
}

func (u *userCounter) LogoutURL(dest string) (string, error) {
	ret, err := u.u.LogoutURL(dest)
	return ret, u.c.LogoutURL.up(err)
}

func (u *userCounter) OAuthConsumerKey() (string, error) {
	ret, err := u.u.OAuthConsumerKey()
	return ret, u.c.OAuthConsumerKey.up(err)
}

func (u *userCounter) GetTestable() user.Testable {
	return u.u.GetTestable()
}

// FilterUser installs a counter User filter in the context.
func FilterUser(c context.Context) (context.Context, *UserCounter) {
	state := &UserCounter{}
	return user.AddFilters(c, func(ic context.Context, u user.RawInterface) user.RawInterface {
		return &userCounter{state, u}
	}), state
}
