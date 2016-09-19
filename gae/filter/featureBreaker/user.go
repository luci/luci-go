// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package featureBreaker

import (
	"github.com/luci/gae/service/user"
	"golang.org/x/net/context"
)

type userState struct {
	*state

	user.RawInterface
}

var _ user.RawInterface = (*userState)(nil)

func (u *userState) CurrentOAuth(scopes ...string) (ret *user.User, err error) {
	err = u.run(func() (err error) {
		ret, err = u.RawInterface.CurrentOAuth(scopes...)
		return
	})
	return
}

func (u *userState) LoginURL(dest string) (ret string, err error) {
	err = u.run(func() (err error) {
		ret, err = u.RawInterface.LoginURL(dest)
		return
	})
	return
}

func (u *userState) LoginURLFederated(dest, identity string) (ret string, err error) {
	err = u.run(func() (err error) {
		ret, err = u.RawInterface.LoginURLFederated(dest, identity)
		return
	})
	return
}

func (u *userState) LogoutURL(dest string) (ret string, err error) {
	err = u.run(func() (err error) {
		ret, err = u.RawInterface.LogoutURL(dest)
		return
	})
	return
}

func (u *userState) OAuthConsumerKey() (ret string, err error) {
	err = u.run(func() (err error) {
		ret, err = u.RawInterface.OAuthConsumerKey()
		return
	})
	return
}

// FilterUser installs a featureBreaker user filter in the context.
func FilterUser(c context.Context, defaultError error) (context.Context, FeatureBreaker) {
	state := newState(defaultError)
	return user.AddFilters(c, func(ic context.Context, i user.RawInterface) user.RawInterface {
		return &userState{state, i}
	}), state
}
