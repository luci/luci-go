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
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net/mail"
	"net/url"
	"strings"
	"sync"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/user"
)

type userData struct {
	sync.RWMutex
	user *user.User
}

// userImpl is a contextual pointer to the current userData.
type userImpl struct {
	data *userData
}

var _ user.RawInterface = (*userImpl)(nil)

// useUser adds a user.RawInterface implementation to context, accessible
// by user.Raw(c) or the exported user methods.
func useUser(c context.Context) context.Context {
	data := &userData{}

	return user.SetFactory(c, func(ic context.Context) user.RawInterface {
		return &userImpl{data}
	})
}

func (u *userImpl) Current() *user.User {
	u.data.RLock()
	defer u.data.RUnlock()
	if u.data.user != nil && u.data.user.ClientID == "" {
		ret := *u.data.user
		return &ret
	}
	return nil
}

func (u *userImpl) CurrentOAuth(scopes ...string) (*user.User, error) {
	// TODO(riannucci): something more clever in the Testable interface here?
	u.data.RLock()
	defer u.data.RUnlock()
	if u.data.user != nil && u.data.user.ClientID != "" {
		ret := *u.data.user
		return &ret, nil
	}
	return nil, nil
}

func (u *userImpl) IsAdmin() bool {
	u.data.RLock()
	defer u.data.RUnlock()
	return u.data.user != nil && u.data.user.Admin
}

func (u *userImpl) LoginURL(dest string) (string, error) {
	return "https://fakeapp.example.com/_ah/login?redirect=" + url.QueryEscape(dest), nil
}

func (u *userImpl) LogoutURL(dest string) (string, error) {
	return "https://fakeapp.example.com/_ah/logout?redirect=" + url.QueryEscape(dest), nil
}

func (u *userImpl) LoginURLFederated(dest, identity string) (string, error) {
	return "", fmt.Errorf("LoginURLFederated is deprecated")
}

func (u *userImpl) OAuthConsumerKey() (string, error) {
	return "", fmt.Errorf("OAuthConsumerKey is deprecated")
}

func (u *userImpl) GetTestable() user.Testable { return u }

func (u *userImpl) SetUser(user *user.User) {
	u.data.Lock()
	defer u.data.Unlock()
	u.data.user = user
}

func (u *userImpl) Login(email, clientID string, admin bool) {
	adr, err := mail.ParseAddress(email)
	if err != nil {
		panic(err)
	}
	email = adr.Address

	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		panic(fmt.Errorf("%q doesn't seem to be a valid email", email))
	}

	id := sha256.Sum256([]byte("ID:" + email))

	u.SetUser(&user.User{
		Email:      email,
		AuthDomain: parts[1],
		Admin:      admin,

		ID:       fmt.Sprint(binary.LittleEndian.Uint64(id[:])),
		ClientID: clientID,
	})
}

func (u *userImpl) Logout() {
	u.SetUser(nil)
}
