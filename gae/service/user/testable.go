// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package user

// Testable is the interface that test implimentations will provide.
type Testable interface {
	// SetUser sets the user to a pre-populated User object.
	SetUser(*User)

	// Login will generate and set a new User object with values derived from
	// email clientID, and admin values. If clientID is provided, the User will
	// look like they logged in with OAuth. If it's empty, then this will look
	// like they logged in via the cookie auth method.
	Login(email, clientID string, admin bool)

	// Equivalent to SetUser(nil), but a bit more obvious to read in the code :).
	Logout()
}
