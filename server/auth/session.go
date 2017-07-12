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

package auth

import (
	"time"

	"golang.org/x/net/context"
)

// SessionStore keeps user sessions in some permanent storage. SessionStore is
// used by some authentication methods (e.g. openid.AuthMethod).
type SessionStore interface {
	// OpenSession create a new session for a user with given expiration time.
	// It returns unique session ID.
	OpenSession(c context.Context, userID string, u *User, exp time.Time) (string, error)

	// CloseSession closes a session given its ID. Does nothing if session is
	// already closed or doesn't exist. Returns only transient errors.
	CloseSession(c context.Context, sessionID string) error

	// GetSession returns existing non-expired session given its ID. Returns nil
	// if session doesn't exist, closed or expired. Returns only transient errors.
	GetSession(c context.Context, sessionID string) (*Session, error)
}

// Session is returned by SessionStore.GetSession(...).
type Session struct {
	SessionID string    // same as `sessionID` passed to GetSession()
	UserID    string    // authentication provider specific user id
	User      User      // user profile, including identity string
	Exp       time.Time // when the session expires
}
