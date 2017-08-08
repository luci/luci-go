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

package prod

import (
	gae_user "go.chromium.org/gae/service/user"
	"golang.org/x/net/context"
	"google.golang.org/appengine/user"
)

// useUser adds a user service implementation to context, accessible
// by "go.chromium.org/gae/service/user".Raw(c) or the exported user service
// methods.
func useUser(c context.Context) context.Context {
	return gae_user.SetFactory(c, func(ci context.Context) gae_user.RawInterface {
		return userImpl{getAEContext(ci)}
	})
}

type userImpl struct {
	aeCtx context.Context
}

func (u userImpl) IsAdmin() bool {
	return user.IsAdmin(u.aeCtx)
}

func (u userImpl) LoginURL(dest string) (string, error) {
	return user.LoginURL(u.aeCtx, dest)
}

func (u userImpl) LoginURLFederated(dest, identity string) (string, error) {
	return user.LoginURLFederated(u.aeCtx, dest, identity)
}

func (u userImpl) LogoutURL(dest string) (string, error) {
	return user.LogoutURL(u.aeCtx, dest)
}

func (u userImpl) Current() *gae_user.User {
	return (*gae_user.User)(user.Current(u.aeCtx))
}

func (u userImpl) CurrentOAuth(scopes ...string) (*gae_user.User, error) {
	usr, err := user.CurrentOAuth(u.aeCtx, scopes...)
	if err != nil {
		return nil, err
	}
	return (*gae_user.User)(usr), nil
}

func (u userImpl) OAuthConsumerKey() (string, error) {
	return user.OAuthConsumerKey(u.aeCtx)
}

func (u userImpl) GetTestable() gae_user.Testable {
	return nil
}
