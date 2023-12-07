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

package deprecated

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"

	"go.chromium.org/luci/server/auth"
)

// MemorySessionStore implement SessionStore.
type MemorySessionStore struct {
	lock    sync.Mutex
	store   map[string]Session
	counter int
}

// OpenSession create a new session for a user with given expiration time.
// It returns unique session ID.
func (s *MemorySessionStore) OpenSession(ctx context.Context, userID string, u *auth.User, exp time.Time) (string, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.counter++
	if s.store == nil {
		s.store = make(map[string]Session, 1)
	}
	sid := fmt.Sprintf("%s/%d", userID, s.counter)
	s.store[sid] = Session{
		SessionID: sid,
		UserID:    userID,
		User:      *u,
		Exp:       exp,
	}
	return sid, nil
}

// CloseSession closes a session given its ID. Does nothing if session is
// already closed or doesn't exist. Returns only transient errors.
func (s *MemorySessionStore) CloseSession(ctx context.Context, sessionID string) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	delete(s.store, sessionID)
	return nil
}

// GetSession returns existing non-expired session given its ID. Returns nil
// if session doesn't exist, closed or expired. Returns only transient errors.
func (s *MemorySessionStore) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if session, ok := s.store[sessionID]; ok && clock.Now(ctx).Before(session.Exp) {
		return &session, nil
	}
	return nil, nil
}
