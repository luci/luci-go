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

package user

import "google.golang.org/appengine/user"

// User is a mimic of https://godoc.org/google.golang.org/appengine/user#User
//
// It's provided here for convenience, and is compile-time checked to be
// identical.
type User struct {
	Email             string
	AuthDomain        string
	Admin             bool
	ID                string
	ClientID          string
	FederatedIdentity string
	FederatedProvider string
}

var _ User = (User)(user.User{})
