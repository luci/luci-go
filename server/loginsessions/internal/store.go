// Copyright 2022 The LUCI Authors.
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

package internal

import (
	"context"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/server/loginsessions/internal/statepb"
)

// ErrNoSession is returned by SessionStore if the login session is missing.
var ErrNoSession = errors.New("no login session")

// SessionStore is a storage layer for login sessions.
type SessionStore interface {
	// Create transactionally stores a session if it didn't exist before.
	//
	// The caller should have session.Id populated already with a random ID.
	//
	// Returns an error if there's already such session or the transaction failed.
	Create(ctx context.Context, session *statepb.LoginSession) error

	// Get returns an existing session or ErrNoSession if it is missing.
	//
	// Always returns a new copy of the protobuf message that can be safely
	// mutated by the caller.
	Get(ctx context.Context, sessionID string) (*statepb.LoginSession, error)

	// Update transactionally updates an existing session.
	//
	// The callback is called to mutate the session in-place. The resulting
	// session is then stored back (if it really was mutated). The callback may
	// be called multiple times if the transaction is retried.
	//
	// If there's no such session returns ErrNoSession. May return other errors
	// if the transaction fails.
	//
	// On success returns the session that is stored in the store now.
	Update(ctx context.Context, sessionID string, cb func(*statepb.LoginSession)) (*statepb.LoginSession, error)

	// Cleanup deletes login sessions that expired sufficiently long ago.
	Cleanup(ctx context.Context) error
}

////////////////////////////////////////////////////////////////////////////////

// MemorySessionStore implements SessionStore using an in-memory map.
//
// For tests and running locally during development.
type MemorySessionStore struct {
	m        sync.Mutex
	sessions map[string]*statepb.LoginSession
}

func (s *MemorySessionStore) Create(ctx context.Context, session *statepb.LoginSession) error {
	if session.Id == "" {
		panic("session ID is empty")
	}
	s.m.Lock()
	defer s.m.Unlock()
	if s.sessions[session.Id] != nil {
		return errors.New("already have a session with this ID")
	}
	if s.sessions == nil {
		s.sessions = make(map[string]*statepb.LoginSession, 1)
	}
	s.sessions[session.Id] = proto.Clone(session).(*statepb.LoginSession)
	return nil
}

func (s *MemorySessionStore) Get(ctx context.Context, sessionID string) (*statepb.LoginSession, error) {
	s.m.Lock()
	defer s.m.Unlock()
	if session := s.sessions[sessionID]; session != nil {
		return proto.Clone(session).(*statepb.LoginSession), nil
	}
	return nil, ErrNoSession
}

func (s *MemorySessionStore) Update(ctx context.Context, sessionID string, cb func(*statepb.LoginSession)) (*statepb.LoginSession, error) {
	s.m.Lock()
	defer s.m.Unlock()
	session := s.sessions[sessionID]
	if session == nil {
		return nil, ErrNoSession
	}
	clone := proto.Clone(session).(*statepb.LoginSession)
	cb(clone)
	if clone.Id != sessionID {
		panic("changing session ID is forbidden")
	}
	s.sessions[sessionID] = proto.Clone(clone).(*statepb.LoginSession)
	return clone, nil
}

func (s *MemorySessionStore) Cleanup(ctx context.Context) error {
	// Don't care about the cleanup for in-memory implementation.
	return nil
}

////////////////////////////////////////////////////////////////////////////////

// Keep session for 7 extra days in case we need to investigate login issues.
const sessionCleanupDelay = 7 * 24 * time.Hour

type loginSessionEntity struct {
	_extra datastore.PropertyMap `gae:"-,extra"`
	_kind  string                `gae:"$kind,loginsessions.LoginSession"`

	ID      string `gae:"$id"`
	Session *statepb.LoginSession
	TTL     time.Time
}

// DatastoreSessionStore implements SessionStore using Cloud Datastore.
type DatastoreSessionStore struct{}

func (s *DatastoreSessionStore) Create(ctx context.Context, session *statepb.LoginSession) error {
	if session.Id == "" {
		panic("session ID is empty")
	}
	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		switch err := datastore.Get(ctx, &loginSessionEntity{ID: session.Id}); {
		case err == nil:
			return errors.New("already have a session with this ID")
		case err != datastore.ErrNoSuchEntity:
			return errors.Fmt("failed to check if the session already exists: %w", err)
		}
		return datastore.Put(ctx, &loginSessionEntity{
			ID:      session.Id,
			Session: session,
			TTL:     session.Expiry.AsTime().Add(sessionCleanupDelay).UTC(),
		})
	}, nil)
}

func (s *DatastoreSessionStore) Get(ctx context.Context, sessionID string) (*statepb.LoginSession, error) {
	ent := loginSessionEntity{ID: sessionID}
	switch err := datastore.Get(ctx, &ent); {
	case err == nil:
		return ent.Session, nil
	case err == datastore.ErrNoSuchEntity:
		return nil, ErrNoSession
	default:
		return nil, errors.Fmt("datastore error fetching session: %w", err)
	}
}

func (s *DatastoreSessionStore) Update(ctx context.Context, sessionID string, cb func(*statepb.LoginSession)) (*statepb.LoginSession, error) {
	var stored *statepb.LoginSession

	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		ent := loginSessionEntity{ID: sessionID}
		switch err := datastore.Get(ctx, &ent); {
		case err == datastore.ErrNoSuchEntity:
			return ErrNoSession
		case err != nil:
			return errors.Fmt("error fetching session: %w", err)
		}
		clone := proto.Clone(ent.Session).(*statepb.LoginSession)
		cb(clone)
		if !proto.Equal(ent.Session, clone) {
			if ent.Session.Id != clone.Id {
				panic("changing session ID is forbidden")
			}
			ent.Session = clone
			ent.TTL = clone.Expiry.AsTime().Add(sessionCleanupDelay).UTC()
			if err := datastore.Put(ctx, &ent); err != nil {
				return errors.Fmt("failed to store updated session: %w", err)
			}
		}
		stored = clone
		return nil
	}, nil)

	switch {
	case err == ErrNoSession:
		return nil, err
	case err != nil:
		return nil, errors.Fmt("session transaction error: %w", err)
	default:
		return stored, nil
	}
}

func (s *DatastoreSessionStore) Cleanup(ctx context.Context) error {
	q := datastore.NewQuery("loginsessions.LoginSession").
		Lt("TTL", clock.Now(ctx).UTC()).
		KeysOnly(true)

	var batch []*datastore.Key

	// Best effort deletion. The cron will keep retrying anyway.
	deleteBatch := func() {
		if err := datastore.Delete(ctx, batch); err != nil {
			logging.Warningf(ctx, "Failed to delete a batch of sessions: %s", err)
		} else {
			batch = batch[:0]
		}
	}

	err := datastore.Run(ctx, q, func(key *datastore.Key) {
		logging.Infof(ctx, "Cleaning up session %s", key.StringID())
		batch = append(batch, key)
		if len(batch) >= 50 {
			deleteBatch()
		}
	})

	deleteBatch()
	return err
}
