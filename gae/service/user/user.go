// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

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
