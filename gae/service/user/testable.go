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
