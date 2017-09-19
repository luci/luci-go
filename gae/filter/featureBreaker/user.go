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

package featureBreaker

import (
	"go.chromium.org/gae/service/user"
	"golang.org/x/net/context"
)

type userState struct {
	*state

	c context.Context
	user.RawInterface
}

var _ user.RawInterface = (*userState)(nil)

func (u *userState) CurrentOAuth(scopes ...string) (ret *user.User, err error) {
	err = u.run(u.c, func() (err error) {
		ret, err = u.RawInterface.CurrentOAuth(scopes...)
		return
	})
	return
}

func (u *userState) LoginURL(dest string) (ret string, err error) {
	err = u.run(u.c, func() (err error) {
		ret, err = u.RawInterface.LoginURL(dest)
		return
	})
	return
}

func (u *userState) LoginURLFederated(dest, identity string) (ret string, err error) {
	err = u.run(u.c, func() (err error) {
		ret, err = u.RawInterface.LoginURLFederated(dest, identity)
		return
	})
	return
}

func (u *userState) LogoutURL(dest string) (ret string, err error) {
	err = u.run(u.c, func() (err error) {
		ret, err = u.RawInterface.LogoutURL(dest)
		return
	})
	return
}

func (u *userState) OAuthConsumerKey() (ret string, err error) {
	err = u.run(u.c, func() (err error) {
		ret, err = u.RawInterface.OAuthConsumerKey()
		return
	})
	return
}

// FilterUser installs a featureBreaker user filter in the context.
func FilterUser(c context.Context, defaultError error) (context.Context, FeatureBreaker) {
	state := newState(defaultError)
	return user.AddFilters(c, func(ic context.Context, i user.RawInterface) user.RawInterface {
		return &userState{state, ic, i}
	}), state
}
